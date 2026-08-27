package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// reserveChannelKeyRPMLua checks all candidate keys in the scheduler-provided
// order and consumes one request from the first key with capacity. Each
// candidate occupies two KEYS/ARGV slots: the shared per-key default window,
// then the optional per-model window. A limit of 0 skips that window. Redis
// TIME is used instead of application time, so all gateway nodes agree on the
// same natural-minute bucket even when their clocks differ.
//
// All KEYS include the same {channel-N} hash tag, allowing this script to work
// with Redis Cluster without a cross-slot error.
const reserveChannelKeyRPMLua = `
local now = redis.call('TIME')
local bucket = math.floor(tonumber(now[1]) / 60)
local candidate_count = math.floor(#KEYS / 2)

local function window_count(redis_key)
    local state = redis.call('HMGET', redis_key, 'bucket', 'count')
    local stored_bucket = tonumber(state[1])
    local count = tonumber(state[2]) or 0
    if stored_bucket ~= bucket then
        count = 0
    end
    return count
end

local function window_has_capacity(redis_key, limit)
    if limit <= 0 then
        return true
    end
    return window_count(redis_key) < limit
end

local function consume_window(redis_key, limit)
    if limit <= 0 then
        return
    end
    local count = window_count(redis_key) + 1
    redis.call('HMSET', redis_key, 'bucket', bucket, 'count', count)
    redis.call('EXPIRE', redis_key, 120)
end

for i = 1, candidate_count do
    local default_key = KEYS[2 * i - 1]
    local model_key = KEYS[2 * i]
    local default_limit = tonumber(ARGV[2 * i - 1])
    local model_limit = tonumber(ARGV[2 * i])
    if window_has_capacity(default_key, default_limit) and window_has_capacity(model_key, model_limit) then
        consume_window(default_key, default_limit)
        consume_window(model_key, model_limit)
        return {i, bucket, 1}
    end
end

return {0, bucket, 0}
`

// releaseChannelKeyRPMLua is used only when RPM admission succeeded but the
// subsequent quota reservation lost a race. It decrements the exact bucket that
// was reserved and never touches a newer minute.
const releaseChannelKeyRPMLua = `
local expected_bucket = tonumber(ARGV[1])
local state = redis.call('HMGET', KEYS[1], 'bucket', 'count')
local stored_bucket = tonumber(state[1])
local count = tonumber(state[2]) or 0

if stored_bucket ~= expected_bucket or count <= 0 then
    return 0
end

redis.call('HINCRBY', KEYS[1], 'count', -1)
return 1
`

type channelKeyRPMLease struct {
	KeyId      int64
	Identities []channelKeyRPMIdentity
	RedisKeys  []string
	Bucket     int64
}

type channelKeyRPMIdentity struct {
	KeyId int64
	Model string
}

type channelKeyRPMWindow struct {
	identity channelKeyRPMIdentity
	limit    int
}

type localChannelKeyRPMState struct {
	Bucket int64
	Count  int
}

type localChannelKeyRPMShard struct {
	sync.Mutex
	States            map[channelKeyRPMIdentity]localChannelKeyRPMState
	LastCleanupBucket int64
}

const channelKeyRPMShardCount = 64

var localChannelKeyRPMShards [channelKeyRPMShardCount]localChannelKeyRPMShard

func init() {
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
	}
}

func channelKeyRPMWindows(channel *model.Channel, key *model.ChannelKey, modelName string) []channelKeyRPMWindow {
	windows := make([]channelKeyRPMWindow, 0, 2)
	defaultLimit := key.EffectiveRpmLimit(channel)
	if defaultLimit > 0 {
		windows = append(windows, channelKeyRPMWindow{
			identity: channelKeyRPMIdentity{KeyId: key.Id},
			limit:    defaultLimit,
		})
	}
	modelLimit, hasModelLimit := key.ModelRpmLimit(modelName)
	if hasModelLimit && modelLimit > 0 {
		windows = append(windows, channelKeyRPMWindow{
			identity: channelKeyRPMIdentity{
				KeyId: key.Id,
				Model: strings.TrimSpace(modelName),
			},
			limit: modelLimit,
		})
	}
	return windows
}

func channelKeyRPMLeaseFromWindows(
	channelId int,
	keyId int64,
	windows []channelKeyRPMWindow,
	bucket int64,
	useRedis bool,
) *channelKeyRPMLease {
	lease := &channelKeyRPMLease{
		KeyId:      keyId,
		Identities: make([]channelKeyRPMIdentity, 0, len(windows)),
		Bucket:     bucket,
	}
	if useRedis {
		lease.RedisKeys = make([]string, 0, len(windows))
	}
	for _, window := range windows {
		lease.Identities = append(lease.Identities, window.identity)
		if useRedis {
			lease.RedisKeys = append(
				lease.RedisKeys,
				channelKeyRPMRedisKey(channelId, window.identity),
			)
		}
	}
	return lease
}

func lockLocalChannelKeyRPMShards(
	identities []channelKeyRPMIdentity,
	bucket int64,
) []*localChannelKeyRPMShard {
	seen := make(map[uint64]struct{}, len(identities))
	indexes := make([]uint64, 0, len(identities))
	for _, identity := range identities {
		index := channelKeyRPMShardIndex(identity)
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	sort.Slice(indexes, func(left, right int) bool {
		return indexes[left] < indexes[right]
	})
	shards := make([]*localChannelKeyRPMShard, 0, len(indexes))
	for _, index := range indexes {
		shard := &localChannelKeyRPMShards[index]
		shard.Lock()
		if shard.LastCleanupBucket != bucket {
			for keyIdentity, stale := range shard.States {
				if stale.Bucket < bucket {
					delete(shard.States, keyIdentity)
				}
			}
			shard.LastCleanupBucket = bucket
		}
		shards = append(shards, shard)
	}
	return shards
}

func unlockLocalChannelKeyRPMShards(shards []*localChannelKeyRPMShard) {
	for i := len(shards) - 1; i >= 0; i-- {
		shards[i].Unlock()
	}
}

func channelKeyRPMRedisKey(channelId int, identity channelKeyRPMIdentity) string {
	if identity.Model == "" {
		return fmt.Sprintf("new-api:key-rpm:{channel-%d}:key-%d", channelId, identity.KeyId)
	}
	modelHash := sha256.Sum256([]byte(identity.Model))
	return fmt.Sprintf("new-api:key-rpm:{channel-%d}:key-%d:model-%x", channelId, identity.KeyId, modelHash)
}

func channelKeyRPMShardIndex(identity channelKeyRPMIdentity) uint64 {
	hash := uint64(identity.KeyId)
	for i := 0; i < len(identity.Model); i++ {
		hash ^= uint64(identity.Model[i])
		hash *= 1099511628211
	}
	return hash % channelKeyRPMShardCount
}

func reserveChannelKeyRPM(
	ctx context.Context,
	channel *model.Channel,
	keys []*model.ChannelKey,
	modelName string,
) (*channelKeyRPMLease, int, error) {
	if len(keys) == 0 {
		return nil, -1, nil
	}

	windowsByKey := make([][]channelKeyRPMWindow, len(keys))
	allUnlimited := true
	for i, key := range keys {
		windowsByKey[i] = channelKeyRPMWindows(channel, key, modelName)
		if len(windowsByKey[i]) > 0 {
			allUnlimited = false
		}
	}
	if allUnlimited {
		return &channelKeyRPMLease{KeyId: keys[0].Id}, 0, nil
	}

	if common.RedisEnabled {
		redisKeys := make([]string, 0, len(keys)*2)
		args := make([]any, 0, len(keys)*2)
		for _, key := range keys {
			defaultLimit := key.EffectiveRpmLimit(channel)
			modelLimit := 0
			if limit, ok := key.ModelRpmLimit(modelName); ok {
				modelLimit = limit
			}
			redisKeys = append(
				redisKeys,
				channelKeyRPMRedisKey(channel.Id, channelKeyRPMIdentity{KeyId: key.Id}),
				channelKeyRPMRedisKey(channel.Id, channelKeyRPMIdentity{
					KeyId: key.Id,
					Model: strings.TrimSpace(modelName),
				}),
			)
			args = append(args, defaultLimit, modelLimit)
		}
		values, err := common.RDB.Eval(ctx, reserveChannelKeyRPMLua, redisKeys, args...).Slice()
		if err != nil {
			return nil, -1, fmt.Errorf("reserve channel key RPM: %w", err)
		}
		if len(values) != 3 {
			return nil, -1, errors.New("invalid channel key RPM script response")
		}
		selected, err := redisResultInt64(values[0])
		if err != nil {
			return nil, -1, err
		}
		bucket, err := redisResultInt64(values[1])
		if err != nil {
			return nil, -1, err
		}
		if selected <= 0 {
			return nil, -1, nil
		}
		index := int(selected - 1)
		if index < 0 || index >= len(keys) {
			return nil, -1, errors.New("channel key RPM script selected an invalid key")
		}
		return channelKeyRPMLeaseFromWindows(
			channel.Id,
			keys[index].Id,
			windowsByKey[index],
			bucket,
			true,
		), index, nil
	}

	// Without Redis, exact cross-node limiting is impossible. This sharded
	// fallback remains exact for a single gateway process and avoids one global
	// mutex becoming a hot lock when many independent keys are active.
	bucket := time.Now().Unix() / 60
	for i, key := range keys {
		windows := windowsByKey[i]
		if len(windows) == 0 {
			return &channelKeyRPMLease{KeyId: key.Id, Bucket: bucket}, i, nil
		}
		identities := make([]channelKeyRPMIdentity, len(windows))
		for windowIndex, window := range windows {
			identities[windowIndex] = window.identity
		}
		shards := lockLocalChannelKeyRPMShards(identities, bucket)
		allowed := true
		for _, window := range windows {
			shard := &localChannelKeyRPMShards[channelKeyRPMShardIndex(window.identity)]
			state := shard.States[window.identity]
			if state.Bucket != bucket {
				state = localChannelKeyRPMState{Bucket: bucket}
			}
			if state.Count >= window.limit {
				allowed = false
				break
			}
		}
		if !allowed {
			unlockLocalChannelKeyRPMShards(shards)
			continue
		}
		for _, window := range windows {
			shard := &localChannelKeyRPMShards[channelKeyRPMShardIndex(window.identity)]
			state := shard.States[window.identity]
			if state.Bucket != bucket {
				state = localChannelKeyRPMState{Bucket: bucket}
			}
			state.Count++
			shard.States[window.identity] = state
		}
		unlockLocalChannelKeyRPMShards(shards)
		return channelKeyRPMLeaseFromWindows(channel.Id, key.Id, windows, bucket, false), i, nil
	}
	return nil, -1, nil
}

func redisResultInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return 0, fmt.Errorf("unexpected Redis integer type %T", value)
	}
}

func releaseChannelKeyRPM(ctx context.Context, lease *channelKeyRPMLease) {
	if lease == nil || lease.KeyId <= 0 {
		return
	}
	if common.RedisEnabled && len(lease.RedisKeys) > 0 {
		for _, redisKey := range lease.RedisKeys {
			if err := common.RDB.Eval(ctx, releaseChannelKeyRPMLua, []string{redisKey}, lease.Bucket).Err(); err != nil {
				common.SysLog(fmt.Sprintf("failed to release channel key RPM: key_id=%d, error=%v", lease.KeyId, err))
			}
		}
		return
	}

	if len(lease.Identities) == 0 {
		return
	}
	shards := lockLocalChannelKeyRPMShards(lease.Identities, lease.Bucket)
	for _, identity := range lease.Identities {
		shard := &localChannelKeyRPMShards[channelKeyRPMShardIndex(identity)]
		state := shard.States[identity]
		if state.Bucket == lease.Bucket && state.Count > 0 {
			state.Count--
			shard.States[identity] = state
		}
	}
	unlockLocalChannelKeyRPMShards(shards)
}

func orderedChannelKeys(channel *model.Channel) ([]*model.ChannelKey, error) {
	allKeys := model.CacheGetChannelKeys(channel.Id)
	if !common.MemoryCacheEnabled || len(allKeys) == 0 {
		var err error
		allKeys, err = model.GetActiveChannelKeys(channel.Id)
		if err != nil {
			return nil, err
		}
	}
	enabled := make([]*model.ChannelKey, 0, len(allKeys))
	for _, key := range allKeys {
		if key.Status == model.ChannelKeyStatusEnabled {
			enabled = append(enabled, key)
		}
	}
	if len(enabled) <= 1 {
		return enabled, nil
	}

	start := 0
	lock := model.GetChannelPollingLock(channel.Id)
	lock.Lock()
	switch channel.ChannelInfo.MultiKeyMode {
	case constant.MultiKeyModePolling:
		start = channel.ChannelInfo.MultiKeyPollingIndex % len(enabled)
		channel.ChannelInfo.MultiKeyPollingIndex = (start + 1) % len(enabled)
	default:
		start = rand.Intn(len(enabled))
	}
	lock.Unlock()

	ordered := make([]*model.ChannelKey, 0, len(enabled))
	ordered = append(ordered, enabled[start:]...)
	ordered = append(ordered, enabled[:start]...)
	return ordered, nil
}

// AdmitChannelKey selects one credential with both RPM and quota capacity. RPM
// is counted per concrete upstream attempt; quota is reserved before the
// adaptor can send the request. Capacity misses try sibling keys and do not
// consume the outer upstream-error retry budget.
func AdmitChannelKey(c *gin.Context, relayInfo *relaycommon.RelayInfo, channel *model.Channel, estimatedQuota int) *types.NewAPIError {
	if relayInfo == nil || channel == nil {
		return types.NewError(errors.New("missing relay or channel for key admission"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if estimatedQuota < 0 || estimatedQuota > common.MaxQuota {
		return types.NewError(fmt.Errorf("invalid channel key estimated quota: %d", estimatedQuota), types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry())
	}

	candidates, err := orderedChannelKeys(channel)
	if err != nil {
		return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if len(candidates) == 0 {
		return types.NewErrorWithStatusCode(
			errors.New("channel has no enabled key"),
			types.ErrorCodeChannelNoAvailableKey,
			http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry(),
		)
	}

	requestId := relayInfo.RequestId
	if requestId == "" {
		requestId = common.NewRequestId()
	}
	sawQuotaCapacityFailure := false

	for len(candidates) > 0 {
		lease, index, err := reserveChannelKeyRPM(
			c.Request.Context(),
			channel,
			candidates,
			relayInfo.OriginModelName,
		)
		if err != nil {
			return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		if lease == nil || index < 0 {
			break
		}

		key := candidates[index]
		effectiveQuotaLimit := key.EffectiveQuotaLimit(channel)
		reservationId := int64(0)
		if effectiveQuotaLimit > 0 {
			// The random suffix distinguishes capacity retries that happen at the
			// same outer retry index (for example, auto-group fallback).
			attemptId := fmt.Sprintf("%s:retry:%d:key:%d:%s", requestId, relayInfo.RetryIndex, key.Id, common.NewRequestId())
			reservation, reserveErr := model.ReserveChannelKeyQuota(
				attemptId,
				requestId,
				key.Id,
				int64(estimatedQuota),
				time.Now().Add(30*time.Minute).Unix(),
			)
			if reserveErr != nil {
				releaseChannelKeyRPM(c.Request.Context(), lease)
				if errors.Is(reserveErr, model.ErrChannelKeyUnavailable) {
					// Another gateway may have exhausted this key. Refresh this
					// node once on the authoritative DB rejection so subsequent
					// requests do not repeatedly select a stale cached key.
					if cacheErr := model.ReloadChannelKeyCache(channel.Id); cacheErr != nil {
						common.SysLog(fmt.Sprintf(
							"failed to refresh unavailable channel key cache: channel_id=%d, error=%v",
							channel.Id,
							cacheErr,
						))
					}
					sawQuotaCapacityFailure = true
					candidates = append(candidates[:index], candidates[index+1:]...)
					continue
				}
				if errors.Is(reserveErr, model.ErrChannelKeyQuotaInsufficient) {
					sawQuotaCapacityFailure = true
					candidates = append(candidates[:index], candidates[index+1:]...)
					continue
				}
				return types.NewError(reserveErr, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
			}
			reservationId = reservation.Id
		}

		common.SetContextKey(c, constant.ContextKeyChannelKeyId, key.Id)
		common.SetContextKey(c, constant.ContextKeyChannelKey, key.Key)
		common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, key.Position)
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)

		relayInfo.ChannelKeyQuotaReservationId = reservationId
		relayInfo.ChannelKeyQuotaSettled = false
		return nil
	}

	if sawQuotaCapacityFailure {
		return types.NewErrorWithStatusCode(
			errors.New("all channel keys have insufficient quota"),
			types.ErrorCodeChannelKeyQuotaInsufficient,
			http.StatusServiceUnavailable,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return NewChannelKeyRPMExhaustedError()
}

func NewChannelKeyRPMExhaustedError() *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New("Resource exhausted. Please try again later..."),
		types.ErrorCodeChannelKeyRateLimited,
		http.StatusTooManyRequests,
		types.ErrOptionWithSkipRetry(),
	)
}

// ReleaseChannelKeyQuotaReservation releases only an unsettled quota lease.
// RPM is deliberately not released here because AdmitChannelKey runs directly
// before relay dispatch; a failed upstream attempt still consumed provider RPM.
func ReleaseChannelKeyQuotaReservation(relayInfo *relaycommon.RelayInfo) {
	if relayInfo == nil || relayInfo.ChannelKeyQuotaReservationId <= 0 || relayInfo.ChannelKeyQuotaSettled {
		return
	}
	if err := model.ReleaseChannelKeyQuota(relayInfo.ChannelKeyQuotaReservationId); err != nil {
		common.SysLog(fmt.Sprintf("failed to release channel key quota reservation: reservation_id=%d, error=%v", relayInfo.ChannelKeyQuotaReservationId, err))
		return
	}
	relayInfo.ChannelKeyQuotaSettled = true
}

package model

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"gorm.io/gorm"
)

type ChannelKeyModelRpmLimits map[string]int

func (limits ChannelKeyModelRpmLimits) Value() (driver.Value, error) {
	if limits == nil {
		limits = ChannelKeyModelRpmLimits{}
	}
	data, err := common.Marshal(limits)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func (limits *ChannelKeyModelRpmLimits) Scan(value any) error {
	if value == nil {
		*limits = ChannelKeyModelRpmLimits{}
		return nil
	}
	var data []byte
	switch typed := value.(type) {
	case []byte:
		data = typed
	case string:
		data = []byte(typed)
	default:
		return fmt.Errorf("unsupported channel key model RPM limits type %T", value)
	}
	if len(data) == 0 {
		*limits = ChannelKeyModelRpmLimits{}
		return nil
	}
	return common.Unmarshal(data, limits)
}

func (limits ChannelKeyModelRpmLimits) Normalize() (ChannelKeyModelRpmLimits, error) {
	if len(limits) > 1000 {
		return nil, errors.New("model RPM limits cannot contain more than 1000 models")
	}
	normalized := make(ChannelKeyModelRpmLimits, len(limits))
	for modelName, rpm := range limits {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return nil, errors.New("model RPM limit requires a model name")
		}
		if len(modelName) > 255 {
			return nil, errors.New("model name in RPM limit cannot exceed 255 characters")
		}
		if rpm < 0 {
			return nil, fmt.Errorf("model RPM limit for %s cannot be negative", modelName)
		}
		if _, exists := normalized[modelName]; exists {
			return nil, fmt.Errorf("duplicate model RPM limit for %s", modelName)
		}
		normalized[modelName] = rpm
	}
	return normalized, nil
}

const (
	ChannelKeyStatusUnknown = iota
	ChannelKeyStatusEnabled
	ChannelKeyStatusManuallyDisabled
	ChannelKeyStatusErrorDisabled
	ChannelKeyStatusQuotaExhausted
	ChannelKeyStatusArchived
)

const (
	ChannelKeyReservationReserved  = "reserved"
	ChannelKeyReservationCommitted = "committed"
	ChannelKeyReservationReleased  = "released"
)

const (
	ChannelKeyEventQuotaExhausted = "quota_exhausted"
	ChannelKeyEventQuotaReset     = "quota_reset"
	ChannelKeyEventLimitChanged   = "limit_changed"
	ChannelKeyEventRestored       = "restored"
	ChannelKeyEventQuotaAdjusted  = "quota_adjusted"
	ChannelKeyEventArchived       = "archived"
)

const (
	channelStatusReasonAllKeysDisabled = "All keys are disabled"
	channelStatusReasonQuotaExhausted  = "All channel keys have exhausted their quota"
)

var (
	ErrChannelKeyUnavailable       = errors.New("channel key is unavailable")
	ErrChannelKeyQuotaInsufficient = errors.New("channel key quota is insufficient")
	ErrChannelKeyReservationClosed = errors.New("channel key quota reservation is already closed")
)

// ChannelKey gives every credential a stable identity. Legacy multi-key channels
// store credentials in a newline-delimited Channel.Key string and address them by
// array index. An index is not safe for accounting because deleting or reordering
// credentials changes its meaning. ID remains stable across reorder operations and
// is therefore the identity used by RPM, quota reservations, task settlement and
// audit events.
//
// Key is intentionally excluded from JSON. Management APIs must expose only a
// masked preview and must never write the credential or Fingerprint to logs.
type ChannelKey struct {
	Id          int64  `json:"id"`
	ChannelId   int    `json:"channel_id" gorm:"index;not null"`
	Position    int    `json:"position" gorm:"index;not null"`
	Key         string `json:"-" gorm:"type:text;not null"`
	Fingerprint string `json:"-" gorm:"type:varchar(64);index;not null"`
	Status      int    `json:"status" gorm:"index;not null"`

	// A nil override inherits the corresponding default from Channel. Zero is an
	// explicit unlimited value, while a positive value is an independent per-key
	// limit.
	RpmLimit       *int                     `json:"rpm_limit"`
	QuotaLimit     *int64                   `json:"quota_limit" gorm:"bigint"`
	ModelRpmLimits ChannelKeyModelRpmLimits `json:"model_rpm_limits" gorm:"type:text"`

	// QuotaUsed is the current budget-cycle consumption. QuotaReserved protects
	// the remaining budget from concurrent in-flight requests. LifetimeQuota is
	// never reset and remains available for accounting and audits.
	QuotaUsed     int64 `json:"quota_used" gorm:"bigint;not null"`
	QuotaReserved int64 `json:"quota_reserved" gorm:"bigint;not null"`
	LifetimeQuota int64 `json:"lifetime_quota" gorm:"bigint;not null"`

	DisabledReason string `json:"disabled_reason" gorm:"type:varchar(255)"`
	DisabledAt     int64  `json:"disabled_at" gorm:"bigint"`
	ExhaustedAt    int64  `json:"exhausted_at" gorm:"bigint"`
	ArchivedAt     int64  `json:"archived_at" gorm:"bigint"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (key *ChannelKey) EffectiveRpmLimit(channel *Channel) int {
	if key != nil && key.RpmLimit != nil {
		return *key.RpmLimit
	}
	if channel == nil {
		return 0
	}
	return channel.KeyRpmLimit
}

func (key *ChannelKey) EffectiveRpmLimitForModel(channel *Channel, modelName string) int {
	defaultLimit := key.EffectiveRpmLimit(channel)
	modelLimit, hasModelLimit := key.ModelRpmLimit(modelName)
	if !hasModelLimit || modelLimit <= 0 {
		return defaultLimit
	}
	if defaultLimit <= 0 || modelLimit < defaultLimit {
		return modelLimit
	}
	return defaultLimit
}

func (key *ChannelKey) ModelRpmLimit(modelName string) (int, bool) {
	if key == nil || key.ModelRpmLimits == nil {
		return 0, false
	}
	limit, ok := key.ModelRpmLimits[strings.TrimSpace(modelName)]
	return limit, ok
}

func (key *ChannelKey) EffectiveQuotaLimit(channel *Channel) int64 {
	if key != nil && key.QuotaLimit != nil {
		return *key.QuotaLimit
	}
	if channel == nil {
		return 0
	}
	return channel.KeyQuotaLimit
}

func (key *ChannelKey) Preview() string {
	if key == nil {
		return ""
	}
	value := strings.TrimSpace(key.Key)
	if len(value) <= 10 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}

// ChannelKeyQuotaReservation makes reserve/commit/release idempotent. AttemptId
// is unique for one concrete upstream attempt; retries must use a new attempt ID.
type ChannelKeyQuotaReservation struct {
	Id            int64  `json:"id"`
	AttemptId     string `json:"attempt_id" gorm:"type:varchar(128);uniqueIndex;not null"`
	RequestId     string `json:"request_id" gorm:"type:varchar(64);index;not null"`
	ChannelId     int    `json:"channel_id" gorm:"index;not null"`
	ChannelKeyId  int64  `json:"channel_key_id" gorm:"index;not null"`
	ReservedQuota int64  `json:"reserved_quota" gorm:"bigint;not null"`
	ActualQuota   int64  `json:"actual_quota" gorm:"bigint;not null"`
	Status        string `json:"status" gorm:"type:varchar(16);index;not null"`
	ExpiresAt     int64  `json:"expires_at" gorm:"bigint;index"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint"`
}

// ChannelKeyEvent is an append-only operational audit. It deliberately stores
// only the stable key ID and quota snapshot, never the credential itself.
type ChannelKeyEvent struct {
	Id           int64  `json:"id"`
	ChannelId    int    `json:"channel_id" gorm:"index;not null"`
	ChannelKeyId int64  `json:"channel_key_id" gorm:"index;not null"`
	EventType    string `json:"event_type" gorm:"type:varchar(32);index;not null"`
	QuotaUsed    int64  `json:"quota_used" gorm:"bigint"`
	QuotaLimit   int64  `json:"quota_limit" gorm:"bigint"`
	RequestId    string `json:"request_id" gorm:"type:varchar(64);index"`
	Reason       string `json:"reason" gorm:"type:varchar(255)"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint"`
}

// ChannelKeyArchive is the independent exhausted-key library. The active
// ChannelKey row remains as a non-schedulable accounting tombstone so delayed
// task settlement can keep using its stable ID; the credential snapshot lives
// here for explicit administrator restore and audit workflows.
//
// Key and Fingerprint are never serialized. Controllers must return only the
// masked preview produced by Preview.
type ChannelKeyArchive struct {
	Id            int64  `json:"id"`
	OriginalKeyId int64  `json:"original_key_id" gorm:"index;not null"`
	ChannelId     int    `json:"channel_id" gorm:"index;not null"`
	Position      int    `json:"position" gorm:"not null"`
	Key           string `json:"-" gorm:"type:text;not null"`
	Fingerprint   string `json:"-" gorm:"type:varchar(64);index;not null"`

	RpmLimit       *int                     `json:"rpm_limit"`
	QuotaLimit     *int64                   `json:"quota_limit" gorm:"bigint"`
	ModelRpmLimits ChannelKeyModelRpmLimits `json:"model_rpm_limits" gorm:"type:text"`
	// EffectiveQuotaLimit captures the inherited/default limit that actually
	// caused this exhaustion cycle, even when QuotaLimit itself is nil.
	EffectiveQuotaLimit int64 `json:"effective_quota_limit" gorm:"bigint;not null"`

	QuotaUsed     int64 `json:"quota_used" gorm:"bigint;not null"`
	QuotaReserved int64 `json:"quota_reserved" gorm:"bigint;not null"`
	LifetimeQuota int64 `json:"lifetime_quota" gorm:"bigint;not null"`

	Reason      string `json:"reason" gorm:"type:varchar(255)"`
	ExhaustedAt int64  `json:"exhausted_at" gorm:"bigint;index"`
	ArchivedAt  int64  `json:"archived_at" gorm:"bigint;index"`
	RestoredAt  int64  `json:"restored_at" gorm:"bigint;index"`
}

func (archive *ChannelKeyArchive) Preview() string {
	if archive == nil {
		return ""
	}
	value := strings.TrimSpace(archive.Key)
	if len(value) <= 10 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}

var (
	channelKeyCacheLock sync.RWMutex
	channelKeyCache     = make(map[int][]*ChannelKey)
	channelKeyByIdCache = make(map[int64]*ChannelKey)
	channelKeyAvailable = make(map[int]bool)
	channelKeyKnown     = make(map[int]bool)
)

// InitChannelKeyCache replaces the entire key snapshot atomically. Callers read
// immutable rows under an RLock, avoiding a database query on every relay.
func InitChannelKeyCache(keys []*ChannelKey) {
	byChannel := make(map[int][]*ChannelKey)
	byId := make(map[int64]*ChannelKey, len(keys))
	available := make(map[int]bool)
	known := make(map[int]bool)
	for _, key := range keys {
		if key != nil {
			known[key.ChannelId] = true
		}
		if key == nil || key.Status == ChannelKeyStatusArchived {
			continue
		}
		byChannel[key.ChannelId] = append(byChannel[key.ChannelId], key)
		byId[key.Id] = key
		if key.Status == ChannelKeyStatusEnabled {
			available[key.ChannelId] = true
		}
	}
	for channelId := range byChannel {
		sort.Slice(byChannel[channelId], func(i, j int) bool {
			return byChannel[channelId][i].Position < byChannel[channelId][j].Position
		})
	}

	channelKeyCacheLock.Lock()
	channelKeyCache = byChannel
	channelKeyByIdCache = byId
	channelKeyAvailable = available
	channelKeyKnown = known
	channelKeyCacheLock.Unlock()
}

func CacheChannelKeyAvailability(channelId int) (available bool, known bool) {
	channelKeyCacheLock.RLock()
	defer channelKeyCacheLock.RUnlock()
	return channelKeyAvailable[channelId], channelKeyKnown[channelId]
}

func CacheGetChannelKeys(channelId int) []*ChannelKey {
	channelKeyCacheLock.RLock()
	defer channelKeyCacheLock.RUnlock()

	keys := channelKeyCache[channelId]
	result := make([]*ChannelKey, len(keys))
	copy(result, keys)
	return result
}

func CacheGetChannelKey(id int64) (*ChannelKey, bool) {
	channelKeyCacheLock.RLock()
	defer channelKeyCacheLock.RUnlock()
	key, ok := channelKeyByIdCache[id]
	return key, ok
}

// SyncChannelKeyStatusSummary projects the stable key rows back into the
// channel-level summary used by the channel list and legacy multi-key paths.
// ChannelKey remains the source of truth; rebuilding the maps avoids stale
// quota-exhausted counts and safely repairs earlier partial updates.
func SyncChannelKeyStatusSummary(channelId int) error {
	var updatedChannel Channel
	previousStatus := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Where("id = ?", channelId).First(&channel).Error; err != nil {
			return err
		}
		previousStatus = channel.Status

		var keys []*ChannelKey
		if err := tx.
			Where("channel_id = ? AND status <> ?", channelId, ChannelKeyStatusArchived).
			Order("position ASC").
			Find(&keys).Error; err != nil {
			return err
		}
		if len(keys) == 0 {
			updatedChannel = channel
			return nil
		}

		statusList := make(map[int]int)
		reasons := make(map[int]string)
		disabledTimes := make(map[int]int64)
		enabledCount := 0
		quotaExhaustedCount := 0
		for _, key := range keys {
			switch key.Status {
			case ChannelKeyStatusEnabled:
				enabledCount++
				continue
			case ChannelKeyStatusManuallyDisabled:
				statusList[key.Position] = common.ChannelStatusManuallyDisabled
			case ChannelKeyStatusQuotaExhausted:
				quotaExhaustedCount++
				statusList[key.Position] = common.ChannelStatusAutoDisabled
			default:
				statusList[key.Position] = common.ChannelStatusAutoDisabled
			}
			if key.DisabledReason != "" {
				reasons[key.Position] = key.DisabledReason
			}
			if key.DisabledAt > 0 {
				disabledTimes[key.Position] = key.DisabledAt
			}
		}

		if channel.ChannelInfo.IsMultiKey {
			channel.ChannelInfo.MultiKeySize = len(keys)
			channel.ChannelInfo.MultiKeyStatusList = statusList
			channel.ChannelInfo.MultiKeyDisabledReason = reasons
			channel.ChannelInfo.MultiKeyDisabledTime = disabledTimes
		}

		info := channel.GetOtherInfo()
		currentReason, _ := info["status_reason"].(string)
		derivedReason := currentReason == channelStatusReasonAllKeysDisabled ||
			currentReason == channelStatusReasonQuotaExhausted
		if enabledCount == 0 &&
			(channel.Status == common.ChannelStatusEnabled ||
				(channel.Status == common.ChannelStatusAutoDisabled && derivedReason)) {
			reason := channelStatusReasonAllKeysDisabled
			if quotaExhaustedCount == len(keys) {
				reason = channelStatusReasonQuotaExhausted
			}
			if channel.Status == common.ChannelStatusEnabled || info["status_time"] == nil {
				info["status_time"] = common.GetTimestamp()
			}
			channel.Status = common.ChannelStatusAutoDisabled
			info["status_reason"] = reason
			channel.SetOtherInfo(info)
		} else if enabledCount > 0 &&
			channel.Status == common.ChannelStatusAutoDisabled &&
			derivedReason {
			channel.Status = common.ChannelStatusEnabled
			delete(info, "status_reason")
			delete(info, "status_time")
			channel.SetOtherInfo(info)
		}

		if err := tx.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Updates(map[string]any{
				"channel_info": channel.ChannelInfo,
				"status":       channel.Status,
				"other_info":   channel.OtherInfo,
			}).Error; err != nil {
			return err
		}
		if channel.Status != previousStatus {
			if err := tx.Model(&Ability{}).
				Where("channel_id = ?", channel.Id).
				Update("enabled", channel.Status == common.ChannelStatusEnabled).Error; err != nil {
				return err
			}
		}
		updatedChannel = channel
		return nil
	})
	if err != nil {
		return err
	}

	if common.MemoryCacheEnabled {
		if updatedChannel.Status != previousStatus && updatedChannel.Status == common.ChannelStatusEnabled {
			InitChannelCache()
		} else {
			CacheUpdateChannel(&updatedChannel)
			if updatedChannel.Status != common.ChannelStatusEnabled {
				CacheUpdateChannelStatus(updatedChannel.Id, updatedChannel.Status)
			}
		}
	}
	return nil
}

// ReloadChannelKeyCache refreshes one channel after a management or exhaustion
// transition. It replaces only that channel's slice, avoiding a global cache
// rebuild on the settlement hot path.
func ReloadChannelKeyCache(channelId int) error {
	var keys []*ChannelKey
	if err := DB.
		Where("channel_id = ? AND status <> ?", channelId, ChannelKeyStatusArchived).
		Order("position ASC").
		Find(&keys).Error; err != nil {
		return err
	}

	channelKeyCacheLock.Lock()
	for id, cached := range channelKeyByIdCache {
		if cached.ChannelId == channelId {
			delete(channelKeyByIdCache, id)
		}
	}
	channelKeyCache[channelId] = keys
	channelKeyKnown[channelId] = true
	channelKeyAvailable[channelId] = false
	for _, key := range keys {
		channelKeyByIdCache[key.Id] = key
		if key.Status == ChannelKeyStatusEnabled {
			channelKeyAvailable[channelId] = true
		}
	}
	channelKeyCacheLock.Unlock()
	return nil
}

func refreshChannelKeyDerivedState(channelId int) error {
	if err := SyncChannelKeyStatusSummary(channelId); err != nil {
		return err
	}
	return ReloadChannelKeyCache(channelId)
}

// channelKeyFingerprint is used only to reconcile legacy Channel.Key entries
// with stable rows. The digest is never returned by APIs or included in logs.
// Credentials are expected to be high-entropy secrets; a future encrypted-key
// migration can replace this reconciliation digest without changing runtime IDs.
func channelKeyFingerprint(channelType int, key string) string {
	identity := strings.TrimSpace(key)

	// Codex refresh replaces access_token/refresh_token in place. Account ID is
	// the durable upstream credential identity, so token rotation must not reset
	// RPM/quota accounting or create a new archived key.
	if channelType == constant.ChannelTypeCodex {
		var credential map[string]any
		if common.Unmarshal([]byte(identity), &credential) == nil {
			if accountId := strings.TrimSpace(fmt.Sprintf("%v", credential["account_id"])); accountId != "" && accountId != "<nil>" {
				identity = "codex:" + accountId
			}
		}
	}

	// Vertex service-account JSON may be re-serialized with different whitespace.
	// The account/project tuple is stable while the JSON representation is not.
	if channelType == constant.ChannelTypeVertexAi {
		var credential map[string]any
		if common.Unmarshal([]byte(identity), &credential) == nil {
			email := strings.TrimSpace(fmt.Sprintf("%v", credential["client_email"]))
			project := strings.TrimSpace(fmt.Sprintf("%v", credential["project_id"]))
			if email != "" && email != "<nil>" {
				identity = "vertex:" + project + ":" + email
			}
		}
	}

	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

// SyncChannelKeys reconciles the legacy credential list with stable ChannelKey
// rows. Matching fingerprints retain their IDs and accounting when reordered.
// Removed credentials are archived instead of deleted so delayed task settlement
// and audit history keep referring to a valid row.
func SyncChannelKeys(channel *Channel) error {
	if channel == nil || channel.Id <= 0 {
		return errors.New("invalid channel")
	}
	if err := syncChannelKeysWithDB(DB, channel); err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channel.Id)
}

// CopyChannelKeyLimits copies per-key RPM, quota, and model RPM overrides from
// sourceChannelId onto destChannelId. Credentials are matched by fingerprint in
// position order, the same way SyncChannelKeys reconciles a copied key list.
// Usage counters stay at zero on the destination because a clone starts a new
// budget cycle.
func CopyChannelKeyLimits(sourceChannelId, destChannelId int) error {
	if sourceChannelId <= 0 || destChannelId <= 0 {
		return errors.New("invalid channel")
	}
	if sourceChannelId == destChannelId {
		return nil
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		var sourceKeys []*ChannelKey
		if err := tx.
			Where("channel_id = ? AND status <> ?", sourceChannelId, ChannelKeyStatusArchived).
			Order("position ASC").
			Find(&sourceKeys).Error; err != nil {
			return err
		}
		if len(sourceKeys) == 0 {
			return nil
		}

		var destKeys []*ChannelKey
		if err := lockForUpdate(tx).
			Where("channel_id = ? AND status <> ?", destChannelId, ChannelKeyStatusArchived).
			Order("position ASC").
			Find(&destKeys).Error; err != nil {
			return err
		}

		available := make(map[string][]*ChannelKey)
		for _, key := range sourceKeys {
			available[key.Fingerprint] = append(available[key.Fingerprint], key)
		}

		now := common.GetTimestamp()
		for _, dest := range destKeys {
			queue := available[dest.Fingerprint]
			if len(queue) == 0 {
				continue
			}
			source := queue[0]
			available[dest.Fingerprint] = queue[1:]

			updates := map[string]any{
				"updated_at": now,
			}
			if source.RpmLimit != nil {
				rpmLimit := *source.RpmLimit
				updates["rpm_limit"] = rpmLimit
			} else {
				updates["rpm_limit"] = nil
			}
			if source.QuotaLimit != nil {
				quotaLimit := *source.QuotaLimit
				updates["quota_limit"] = quotaLimit
			} else {
				updates["quota_limit"] = nil
			}
			copiedLimits := ChannelKeyModelRpmLimits{}
			for modelName, rpm := range source.ModelRpmLimits {
				copiedLimits[modelName] = rpm
			}
			updates["model_rpm_limits"] = copiedLimits
			if err := tx.Model(&ChannelKey{}).Where("id = ?", dest.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(destChannelId)
}

// MigrateLegacyChannelKeys backfills only channels that do not have any active
// stable key rows yet. Each channel is migrated in its own transaction, so a
// crash can be resumed safely without holding a database-wide transaction.
func MigrateLegacyChannelKeys() error {
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return err
	}

	var migratedChannelIds []int
	if err := DB.Model(&ChannelKey{}).
		Where("status <> ?", ChannelKeyStatusArchived).
		Distinct("channel_id").
		Pluck("channel_id", &migratedChannelIds).Error; err != nil {
		return err
	}
	migrated := make(map[int]struct{}, len(migratedChannelIds))
	for _, channelId := range migratedChannelIds {
		migrated[channelId] = struct{}{}
	}

	for _, channel := range channels {
		if _, ok := migrated[channel.Id]; !ok {
			if err := syncChannelKeysWithDB(DB, channel); err != nil {
				return fmt.Errorf("migrate channel %d keys: %w", channel.Id, err)
			}
		}
		if err := SyncChannelKeyStatusSummary(channel.Id); err != nil {
			return fmt.Errorf("sync channel %d key status summary: %w", channel.Id, err)
		}
	}
	return nil
}

func syncChannelKeysWithDB(db *gorm.DB, channel *Channel) error {
	rawKeys := channel.GetKeys()
	if len(rawKeys) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var existing []*ChannelKey
		if err := lockForUpdate(tx).
			Where("channel_id = ? AND status <> ?", channel.Id, ChannelKeyStatusArchived).
			Order("position ASC").
			Find(&existing).Error; err != nil {
			return err
		}

		// A queue per fingerprint correctly handles an accidentally duplicated
		// credential without assigning one database row to two positions.
		available := make(map[string][]*ChannelKey)
		for _, key := range existing {
			available[key.Fingerprint] = append(available[key.Fingerprint], key)
		}

		usedIds := make(map[int64]struct{}, len(rawKeys))
		now := common.GetTimestamp()
		for position, rawKey := range rawKeys {
			fingerprint := channelKeyFingerprint(channel.Type, rawKey)
			queue := available[fingerprint]
			if len(queue) > 0 {
				key := queue[0]
				available[fingerprint] = queue[1:]
				usedIds[key.Id] = struct{}{}

				if key.Position != position || key.Key != rawKey {
					if err := tx.Model(&ChannelKey{}).
						Where("id = ?", key.Id).
						Updates(map[string]any{
							"position":   position,
							"key":        rawKey,
							"updated_at": now,
						}).Error; err != nil {
						return err
					}
				}
				continue
			}

			status := ChannelKeyStatusEnabled
			reason := ""
			disabledAt := int64(0)
			if legacyStatus, ok := channel.ChannelInfo.MultiKeyStatusList[position]; ok {
				switch legacyStatus {
				case common.ChannelStatusManuallyDisabled:
					status = ChannelKeyStatusManuallyDisabled
				case common.ChannelStatusAutoDisabled:
					status = ChannelKeyStatusErrorDisabled
				}
			}
			if channel.ChannelInfo.MultiKeyDisabledReason != nil {
				reason = channel.ChannelInfo.MultiKeyDisabledReason[position]
			}
			if channel.ChannelInfo.MultiKeyDisabledTime != nil {
				disabledAt = channel.ChannelInfo.MultiKeyDisabledTime[position]
			}

			key := &ChannelKey{
				ChannelId:      channel.Id,
				Position:       position,
				Key:            rawKey,
				Fingerprint:    fingerprint,
				Status:         status,
				DisabledReason: reason,
				DisabledAt:     disabledAt,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := tx.Create(key).Error; err != nil {
				return err
			}
			usedIds[key.Id] = struct{}{}
		}

		for _, key := range existing {
			if _, ok := usedIds[key.Id]; ok {
				continue
			}
			// In-flight requests keep settling by stable ID after archival. Do
			// not block credential removal or keep the removed secret schedulable
			// merely because an older request still owns a reservation.
			if err := tx.Model(&ChannelKey{}).
				Where("id = ?", key.Id).
				Updates(map[string]any{
					"status":          ChannelKeyStatusArchived,
					"archived_at":     now,
					"disabled_reason": "credential removed from channel",
					"updated_at":      now,
				}).Error; err != nil {
				return err
			}
		}

		// Re-evaluate only keys that inherit the channel default. Lowering the
		// default can exhaust a key immediately; raising/removing it can restore
		// capacity without requiring a separate per-key reset.
		var inherited []*ChannelKey
		if err := lockForUpdate(tx).
			Where("channel_id = ? AND quota_limit IS NULL AND status <> ?", channel.Id, ChannelKeyStatusArchived).
			Find(&inherited).Error; err != nil {
			return err
		}
		for _, key := range inherited {
			shouldExhaust := channel.KeyQuotaLimit > 0 && key.QuotaUsed >= channel.KeyQuotaLimit
			switch {
			case shouldExhaust && key.Status == ChannelKeyStatusEnabled:
				key.Status = ChannelKeyStatusQuotaExhausted
				key.DisabledReason = "quota limit exhausted"
				key.DisabledAt = now
				key.ExhaustedAt = now
				key.ArchivedAt = now
				key.UpdatedAt = now
				if err := archiveExhaustedChannelKey(
					tx, key, channel.KeyQuotaLimit, "", key.DisabledReason, now,
				); err != nil {
					return err
				}
				if err := tx.Save(key).Error; err != nil {
					return err
				}
			case !shouldExhaust && key.Status == ChannelKeyStatusQuotaExhausted:
				key.Status = ChannelKeyStatusEnabled
				key.DisabledReason = ""
				key.DisabledAt = 0
				key.ExhaustedAt = 0
				key.ArchivedAt = 0
				key.UpdatedAt = now
				if err := markChannelKeyArchivesRestored(tx, key.Id, now); err != nil {
					return err
				}
				if err := tx.Save(key).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func effectiveChannelKeyQuotaLimit(tx *gorm.DB, key *ChannelKey) (int64, error) {
	if key.QuotaLimit != nil {
		return *key.QuotaLimit, nil
	}
	var channel Channel
	if err := tx.Select("key_quota_limit").Where("id = ?", key.ChannelId).First(&channel).Error; err != nil {
		return 0, err
	}
	return channel.KeyQuotaLimit, nil
}

// archiveExhaustedChannelKey writes the independent archive snapshot and audit
// event inside the caller's accounting transaction. Callers invoke it only on
// the enabled -> quota-exhausted transition, which prevents duplicate active
// archive entries under concurrent settlement.
func archiveExhaustedChannelKey(tx *gorm.DB, key *ChannelKey, limit int64, requestId, reason string, now int64) error {
	if err := tx.Create(&ChannelKeyArchive{
		OriginalKeyId:       key.Id,
		ChannelId:           key.ChannelId,
		Position:            key.Position,
		Key:                 key.Key,
		Fingerprint:         key.Fingerprint,
		RpmLimit:            key.RpmLimit,
		QuotaLimit:          key.QuotaLimit,
		ModelRpmLimits:      key.ModelRpmLimits,
		EffectiveQuotaLimit: limit,
		QuotaUsed:           key.QuotaUsed,
		QuotaReserved:       key.QuotaReserved,
		LifetimeQuota:       key.LifetimeQuota,
		Reason:              reason,
		ExhaustedAt:         now,
		ArchivedAt:          now,
	}).Error; err != nil {
		return err
	}
	return tx.Create(&ChannelKeyEvent{
		ChannelId:    key.ChannelId,
		ChannelKeyId: key.Id,
		EventType:    ChannelKeyEventQuotaExhausted,
		QuotaUsed:    key.QuotaUsed,
		QuotaLimit:   limit,
		RequestId:    requestId,
		Reason:       reason,
		CreatedAt:    now,
	}).Error
}

// markChannelKeyArchivesRestored closes every currently active archive for one
// stable key. Updating all matching rows also repairs historical duplicates
// safely if an older deployment created more than one open snapshot.
func markChannelKeyArchivesRestored(tx *gorm.DB, keyId int64, now int64) error {
	return tx.Model(&ChannelKeyArchive{}).
		Where("original_key_id = ? AND restored_at = 0", keyId).
		Update("restored_at", now).Error
}

// updateActiveChannelKeyArchiveAccounting keeps the independent snapshot
// accurate while requests admitted before exhaustion finish or release later.
func updateActiveChannelKeyArchiveAccounting(tx *gorm.DB, key *ChannelKey) error {
	return tx.Model(&ChannelKeyArchive{}).
		Where("original_key_id = ? AND restored_at = 0", key.Id).
		Updates(map[string]any{
			"quota_used":     key.QuotaUsed,
			"quota_reserved": key.QuotaReserved,
			"lifetime_quota": key.LifetimeQuota,
		}).Error
}

// ReserveChannelKeyQuota serializes reservations for one key with a row lock.
// The subtraction-based capacity check avoids overflow in used+reserved+quota.
func ReserveChannelKeyQuota(attemptId, requestId string, channelKeyId int64, quota int64, expiresAt int64) (*ChannelKeyQuotaReservation, error) {
	if attemptId == "" || requestId == "" || channelKeyId <= 0 {
		return nil, errors.New("invalid channel key quota reservation")
	}
	if quota < 0 || quota > common.MaxQuota {
		return nil, fmt.Errorf("invalid channel key quota: %d", quota)
	}

	var reservation *ChannelKeyQuotaReservation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := lockForUpdate(tx).Where("id = ?", channelKeyId).First(&key).Error; err != nil {
			return err
		}
		if key.Status != ChannelKeyStatusEnabled {
			return ErrChannelKeyUnavailable
		}

		// Reclaim reservations left by a crashed gateway before checking
		// capacity. The query is skipped when the aggregate is zero, keeping the
		// normal sequential hot path to one locked key row plus one insert.
		if key.QuotaReserved > 0 {
			var expired []ChannelKeyQuotaReservation
			now := common.GetTimestamp()
			if err := lockForUpdate(tx).
				Where("channel_key_id = ? AND status = ? AND expires_at > 0 AND expires_at <= ?",
					key.Id, ChannelKeyReservationReserved, now).
				Find(&expired).Error; err != nil {
				return err
			}
			var expiredQuota int64
			expiredIds := make([]int64, 0, len(expired))
			for _, stale := range expired {
				if stale.ReservedQuota > math.MaxInt64-expiredQuota {
					return errors.New("expired channel key reservation overflow")
				}
				expiredQuota += stale.ReservedQuota
				expiredIds = append(expiredIds, stale.Id)
			}
			if expiredQuota > 0 {
				if expiredQuota > key.QuotaReserved {
					return errors.New("expired channel key reservation exceeds aggregate")
				}
				key.QuotaReserved -= expiredQuota
				if err := tx.Model(&ChannelKeyQuotaReservation{}).
					Where("id IN ?", expiredIds).
					Updates(map[string]any{
						"status":     ChannelKeyReservationReleased,
						"updated_at": now,
					}).Error; err != nil {
					return err
				}
			}
		}

		var existing ChannelKeyQuotaReservation
		err := tx.Where("attempt_id = ?", attemptId).First(&existing).Error
		if err == nil {
			if existing.Status != ChannelKeyReservationReserved {
				return ErrChannelKeyReservationClosed
			}
			reservation = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		limit, err := effectiveChannelKeyQuotaLimit(tx, &key)
		if err != nil {
			return err
		}
		reservedQuota := quota
		if limit > 0 && quota > 0 {
			remaining := limit - key.QuotaUsed
			if remaining <= key.QuotaReserved {
				return ErrChannelKeyQuotaInsufficient
			}
			// Reserve the remaining capacity for exactly one final request when
			// the normal estimate is larger than the budget left on this key.
			// The row lock prevents another request from using the same tail
			// capacity. Settlement records the full actual charge and archives
			// the key if that final request reaches or crosses the limit.
			available := remaining - key.QuotaReserved
			if reservedQuota > available {
				reservedQuota = available
			}
		}
		if reservedQuota > math.MaxInt64-key.QuotaReserved {
			return errors.New("channel key reserved quota overflow")
		}

		key.QuotaReserved += reservedQuota
		key.UpdatedAt = common.GetTimestamp()
		if err := tx.Model(&ChannelKey{}).
			Where("id = ?", key.Id).
			Select("quota_reserved", "updated_at").
			Updates(&key).Error; err != nil {
			return err
		}

		reservation = &ChannelKeyQuotaReservation{
			AttemptId:     attemptId,
			RequestId:     requestId,
			ChannelId:     key.ChannelId,
			ChannelKeyId:  key.Id,
			ReservedQuota: reservedQuota,
			Status:        ChannelKeyReservationReserved,
			ExpiresAt:     expiresAt,
			CreatedAt:     common.GetTimestamp(),
			UpdatedAt:     common.GetTimestamp(),
		}
		return tx.Create(reservation).Error
	})
	return reservation, err
}

type ChannelKeyQuotaCommitResult struct {
	ChannelId    int
	ChannelKeyId int64
	QuotaUsed    int64
	QuotaLimit   int64
	Exhausted    bool
}

// CommitChannelKeyQuota is idempotent and copies an exhausted credential into
// the independent archive library in the same transaction as accounting. The
// active row becomes a non-schedulable tombstone for delayed task settlement.
func CommitChannelKeyQuota(reservationId int64, actualQuota int64) (*ChannelKeyQuotaCommitResult, error) {
	if reservationId <= 0 || actualQuota < 0 || actualQuota > common.MaxQuota {
		return nil, errors.New("invalid channel key quota settlement")
	}

	result := &ChannelKeyQuotaCommitResult{}
	exhaustionTransition := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var reservation ChannelKeyQuotaReservation
		if err := lockForUpdate(tx).Where("id = ?", reservationId).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == ChannelKeyReservationCommitted {
			result.ChannelId = reservation.ChannelId
			result.ChannelKeyId = reservation.ChannelKeyId
			return nil
		}
		if reservation.Status != ChannelKeyReservationReserved {
			return ErrChannelKeyReservationClosed
		}

		var key ChannelKey
		if err := lockForUpdate(tx).Where("id = ?", reservation.ChannelKeyId).First(&key).Error; err != nil {
			return err
		}
		if key.QuotaReserved < reservation.ReservedQuota {
			return errors.New("channel key reserved quota is inconsistent")
		}
		if actualQuota > math.MaxInt64-key.QuotaUsed || actualQuota > math.MaxInt64-key.LifetimeQuota {
			return errors.New("channel key consumed quota overflow")
		}

		key.QuotaReserved -= reservation.ReservedQuota
		key.QuotaUsed += actualQuota
		key.LifetimeQuota += actualQuota
		key.UpdatedAt = common.GetTimestamp()

		limit, err := effectiveChannelKeyQuotaLimit(tx, &key)
		if err != nil {
			return err
		}
		exhausted := limit > 0 && key.QuotaUsed >= limit
		if exhausted && key.Status == ChannelKeyStatusEnabled {
			exhaustionTransition = true
			now := common.GetTimestamp()
			key.Status = ChannelKeyStatusQuotaExhausted
			key.DisabledReason = "quota limit exhausted"
			key.DisabledAt = now
			key.ExhaustedAt = now
			key.ArchivedAt = now
			if err := archiveExhaustedChannelKey(
				tx, &key, limit, reservation.RequestId, key.DisabledReason, now,
			); err != nil {
				return err
			}
		} else if key.Status == ChannelKeyStatusQuotaExhausted {
			if err := updateActiveChannelKeyArchiveAccounting(tx, &key); err != nil {
				return err
			}
		}

		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChannelKeyQuotaReservation{}).
			Where("id = ?", reservation.Id).
			Updates(map[string]any{
				"actual_quota": actualQuota,
				"status":       ChannelKeyReservationCommitted,
				"updated_at":   common.GetTimestamp(),
			}).Error; err != nil {
			return err
		}

		result.ChannelId = key.ChannelId
		result.ChannelKeyId = key.Id
		result.QuotaUsed = key.QuotaUsed
		result.QuotaLimit = limit
		result.Exhausted = exhausted
		return nil
	})
	if err != nil {
		return result, err
	}
	if exhaustionTransition {
		if err := refreshChannelKeyDerivedState(result.ChannelId); err != nil {
			return result, err
		}
	}
	return result, nil
}

func ReleaseChannelKeyQuota(reservationId int64) error {
	if reservationId <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var reservation ChannelKeyQuotaReservation
		if err := lockForUpdate(tx).Where("id = ?", reservationId).First(&reservation).Error; err != nil {
			return err
		}
		if reservation.Status == ChannelKeyReservationReleased || reservation.Status == ChannelKeyReservationCommitted {
			return nil
		}
		if reservation.Status != ChannelKeyReservationReserved {
			return ErrChannelKeyReservationClosed
		}

		var key ChannelKey
		if err := lockForUpdate(tx).Where("id = ?", reservation.ChannelKeyId).First(&key).Error; err != nil {
			return err
		}
		if key.QuotaReserved < reservation.ReservedQuota {
			return errors.New("channel key reserved quota is inconsistent")
		}

		key.QuotaReserved -= reservation.ReservedQuota
		key.UpdatedAt = common.GetTimestamp()
		if key.Status == ChannelKeyStatusQuotaExhausted {
			if err := updateActiveChannelKeyArchiveAccounting(tx, &key); err != nil {
				return err
			}
		}
		if err := tx.Model(&ChannelKey{}).
			Where("id = ?", key.Id).
			Select("quota_reserved", "updated_at").
			Updates(&key).Error; err != nil {
			return err
		}
		return tx.Model(&ChannelKeyQuotaReservation{}).
			Where("id = ?", reservation.Id).
			Updates(map[string]any{
				"status":     ChannelKeyReservationReleased,
				"updated_at": common.GetTimestamp(),
			}).Error
	})
}

func GetChannelKeys(channelId int, status *int, offset, limit int) ([]*ChannelKey, int64, error) {
	query := DB.Model(&ChannelKey{}).
		Where("channel_id = ? AND status <> ?", channelId, ChannelKeyStatusArchived)
	if status != nil {
		query = query.Where("status = ?", *status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var keys []*ChannelKey
	if err := query.Order("position ASC, id ASC").Offset(offset).Limit(limit).Find(&keys).Error; err != nil {
		return nil, 0, err
	}
	return keys, total, nil
}

// GetChannelKeyArchives lists only currently archived exhaustion cycles. A
// restored snapshot remains in the table for audit history but is excluded from
// the active exhausted-key library.
func GetChannelKeyArchives(channelId, offset, limit int) ([]*ChannelKeyArchive, int64, error) {
	return GetChannelKeyArchivesGlobal(&channelId, offset, limit)
}

// GetChannelKeyArchivesGlobal powers the standalone exhausted-key library.
// channelId=nil returns archives across all channels; callers can provide a
// channel ID for server-side filtering without loading unrelated secrets.
func GetChannelKeyArchivesGlobal(channelId *int, offset, limit int) ([]*ChannelKeyArchive, int64, error) {
	query := DB.Model(&ChannelKeyArchive{}).Where("restored_at = 0")
	if channelId != nil {
		query = query.Where("channel_id = ?", *channelId)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var archives []*ChannelKeyArchive
	if err := query.
		Order("archived_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Find(&archives).Error; err != nil {
		return nil, 0, err
	}
	return archives, total, nil
}

func GetChannelKeyArchiveSecret(archiveId int64) (string, error) {
	if archiveId <= 0 {
		return "", errors.New("invalid channel key archive")
	}
	var archive ChannelKeyArchive
	if err := DB.Select("key").
		Where("id = ? AND restored_at = 0", archiveId).
		First(&archive).Error; err != nil {
		return "", err
	}
	if archive.Key == "" {
		return "", errors.New("archived channel key is unavailable")
	}
	return archive.Key, nil
}

// DeleteChannelKeyArchive permanently removes an exhausted credential from its
// channel and deletes every archived secret snapshot for the stable key. The
// stable accounting row is retained with an empty credential so delayed
// settlement and event history can continue referring to its ID.
func DeleteChannelKeyArchive(archiveId int64) error {
	if archiveId <= 0 {
		return errors.New("invalid channel key archive")
	}

	var archive ChannelKeyArchive
	if err := DB.Select("channel_id").
		Where("id = ? AND restored_at = 0", archiveId).
		First(&archive).Error; err != nil {
		return err
	}
	channelId := archive.ChannelId
	lock := GetChannelPollingLock(channelId)
	lock.Lock()
	defer lock.Unlock()

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).
			Where("id = ? AND restored_at = 0", archiveId).
			First(&archive).Error; err != nil {
			return err
		}

		var key ChannelKey
		if err := lockForUpdate(tx).
			Where("id = ? AND channel_id = ?", archive.OriginalKeyId, archive.ChannelId).
			First(&key).Error; err != nil {
			return err
		}
		if key.Status != ChannelKeyStatusQuotaExhausted {
			return errors.New("channel key is not quota exhausted")
		}

		var channel Channel
		if err := lockForUpdate(tx).Where("id = ?", archive.ChannelId).First(&channel).Error; err != nil {
			return err
		}
		keys := channel.GetKeys()
		if len(keys) <= 1 {
			return errors.New("cannot delete the last channel key")
		}

		removeAt := key.Position
		if removeAt < 0 || removeAt >= len(keys) ||
			channelKeyFingerprint(channel.Type, keys[removeAt]) != key.Fingerprint {
			removeAt = -1
			for position, candidate := range keys {
				if channelKeyFingerprint(channel.Type, candidate) == key.Fingerprint {
					removeAt = position
					break
				}
			}
		}
		if removeAt < 0 {
			return errors.New("archived channel key is no longer present in the channel")
		}

		remaining := append(append([]string{}, keys[:removeAt]...), keys[removeAt+1:]...)
		channel.Key = strings.Join(remaining, "\n")
		channel.Keys = nil
		channel.ChannelInfo.MultiKeySize = len(remaining)
		if err := tx.Model(&Channel{}).
			Where("id = ?", channel.Id).
			Updates(map[string]any{
				"key":          channel.Key,
				"channel_info": channel.ChannelInfo,
			}).Error; err != nil {
			return err
		}
		if err := syncChannelKeysWithDB(tx, &channel); err != nil {
			return err
		}
		if err := tx.Model(&ChannelKey{}).
			Where("id = ?", key.Id).
			Update("key", "").Error; err != nil {
			return err
		}
		return tx.Where("original_key_id = ?", key.Id).Delete(&ChannelKeyArchive{}).Error
	})
	if err != nil {
		return err
	}
	if err := refreshChannelKeyDerivedState(channelId); err != nil {
		return err
	}
	InitChannelCache()
	return nil
}

// RestoreChannelKeyArchive starts a new quota cycle for an exhausted key. The
// active accounting tombstone and archive snapshot are locked together so two
// administrators cannot restore the same key twice.
func RestoreChannelKeyArchive(channelId int, archiveId int64) error {
	return restoreChannelKeyArchive(&channelId, archiveId)
}

// RestoreChannelKeyArchiveById is used by the global archive page, where the
// archive row itself is the authoritative source of its parent channel.
func RestoreChannelKeyArchiveById(archiveId int64) error {
	return restoreChannelKeyArchive(nil, archiveId)
}

func restoreChannelKeyArchive(channelId *int, archiveId int64) error {
	var restoredChannelId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var archive ChannelKeyArchive
		archiveQuery := lockForUpdate(tx).Where("id = ? AND restored_at = 0", archiveId)
		if channelId != nil {
			archiveQuery = archiveQuery.Where("channel_id = ?", *channelId)
		}
		if err := archiveQuery.First(&archive).Error; err != nil {
			return err
		}
		restoredChannelId = archive.ChannelId

		var key ChannelKey
		if err := lockForUpdate(tx).
			Where("id = ? AND channel_id = ?", archive.OriginalKeyId, archive.ChannelId).
			First(&key).Error; err != nil {
			return err
		}
		if key.Status != ChannelKeyStatusQuotaExhausted {
			return errors.New("channel key is not quota exhausted")
		}
		if key.QuotaReserved != 0 {
			return errors.New("channel key has in-flight quota reservations")
		}

		now := common.GetTimestamp()
		key.QuotaUsed = 0
		key.Status = ChannelKeyStatusEnabled
		key.DisabledReason = ""
		key.DisabledAt = 0
		key.ExhaustedAt = 0
		key.ArchivedAt = 0
		key.UpdatedAt = now
		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		if err := markChannelKeyArchivesRestored(tx, key.Id, now); err != nil {
			return err
		}
		return tx.Create(&ChannelKeyEvent{
			ChannelId:    archive.ChannelId,
			ChannelKeyId: key.Id,
			EventType:    ChannelKeyEventRestored,
			QuotaUsed:    0,
			Reason:       "key restored from exhausted archive",
			CreatedAt:    now,
		}).Error
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(restoredChannelId)
}

func GetChannelKeyByPosition(channelId, position int) (*ChannelKey, error) {
	var key ChannelKey
	err := DB.Where("channel_id = ? AND position = ? AND status <> ?", channelId, position, ChannelKeyStatusArchived).
		First(&key).Error
	return &key, err
}

func GetActiveChannelKeys(channelId int) ([]*ChannelKey, error) {
	var keys []*ChannelKey
	err := DB.Where("channel_id = ? AND status <> ?", channelId, ChannelKeyStatusArchived).
		Order("position ASC").
		Find(&keys).Error
	return keys, err
}

// SetChannelKeyOperationalStatus mirrors legacy multi-key management and
// upstream error disable operations into the stable row. quota_exhausted keys
// cannot be re-enabled through this generic operation; they require an explicit
// quota reset or limit increase.
func SetChannelKeyOperationalStatus(channelId int, keyId int64, status int, reason string) error {
	if status != ChannelKeyStatusEnabled &&
		status != ChannelKeyStatusManuallyDisabled &&
		status != ChannelKeyStatusErrorDisabled {
		return errors.New("invalid operational channel key status")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := lockForUpdate(tx).
			Where("id = ? AND channel_id = ?", keyId, channelId).
			First(&key).Error; err != nil {
			return err
		}
		if key.Status == ChannelKeyStatusQuotaExhausted {
			if status == ChannelKeyStatusEnabled {
				return errors.New("quota-exhausted key must be reset before enabling")
			}
			// Quota exhaustion has stronger semantics than an upstream-error
			// disable and must not be overwritten by a concurrent request.
			return nil
		}
		if key.Status == ChannelKeyStatusArchived {
			return errors.New("archived key cannot be changed")
		}

		key.Status = status
		key.UpdatedAt = common.GetTimestamp()
		if status == ChannelKeyStatusEnabled {
			key.DisabledReason = ""
			key.DisabledAt = 0
		} else {
			key.DisabledReason = reason
			key.DisabledAt = common.GetTimestamp()
		}
		return tx.Save(&key).Error
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channelId)
}

func SetChannelKeyStatusByCredential(channel *Channel, credential string, status int, reason string) error {
	if channel == nil || credential == "" {
		return nil
	}
	fingerprint := channelKeyFingerprint(channel.Type, credential)
	var key ChannelKey
	if err := DB.Where("channel_id = ? AND fingerprint = ? AND status <> ?", channel.Id, fingerprint, ChannelKeyStatusArchived).
		Order("position ASC").
		First(&key).Error; err != nil {
		return err
	}
	return SetChannelKeyOperationalStatus(channel.Id, key.Id, status, reason)
}

func SetAllChannelKeysOperationalStatus(channelId int, status int, reason string) error {
	if status != ChannelKeyStatusEnabled && status != ChannelKeyStatusManuallyDisabled {
		return errors.New("invalid bulk channel key status")
	}
	now := common.GetTimestamp()
	query := DB.Model(&ChannelKey{}).Where("channel_id = ?", channelId)
	updates := map[string]any{"status": status, "updated_at": now}
	if status == ChannelKeyStatusEnabled {
		query = query.Where("status IN ?", []int{ChannelKeyStatusManuallyDisabled, ChannelKeyStatusErrorDisabled})
		updates["disabled_reason"] = ""
		updates["disabled_at"] = 0
	} else {
		query = query.Where("status = ?", ChannelKeyStatusEnabled)
		updates["disabled_reason"] = reason
		updates["disabled_at"] = now
	}
	if err := query.Updates(updates).Error; err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channelId)
}

func UpdateChannelKeyLimits(
	channelId int,
	keyId int64,
	rpmLimit *int,
	quotaLimit *int64,
	modelRpmLimits *ChannelKeyModelRpmLimits,
) error {
	if rpmLimit != nil && *rpmLimit < 0 {
		return errors.New("key RPM limit cannot be negative")
	}
	if quotaLimit != nil && (*quotaLimit < 0 || *quotaLimit > int64(common.MaxQuota)) {
		return fmt.Errorf("key quota limit must be between 0 and %d", common.MaxQuota)
	}
	var normalizedModelRpmLimits ChannelKeyModelRpmLimits
	if modelRpmLimits != nil {
		var err error
		normalizedModelRpmLimits, err = modelRpmLimits.Normalize()
		if err != nil {
			return err
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := lockForUpdate(tx).
			Where("id = ? AND channel_id = ?", keyId, channelId).
			First(&key).Error; err != nil {
			return err
		}
		if key.Status == ChannelKeyStatusArchived {
			return errors.New("archived key cannot be changed")
		}

		key.RpmLimit = rpmLimit
		key.QuotaLimit = quotaLimit
		if modelRpmLimits != nil {
			key.ModelRpmLimits = normalizedModelRpmLimits
		}
		key.UpdatedAt = common.GetTimestamp()
		effectiveQuotaLimit, err := effectiveChannelKeyQuotaLimit(tx, &key)
		if err != nil {
			return err
		}
		if key.Status == ChannelKeyStatusQuotaExhausted &&
			(effectiveQuotaLimit == 0 || effectiveQuotaLimit > key.QuotaUsed) {
			key.Status = ChannelKeyStatusEnabled
			key.DisabledReason = ""
			key.DisabledAt = 0
			key.ExhaustedAt = 0
			key.ArchivedAt = 0
			if err := markChannelKeyArchivesRestored(tx, key.Id, common.GetTimestamp()); err != nil {
				return err
			}
		}
		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		eventQuotaLimit := int64(0)
		if quotaLimit != nil {
			eventQuotaLimit = *quotaLimit
		}
		return tx.Create(&ChannelKeyEvent{
			ChannelId:    channelId,
			ChannelKeyId: keyId,
			EventType:    ChannelKeyEventLimitChanged,
			QuotaUsed:    key.QuotaUsed,
			QuotaLimit:   eventQuotaLimit,
			Reason:       "key limits updated",
			CreatedAt:    common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channelId)
}

// ResetChannelKeyQuota starts a new budget cycle without erasing lifetime
// consumption. In-flight reservations make a reset ambiguous, so the operation
// is rejected until all synchronous/asynchronous attempts settle.
func ResetChannelKeyQuota(channelId int, keyId int64) error {
	err := DB.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := lockForUpdate(tx).
			Where("id = ? AND channel_id = ?", keyId, channelId).
			First(&key).Error; err != nil {
			return err
		}
		if key.Status == ChannelKeyStatusArchived {
			return errors.New("archived key cannot be reset")
		}
		if key.QuotaReserved != 0 {
			return errors.New("channel key has in-flight quota reservations")
		}

		key.QuotaUsed = 0
		key.Status = ChannelKeyStatusEnabled
		key.DisabledReason = ""
		key.DisabledAt = 0
		key.ExhaustedAt = 0
		key.ArchivedAt = 0
		key.UpdatedAt = common.GetTimestamp()
		if err := markChannelKeyArchivesRestored(tx, key.Id, key.UpdatedAt); err != nil {
			return err
		}
		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		return tx.Create(&ChannelKeyEvent{
			ChannelId:    channelId,
			ChannelKeyId: keyId,
			EventType:    ChannelKeyEventQuotaReset,
			QuotaUsed:    0,
			Reason:       "key quota reset",
			CreatedAt:    common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channelId)
}

// AdjustChannelKeyQuota applies a post-settlement correction used by delayed
// task refund/recalculation. The key row is locked so concurrent task updates
// cannot underflow QuotaUsed or lose increments.
func AdjustChannelKeyQuota(keyId int64, delta int64, requestId, reason string) error {
	if keyId <= 0 || delta == 0 {
		return nil
	}
	if delta == math.MinInt64 {
		return errors.New("channel key quota adjustment underflow")
	}
	var channelId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var key ChannelKey
		if err := lockForUpdate(tx).Where("id = ?", keyId).First(&key).Error; err != nil {
			return err
		}
		channelId = key.ChannelId
		if delta < 0 && key.QuotaUsed < -delta {
			return errors.New("channel key quota adjustment would underflow")
		}
		if delta > 0 && (delta > math.MaxInt64-key.QuotaUsed || delta > math.MaxInt64-key.LifetimeQuota) {
			return errors.New("channel key quota adjustment would overflow")
		}

		key.QuotaUsed += delta
		if delta > 0 {
			key.LifetimeQuota += delta
		}
		limit, err := effectiveChannelKeyQuotaLimit(tx, &key)
		if err != nil {
			return err
		}
		now := common.GetTimestamp()
		if limit > 0 && key.QuotaUsed >= limit && key.Status == ChannelKeyStatusEnabled {
			key.Status = ChannelKeyStatusQuotaExhausted
			key.DisabledReason = "quota limit exhausted"
			key.DisabledAt = now
			key.ExhaustedAt = now
			key.ArchivedAt = now
			if err := archiveExhaustedChannelKey(
				tx, &key, limit, requestId, key.DisabledReason, now,
			); err != nil {
				return err
			}
		} else if key.Status == ChannelKeyStatusQuotaExhausted && (limit == 0 || key.QuotaUsed < limit) {
			key.Status = ChannelKeyStatusEnabled
			key.DisabledReason = ""
			key.DisabledAt = 0
			key.ExhaustedAt = 0
			key.ArchivedAt = 0
			if err := markChannelKeyArchivesRestored(tx, key.Id, now); err != nil {
				return err
			}
		}
		key.UpdatedAt = now
		if err := tx.Save(&key).Error; err != nil {
			return err
		}
		return tx.Create(&ChannelKeyEvent{
			ChannelId:    key.ChannelId,
			ChannelKeyId: key.Id,
			EventType:    ChannelKeyEventQuotaAdjusted,
			QuotaUsed:    key.QuotaUsed,
			QuotaLimit:   limit,
			RequestId:    requestId,
			Reason:       reason,
			CreatedAt:    now,
		}).Error
	})
	if err != nil {
		return err
	}
	return refreshChannelKeyDerivedState(channelId)
}

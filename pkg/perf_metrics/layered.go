package perfmetrics

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/go-redis/redis/v8"
)

const layeredBucketTTL = 2 * time.Hour

type LayeredDimension string

const (
	LayeredDimAll         LayeredDimension = "all"
	LayeredDimChannelType LayeredDimension = "channel_type"
	LayeredDimChannel     LayeredDimension = "channel"
	LayeredDimModel       LayeredDimension = "model"
)

type LayeredWindow struct {
	Label        string  `json:"label"`
	Seconds      int64   `json:"seconds"`
	SuccessRate  float64 `json:"success_rate"`
	RequestCount int64   `json:"request_count"`
}

type LayeredResult struct {
	AsOf            int64           `json:"as_of"`
	Dimension       string          `json:"dimension"`
	ChannelType     int             `json:"channel_type,omitempty"`
	ChannelTypeName string          `json:"channel_type_name,omitempty"`
	ChannelID       int             `json:"channel_id,omitempty"`
	ModelName       string          `json:"model_name,omitempty"`
	Windows         []LayeredWindow `json:"windows"`
}

type layeredWindowDef struct {
	Label   string
	Seconds int64
}

var layeredWindows = []layeredWindowDef{
	{Label: "1min", Seconds: 60},
	{Label: "5min", Seconds: 300},
	{Label: "15min", Seconds: 900},
	{Label: "30min", Seconds: 1800},
	{Label: "1h", Seconds: 3600},
}

type layeredBucketKey struct {
	dim    LayeredDimension
	id     int
	minute int64
}

type layeredModelKey struct {
	model  string
	minute int64
}

type layeredCounters struct {
	requestCount atomic.Int64
	successCount atomic.Int64
}

var layeredHotBuckets sync.Map
var layeredModelBuckets sync.Map

func minuteStart(ts int64) int64 {
	return ts - (ts % 60)
}

func layeredRedisKey(dim LayeredDimension, id int, minute int64) string {
	switch dim {
	case LayeredDimChannelType:
		return fmt.Sprintf("perf:layer:type:%d:%d", id, minute)
	case LayeredDimChannel:
		return fmt.Sprintf("perf:layer:ch:%d:%d", id, minute)
	default:
		return fmt.Sprintf("perf:layer:all:%d", minute)
	}
}

func layeredModelRedisKey(modelName string, minute int64) string {
	return fmt.Sprintf("perf:layer:model:%s:%d", modelName, minute)
}

func recordLayered(sample Sample) {
	now := time.Now().Unix()
	minute := minuteStart(now)
	modelName := strings.TrimSpace(sample.Model)

	incrLayeredMemory(LayeredDimAll, 0, minute, sample.Success)
	if sample.ChannelType > 0 {
		incrLayeredMemory(LayeredDimChannelType, sample.ChannelType, minute, sample.Success)
	}
	if sample.ChannelID > 0 {
		incrLayeredMemory(LayeredDimChannel, sample.ChannelID, minute, sample.Success)
	}
	if modelName != "" {
		incrLayeredModelMemory(modelName, minute, sample.Success)
	}

	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pipe := common.RDB.TxPipeline()
	addLayeredRedisIncr(pipe, ctx, layeredRedisKey(LayeredDimAll, 0, minute), sample.Success)
	if sample.ChannelType > 0 {
		addLayeredRedisIncr(pipe, ctx, layeredRedisKey(LayeredDimChannelType, sample.ChannelType, minute), sample.Success)
	}
	if sample.ChannelID > 0 {
		addLayeredRedisIncr(pipe, ctx, layeredRedisKey(LayeredDimChannel, sample.ChannelID, minute), sample.Success)
	}
	if modelName != "" {
		addLayeredRedisIncr(pipe, ctx, layeredModelRedisKey(modelName, minute), sample.Success)
	}
	_, _ = pipe.Exec(ctx)
}

func addLayeredRedisIncr(pipe redis.Pipeliner, ctx context.Context, key string, success bool) {
	pipe.HIncrBy(ctx, key, "req", 1)
	if success {
		pipe.HIncrBy(ctx, key, "ok", 1)
	}
	pipe.Expire(ctx, key, layeredBucketTTL)
}

func incrLayeredMemory(dim LayeredDimension, id int, minute int64, success bool) {
	key := layeredBucketKey{dim: dim, id: id, minute: minute}
	actual, _ := layeredHotBuckets.LoadOrStore(key, &layeredCounters{})
	counters := actual.(*layeredCounters)
	counters.requestCount.Add(1)
	if success {
		counters.successCount.Add(1)
	}
}

func incrLayeredModelMemory(modelName string, minute int64, success bool) {
	key := layeredModelKey{model: modelName, minute: minute}
	actual, _ := layeredModelBuckets.LoadOrStore(key, &layeredCounters{})
	counters := actual.(*layeredCounters)
	counters.requestCount.Add(1)
	if success {
		counters.successCount.Add(1)
	}
}

// Deprecated path kept for tests that call the old helper directly.
func incrLayeredBucket(dim LayeredDimension, id int, minute int64, success bool) {
	incrLayeredMemory(dim, id, minute, success)
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pipe := common.RDB.TxPipeline()
	addLayeredRedisIncr(pipe, ctx, layeredRedisKey(dim, id, minute), success)
	_, _ = pipe.Exec(ctx)
}

func QueryLayered(dim LayeredDimension, id int) (LayeredResult, error) {
	switch dim {
	case LayeredDimAll, LayeredDimChannelType, LayeredDimChannel:
	default:
		return LayeredResult{}, fmt.Errorf("invalid dimension")
	}
	if dim == LayeredDimChannelType && id <= 0 {
		return LayeredResult{}, fmt.Errorf("channel_type is required")
	}
	if dim == LayeredDimChannel && id <= 0 {
		return LayeredResult{}, fmt.Errorf("channel_id is required")
	}

	now := time.Now().Unix()
	result := LayeredResult{
		AsOf:      now,
		Dimension: string(dim),
		Windows:   make([]LayeredWindow, 0, len(layeredWindows)),
	}
	if dim == LayeredDimChannelType {
		result.ChannelType = id
		result.ChannelTypeName = constant.GetChannelTypeName(id)
	}
	if dim == LayeredDimChannel {
		result.ChannelID = id
	}

	useRedis := common.RedisEnabled && common.RDB != nil
	for _, window := range layeredWindows {
		req, ok := sumLayeredWindow(dim, id, now-window.Seconds, now, useRedis)
		result.Windows = append(result.Windows, buildLayeredWindow(window, req, ok))
	}
	return result, nil
}

func QueryLayeredModel(modelName string) (LayeredResult, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return LayeredResult{}, fmt.Errorf("model is required")
	}
	now := time.Now().Unix()
	result := LayeredResult{
		AsOf:      now,
		Dimension: string(LayeredDimModel),
		ModelName: modelName,
		Windows:   make([]LayeredWindow, 0, len(layeredWindows)),
	}
	useRedis := common.RedisEnabled && common.RDB != nil
	for _, window := range layeredWindows {
		req, ok := sumLayeredModelWindow(modelName, now-window.Seconds, now, useRedis)
		result.Windows = append(result.Windows, buildLayeredWindow(window, req, ok))
	}
	return result, nil
}

func buildLayeredWindow(window layeredWindowDef, req, ok int64) LayeredWindow {
	rate := 0.0
	if req > 0 {
		rate = math.Round(float64(ok)/float64(req)*10000) / 100
	}
	return LayeredWindow{
		Label:        window.Label,
		Seconds:      window.Seconds,
		SuccessRate:  rate,
		RequestCount: req,
	}
}

func sumLayeredWindow(dim LayeredDimension, id int, start, end int64, useRedis bool) (req, ok int64) {
	if end < start {
		return 0, 0
	}
	first := minuteStart(start)
	last := minuteStart(end)
	if useRedis {
		req, ok = sumLayeredRedis(dim, id, first, last)
		if req > 0 {
			return req, ok
		}
	}
	return sumLayeredMemory(dim, id, first, last)
}

func sumLayeredModelWindow(modelName string, start, end int64, useRedis bool) (req, ok int64) {
	if end < start {
		return 0, 0
	}
	first := minuteStart(start)
	last := minuteStart(end)
	if useRedis {
		req, ok = sumLayeredModelRedis(modelName, first, last)
		if req > 0 {
			return req, ok
		}
	}
	return sumLayeredModelMemory(modelName, first, last)
}

func sumLayeredMemory(dim LayeredDimension, id int, first, last int64) (req, ok int64) {
	for minute := first; minute <= last; minute += 60 {
		value, exists := layeredHotBuckets.Load(layeredBucketKey{dim: dim, id: id, minute: minute})
		if !exists {
			continue
		}
		counters := value.(*layeredCounters)
		req += counters.requestCount.Load()
		ok += counters.successCount.Load()
	}
	return req, ok
}

func sumLayeredModelMemory(modelName string, first, last int64) (req, ok int64) {
	for minute := first; minute <= last; minute += 60 {
		value, exists := layeredModelBuckets.Load(layeredModelKey{model: modelName, minute: minute})
		if !exists {
			continue
		}
		counters := value.(*layeredCounters)
		req += counters.requestCount.Load()
		ok += counters.successCount.Load()
	}
	return req, ok
}

func sumLayeredRedis(dim LayeredDimension, id int, first, last int64) (req, ok int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := common.RDB.Pipeline()
	cmds := make([]*redis.StringStringMapCmd, 0, int((last-first)/60)+1)
	for minute := first; minute <= last; minute += 60 {
		cmds = append(cmds, pipe.HGetAll(ctx, layeredRedisKey(dim, id, minute)))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return 0, 0
	}

	for _, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil || len(values) == 0 {
			continue
		}
		req += parseRedisInt(values["req"])
		ok += parseRedisInt(values["ok"])
	}
	return req, ok
}

func sumLayeredModelRedis(modelName string, first, last int64) (req, ok int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := common.RDB.Pipeline()
	cmds := make([]*redis.StringStringMapCmd, 0, int((last-first)/60)+1)
	for minute := first; minute <= last; minute += 60 {
		cmds = append(cmds, pipe.HGetAll(ctx, layeredModelRedisKey(modelName, minute)))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return 0, 0
	}
	for _, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil || len(values) == 0 {
			continue
		}
		req += parseRedisInt(values["req"])
		ok += parseRedisInt(values["ok"])
	}
	return req, ok
}

// ClearChannelLayeredMetrics removes in-memory and Redis layered stats for a channel.
func ClearChannelLayeredMetrics(channelID int) {
	if channelID <= 0 {
		return
	}
	layeredHotBuckets.Range(func(key, _ any) bool {
		k := key.(layeredBucketKey)
		if k.dim == LayeredDimChannel && k.id == channelID {
			layeredHotBuckets.Delete(key)
		}
		return true
	})

	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	now := time.Now().Unix()
	first := minuteStart(now - int64(layeredBucketTTL.Seconds()))
	last := minuteStart(now)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pipe := common.RDB.Pipeline()
	for minute := first; minute <= last; minute += 60 {
		pipe.Del(ctx, layeredRedisKey(LayeredDimChannel, channelID, minute))
	}
	_, _ = pipe.Exec(ctx)
}

func cleanupLayeredMemory(now int64) {
	cutoff := minuteStart(now) - int64(layeredBucketTTL.Seconds())
	layeredHotBuckets.Range(func(key, _ any) bool {
		k := key.(layeredBucketKey)
		if k.minute < cutoff {
			layeredHotBuckets.Delete(key)
		}
		return true
	})
	layeredModelBuckets.Range(func(key, _ any) bool {
		k := key.(layeredModelKey)
		if k.minute < cutoff {
			layeredModelBuckets.Delete(key)
		}
		return true
	})
}

func layeredCleanupLoop() {
	for {
		time.Sleep(5 * time.Minute)
		cleanupLayeredMemory(time.Now().Unix())
	}
}

const MaxLayeredChannelBatch = 50

type ChannelLayeredMetric struct {
	SuccessRate  float64 `json:"success_rate"`
	RequestCount int64   `json:"request_count"`
}

type ChannelLayeredBatchResult struct {
	AsOf          int64                        `json:"as_of"`
	WindowSeconds int64                        `json:"window_seconds"`
	Items         map[int]ChannelLayeredMetric `json:"items"`
}

func normalizeLayeredWindowSeconds(windowSeconds int64) int64 {
	switch windowSeconds {
	case 60, 300, 900, 1800, 3600:
		return windowSeconds
	default:
		return 3600
	}
}

// QueryLayeredChannels returns one window of success metrics for the given
// channel IDs. Callers should pre-filter to enabled channels.
func QueryLayeredChannels(ids []int, windowSeconds int64) (ChannelLayeredBatchResult, error) {
	windowSeconds = normalizeLayeredWindowSeconds(windowSeconds)
	now := time.Now().Unix()
	result := ChannelLayeredBatchResult{
		AsOf:          now,
		WindowSeconds: windowSeconds,
		Items:         make(map[int]ChannelLayeredMetric, len(ids)),
	}
	if len(ids) == 0 {
		return result, nil
	}
	if len(ids) > MaxLayeredChannelBatch {
		return ChannelLayeredBatchResult{}, fmt.Errorf("too many channel ids (max %d)", MaxLayeredChannelBatch)
	}

	uniqueIDs := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	useRedis := common.RedisEnabled && common.RDB != nil
	start := now - windowSeconds
	first := minuteStart(start)
	last := minuteStart(now)

	totals := make(map[int][2]int64, len(uniqueIDs))
	if useRedis {
		totals = sumLayeredChannelsRedis(uniqueIDs, first, last)
	}
	for _, id := range uniqueIDs {
		req, ok := totals[id][0], totals[id][1]
		if req == 0 {
			req, ok = sumLayeredMemory(LayeredDimChannel, id, first, last)
		}
		rate := 0.0
		if req > 0 {
			rate = math.Round(float64(ok)/float64(req)*10000) / 100
		}
		result.Items[id] = ChannelLayeredMetric{
			SuccessRate:  rate,
			RequestCount: req,
		}
	}
	return result, nil
}

func sumLayeredChannelsRedis(ids []int, first, last int64) map[int][2]int64 {
	totals := make(map[int][2]int64, len(ids))
	if len(ids) == 0 || !common.RedisEnabled || common.RDB == nil {
		return totals
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	type pending struct {
		id  int
		cmd *redis.StringStringMapCmd
	}
	pendings := make([]pending, 0, len(ids)*int((last-first)/60+1))
	pipe := common.RDB.Pipeline()
	for _, id := range ids {
		for minute := first; minute <= last; minute += 60 {
			pendings = append(pendings, pending{
				id:  id,
				cmd: pipe.HGetAll(ctx, layeredRedisKey(LayeredDimChannel, id, minute)),
			})
		}
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return totals
	}

	for _, item := range pendings {
		values, err := item.cmd.Result()
		if err != nil || len(values) == 0 {
			continue
		}
		cur := totals[item.id]
		cur[0] += parseRedisInt(values["req"])
		cur[1] += parseRedisInt(values["ok"])
		totals[item.id] = cur
	}
	return totals
}

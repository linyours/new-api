package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
)

// Channel health uses 1-minute buckets over a 5-minute window.
// List and alerts share the same counters so they cannot diverge.
const (
	channelHealthWindowMinutes     = 5
	channelHealthCooldown          = 20 * time.Minute
	channelHealthBucketTTL         = 6 * time.Minute
	channelHealthShardCount        = 32
	channelHealthRecentErrorLimit  = 5
	channelHealthErrorPreviewRunes = 200
)

// ChannelHealthSnapshot is the list/alert view of one channel's recent window.
type ChannelHealthSnapshot struct {
	WindowSeconds int     `json:"window_seconds"`
	Total         int64   `json:"total"`
	Success       int64   `json:"success"`
	Bad           int64   `json:"bad"`
	SuccessRate   float64 `json:"success_rate"`
	ErrorRate     float64 `json:"error_rate"`
	MinTotal      int     `json:"min_total"`
	AlertBelow    float64 `json:"alert_below"`
	SampleReady   bool    `json:"sample_ready"`
}

type channelHealthCounts struct {
	Total   int64
	Success int64
	Bad     int64
}

type channelHealthShard struct {
	mu      sync.Mutex
	buckets map[int]map[int64]channelHealthCounts
}

var (
	channelHealthShards     [channelHealthShardCount]channelHealthShard
	channelHealthAlertUntil sync.Map
	notifyChannelHealthFn   = notifyChannelHealthAlert
)

func init() {
	for i := range channelHealthShards {
		channelHealthShards[i].buckets = make(map[int]map[int64]channelHealthCounts)
	}
}

func healthNotifyType(channelID int) string {
	return fmt.Sprintf("%s_%d", dto.NotifyTypeChannelHealth, channelID)
}

func channelHealthRedisKey(channelID int, minute int64) string {
	return fmt.Sprintf("channel_health:%d:%d", channelID, minute)
}

func channelHealthAlertKey(channelID int) string {
	return fmt.Sprintf("channel_health_alert:%d", channelID)
}

func channelHealthShardOf(channelID int) *channelHealthShard {
	if channelID < 0 {
		channelID = -channelID
	}
	return &channelHealthShards[channelID%channelHealthShardCount]
}

func currentHealthMinute(now time.Time) int64 {
	return now.Unix() / 60
}

func finalizeHealthSnapshot(counts channelHealthCounts) ChannelHealthSnapshot {
	minTotal := operation_setting.GetChannelHealthMinTotal()
	alertBelow := float64(operation_setting.GetChannelHealthAlertBelowPercent()) / 100
	snap := ChannelHealthSnapshot{
		WindowSeconds: channelHealthWindowMinutes * 60,
		Total:         counts.Total,
		Success:       counts.Success,
		Bad:           counts.Bad,
		MinTotal:      minTotal,
		AlertBelow:    alertBelow,
	}
	if counts.Total > 0 {
		snap.SuccessRate = float64(counts.Success) / float64(counts.Total)
		snap.ErrorRate = float64(counts.Bad) / float64(counts.Total)
	}
	snap.SampleReady = counts.Total >= int64(minTotal)
	return snap
}

// ClassifyChannelHealthOutcome maps one finished channel attempt.
// Local business errors are ignored. Timeouts always count as bad.
// Other failures follow ChannelHealthErrorStatusCodes (default: not 400/429).
func ClassifyChannelHealthOutcome(err *types.NewAPIError) (total, success, bad int64) {
	if err == nil {
		return 1, 1, 0
	}
	if !types.IsRecordErrorLog(err) {
		return 0, 0, 0
	}
	if isChannelHealthTimeout(err) ||
		err.GetErrorCode() == types.ErrorCodeDoRequestFailed ||
		operation_setting.ShouldCountChannelHealthErrorStatusCode(err.StatusCode) {
		return 1, 0, 1
	}
	return 1, 0, 0
}

func isChannelHealthTimeout(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode == http.StatusRequestTimeout || err.StatusCode == http.StatusGatewayTimeout {
		return true
	}
	if err.GetErrorCode() == types.ErrorCodeChannelResponseTimeExceeded {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "i/o timeout")
}

// ObserveChannelHealth records one finished attempt and may enqueue an alert.
func ObserveChannelHealth(channelID int, channelName string, total, success, bad int64) {
	if channelID <= 0 || total < 0 || success < 0 || bad < 0 {
		return
	}
	if total == 0 && success == 0 && bad == 0 {
		return
	}
	if success > total || bad > total {
		return
	}

	now := time.Now()
	if err := incrChannelHealth(channelID, now, total, success, bad); err != nil {
		common.SysError("channel health incr: " + err.Error())
		return
	}
	snap := GetChannelHealthStats([]int{channelID})[strconv.Itoa(channelID)]
	MaybeAlertChannelHealth(channelID, channelName, snap)
}

// MaybeAlertChannelHealth sends one cooldown-gated alert if the current window
// already meets the success-rate rule. Used after live requests and when the
// channel list is loaded, so a red 0% row can notify without waiting for
// another upstream call.
func MaybeAlertChannelHealth(channelID int, channelName string, snap ChannelHealthSnapshot) {
	if channelID <= 0 || !shouldAlertChannelHealth(snap) {
		return
	}
	if snap.Bad <= 0 {
		common.SysLog(fmt.Sprintf(
			"channel health skip #%d %s: success=%.0f%% (%d/%d) below alert, but classified errors=0",
			channelID, channelName, snap.SuccessRate*100, snap.Success, snap.Total,
		))
		return
	}
	if !tryChannelHealthAlertLock(channelID, time.Now()) {
		return
	}
	gopool.Go(func() {
		if !notifyChannelHealthFn(channelID, channelName, snap) {
			releaseChannelHealthAlertLock(channelID)
		}
	})
}

// ObserveChannelHealthFromError classifies and records a failed channel attempt.
func ObserveChannelHealthFromError(channelID int, channelName string, err *types.NewAPIError) {
	total, success, bad := ClassifyChannelHealthOutcome(err)
	ObserveChannelHealth(channelID, channelName, total, success, bad)
}

// ObserveChannelHealthSuccess records a completed upstream request.
func ObserveChannelHealthSuccess(channelID int, channelName string) {
	ObserveChannelHealth(channelID, channelName, 1, 1, 0)
}

// GetChannelHealthStats reads the current window for the given channel IDs only.
func GetChannelHealthStats(ids []int) map[string]ChannelHealthSnapshot {
	out := make(map[string]ChannelHealthSnapshot, len(ids))
	if len(ids) == 0 {
		return out
	}
	unique := uniquePositiveIDs(ids)
	now := time.Now()
	if common.RedisEnabled && common.RDB != nil {
		stats, err := getChannelHealthRedis(unique, now)
		if err == nil {
			return stats
		}
		common.SysError("channel health redis read: " + err.Error())
	}
	return getChannelHealthMemory(unique, now)
}

func uniquePositiveIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func incrChannelHealth(channelID int, now time.Time, total, success, bad int64) error {
	if common.RedisEnabled && common.RDB != nil {
		if err := incrChannelHealthRedis(channelID, now, total, success, bad); err == nil {
			return nil
		} else {
			common.SysError("channel health redis incr fallback to memory: " + err.Error())
		}
	}
	incrChannelHealthMemory(channelID, now, total, success, bad)
	return nil
}

func incrChannelHealthRedis(channelID int, now time.Time, total, success, bad int64) error {
	key := channelHealthRedisKey(channelID, currentHealthMinute(now))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := common.RDB.TxPipeline()
	if total != 0 {
		pipe.HIncrBy(ctx, key, "total", total)
	}
	if success != 0 {
		pipe.HIncrBy(ctx, key, "success", success)
	}
	if bad != 0 {
		pipe.HIncrBy(ctx, key, "bad", bad)
	}
	pipe.Expire(ctx, key, channelHealthBucketTTL)
	_, err := pipe.Exec(ctx)
	return err
}

func getChannelHealthRedis(ids []int, now time.Time) (map[string]ChannelHealthSnapshot, error) {
	out := make(map[string]ChannelHealthSnapshot, len(ids))
	minute := currentHealthMinute(now)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pipe := common.RDB.Pipeline()
	cmds := make([]*redis.StringStringMapCmd, 0, len(ids)*channelHealthWindowMinutes)
	for _, id := range ids {
		for i := 0; i < channelHealthWindowMinutes; i++ {
			cmds = append(cmds, pipe.HGetAll(ctx, channelHealthRedisKey(id, minute-int64(i))))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	cmdIdx := 0
	for _, id := range ids {
		var counts channelHealthCounts
		for i := 0; i < channelHealthWindowMinutes; i++ {
			vals, err := cmds[cmdIdx].Result()
			cmdIdx++
			if err != nil && !errors.Is(err, redis.Nil) {
				return nil, err
			}
			counts.Total += parseHealthField(vals["total"])
			counts.Success += parseHealthField(vals["success"])
			counts.Bad += parseHealthField(vals["bad"])
		}
		out[strconv.Itoa(id)] = finalizeHealthSnapshot(counts)
	}
	return out, nil
}

func parseHealthField(raw string) int64 {
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func incrChannelHealthMemory(channelID int, now time.Time, total, success, bad int64) {
	minute := currentHealthMinute(now)
	shard := channelHealthShardOf(channelID)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	byMinute := shard.buckets[channelID]
	if byMinute == nil {
		byMinute = make(map[int64]channelHealthCounts)
		shard.buckets[channelID] = byMinute
	}
	expireBefore := minute - int64(channelHealthWindowMinutes)
	for key := range byMinute {
		if key < expireBefore {
			delete(byMinute, key)
		}
	}
	cur := byMinute[minute]
	cur.Total += total
	cur.Success += success
	cur.Bad += bad
	byMinute[minute] = cur
}

func getChannelHealthMemory(ids []int, now time.Time) map[string]ChannelHealthSnapshot {
	out := make(map[string]ChannelHealthSnapshot, len(ids))
	minute := currentHealthMinute(now)
	expireBefore := minute - int64(channelHealthWindowMinutes)
	for _, id := range ids {
		shard := channelHealthShardOf(id)
		shard.mu.Lock()
		var counts channelHealthCounts
		if byMinute := shard.buckets[id]; byMinute != nil {
			for key, bucket := range byMinute {
				if key < expireBefore {
					delete(byMinute, key)
					continue
				}
				if key > minute || key <= minute-int64(channelHealthWindowMinutes) {
					continue
				}
				counts.Total += bucket.Total
				counts.Success += bucket.Success
				counts.Bad += bucket.Bad
			}
		}
		shard.mu.Unlock()
		out[strconv.Itoa(id)] = finalizeHealthSnapshot(counts)
	}
	return out
}

func tryChannelHealthAlertLock(channelID int, now time.Time) bool {
	if common.RedisEnabled && common.RDB != nil {
		ok, err := tryChannelHealthAlertLockRedis(channelID)
		if err == nil {
			return ok
		}
		common.SysError("channel health alert lock redis fallback: " + err.Error())
	}
	return tryChannelHealthAlertLockMemory(channelID, now)
}

func tryChannelHealthAlertLockRedis(channelID int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return common.RDB.SetNX(ctx, channelHealthAlertKey(channelID), "1", channelHealthCooldown).Result()
}

func tryChannelHealthAlertLockMemory(channelID int, now time.Time) bool {
	until := now.Add(channelHealthCooldown)
	actual, loaded := channelHealthAlertUntil.LoadOrStore(channelID, until)
	if !loaded {
		return true
	}
	prev, ok := actual.(time.Time)
	if !ok || !now.Before(prev) {
		channelHealthAlertUntil.Store(channelID, until)
		return true
	}
	return false
}

func releaseChannelHealthAlertLock(channelID int) {
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := common.RDB.Del(ctx, channelHealthAlertKey(channelID)).Err(); err != nil {
			common.SysError("channel health alert lock release: " + err.Error())
		}
	}
	channelHealthAlertUntil.Delete(channelID)
}

var sendChannelHealthWebhookFn = SendWebhookNotify

func shouldAlertChannelHealth(snap ChannelHealthSnapshot) bool {
	if !snap.SampleReady {
		return false
	}
	return snap.SuccessRate*100 < float64(operation_setting.GetChannelHealthAlertBelowPercent())
}

func notifyChannelHealthAlert(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
	since := time.Now().Add(-time.Duration(channelHealthWindowMinutes) * time.Minute).Unix()
	logs, err := model.GetRecentErrorLogsByChannel(channelID, since, channelHealthRecentErrorLimit)
	if err != nil {
		common.SysError("channel health recent errors: " + err.Error())
	}
	subject, content := formatChannelHealthAlert(channelName, channelID, snap, logs)
	notifyType := healthNotifyType(channelID)
	webhookURL := strings.TrimSpace(operation_setting.ChannelHealthWebhookUrl)
	common.SysLog(fmt.Sprintf(
		"channel health alert: #%d %s success=%.0f%% (%d/%d) below=%d%% webhook=%t",
		channelID,
		channelName,
		snap.SuccessRate*100,
		snap.Success,
		snap.Total,
		operation_setting.GetChannelHealthAlertBelowPercent(),
		webhookURL != "",
	))
	sent := sendDedicatedChannelHealthWebhook(notifyType, subject, content)
	NotifyRootUser(notifyType, subject, content)
	if webhookURL == "" {
		return true
	}
	return sent
}

func formatChannelHealthAlert(channelName string, channelID int, snap ChannelHealthSnapshot, logs []*model.Log) (string, string) {
	subject := fmt.Sprintf("渠道「%s」（#%d）近5分钟成功率过低", channelName, channelID)
	var b strings.Builder
	fmt.Fprintf(&b, "**渠道** %s `#%d`\n", channelName, channelID)
	fmt.Fprintf(&b, "**近5分钟成功率** %.0f%%（%d / %d）\n", snap.SuccessRate*100, snap.Success, snap.Total)
	fmt.Fprintf(&b, "**告警线** 低于 %d%%\n", operation_setting.GetChannelHealthAlertBelowPercent())
	fmt.Fprintf(&b, "**计入错误** %d（超时与配置的错误状态码）\n", snap.Bad)
	b.WriteString("**处理** 未自动禁用")
	if len(logs) == 0 {
		b.WriteString("\n\n**最近错误** 暂无（窗口内可能尚未写入错误日志）")
		return subject, b.String()
	}
	b.WriteString("\n\n**最近错误**")
	for i, log := range logs {
		if log == nil {
			continue
		}
		fmt.Fprintf(&b, "\n%d. %s", i+1, formatChannelHealthErrorLine(log))
	}
	return subject, b.String()
}

func formatChannelHealthErrorLine(log *model.Log) string {
	when := time.Unix(log.CreatedAt, 0).Local().Format("01-02 15:04:05")
	parts := []string{when}
	if modelName := strings.TrimSpace(log.ModelName); modelName != "" {
		parts = append(parts, modelName)
	}
	if status := channelHealthErrorStatus(log.Other); status > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", status))
	}
	preview := truncateChannelHealthError(log.Content)
	if preview == "" {
		return strings.Join(parts, " · ")
	}
	return strings.Join(parts, " · ") + "\n    " + preview
}

func channelHealthErrorStatus(other string) int {
	fields, err := common.StrToMap(other)
	if err != nil || fields == nil {
		return 0
	}
	switch v := fields["status_code"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		n, convErr := strconv.Atoi(strings.TrimSpace(v))
		if convErr != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

func truncateChannelHealthError(raw string) string {
	s := strings.Join(strings.Fields(raw), " ")
	runes := []rune(s)
	if len(runes) <= channelHealthErrorPreviewRunes {
		return s
	}
	return string(runes[:channelHealthErrorPreviewRunes]) + "…"
}

func sendDedicatedChannelHealthWebhook(notifyType, subject, content string) bool {
	webhookURL := strings.TrimSpace(operation_setting.ChannelHealthWebhookUrl)
	if webhookURL == "" {
		common.SysError("channel health webhook skipped: ChannelHealthWebhookUrl is empty")
		return false
	}
	err := sendChannelHealthWebhookFn(
		webhookURL,
		operation_setting.ChannelHealthWebhookSecret,
		dto.NewNotify(notifyType, subject, content, nil),
	)
	if err != nil {
		common.SysError("channel health webhook failed: " + err.Error())
		return false
	}
	return true
}

func resetChannelHealthForTest() {
	for i := range channelHealthShards {
		shard := &channelHealthShards[i]
		shard.mu.Lock()
		shard.buckets = make(map[int]map[int64]channelHealthCounts)
		shard.mu.Unlock()
	}
	channelHealthAlertUntil = sync.Map{}
}

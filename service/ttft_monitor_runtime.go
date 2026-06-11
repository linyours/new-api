package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

func HandleConsumeLogForTTFTMonitor(logEntry *model.Log, userId int, params model.RecordConsumeLogParams) {
	if !common.IsMasterNode {
		return
	}
	if params.ChannelId <= 0 || !params.IsStream || params.Other == nil {
		return
	}

	cfg, err := GetTTFTMonitorConfig()
	if err != nil || !cfg.Enabled {
		return
	}
	if !isTTFTMonitorChannel(cfg.ChannelIDs, params.ChannelId) {
		return
	}

	frtMs, ok := getFRTMs(params.Other)
	if !ok {
		return
	}
	if frtMs <= float64(cfg.ThresholdSeconds*1000) {
		return
	}

	nowMs := time.Now().UnixMilli()
	count, err := addSlowTTFTEvent(params.ChannelId, nowMs, cfg.WindowSeconds)
	if err != nil {
		common.SysLog(fmt.Sprintf("ttft monitor add event failed: %v", err))
		return
	}
	if count < int64(cfg.CountThreshold) {
		return
	}

	cooldownKey := ttftMonitorCooldownRedisKey(params.ChannelId)
	inCooldown, err := isInTTFTMonitorCooldown(cooldownKey)
	if err != nil {
		common.SysLog(fmt.Sprintf("ttft monitor cooldown check failed: %v", err))
		return
	}
	if inCooldown {
		return
	}

	subject := fmt.Sprintf("【异常告警】通道 #%d TTFT 超阈值", params.ChannelId)
	content := buildTTFTAlertContent(logEntry, userId, params, cfg, frtMs, count)

	if cfg.NotifyType == "webhook" {
		notify := dto.NewNotify(
			fmt.Sprintf("%s_%d", dto.NotifyTypeTTFTAlert, params.ChannelId),
			subject,
			content,
			nil,
		)
		if err = SendWebhookNotify(cfg.WebhookURL, cfg.WebhookSecret, notify); err != nil {
			common.SysLog(fmt.Sprintf("ttft monitor webhook notify failed: %v", err))
			return
		}
	} else {
		// Use static type so existing per-type notification limit works as designed.
		NotifyRootUser(dto.NotifyTypeTTFTAlert, subject, content)
	}

	if err = setTTFTMonitorCooldown(cooldownKey, cfg.CooldownSeconds); err != nil {
		common.SysLog(fmt.Sprintf("ttft monitor cooldown set failed: %v", err))
	}
}

func buildTTFTAlertContent(
	logEntry *model.Log,
	userId int,
	params model.RecordConsumeLogParams,
	cfg TTFTMonitorConfig,
	frtMs float64,
	count int64,
) string {
	username := ""
	requestID := ""
	upstreamRequestID := ""
	if logEntry != nil {
		username = logEntry.Username
		requestID = logEntry.RequestId
		upstreamRequestID = logEntry.UpstreamRequestId
	}
	if username == "" {
		username = "-"
	}
	if requestID == "" {
		requestID = "-"
	}
	if upstreamRequestID == "" {
		upstreamRequestID = "-"
	}

	return fmt.Sprintf(
		"错误级别：严重\n"+
			"告警类型：TTFT 慢首字告警\n"+
			"模型信息：%s\n"+
			"用户信息：userId=%d, username=%s\n"+
			"分组信息：%s\n"+
			"渠道信息：channelId=%d, tokenId=%d, tokenName=%s\n"+
			"触发条件：最近 %d 秒内慢首字次数 >= %d（阈值 %d 秒）\n"+
			"当前值：慢首字次数=%d, 当前TTFT=%.3f 秒, 用时=%d 秒\n"+
			"请求路径：%v\n"+
			"requestId：%s\n"+
			"upstreamRequestId：%s\n"+
			"告警时间：%s",
		params.ModelName,
		userId,
		username,
		params.Group,
		params.ChannelId,
		params.TokenId,
		params.TokenName,
		cfg.WindowSeconds,
		cfg.CountThreshold,
		cfg.ThresholdSeconds,
		count,
		frtMs/1000.0,
		params.UseTimeSeconds,
		params.Other["request_path"],
		requestID,
		upstreamRequestID,
		time.Now().Format("2006-01-02 15:04:05"),
	)
}

func isTTFTMonitorChannel(channelIDs []int, channelID int) bool {
	for _, id := range channelIDs {
		if id == channelID {
			return true
		}
	}
	return false
}

func getFRTMs(other map[string]interface{}) (float64, bool) {
	raw, ok := other["frt"]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func ttftMonitorWindowRedisKey(channelID int) string {
	return fmt.Sprintf("monitor:ttft:window:%d", channelID)
}

func ttftMonitorCooldownRedisKey(channelID int) string {
	return fmt.Sprintf("monitor:ttft:cooldown:%d", channelID)
}

func addSlowTTFTEvent(channelID int, nowMs int64, windowSeconds int) (int64, error) {
	if common.RDB == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	ctx := context.Background()
	windowStart := nowMs - int64(windowSeconds)*1000
	key := ttftMonitorWindowRedisKey(channelID)
	member := fmt.Sprintf("%d-%d", nowMs, time.Now().UnixNano())

	pipe := common.RDB.TxPipeline()
	pipe.ZAdd(ctx, key, &redis.Z{Score: float64(nowMs), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart-1, 10))
	cardCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, time.Duration(windowSeconds+120)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return cardCmd.Val(), nil
}

func isInTTFTMonitorCooldown(key string) (bool, error) {
	if common.RDB == nil {
		return false, fmt.Errorf("redis client is nil")
	}
	ctx := context.Background()
	n, err := common.RDB.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func setTTFTMonitorCooldown(key string, cooldownSeconds int) error {
	if common.RDB == nil {
		return fmt.Errorf("redis client is nil")
	}
	ctx := context.Background()
	return common.RDB.Set(ctx, key, "1", time.Duration(cooldownSeconds)*time.Second).Err()
}

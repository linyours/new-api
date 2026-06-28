package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
)

// ErrorLogAlertPayload 是错误告警的上下文快照。
//
// 这里明确使用“快照结构体”而不是直接在 goroutine 中读取 gin.Context，原因是：
// 1) 请求结束后上下文生命周期不可控；
// 2) 并发重试时上下文内容可能被后续流程覆盖；
// 3) 快照可以确保告警内容与触发错误时刻严格一致。
type ErrorLogAlertPayload struct {
	UserID            int
	Username          string
	Group             string
	ModelName         string
	TokenName         string
	ChannelID         int
	ChannelName       string
	ChannelType       int
	StatusCode        int
	ErrorCode         string
	ErrorType         string
	ErrorMessage      string
	RequestPath       string
	RequestID         string
	UpstreamRequestID string
	UseTimeSeconds    int
	IsStream          bool
	RetryChain        []string
}

// HandleErrorLogWebhookAlert 在渠道报错时发送 webhook 告警。
//
// 关键设计点：
// - 与错误日志写库开关完全解耦，只受 ErrorWebhookAlertEnabled 控制；
// - 仅 master 节点发告警，避免多节点重复推送；
// - 发送失败仅打系统日志，不影响主请求链路。
func HandleErrorLogWebhookAlert(payload ErrorLogAlertPayload) {
	if !common.IsMasterNode {
		return
	}
	if !setting.ErrorWebhookAlertEnabled {
		return
	}

	webhookURL := strings.TrimSpace(setting.ErrorWebhookAlertURL)
	if webhookURL == "" {
		return
	}

	subject := fmt.Sprintf("【渠道错误告警】通道 #%d 调用失败", payload.ChannelID)
	content := buildErrorLogAlertContent(payload)
	notify := dto.NewNotify(dto.NotifyTypeChannelErrorAlert, subject, content, nil)
	if err := SendWebhookNotify(webhookURL, setting.ErrorWebhookAlertSecret, notify); err != nil {
		common.SysLog(fmt.Sprintf("error webhook alert failed: %v", err))
	}
}

func buildErrorLogAlertContent(payload ErrorLogAlertPayload) string {
	username := normalizeAlertField(payload.Username)
	group := normalizeAlertField(payload.Group)
	modelName := normalizeAlertField(payload.ModelName)
	tokenName := normalizeAlertField(payload.TokenName)
	channelName := normalizeAlertField(payload.ChannelName)
	requestPath := normalizeAlertField(payload.RequestPath)
	errorCode := normalizeAlertField(payload.ErrorCode)
	errorType := normalizeAlertField(payload.ErrorType)
	errorMessage := normalizeAlertField(payload.ErrorMessage)
	requestID := normalizeAlertField(payload.RequestID)
	upstreamRequestID := normalizeAlertField(payload.UpstreamRequestID)

	return fmt.Sprintf(
		"错误级别：严重\n"+
			"告警类型：渠道调用错误\n"+
			"用户信息：userId=%d, username=%s, group=%s\n"+
			"模型信息：%s\n"+
			"令牌信息：%s\n"+
			"渠道信息：channelId=%d, channelName=%s, channelType=%d\n"+
			"请求信息：path=%s, isStream=%t, useTime=%d秒\n"+
			"错误信息：status=%d, errorType=%s, errorCode=%s\n"+
			"错误详情：%s\n"+
			"requestId：%s\n"+
			"upstreamRequestId：%s\n"+
			"重试链路：%v\n"+
			"告警时间：%s",
		payload.UserID,
		username,
		group,
		modelName,
		tokenName,
		payload.ChannelID,
		channelName,
		payload.ChannelType,
		requestPath,
		payload.IsStream,
		payload.UseTimeSeconds,
		payload.StatusCode,
		errorType,
		errorCode,
		errorMessage,
		requestID,
		upstreamRequestID,
		payload.RetryChain,
		time.Now().Format("2006-01-02 15:04:05"),
	)
}

func normalizeAlertField(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

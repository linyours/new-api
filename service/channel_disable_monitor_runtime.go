package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func HandleChannelAutoDisabled(channelError types.ChannelError, reason string) {
	if !common.IsMasterNode {
		return
	}

	cfg, err := GetChannelDisableMonitorConfig()
	if err != nil || !cfg.Enabled {
		return
	}

	subject := fmt.Sprintf("【自动禁用告警】通道 #%d 已自动禁用", channelError.ChannelId)
	content := buildChannelAutoDisableAlertContent(channelError, reason)

	if cfg.NotifyType == "webhook" {
		notify := dto.NewNotify(
			fmt.Sprintf("%s_%d", dto.NotifyTypeChannelAutoDisableAlert, channelError.ChannelId),
			subject,
			content,
			nil,
		)
		if err = SendWebhookNotify(cfg.WebhookURL, cfg.WebhookSecret, notify); err != nil {
			common.SysLog(fmt.Sprintf("channel disable monitor webhook notify failed: %v", err))
			return
		}
	} else {
		NotifyRootUser(dto.NotifyTypeChannelAutoDisableAlert, subject, content)
	}
}

func buildChannelAutoDisableAlertContent(channelError types.ChannelError, reason string) string {
	if reason == "" {
		reason = "-"
	}
	return fmt.Sprintf(
		"错误级别：严重\n"+
			"告警类型：通道自动禁用\n"+
			"渠道信息：channelId=%d, channelName=%s, channelType=%s, autoBan=%t, isMultiKey=%t\n"+
			"渠道状态：%d（自动禁用）\n"+
			"触发原因：%s\n"+
			"告警时间：%s",
		channelError.ChannelId,
		channelError.ChannelName,
		constant.GetChannelTypeName(channelError.ChannelType),
		channelError.AutoBan,
		channelError.IsMultiKey,
		common.ChannelStatusAutoDisabled,
		reason,
		time.Now().Format("2006-01-02 15:04:05"),
	)
}

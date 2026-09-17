package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookPlatform(t *testing.T) {
	assert.Equal(t, "feishu", webhookPlatform("https://open.feishu.cn/open-apis/bot/v2/hook/abc"))
	assert.Equal(t, "feishu", webhookPlatform("https://open.larksuite.com/open-apis/bot/v2/hook/abc"))
	assert.Equal(t, "feishu", webhookPlatform("https://open.larkoffice.com/open-apis/bot/v2/hook/abc"))
	assert.Equal(t, "dingtalk", webhookPlatform("https://oapi.dingtalk.com/robot/send?access_token=abc"))
	assert.Equal(t, "wecom", webhookPlatform("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc"))
	assert.Equal(t, "discord", webhookPlatform("https://discord.com/api/webhooks/1/abc"))
	assert.Equal(t, "slack", webhookPlatform("https://hooks.slack.com/services/T/B/abc"))
	assert.Equal(t, "", webhookPlatform("https://hooks.example.com/channel-health"))
}

func TestMarshalWebhookPayloadForChatPlatforms(t *testing.T) {
	payload := WebhookPayload{
		Type:    "channel_health_24",
		Title:   "渠道「模版-123」（#24）近5分钟成功率过低",
		Content: "**渠道** 模版-123 `#24`\n**近5分钟成功率** 0%（0 / 3）",
		Text:    "plain",
	}

	feishu, err := marshalWebhookPayload("https://open.feishu.cn/open-apis/bot/v2/hook/abc", payload)
	require.NoError(t, err)
	var feishuBody map[string]any
	require.NoError(t, common.Unmarshal(feishu, &feishuBody))
	assert.Equal(t, "interactive", feishuBody["msg_type"])
	card, ok := feishuBody["card"].(map[string]any)
	require.True(t, ok)
	header, ok := card["header"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "red", header["template"])
	title, ok := header["title"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "渠道成功率过低", title["content"])
	elements, ok := card["elements"].([]any)
	require.True(t, ok)
	require.Len(t, elements, 1)
	element, ok := elements[0].(map[string]any)
	require.True(t, ok)
	text, ok := element["text"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, payload.Content, text["content"])

	ding, err := marshalWebhookPayload("https://oapi.dingtalk.com/robot/send?access_token=abc", payload)
	require.NoError(t, err)
	var dingBody map[string]any
	require.NoError(t, common.Unmarshal(ding, &dingBody))
	assert.Equal(t, "markdown", dingBody["msgtype"])
	markdown, ok := dingBody["markdown"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, payload.Title, markdown["title"])
	assert.Contains(t, markdown["text"], payload.Content)

	generic, err := marshalWebhookPayload("https://hooks.example.com/channel-health", payload)
	require.NoError(t, err)
	var genericBody WebhookPayload
	require.NoError(t, common.Unmarshal(generic, &genericBody))
	assert.Equal(t, payload.Title, genericBody.Title)
	assert.Equal(t, payload.Content, genericBody.Content)
}

package service

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// WebhookPayload webhook 通知的负载数据
type WebhookPayload struct {
	Type      string        `json:"type"`
	Title     string        `json:"title"`
	Content   string        `json:"content"`
	Text      string        `json:"text,omitempty"`
	Values    []interface{} `json:"values,omitempty"`
	Timestamp int64         `json:"timestamp"`
}

// generateSignature 生成 webhook 签名
func generateSignature(secret string, payload []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// SendWebhookNotify 发送 webhook 通知
func SendWebhookNotify(webhookURL string, secret string, data dto.Notify) error {
	// 处理占位符
	content := data.Content
	for _, value := range data.Values {
		content = fmt.Sprintf(content, value)
	}

	// 构建 webhook 负载
	payload := WebhookPayload{
		Type:      data.Type,
		Title:     data.Title,
		Content:   content,
		Text:      strings.TrimSpace(data.Title + "\n" + content),
		Values:    data.Values,
		Timestamp: time.Now().Unix(),
	}

	payloadBytes, err := marshalWebhookPayload(webhookURL, payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	// 创建 HTTP 请求
	var req *http.Request
	var resp *http.Response

	if system_setting.EnableWorker() {
		// 构建worker请求数据
		workerReq := &WorkerRequest{
			URL:    webhookURL,
			Key:    system_setting.WorkerValidKey,
			Method: http.MethodPost,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: payloadBytes,
		}

		// 如果有secret，添加签名到headers
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			workerReq.Headers["X-Webhook-Signature"] = signature
			workerReq.Headers["Authorization"] = "Bearer " + secret
		}

		resp, err = DoWorkerRequest(workerReq)
		if err != nil {
			return fmt.Errorf("failed to send webhook request through worker: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		}
	} else {
		// SSRF防护：验证Webhook URL（非Worker模式）
		if err := ValidateSSRFProtectedFetchURL(webhookURL); err != nil {
			return fmt.Errorf("request reject: %v", err)
		}

		req, err = http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return fmt.Errorf("failed to create webhook request: %v", err)
		}

		// 设置请求头
		req.Header.Set("Content-Type", "application/json")

		// 如果有 secret，生成签名
		if secret != "" {
			signature := generateSignature(secret, payloadBytes)
			req.Header.Set("X-Webhook-Signature", signature)
		}

		// 发送请求
		client := GetSSRFProtectedHTTPClient()
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to send webhook request: %v", err)
		}
		defer resp.Body.Close()

		// 检查响应状态
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook request failed with status code: %d", resp.StatusCode)
		}
	}

	return nil
}

func marshalWebhookPayload(webhookURL string, payload WebhookPayload) ([]byte, error) {
	text := payload.Text
	if text == "" {
		text = strings.TrimSpace(payload.Title + "\n" + payload.Content)
	}
	switch webhookPlatform(webhookURL) {
	case "feishu":
		return marshalFeishuCard(payload)
	case "dingtalk":
		return common.Marshal(map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"title": payload.Title,
				"text":  strings.TrimSpace("### " + payload.Title + "\n\n" + payload.Content),
			},
		})
	case "wecom":
		return common.Marshal(map[string]any{
			"msgtype": "markdown",
			"markdown": map[string]string{
				"content": strings.TrimSpace("**" + payload.Title + "**\n" + payload.Content),
			},
		})
	case "discord", "slack":
		return common.Marshal(map[string]any{
			"content": text,
			"text":    text,
		})
	default:
		return common.Marshal(payload)
	}
}

func marshalFeishuCard(payload WebhookPayload) ([]byte, error) {
	headerTitle := payload.Title
	template := "blue"
	if strings.HasPrefix(payload.Type, dto.NotifyTypeChannelHealth) {
		headerTitle = "渠道成功率过低"
		template = "red"
	}
	md := strings.TrimSpace(payload.Content)
	if md == "" {
		md = payload.Text
	}
	return common.Marshal(map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"template": template,
				"title": map[string]any{
					"tag":     "plain_text",
					"content": headerTitle,
				},
			},
			"elements": []any{
				map[string]any{
					"tag": "div",
					"text": map[string]any{
						"tag":     "lark_md",
						"content": md,
					},
				},
			},
		},
	})
}

func webhookPlatform(raw string) string {
	u := strings.ToLower(raw)
	switch {
	case strings.Contains(u, "feishu") || strings.Contains(u, "larksuite") || strings.Contains(u, "larkoffice") || strings.Contains(u, ".lark.cn"):
		return "feishu"
	case strings.Contains(u, "dingtalk"):
		return "dingtalk"
	case strings.Contains(u, "qyapi.weixin.qq.com"):
		return "wecom"
	case strings.Contains(u, "discord.com/api/webhooks"):
		return "discord"
	case strings.Contains(u, "hooks.slack.com"):
		return "slack"
	default:
		return ""
	}
}

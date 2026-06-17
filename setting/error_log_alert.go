package setting

// ErrorWebhookAlertEnabled:
// 错误飞书告警总开关。该开关与错误日志写库开关（ERROR_LOG_ENABLED）完全解耦：
// - 关闭错误日志写库，不影响告警发送；
// - 关闭该开关时，即使有渠道错误也不会发 webhook 告警。
var ErrorWebhookAlertEnabled = false

// ErrorWebhookAlertURL:
// 错误告警 webhook 地址（通常为飞书机器人地址）。
// 为空时表示不发送 webhook 告警。
var ErrorWebhookAlertURL = ""

// ErrorWebhookAlertSecret:
// webhook 签名密钥（可选）。如果 webhook 接收方需要签名校验，可配置此值。
var ErrorWebhookAlertSecret = ""

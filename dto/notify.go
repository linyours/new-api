package dto

type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed             = "quota_exceed"
	NotifyTypeChannelUpdate           = "channel_update"
	NotifyTypeChannelTest             = "channel_test"
	NotifyTypeTTFTAlert               = "ttft_alert"
	NotifyTypeChannelAutoDisableAlert = "channel_auto_disable_alert"
	// NotifyTypeChannelErrorAlert:
	// 专门用于“渠道调用报错”的实时告警类型，和自动禁用告警分开，便于后续统计与扩展。
	NotifyTypeChannelErrorAlert = "channel_error_alert"
)

func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}

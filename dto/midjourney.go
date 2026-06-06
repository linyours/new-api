package dto

import (
	"bytes"
	"encoding/json"
)

//type SimpleMjRequest struct {
//	Prompt   string `json:"prompt"`
//	CustomId string `json:"customId"`
//	Action   string `json:"action"`
//	Content  string `json:"content"`
//}

type SwapFaceRequest struct {
	SourceBase64 string `json:"sourceBase64"`
	TargetBase64 string `json:"targetBase64"`
}

type MidjourneyRequest struct {
	Prompt      string   `json:"prompt"`
	CustomId    string   `json:"customId"`
	BotType     string   `json:"botType"`
	NotifyHook  string   `json:"notifyHook"`
	Action      string   `json:"action"`
	Index       int      `json:"index"`
	State       string   `json:"state"`
	TaskId      string   `json:"taskId"`
	Base64Array []string `json:"base64Array"`
	Content     string   `json:"content"`
	MaskBase64  string   `json:"maskBase64"`
}

type MidjourneyResponse struct {
	Code        int         `json:"code"`
	Description string      `json:"description"`
	Properties  interface{} `json:"properties"`
	Result      string      `json:"result"`
}

type MidjourneyUploadResponse struct {
	Code        int      `json:"code"`
	Description string   `json:"description"`
	Result      []string `json:"result"`
}

type MidjourneyResponseWithStatusCode struct {
	StatusCode int `json:"statusCode"`
	Response   MidjourneyResponse
}

type MidjourneyDto struct {
	MjId        string      `json:"id"`
	Action      string      `json:"action"`
	CustomId    string      `json:"customId"`
	BotType     string      `json:"botType"`
	Prompt      string      `json:"prompt"`
	PromptEn    string      `json:"promptEn"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	SubmitTime  int64       `json:"submitTime"`
	StartTime   int64       `json:"startTime"`
	FinishTime  int64       `json:"finishTime"`
	ImageUrl    string      `json:"imageUrl"`
	ImageUrls   []ImgUrls   `json:"imageUrls"`
	VideoUrl    string      `json:"videoUrl"`
	VideoUrls   []ImgUrls   `json:"videoUrls"`
	Status      string      `json:"status"`
	Progress    string      `json:"progress"`
	FailReason  string      `json:"failReason"`
	Buttons     any         `json:"buttons"`
	MaskBase64  string      `json:"maskBase64"`
	Properties  *Properties `json:"properties"`
}

// UnmarshalJSON accepts imageUrls as objects, image_urls (snake_case), or a JSON array of URL strings.
func (m *MidjourneyDto) UnmarshalJSON(data []byte) error {
	type MidjourneyDtoJSON MidjourneyDto
	var aux struct {
		MidjourneyDtoJSON
		ImageURLs []ImgUrls `json:"image_urls"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*m = MidjourneyDto(aux.MidjourneyDtoJSON)
	if len(m.ImageUrls) == 0 && len(aux.ImageURLs) > 0 {
		m.ImageUrls = aux.ImageURLs
	}
	flexMidjourneyImageUrls(data, m)
	return nil
}

func flexMidjourneyImageUrls(data []byte, m *MidjourneyDto) {
	if len(m.ImageUrls) > 0 {
		return
	}
	var probe struct {
		Camel json.RawMessage `json:"imageUrls"`
		Snake json.RawMessage `json:"image_urls"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return
	}
	raw := probe.Camel
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		raw = probe.Snake
	}
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		return
	}
	var objs []ImgUrls
	if err := json.Unmarshal(raw, &objs); err == nil && len(objs) > 0 {
		m.ImageUrls = objs
		return
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err != nil {
		return
	}
	for _, s := range strs {
		if s == "" {
			continue
		}
		m.ImageUrls = append(m.ImageUrls, ImgUrls{Url: s})
	}
}

type ImgUrls struct {
	Url string `json:"url"`
}

type MidjourneyStatus struct {
	Status int `json:"status"`
}
type MidjourneyWithoutStatus struct {
	Id          int    `json:"id"`
	Code        int    `json:"code"`
	UserId      int    `json:"user_id" gorm:"index"`
	Action      string `json:"action"`
	MjId        string `json:"mj_id" gorm:"index"`
	Prompt      string `json:"prompt"`
	PromptEn    string `json:"prompt_en"`
	Description string `json:"description"`
	State       string `json:"state"`
	SubmitTime  int64  `json:"submit_time"`
	StartTime   int64  `json:"start_time"`
	FinishTime  int64  `json:"finish_time"`
	ImageUrl    string    `json:"image_url"`
	ImageUrls   []ImgUrls `json:"image_urls,omitempty"`
	Progress    string    `json:"progress"`
	FailReason  string `json:"fail_reason"`
	ChannelId   int    `json:"channel_id"`
}

type ActionButton struct {
	CustomId any `json:"customId"`
	Emoji    any `json:"emoji"`
	Label    any `json:"label"`
	Type     any `json:"type"`
	Style    any `json:"style"`
}

type Properties struct {
	FinalPrompt   string `json:"finalPrompt"`
	FinalZhPrompt string `json:"finalZhPrompt"`
}

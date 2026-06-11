package service

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const ttftMonitorConfigRedisKey = "monitor:ttft:config"

type TTFTMonitorConfig struct {
	Enabled          bool  `json:"enabled"`
	ChannelIDs       []int `json:"channel_ids"`
	ThresholdSeconds int   `json:"threshold_seconds"`
	WindowSeconds    int   `json:"window_seconds"`
	CountThreshold   int   `json:"count_threshold"`
	CooldownSeconds  int   `json:"cooldown_seconds"`
	NotifyType       string `json:"notify_type"`
	WebhookURL       string `json:"webhook_url,omitempty"`
	WebhookSecret    string `json:"webhook_secret,omitempty"`
}

func DefaultTTFTMonitorConfig() TTFTMonitorConfig {
	return TTFTMonitorConfig{
		Enabled:          false,
		ChannelIDs:       []int{},
		ThresholdSeconds: 60,
		WindowSeconds:    60,
		CountThreshold:   10,
		CooldownSeconds:  300,
		NotifyType:       "root_notify",
		WebhookURL:       "",
		WebhookSecret:    "",
	}
}

func normalizeTTFTMonitorConfig(cfg *TTFTMonitorConfig) {
	if cfg.ChannelIDs == nil {
		cfg.ChannelIDs = []int{}
	}
	cfg.ChannelIDs = normalizeChannelIDs(cfg.ChannelIDs)
	if cfg.ThresholdSeconds <= 0 {
		cfg.ThresholdSeconds = 60
	}
	if cfg.WindowSeconds <= 0 {
		cfg.WindowSeconds = 60
	}
	if cfg.CountThreshold <= 0 {
		cfg.CountThreshold = 10
	}
	if cfg.CooldownSeconds <= 0 {
		cfg.CooldownSeconds = 300
	}
	if cfg.NotifyType == "" {
		cfg.NotifyType = "root_notify"
	}
	cfg.NotifyType = strings.TrimSpace(cfg.NotifyType)
	cfg.WebhookURL = strings.TrimSpace(cfg.WebhookURL)
	cfg.WebhookSecret = strings.TrimSpace(cfg.WebhookSecret)
}

func normalizeChannelIDs(channelIDs []int) []int {
	if len(channelIDs) == 0 {
		return []int{}
	}
	seen := make(map[int]struct{}, len(channelIDs))
	result := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if channelID <= 0 {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		seen[channelID] = struct{}{}
		result = append(result, channelID)
	}
	return result
}

func ValidateTTFTMonitorConfig(cfg *TTFTMonitorConfig) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	normalizeTTFTMonitorConfig(cfg)
	if cfg.NotifyType != "root_notify" && cfg.NotifyType != "webhook" {
		return errors.New("notify_type must be root_notify or webhook")
	}
	if cfg.NotifyType == "webhook" {
		if cfg.WebhookURL == "" {
			return errors.New("webhook_url is required when notify_type is webhook")
		}
		parsed, err := url.ParseRequestURI(cfg.WebhookURL)
		if err != nil {
			return errors.New("webhook_url is invalid")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("webhook_url must start with http:// or https://")
		}
	}
	return nil
}

func GetTTFTMonitorConfig() (TTFTMonitorConfig, error) {
	defaultConfig := DefaultTTFTMonitorConfig()
	if !common.RedisEnabled || common.RDB == nil {
		return defaultConfig, nil
	}
	raw, err := common.RedisGet(ttftMonitorConfigRedisKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaultConfig, nil
	}
	var cfg TTFTMonitorConfig
	if err = common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return defaultConfig, nil
	}
	normalizeTTFTMonitorConfig(&cfg)
	if err = ValidateTTFTMonitorConfig(&cfg); err != nil {
		return defaultConfig, nil
	}
	return cfg, nil
}

func SaveTTFTMonitorConfig(cfg TTFTMonitorConfig) error {
	if !common.RedisEnabled || common.RDB == nil {
		return errors.New("redis is not enabled")
	}
	if err := ValidateTTFTMonitorConfig(&cfg); err != nil {
		return err
	}
	payload, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	return common.RedisSet(ttftMonitorConfigRedisKey, string(payload), 0*time.Second)
}

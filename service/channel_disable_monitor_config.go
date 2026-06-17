package service

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const channelDisableMonitorConfigRedisKey = "monitor:channel_disable:config"

type ChannelDisableMonitorConfig struct {
	Enabled       bool   `json:"enabled"`
	NotifyType    string `json:"notify_type"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func DefaultChannelDisableMonitorConfig() ChannelDisableMonitorConfig {
	return ChannelDisableMonitorConfig{
		Enabled:       false,
		NotifyType:    "root_notify",
		WebhookURL:    "",
		WebhookSecret: "",
	}
}

func normalizeChannelDisableMonitorConfig(cfg *ChannelDisableMonitorConfig) {
	if cfg.NotifyType == "" {
		cfg.NotifyType = "root_notify"
	}
	cfg.NotifyType = strings.TrimSpace(cfg.NotifyType)
	cfg.WebhookURL = strings.TrimSpace(cfg.WebhookURL)
	cfg.WebhookSecret = strings.TrimSpace(cfg.WebhookSecret)
}

func ValidateChannelDisableMonitorConfig(cfg *ChannelDisableMonitorConfig) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	normalizeChannelDisableMonitorConfig(cfg)
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

func GetChannelDisableMonitorConfig() (ChannelDisableMonitorConfig, error) {
	defaultConfig := DefaultChannelDisableMonitorConfig()
	if !common.RedisEnabled || common.RDB == nil {
		return defaultConfig, nil
	}
	raw, err := common.RedisGet(channelDisableMonitorConfigRedisKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return defaultConfig, nil
	}
	var cfg ChannelDisableMonitorConfig
	if err = common.UnmarshalJsonStr(raw, &cfg); err != nil {
		return defaultConfig, nil
	}
	normalizeChannelDisableMonitorConfig(&cfg)
	if err = ValidateChannelDisableMonitorConfig(&cfg); err != nil {
		return defaultConfig, nil
	}
	return cfg, nil
}

func SaveChannelDisableMonitorConfig(cfg ChannelDisableMonitorConfig) error {
	if !common.RedisEnabled || common.RDB == nil {
		return errors.New("redis is not enabled")
	}
	if err := ValidateChannelDisableMonitorConfig(&cfg); err != nil {
		return err
	}
	payload, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	return common.RedisSet(channelDisableMonitorConfigRedisKey, string(payload), 0*time.Second)
}

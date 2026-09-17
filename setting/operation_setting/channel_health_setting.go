package operation_setting

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultChannelHealthMinTotal          = 30
	MaxChannelHealthMinTotal              = 1000
	DefaultChannelHealthAlertBelowPercent = 50
	MaxChannelHealthAlertBelowPercent     = 100
)

var ChannelHealthMinTotal = DefaultChannelHealthMinTotal
var ChannelHealthAlertBelowPercent = DefaultChannelHealthAlertBelowPercent
var ChannelHealthWebhookUrl string
var ChannelHealthWebhookSecret string

func GetChannelHealthMinTotal() int {
	if ChannelHealthMinTotal < 1 {
		return DefaultChannelHealthMinTotal
	}
	if ChannelHealthMinTotal > MaxChannelHealthMinTotal {
		return MaxChannelHealthMinTotal
	}
	return ChannelHealthMinTotal
}

func ValidateChannelHealthMinTotal(raw string) error {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > MaxChannelHealthMinTotal {
		return fmt.Errorf("minimum sample size must be between 1 and %d", MaxChannelHealthMinTotal)
	}
	return nil
}

func GetChannelHealthAlertBelowPercent() int {
	if ChannelHealthAlertBelowPercent < 1 {
		return DefaultChannelHealthAlertBelowPercent
	}
	if ChannelHealthAlertBelowPercent > MaxChannelHealthAlertBelowPercent {
		return MaxChannelHealthAlertBelowPercent
	}
	return ChannelHealthAlertBelowPercent
}

func ValidateChannelHealthAlertBelowPercent(raw string) error {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 || n > MaxChannelHealthAlertBelowPercent {
		return fmt.Errorf("alert below success rate must be between 1 and %d", MaxChannelHealthAlertBelowPercent)
	}
	return nil
}

func ValidateChannelHealthWebhookURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("invalid webhook url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook url must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid webhook url")
	}
	return nil
}

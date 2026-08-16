package channel_selector

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/channel_selector_setting"
)

// Enabled reports whether multi-factor selection is on.
// Backed by channel_selector_setting.enabled (DB/options hot update).
// When false, callers must keep the official priority/weight path.
func Enabled() bool {
	return channel_selector_setting.IsEnabled()
}

// SettleEnabled reports whether channel cost_price multiplies actual settle quota.
// Backed by channel_selector_setting.cost_settle_enabled.
// Pre-consume must never call settle helpers regardless of this flag.
func SettleEnabled() bool {
	return channel_selector_setting.IsCostSettleEnabled()
}

// DebugLoggingEnabled reports whether pick-factor logs should print.
// True when CHANNEL_SELECTOR_DEBUG=true or global DEBUG=true.
func DebugLoggingEnabled() bool {
	return common.DebugEnabled || channel_selector_setting.IsDebugLoggingEnabled()
}

// SetEnabled / SetSettleEnabled are for tests and optional direct toggles.
func SetEnabled(v bool) {
	channel_selector_setting.SetEnabled(v)
}

func SetSettleEnabled(v bool) {
	channel_selector_setting.SetCostSettleEnabled(v)
}

// Config holds scoring weights and cold-start knobs.
type Config struct {
	WPrice          float64
	WSuccess        float64
	WLatency        float64
	ExploreRate     float64
	ExploreRateCold float64 // min eps while any candidate is cold-start
	MinSamples      int64
	PriorN          float64
	PriorRate       float64
	Temperature     float64
	WindowSec       int64
}

// DefaultConfig returns scoring defaults. ExploreRate / ExploreRateCold / MinSamples
// come from channel_selector_setting (hot-updatable via options API; env seeds process defaults).
func DefaultConfig() Config {
	return Config{
		WPrice:          0.25,
		WSuccess:        0.45,
		WLatency:        0.30,
		ExploreRate:     channel_selector_setting.ExploreRate(),
		ExploreRateCold: channel_selector_setting.ExploreRateCold(),
		MinSamples:      channel_selector_setting.MinSamples(),
		PriorN:          20,
		PriorRate:       0.95,
		Temperature:     0.15,
		WindowSec:       900,
	}
}

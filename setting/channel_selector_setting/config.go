package channel_selector_setting

import (
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ChannelSelectorSetting is hot-updatable via options API:
//
//	channel_selector_setting.enabled
//	channel_selector_setting.cost_settle_enabled
//	channel_selector_setting.explore_rate
//	channel_selector_setting.explore_rate_cold
//	channel_selector_setting.min_samples
//
// Env vars seed defaults before DB options load:
//
//	CHANNEL_SELECTOR_ENABLED
//	CHANNEL_COST_SETTLE_ENABLED
//	CHANNEL_SELECTOR_DEBUG — log pick factors at INFO (also follows global DEBUG=true)
//	CHANNEL_SELECTOR_EXPLORE_RATE
//	CHANNEL_SELECTOR_EXPLORE_RATE_COLD
//	CHANNEL_SELECTOR_MIN_SAMPLES
type ChannelSelectorSetting struct {
	Enabled           bool    `json:"enabled"`
	CostSettleEnabled bool    `json:"cost_settle_enabled"`
	ExploreRate       float64 `json:"explore_rate"`
	ExploreRateCold   float64 `json:"explore_rate_cold"`
	MinSamples        int64   `json:"min_samples"`
}

var channelSelectorSetting = ChannelSelectorSetting{
	Enabled:           envBool("CHANNEL_SELECTOR_ENABLED", false),
	CostSettleEnabled: envBool("CHANNEL_COST_SETTLE_ENABLED", false),
	ExploreRate:       envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE", 0.08),
	ExploreRateCold:   envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_COLD", 0.15),
	MinSamples:        envInt64Min("CHANNEL_SELECTOR_MIN_SAMPLES", 30, 0),
}

// debugLogging is process-level (env only); not persisted via options API.
var debugLogging = envBool("CHANNEL_SELECTOR_DEBUG", false)

func init() {
	config.GlobalConfig.Register("channel_selector_setting", &channelSelectorSetting)
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envFloat01(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return def
	}
	return clamp01(f)
}

func envInt64Min(key string, def, min int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	return n
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// GetSetting returns a copy of the current setting.
func GetSetting() ChannelSelectorSetting {
	return channelSelectorSetting
}

// IsEnabled reports whether multi-factor channel selection is on.
func IsEnabled() bool {
	return channelSelectorSetting.Enabled
}

// IsCostSettleEnabled reports whether settle uses channel cost_price.
func IsCostSettleEnabled() bool {
	return channelSelectorSetting.CostSettleEnabled
}

// IsDebugLoggingEnabled reports whether pick-factor logs should be emitted.
func IsDebugLoggingEnabled() bool {
	return debugLogging
}

// ExploreRate is the base ε-greedy explore probability (clamped to [0,1]).
func ExploreRate() float64 {
	return clamp01(channelSelectorSetting.ExploreRate)
}

// ExploreRateCold is the minimum explore rate while any candidate is cold-start.
func ExploreRateCold() float64 {
	return clamp01(channelSelectorSetting.ExploreRateCold)
}

// MinSamples is the cold-start sample threshold.
func MinSamples() int64 {
	if channelSelectorSetting.MinSamples < 0 {
		return 0
	}
	return channelSelectorSetting.MinSamples
}

// SetEnabled updates the in-memory flag (tests / direct toggles).
func SetEnabled(v bool) {
	channelSelectorSetting.Enabled = v
}

// SetCostSettleEnabled updates the in-memory settle flag.
func SetCostSettleEnabled(v bool) {
	channelSelectorSetting.CostSettleEnabled = v
}

// SetDebugLoggingEnabled updates the in-memory debug flag (tests / direct toggles).
func SetDebugLoggingEnabled(v bool) {
	debugLogging = v
}

// SetExploreRate updates the in-memory explore rate (tests / direct toggles).
func SetExploreRate(v float64) {
	channelSelectorSetting.ExploreRate = clamp01(v)
}

// SetExploreRateCold updates the cold-start explore floor (tests / direct toggles).
func SetExploreRateCold(v float64) {
	channelSelectorSetting.ExploreRateCold = clamp01(v)
}

// SetMinSamples updates the cold-start sample threshold (tests / direct toggles).
func SetMinSamples(v int64) {
	if v < 0 {
		channelSelectorSetting.MinSamples = 0
		return
	}
	channelSelectorSetting.MinSamples = v
}

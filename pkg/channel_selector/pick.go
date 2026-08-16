package channel_selector

import (
	"fmt"

	"github.com/QuantumNous/new-api/logger"
)

// ChannelView is the minimal channel snapshot needed for scoring / hard filters.
// Assembled at thin hooks so this package does not depend on selection internals.
type ChannelView struct {
	ID           int
	Type         int // ChannelType; used by per-type max cost caps
	CostPrice    float64
	ResponseTime int
}

func logPickInfo(format string, args ...any) {
	if !DebugLoggingEnabled() {
		return
	}
	logger.LogInfo(nil, fmt.Sprintf(format, args...))
}

// Pick selects a channel ID from hard-filtered candidates.
// exclude skips channels that already failed in this request.
// maxCostByType optionally caps cost_price by ChannelType (see ExceedsMaxCostPrice).
// Returns 0 when no candidate remains.
func Pick(channels []ChannelView, model string, exclude map[int]struct{}, cfg Config, maxCostByType map[string]float64) int {
	if !Enabled() {
		return 0
	}
	cands := make([]Candidate, 0, len(channels))
	skippedExclude := 0
	skippedMaxCost := 0
	for _, ch := range channels {
		if exclude != nil {
			if _, skip := exclude[ch.ID]; skip {
				skippedExclude++
				logPickInfo("[channel_selector] model=%s skip channel=%d reason=exclude", model, ch.ID)
				continue
			}
		}
		if ExceedsMaxCostPrice(maxCostByType, ch.Type, ch.CostPrice) {
			skippedMaxCost++
			max, _ := ResolveMaxCostPrice(maxCostByType, ch.Type)
			eff := EffectiveCostPrice(ch.CostPrice)
			cost := fmt.Sprintf("%.4f", eff)
			if ch.CostPrice < 0 {
				cost = fmt.Sprintf("%.4f(unset)", eff)
			}
			logPickInfo(
				"[channel_selector] model=%s skip channel=%d type=%d cost=%s max=%.4f reason=max_cost",
				model, ch.ID, ch.Type, cost, max,
			)
			continue
		}
		st := QueryStats(ch.ID, model, cfg.WindowSec)
		cands = append(cands, Candidate{
			ID:           ch.ID,
			Type:         ch.Type,
			CostPrice:    ch.CostPrice,
			Stats:        st,
			ResponseTime: ch.ResponseTime,
		})
	}
	logPickInfo(
		"[channel_selector] model=%s input=%d kept=%d skipped_exclude=%d skipped_max_cost=%d max_cost_rules=%d",
		model, len(channels), len(cands), skippedExclude, skippedMaxCost, len(maxCostByType),
	)
	if len(cands) == 0 {
		logPickInfo("[channel_selector] model=%s picked=0 reason=no_candidates", model)
		return 0
	}
	return selectChannel(cands, cfg, model)
}

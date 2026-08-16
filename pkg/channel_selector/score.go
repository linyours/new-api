package channel_selector

import (
	"fmt"
	"math"
	"math/rand"
	"strings"

	"github.com/QuantumNous/new-api/logger"
)

// Candidate is a scoring snapshot assembled by the pick layer.
type Candidate struct {
	ID           int
	Type         int     // ChannelType; debug / filter context only
	CostPrice    float64 // <0 means unset → EffectiveCostPrice treats as 1 for scoring
	Stats        Stats
	ResponseTime int // ms; weak latency prior when Stats has no latency
}

// Select chooses a channel ID via ε-greedy exploration + three-factor softmax.
func Select(cands []Candidate, cfg Config) int {
	return selectChannel(cands, cfg, "")
}

func selectChannel(cands []Candidate, cfg Config, model string) int {
	if len(cands) == 0 {
		return 0
	}
	if len(cands) == 1 {
		logSelectorDecision(model, "single", cfg, 0, cands, nil, nil, nil, nil, cands[0].ID)
		return cands[0].ID
	}

	for i := range cands {
		if cands[i].Stats.AvgLatMs <= 0 && cands[i].ResponseTime > 0 {
			cands[i].Stats.AvgLatMs = float64(cands[i].ResponseTime)
		}
	}

	eps := cfg.ExploreRate
	coldFloor := cfg.ExploreRateCold
	if coldFloor <= 0 {
		coldFloor = 0.15
	}
	for _, c := range cands {
		if c.Stats.Samples < cfg.MinSamples {
			if eps < coldFloor {
				eps = coldFloor
			}
			break
		}
	}

	// Exploration: prefer cold-start channels so new model/channel bindings get traffic.
	if rand.Float64() < eps {
		var cold []Candidate
		for _, c := range cands {
			if c.Stats.Samples < cfg.MinSamples {
				cold = append(cold, c)
			}
		}
		pool := cands
		if len(cold) > 0 {
			pool = cold
		}
		picked := pool[rand.Intn(len(pool))].ID
		logSelectorDecision(model, fmt.Sprintf("explore(eps=%.3f,cold=%d,pool=%d)", eps, len(cold), len(pool)), cfg, eps, cands, nil, nil, nil, nil, picked)
		return picked
	}

	priceScores := normalizePrice(cands)
	latScores := normalizeLatency(cands)
	succScores := make([]float64, len(cands))
	scores := make([]float64, len(cands))
	for i, c := range cands {
		succ := (float64(c.Stats.OK) + cfg.PriorN*cfg.PriorRate) /
			(float64(c.Stats.Eligible) + cfg.PriorN)
		succScores[i] = succ
		scores[i] = cfg.WPrice*priceScores[i] +
			cfg.WSuccess*succ +
			cfg.WLatency*latScores[i]
	}
	idx := softmaxPick(scores, cfg.Temperature)
	picked := cands[idx].ID
	logSelectorDecision(model, "exploit", cfg, eps, cands, priceScores, succScores, latScores, scores, picked)
	return picked
}

func logSelectorDecision(
	model string,
	mode string,
	cfg Config,
	eps float64,
	cands []Candidate,
	priceScores []float64,
	succScores []float64,
	latScores []float64,
	totalScores []float64,
	picked int,
) {
	if !DebugLoggingEnabled() {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"[channel_selector] model=%s mode=%s picked=%d candidates=%d weights(price=%.2f,success=%.2f,latency=%.2f) eps=%.3f window=%ds",
		model, mode, picked, len(cands), cfg.WPrice, cfg.WSuccess, cfg.WLatency, eps, cfg.WindowSec,
	))
	limit := len(cands)
	if limit > 32 {
		limit = 32
	}
	for i := 0; i < limit; i++ {
		c := cands[i]
		eff := EffectiveCostPrice(c.CostPrice)
		cost := fmt.Sprintf("%.4f", eff)
		if c.CostPrice < 0 {
			cost = fmt.Sprintf("%.4f(unset)", eff)
		}
		price := "-"
		succ := "-"
		lat := "-"
		total := "-"
		if priceScores != nil {
			price = fmt.Sprintf("%.3f", priceScores[i])
		}
		if succScores != nil {
			succ = fmt.Sprintf("%.3f", succScores[i])
		}
		if latScores != nil {
			lat = fmt.Sprintf("%.3f", latScores[i])
		}
		if totalScores != nil {
			total = fmt.Sprintf("%.3f", totalScores[i])
		}
		mark := ""
		if c.ID == picked {
			mark = " *"
		}
		b.WriteString(fmt.Sprintf(
			" | ch=%d type=%d cost=%s samples=%d ok=%d/%d avgLat=%.0fms price=%s succ=%s lat=%s score=%s%s",
			c.ID, c.Type, cost, c.Stats.Samples, c.Stats.OK, c.Stats.Eligible, c.Stats.AvgLatMs,
			price, succ, lat, total, mark,
		))
	}
	if len(cands) > limit {
		b.WriteString(fmt.Sprintf(" | ...(+%d more)", len(cands)-limit))
	}
	logger.LogInfo(nil, b.String())
}

func normalizePrice(cands []Candidate) []float64 {
	out := make([]float64, len(cands))
	if len(cands) == 0 {
		return out
	}
	minV, maxV := math.Inf(1), math.Inf(-1)
	for _, c := range cands {
		cost := EffectiveCostPrice(c.CostPrice)
		if cost < minV {
			minV = cost
		}
		if cost > maxV {
			maxV = cost
		}
	}
	for i, c := range cands {
		cost := EffectiveCostPrice(c.CostPrice)
		if maxV-minV < 1e-9 {
			out[i] = 1
		} else {
			out[i] = 1 - (cost-minV)/(maxV-minV)
		}
	}
	return out
}

func normalizeLatency(cands []Candidate) []float64 {
	out := make([]float64, len(cands))
	minV, maxV := math.Inf(1), math.Inf(-1)
	has := false
	for _, c := range cands {
		if c.Stats.AvgLatMs <= 0 {
			continue
		}
		has = true
		if c.Stats.AvgLatMs < minV {
			minV = c.Stats.AvgLatMs
		}
		if c.Stats.AvgLatMs > maxV {
			maxV = c.Stats.AvgLatMs
		}
	}
	for i, c := range cands {
		if !has || c.Stats.AvgLatMs <= 0 {
			out[i] = 0.5
			continue
		}
		if maxV-minV < 1e-9 {
			out[i] = 1
		} else {
			out[i] = 1 - (c.Stats.AvgLatMs-minV)/(maxV-minV)
		}
	}
	return out
}

func softmaxPick(scores []float64, temp float64) int {
	if temp <= 0 {
		temp = 0.15
	}
	maxS := scores[0]
	for _, s := range scores[1:] {
		if s > maxS {
			maxS = s
		}
	}
	weights := make([]float64, len(scores))
	sum := 0.0
	for i, s := range scores {
		w := math.Exp((s - maxS) / temp)
		weights[i] = w
		sum += w
	}
	r := rand.Float64() * sum
	for i, w := range weights {
		r -= w
		if r <= 0 {
			return i
		}
	}
	return len(scores) - 1
}

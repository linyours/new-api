package channel_selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectSingleCandidate(t *testing.T) {
	id := Select([]Candidate{{ID: 7, CostPrice: -1}}, DefaultConfig())
	assert.Equal(t, 7, id)
}

func TestSelectEmpty(t *testing.T) {
	assert.Equal(t, 0, Select(nil, DefaultConfig()))
}

func TestNormalizePriceCheaperWins(t *testing.T) {
	scores := normalizePrice([]Candidate{
		{ID: 1, CostPrice: 1},
		{ID: 2, CostPrice: 3},
		{ID: 3, CostPrice: -1}, // unset → effective 1, same as cheapest
	})
	require.Len(t, scores, 3)
	assert.InDelta(t, 1.0, scores[0], 1e-9)
	assert.InDelta(t, 0.0, scores[1], 1e-9)
	assert.InDelta(t, 1.0, scores[2], 1e-9)
}

func TestBayesSuccessPriorWhenEmpty(t *testing.T) {
	cfg := DefaultConfig()
	st := Stats{}
	succ := (float64(st.OK) + cfg.PriorN*cfg.PriorRate) / (float64(st.Eligible) + cfg.PriorN)
	assert.InDelta(t, cfg.PriorRate, succ, 1e-9)
}

func TestPickRespectsExclude(t *testing.T) {
	SetEnabled(true)
	defer SetEnabled(false)
	resetAllStatsForTest()

	cfg := DefaultConfig()
	cfg.ExploreRate = 1 // force exploration among remaining
	picked := Pick(
		[]ChannelView{{ID: 1, Type: 1, CostPrice: 1}, {ID: 2, Type: 1, CostPrice: 1}},
		"gpt-test",
		map[int]struct{}{1: {}},
		cfg,
		nil,
	)
	assert.Equal(t, 2, picked)
}

func TestPickDisabledReturnsZero(t *testing.T) {
	SetEnabled(false)
	picked := Pick([]ChannelView{{ID: 1}}, "m", nil, DefaultConfig(), nil)
	assert.Equal(t, 0, picked)
}

func TestPickRespectsMaxCostByType(t *testing.T) {
	SetEnabled(true)
	defer SetEnabled(false)
	resetAllStatsForTest()

	cfg := DefaultConfig()
	cfg.ExploreRate = 0
	cfg.MinSamples = 0
	cfg.WPrice = 1
	cfg.WSuccess = 0
	cfg.WLatency = 0
	cfg.Temperature = 0.001

	picked := Pick(
		[]ChannelView{
			{ID: 1, Type: 14, CostPrice: 0.85}, // over Anthropic cap
			{ID: 2, Type: 14, CostPrice: 0.7},
			{ID: 3, Type: 1, CostPrice: 1.2}, // OpenAI uncapped here
		},
		"claude-test",
		nil,
		cfg,
		map[string]float64{"14": 0.8},
	)
	assert.Equal(t, 2, picked)

	picked = Pick(
		[]ChannelView{
			{ID: 1, Type: 14, CostPrice: 0.85},
			{ID: 2, Type: 14, CostPrice: -1}, // unset → 1 > 0.8, filtered
		},
		"claude-test",
		nil,
		cfg,
		map[string]float64{"14": 0.8},
	)
	assert.Equal(t, 0, picked)

	picked = Pick(
		[]ChannelView{
			{ID: 1, Type: 14, CostPrice: 0.85},
			{ID: 2, Type: 14, CostPrice: -1}, // unset → 1, within cap 1.0 but pricier than 0.85
		},
		"claude-test",
		nil,
		cfg,
		map[string]float64{"14": 1.0},
	)
	assert.Equal(t, 1, picked)

	picked = Pick(
		[]ChannelView{
			{ID: 2, Type: 14, CostPrice: -1}, // unset → 1 within cap
		},
		"claude-test",
		nil,
		cfg,
		map[string]float64{"14": 1.0},
	)
	assert.Equal(t, 2, picked)

	picked = Pick(
		[]ChannelView{{ID: 1, Type: 14, CostPrice: 0.9}},
		"claude-test",
		nil,
		cfg,
		map[string]float64{"14": 0.8},
	)
	assert.Equal(t, 0, picked)
}

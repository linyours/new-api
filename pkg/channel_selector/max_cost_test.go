package channel_selector

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveMaxCostPrice(t *testing.T) {
	byType := map[string]float64{
		"14":                    0.8,
		"1":                     0.9,
		MaxCostPriceDefaultKey: 1.0,
	}

	max, limited := ResolveMaxCostPrice(byType, 14)
	require.True(t, limited)
	assert.Equal(t, 0.8, max)

	max, limited = ResolveMaxCostPrice(byType, 1)
	require.True(t, limited)
	assert.Equal(t, 0.9, max)

	max, limited = ResolveMaxCostPrice(byType, 24)
	require.True(t, limited)
	assert.Equal(t, 1.0, max)

	_, limited = ResolveMaxCostPrice(nil, 14)
	assert.False(t, limited)

	_, limited = ResolveMaxCostPrice(map[string]float64{"14": 0.8}, 24)
	assert.False(t, limited)
}

func TestExceedsMaxCostPrice(t *testing.T) {
	byType := map[string]float64{"14": 0.8, "1": 0.9}

	assert.False(t, ExceedsMaxCostPrice(byType, 14, 0.8)) // equal kept
	assert.True(t, ExceedsMaxCostPrice(byType, 14, 0.81))
	assert.True(t, ExceedsMaxCostPrice(byType, 14, -1))                          // unset → 1 > 0.8
	assert.False(t, ExceedsMaxCostPrice(map[string]float64{"14": 1.0}, 14, -1)) // unset → 1 == 1 kept
	assert.False(t, ExceedsMaxCostPrice(byType, 24, 9))                          // no type/default
	assert.False(t, ExceedsMaxCostPrice(nil, 14, 9))
}

func TestEffectiveCostPrice(t *testing.T) {
	assert.Equal(t, DefaultUnsetCostPrice, EffectiveCostPrice(-1))
	assert.Equal(t, 0.0, EffectiveCostPrice(0))
	assert.Equal(t, 0.6, EffectiveCostPrice(0.6))
}

func TestNormalizeRoutingMaxCostPriceByType(t *testing.T) {
	out, err := NormalizeRoutingMaxCostPriceByType(map[string]float64{
		" 14 ":  0.8,
		"1":     0.9,
		"default": 1,
	})
	require.NoError(t, err)
	require.Equal(t, map[string]float64{"14": 0.8, "1": 0.9, "default": 1}, out)

	_, err = NormalizeRoutingMaxCostPriceByType(map[string]float64{"openai": 1})
	require.Error(t, err)

	_, err = NormalizeRoutingMaxCostPriceByType(map[string]float64{"1": -0.1})
	require.Error(t, err)

	_, err = NormalizeRoutingMaxCostPriceByType(map[string]float64{"1": math.NaN()})
	require.Error(t, err)

	out, err = NormalizeRoutingMaxCostPriceByType(map[string]float64{})
	require.NoError(t, err)
	assert.Nil(t, out)
}

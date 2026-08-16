package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRoutingMaxCostPriceByTypeRoundTrip(t *testing.T) {
	token := &Token{}
	require.NoError(t, token.SetRoutingMaxCostPriceByType(map[string]float64{
		"14":      0.8,
		"default": 1.0,
	}))
	got := token.GetRoutingMaxCostPriceByType()
	require.NotNil(t, got)
	assert.Equal(t, 0.8, got["14"])
	assert.Equal(t, 1.0, got["default"])

	require.NoError(t, token.SetRoutingMaxCostPriceByType(nil))
	assert.Nil(t, token.GetRoutingMaxCostPriceByType())
	assert.Equal(t, "", token.RoutingMaxCostPriceByType)
}

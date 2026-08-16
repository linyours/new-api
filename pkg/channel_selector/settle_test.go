package channel_selector

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestApplyChannelCostPriceToSettlement(t *testing.T) {
	SetSettleEnabled(true)
	defer SetSettleEnabled(false)

	fallback := 1000
	assert.Equal(t, common.QuotaFromFloat(500), ApplyChannelCostPriceToSettlement(0.5, fallback))
	assert.Equal(t, common.QuotaFromFloat(1200), ApplyChannelCostPriceToSettlement(1.2, fallback))
	assert.Equal(t, 0, ApplyChannelCostPriceToSettlement(0, fallback))

	// unset cost falls back
	assert.Equal(t, fallback, ApplyChannelCostPriceToSettlement(-1, fallback))

	SetSettleEnabled(false)
	assert.Equal(t, fallback, ApplyChannelCostPriceToSettlement(0.5, fallback))
}

func TestAnnotateSettleOther(t *testing.T) {
	other := map[string]interface{}{}
	AnnotateSettleOther(other, 9, true, 0.01)
	admin := other["admin_info"].(map[string]interface{})
	assert.Equal(t, "channel_cost_price", admin["settle_by"])
	assert.Equal(t, 0.01, admin["channel_cost_price"])
	assert.Equal(t, "model_price", admin["pre_consume_by"])
}

package channel_selector

import (
	"math"

	"github.com/QuantumNous/new-api/common"
)

// ApplyChannelCostPriceToSettlement multiplies the already-computed settle quota
// by channel cost_price. Callers resolve the channel and pass GetCostPrice().
//
// MUST NOT be used for pre-consume. Pre-consume keeps global model pricing.
//
// Formula:
//
//	actual = fallbackQuota * cost_price
//
// costPrice < 0 means unset. When SettleEnabled is false or inputs are invalid,
// fallbackQuota (official model-price result) is returned unchanged.
//
// This package intentionally does not import model/ to avoid import cycles with
// thin hooks in model/channel_cache.go.
func ApplyChannelCostPriceToSettlement(costPrice float64, fallbackQuota int) int {
	if !SettleEnabled() {
		return fallbackQuota
	}
	if costPrice < 0 || math.IsNaN(costPrice) || math.IsInf(costPrice, 0) {
		return fallbackQuota
	}

	return common.QuotaFromFloat(float64(fallbackQuota) * costPrice)
}

// AnnotateSettleOther writes settle-source fields under admin_info when present.
func AnnotateSettleOther(other map[string]interface{}, channelID int, settledByCost bool, costPrice float64) {
	if other == nil {
		return
	}
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	if !ok || adminInfo == nil {
		adminInfo = map[string]interface{}{}
		other["admin_info"] = adminInfo
	}
	adminInfo["pre_consume_by"] = "model_price"
	if settledByCost {
		adminInfo["settle_by"] = "channel_cost_price"
		adminInfo["channel_cost_price"] = costPrice
		adminInfo["channel_id"] = channelID
	} else {
		adminInfo["settle_by"] = "model_price"
	}
}

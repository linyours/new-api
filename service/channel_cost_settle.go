package service

import (
	"github.com/QuantumNous/new-api/model"
	chselector "github.com/QuantumNous/new-api/pkg/channel_selector"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ApplyChannelCostPriceSettle multiplies the already-computed settle quota by
// channel cost_price. Pre-consume must never call this.
//
// Uses relayInfo.ChannelId (final successful channel). When settle flag is off
// or cost_price is unset, returns fallbackQuota unchanged.
func ApplyChannelCostPriceSettle(relayInfo *relaycommon.RelayInfo, fallbackQuota int) int {
	if relayInfo == nil || !chselector.SettleEnabled() {
		return fallbackQuota
	}
	costPrice := -1.0
	if ch, err := model.CacheGetChannel(relayInfo.ChannelId); err == nil && ch != nil {
		costPrice = ch.GetCostPrice()
	}
	return chselector.ApplyChannelCostPriceToSettlement(costPrice, fallbackQuota)
}

// ChannelCostSettleApplied reports whether settle would use cost_price for logging.
func ChannelCostSettleApplied(relayInfo *relaycommon.RelayInfo) (applied bool, costPrice float64) {
	if relayInfo == nil || !chselector.SettleEnabled() {
		return false, -1
	}
	ch, err := model.CacheGetChannel(relayInfo.ChannelId)
	if err != nil || ch == nil || !ch.HasCostPrice() {
		return false, -1
	}
	return true, ch.GetCostPrice()
}

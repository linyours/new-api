package channel_selector

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MaxCostPriceDefaultKey is the fallback key in RoutingMaxCostPriceByType.
const MaxCostPriceDefaultKey = "default"

// DefaultUnsetCostPrice is the effective cost used for routing when a channel
// has no cost_price configured (filter + price scoring).
const DefaultUnsetCostPrice = 1.0

// MaxRoutingMaxCostPriceEntries limits abuse / oversized token settings.
const MaxRoutingMaxCostPriceEntries = 128

// EffectiveCostPrice returns the cost used for routing factors.
// Unset values (costPrice < 0) are treated as DefaultUnsetCostPrice (1).
func EffectiveCostPrice(costPrice float64) float64 {
	if costPrice < 0 {
		return DefaultUnsetCostPrice
	}
	return costPrice
}

// ResolveMaxCostPrice returns the effective cap for a channel type.
// Lookup order: exact type key → "default". limited=false means no cap.
func ResolveMaxCostPrice(byType map[string]float64, channelType int) (max float64, limited bool) {
	if len(byType) == 0 {
		return 0, false
	}
	if channelType > 0 {
		if v, ok := byType[strconv.Itoa(channelType)]; ok {
			return v, true
		}
	}
	if v, ok := byType[MaxCostPriceDefaultKey]; ok {
		return v, true
	}
	return 0, false
}

// ExceedsMaxCostPrice reports whether a channel should be excluded by the token's cap.
// Unset cost_price is treated as DefaultUnsetCostPrice (1).
func ExceedsMaxCostPrice(byType map[string]float64, channelType int, costPrice float64) bool {
	if len(byType) == 0 {
		return false
	}
	max, limited := ResolveMaxCostPrice(byType, channelType)
	if !limited {
		return false
	}
	return EffectiveCostPrice(costPrice) > max
}

// NormalizeRoutingMaxCostPriceByType validates and canonicalizes a token-provided map.
// Empty input returns nil (no caps). Rejects NaN/Inf/negative values and invalid keys.
func NormalizeRoutingMaxCostPriceByType(in map[string]float64) (map[string]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxRoutingMaxCostPriceEntries {
		return nil, fmt.Errorf("too many routing max cost entries (max %d)", MaxRoutingMaxCostPriceEntries)
	}
	out := make(map[string]float64, len(in))
	for rawKey, v := range in {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, fmt.Errorf("invalid max cost for %s", key)
		}
		if key != MaxCostPriceDefaultKey {
			id, err := strconv.Atoi(key)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid channel type key %s", key)
			}
			key = strconv.Itoa(id)
		}
		out[key] = v
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

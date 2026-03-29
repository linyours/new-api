package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// MjPerCallQuotaAfterUserDiscount：折前整数 quota × 用户折扣乘子（AGENT），
// 舍入策略与 chat postConsumeQuota 一致（QuotaAfterUserDiscountMultiplier）。
// 用于 MJ 按次、RelayTask 异步任务（Sora/Veo 等）的实际结算、任务表 quota、失败退费与 consume 日志；
// 预扣仍用折前额度（PreConsumeBilling / 定价 result.Quota）。
func MjPerCallQuotaAfterUserDiscount(username, group, modelName string, preQuota int) (finalQuota int, mult decimal.Decimal) {
	one := decimal.NewFromInt(1)
	if preQuota <= 0 {
		return preQuota, one
	}
	mult = GetUserDiscountMultiplier(username, group, modelName)
	preDec := decimal.NewFromInt(int64(preQuota))
	finalQuota = QuotaAfterUserDiscountMultiplier(preDec, mult)
	return finalQuota, mult
}

// preQuotaFromPostDiscountedQuota：由折后整数 quota 与用户折扣乘子反推折前整数 quota（与 QuotaAfterUserDiscountMultiplier 一致）。
func preQuotaFromPostDiscountedQuota(postQuota int, mult decimal.Decimal) int {
	one := decimal.NewFromInt(1)
	if postQuota <= 0 {
		return postQuota
	}
	if mult.LessThanOrEqual(decimal.Zero) {
		return postQuota
	}
	if mult.Equal(one) {
		return postQuota
	}
	// 上界：折后≤折前，折前一般不超过 post/mult 的量级（mult<1 时折前更大）
	hi := int(decimal.NewFromInt(int64(postQuota)).Div(mult).Ceil().IntPart())
	if hi < postQuota {
		hi = postQuota
	}
	for expand := 0; expand < 64; expand++ {
		if QuotaAfterUserDiscountMultiplier(decimal.NewFromInt(int64(hi)), mult) >= postQuota {
			break
		}
		if hi > postQuota*1000000 {
			break
		}
		hi *= 2
	}
	lo := 1
	best := -1
	for lo <= hi {
		mid := lo + (hi-lo)/2
		v := QuotaAfterUserDiscountMultiplier(decimal.NewFromInt(int64(mid)), mult)
		if v == postQuota {
			best = mid
			hi = mid - 1
		} else if v < postQuota {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best > 0 {
		return best
	}
	return int(decimal.NewFromInt(int64(postQuota)).Div(mult).Round(0).IntPart())
}

func setPlatformDiscountMultiplierOther(username, group, modelName string, other map[string]interface{}) {
	username = strings.TrimSpace(username)
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	if appid, ok := DiscountAppIDFromUsername(username); ok {
		pm := GetPlatformToAgentDiscountMultiplier(appid, group, modelName)
		other["platform_discount_multiplier"] = pm.InexactFloat64()
	} else {
		other["platform_discount_multiplier"] = float64(1)
	}
}

// EnrichOtherWithPreDiscountQuotaUSD：折前整数 quota（与定价/重算一致）→ 折前/折后美金落表；供异步任务 billing log 等无 gin 场景复用。
func EnrichOtherWithPreDiscountQuotaUSD(username, group, modelName string, preQuotaUndiscounted int, mult decimal.Decimal, other map[string]interface{}) {
	dppu := decimal.NewFromFloat(common.QuotaPerUnit)
	preDec := decimal.NewFromInt(int64(preQuotaUndiscounted))
	targetUSD := preDec.Div(dppu).Mul(mult).Round(6)
	other["discount_target_usd"] = targetUSD.StringFixed(6)
	other["pre_user_discount_usd"] = preQuotaUndiscounted
	username = strings.TrimSpace(username)
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	setPlatformDiscountMultiplierOther(username, group, modelName, other)
	if !mult.Equal(decimal.NewFromInt(1)) {
		other["group_ratio"] = mult.InexactFloat64()
		other["discount_int_rounding"] = GetDiscountIntRoundingMode()
	}
}

// EnrichOtherWithPostDiscountQuotaUSD：仅持有折后整数 quota 时（如全额退款）写入与消费侧一致的折扣落表字段（含反推折前 quota）。
func EnrichOtherWithPostDiscountQuotaUSD(username, group, modelName string, postQuotaDiscounted int, mult decimal.Decimal, other map[string]interface{}) {
	one := decimal.NewFromInt(1)
	preUndisc := preQuotaFromPostDiscountedQuota(postQuotaDiscounted, mult)
	dppu := decimal.NewFromFloat(common.QuotaPerUnit)
	preDec := decimal.NewFromInt(int64(preUndisc))
	targetUSD := preDec.Div(dppu).Mul(mult).Round(6)
	other["discount_target_usd"] = targetUSD.StringFixed(6)
	other["pre_user_discount_usd"] = preUndisc
	username = strings.TrimSpace(username)
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	setPlatformDiscountMultiplierOther(username, group, modelName, other)
	if !mult.Equal(one) {
		other["group_ratio"] = mult.InexactFloat64()
		other["discount_int_rounding"] = GetDiscountIntRoundingMode()
	}
}

// EnrichMjConsumeOtherWithDiscount：在 GenerateMjOtherInfo 结果上追加与 compatible_handler 一致的折扣落表字段。
func EnrichMjConsumeOtherWithDiscount(c *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, groupRatio float64, preQuota int, mult decimal.Decimal, other map[string]interface{}) {
	EnrichOtherWithPreDiscountQuotaUSD(c.GetString("username"), relayInfo.UsingGroup, modelName, preQuota, mult, other)
}

package service

import (
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

// EnrichMjConsumeOtherWithDiscount：在 GenerateMjOtherInfo 结果上追加与 compatible_handler 一致的折扣落表字段。
func EnrichMjConsumeOtherWithDiscount(c *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string, groupRatio float64, preQuota int, mult decimal.Decimal, other map[string]interface{}) {
	dppu := decimal.NewFromFloat(common.QuotaPerUnit)
	preDec := decimal.NewFromInt(int64(preQuota))
	other["pre_user_discount_usd"] = preDec.Div(dppu).Round(6).StringFixed(6)

	if appid, ok := DiscountAppIDFromUsername(c.GetString("username")); ok {
		pm := GetPlatformToAgentDiscountMultiplier(appid, relayInfo.UsingGroup, modelName)
		other["platform_discount_multiplier"] = pm.InexactFloat64()
	}
	if !mult.Equal(decimal.NewFromInt(1)) {
		other["group_ratio"] = mult.InexactFloat64()
	}
}

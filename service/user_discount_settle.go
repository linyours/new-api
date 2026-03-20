package service

import (
	"os"
	"strings"

	"github.com/shopspring/decimal"
)

// DiscountIntRounding 控制「折前配额 × 折扣乘子」得到整数 quota 时的舍入方式（仅影响用户折扣结算这一段）。
// 环境变量 DISCOUNT_INT_ROUNDING：
//   - half_down（默认）：5.5 → 5，避免 half away from zero 常变成 6 导致多扣
//   - half_up        ：5.5 → 6（与原 Round(0) 行为接近）
//   - floor / ceil / banker：见 roundDecimalToIntQuota
func discountIntRoundingMode() string {
	m := strings.TrimSpace(strings.ToLower(os.Getenv("DISCOUNT_INT_ROUNDING")))
	if m == "" {
		return "half_down"
	}
	return m
}

// GetDiscountIntRoundingMode 返回当前 DISCOUNT_INT_ROUNDING（未设置时为 half_down），供日志/对账写入 other。
func GetDiscountIntRoundingMode() string {
	return discountIntRoundingMode()
}

// QuotaAfterUserDiscountMultiplier：在折前配额（quota 空间的小数）上乘以折扣乘子，再转为整数 quota。
// 说明：common.QuotaPerUnit=500000 时，最小美金步长为 0.000002；像 0.000011 这类金额对应 5.5 个配额单位，
// 用整数 quota 无法严格同时等于「理论美金」与「整数量」——此处通过 DISCOUNT_INT_ROUNDING 选定取整策略。
func QuotaAfterUserDiscountMultiplier(preQuotaBeforeDiscount, mult decimal.Decimal) int {
	one := decimal.NewFromInt(1)
	if mult.Equal(one) {
		return int(preQuotaBeforeDiscount.Round(0).IntPart())
	}
	raw := preQuotaBeforeDiscount.Mul(mult)
	return roundDecimalToIntQuota(raw, discountIntRoundingMode())
}

func roundDecimalToIntQuota(x decimal.Decimal, mode string) int {
	if x.LessThan(decimal.Zero) {
		return -roundDecimalToIntQuota(x.Neg(), mode)
	}
	switch mode {
	case "floor", "down":
		return int(x.Floor().IntPart())
	case "ceil", "up":
		return int(x.Ceil().IntPart())
	case "half_up":
		return int(x.Round(0).IntPart())
	case "banker", "half_even":
		return int(x.RoundBank(0).IntPart())
	default:
		// half_down（默认）：正数时小数恰好 0.5 向 0 取整（5.5→5），其余按 >=0.5 进位
		whole := x.Truncate(0)
		frac := x.Sub(whole)
		if frac.LessThan(decimal.NewFromFloat(0.5)) {
			return int(whole.IntPart())
		}
		if frac.GreaterThan(decimal.NewFromFloat(0.5)) {
			return int(whole.IntPart()) + 1
		}
		return int(whole.IntPart())
	}
}

package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
)

func init() {
	// 不依赖 main：任何导入 service 的二进制都会在 model 初始化之后注册回调，避免退费日志漏写字段。
	model.FillRefundTaskBillingOtherFunc = FillRefundTaskBillingOtherForModelLog
}

// FillRefundTaskBillingOtherForModelLog 供 model.RecordTaskBillingLog 回调挂载：
// 与 Midjourney 轮询退费、RefundTaskQuota 等路径一致，写入 post-discount 口径的折扣落表字段。
func FillRefundTaskBillingOtherForModelLog(username, group, modelName string, quota int, other map[string]interface{}) {
	if other == nil || quota <= 0 {
		return
	}
	username = strings.TrimSpace(username)
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	mult := GetUserDiscountMultiplier(username, group, modelName)
	EnrichOtherWithPostDiscountQuotaUSD(username, group, modelName, quota, mult, other)
}

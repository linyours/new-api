package service

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// mjPriceDataPerCallStatic 与 relay/helper.ModelPriceHelperPerCall 定价一致，但不依赖 Gin（无 auto_group），供轮询退费等场景重算。
func mjPriceDataPerCallStatic(info *relaycommon.RelayInfo) (types.PriceData, error) {
	if info == nil {
		return types.PriceData{}, fmt.Errorf("relayInfo is nil")
	}
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0,
		GroupSpecialRatio: -1,
	}
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(info.UserGroup, info.UsingGroup)
	if ok {
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(info.UsingGroup)
	}

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if ok {
			modelPrice = defaultPrice
		} else {
			_, ratioSuccess, matchName := ratio_setting.GetModelRatio(info.OriginModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, fmt.Errorf("模型 %s 倍率或价格未配置，请联系管理员设置或开始自用模式；Model %s ratio or price not set, please set or start self-use mode", matchName, matchName)
			}
			modelPrice = float64(common.PreConsumedQuota) / common.QuotaPerUnit
		}
	}
	quota := int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
			quota = 0
			freeModel = true
		}
	}

	return types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}, nil
}

// ApplyMjAgentMirrorAfterUserPostConsume 在用户/令牌额度已通过 PostConsumeQuota 扣减后，
// 按与 SettleBilling（无 BillingSession）相同规则对关联代理同步扣费并更新已用统计。
func ApplyMjAgentMirrorAfterUserPostConsume(ctx context.Context, relayInfo *relaycommon.RelayInfo, username string, preDiscountQuota int, finalUserQuota int) {
	if relayInfo == nil || finalUserQuota <= 0 {
		return
	}
	ResolveAgentActualQuotaByPlatformDiscount(relayInfo, username, decimal.NewFromInt(int64(preDiscountQuota)), finalUserQuota)
	agentCharged := getAgentActualQuotaForSettle(relayInfo, finalUserQuota)
	if agentCharged <= 0 {
		return
	}
	if err := mirrorConsumeQuotaToAgent(ctx, relayInfo.UserId, agentCharged); err != nil {
		logger.LogError(ctx, fmt.Sprintf("MJ 代理同步扣费失败 userId=%d agentQuota=%d: %s", relayInfo.UserId, agentCharged, err.Error()))
		return
	}
	if err := updateAgentUsedQuotaStats(ctx, relayInfo.UserId, agentCharged); err != nil {
		logger.LogError(ctx, fmt.Sprintf("MJ 代理已用额度统计更新失败 userId=%d agentQuota=%d: %s", relayInfo.UserId, agentCharged, err.Error()))
	}
}

// RefundMjAgentMirrorForMidjourneyTask 异步失败时已按 task.Quota 退还主用户后，用与扣费时相同的定价与平台折扣规则重算代理应退额度并返还（不落库 agent 字段）。
func RefundMjAgentMirrorForMidjourneyTask(ctx context.Context, userId int, username, group, modelName string, userChargedQuota int) {
	if userId <= 0 || userChargedQuota <= 0 || modelName == "" {
		return
	}
	ri := &relaycommon.RelayInfo{
		UserId:          userId,
		UserGroup:       group,
		UsingGroup:      group,
		OriginModelName: modelName,
		UserSetting:     dto.UserSetting{},
	}
	pd, err := mjPriceDataPerCallStatic(ri)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("MJ 退费时代理结算定价失败 userId=%d model=%s: %s", userId, modelName, err.Error()))
		return
	}
	ResolveAgentActualQuotaByPlatformDiscount(ri, username, decimal.NewFromInt(int64(pd.Quota)), userChargedQuota)
	agentRefund := getAgentActualQuotaForSettle(ri, userChargedQuota)
	if agentRefund <= 0 {
		return
	}
	if err := mirrorConsumeQuotaToAgent(ctx, userId, -agentRefund); err != nil {
		logger.LogError(ctx, fmt.Sprintf("MJ 代理退费失败 userId=%d agentRefund=%d: %s", userId, agentRefund, err.Error()))
		return
	}
	if err := updateAgentUsedQuotaStatsRefund(ctx, userId, agentRefund); err != nil {
		logger.LogError(ctx, fmt.Sprintf("MJ 代理已用额度统计回退失败 userId=%d agentRefund=%d: %s", userId, agentRefund, err.Error()))
	}
}

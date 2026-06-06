package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/hot"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

const (
	routerAgentUserIDCacheNamespace   = "new-api:router_agent_user_id:v1"
	defaultRouterAgentUserIDTTLSecond = 120
	defaultRouterAgentUserIDNegTTL    = 15
	defaultRouterAgentUserIDCacheCap  = 100000
)

var (
	routerAgentUserIDCacheOnce sync.Once
	routerAgentUserIDCache     *cachex.HybridCache[int]
)

// routerUserQuotaCheckEnabled 默认开启；可设 ROUTER_USER_QUOTA_CHECK=false|0|off|no 关闭（表不存在或本地无库时建议关闭）
func routerUserQuotaCheckEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ROUTER_USER_QUOTA_CHECK")))
	if v == "false" || v == "0" || v == "off" || v == "no" {
		return false
	}
	return true
}

func getRouterAgentUserIDCache() *cachex.HybridCache[int] {
	routerAgentUserIDCacheOnce.Do(func() {
		cacheCap := common.GetEnvOrDefault("ROUTER_AGENT_USER_CACHE_CAP", defaultRouterAgentUserIDCacheCap)
		if cacheCap <= 0 {
			cacheCap = defaultRouterAgentUserIDCacheCap
		}
		ttlSeconds := common.GetEnvOrDefault("ROUTER_AGENT_USER_CACHE_TTL_SECONDS", defaultRouterAgentUserIDTTLSecond)
		if ttlSeconds <= 0 {
			ttlSeconds = defaultRouterAgentUserIDTTLSecond
		}
		ttl := time.Duration(ttlSeconds) * time.Second
		routerAgentUserIDCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(routerAgentUserIDCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, cacheCap).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return routerAgentUserIDCache
}

// getAgentUserIDByUserID 通过当前用户 -> app_id -> role=AGENT 找到代理用户 new_user_id。
// 任一步未命中都返回 0,nil，表示跳过代理逻辑。
func getAgentUserIDByUserID(userId int) (int, error) {
	if !routerUserQuotaCheckEnabled() || userId <= 0 {
		return 0, nil
	}
	cache := getRouterAgentUserIDCache()
	cacheKey := fmt.Sprintf("uid:%d", userId)
	if v, found, err := cache.Get(cacheKey); err == nil && found {
		if v > 0 {
			return v, nil
		}
		return 0, nil
	}

	byUser, err := model.GetRouterUserByNewUserId(int64(userId))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			negTTL := common.GetEnvOrDefault("ROUTER_AGENT_USER_NEG_CACHE_TTL_SECONDS", defaultRouterAgentUserIDNegTTL)
			if negTTL <= 0 {
				negTTL = defaultRouterAgentUserIDNegTTL
			}
			_ = cache.SetWithTTL(cacheKey, 0, time.Duration(negTTL)*time.Second)
			return 0, nil
		}
		return 0, err
	}
	if byUser == nil || byUser.AppId == nil {
		negTTL := common.GetEnvOrDefault("ROUTER_AGENT_USER_NEG_CACHE_TTL_SECONDS", defaultRouterAgentUserIDNegTTL)
		if negTTL <= 0 {
			negTTL = defaultRouterAgentUserIDNegTTL
		}
		_ = cache.SetWithTTL(cacheKey, 0, time.Duration(negTTL)*time.Second)
		return 0, nil
	}
	appId := strings.TrimSpace(*byUser.AppId)
	if appId == "" {
		negTTL := common.GetEnvOrDefault("ROUTER_AGENT_USER_NEG_CACHE_TTL_SECONDS", defaultRouterAgentUserIDNegTTL)
		if negTTL <= 0 {
			negTTL = defaultRouterAgentUserIDNegTTL
		}
		_ = cache.SetWithTTL(cacheKey, 0, time.Duration(negTTL)*time.Second)
		return 0, nil
	}
	agent, err := model.GetAgentRouterUserByAppId(appId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			negTTL := common.GetEnvOrDefault("ROUTER_AGENT_USER_NEG_CACHE_TTL_SECONDS", defaultRouterAgentUserIDNegTTL)
			if negTTL <= 0 {
				negTTL = defaultRouterAgentUserIDNegTTL
			}
			_ = cache.SetWithTTL(cacheKey, 0, time.Duration(negTTL)*time.Second)
			return 0, nil
		}
		return 0, err
	}
	if agent == nil || agent.NewUserId == nil || *agent.NewUserId <= 0 {
		negTTL := common.GetEnvOrDefault("ROUTER_AGENT_USER_NEG_CACHE_TTL_SECONDS", defaultRouterAgentUserIDNegTTL)
		if negTTL <= 0 {
			negTTL = defaultRouterAgentUserIDNegTTL
		}
		_ = cache.SetWithTTL(cacheKey, 0, time.Duration(negTTL)*time.Second)
		return 0, nil
	}
	agentUserID := int(*agent.NewUserId)
	ttlSeconds := common.GetEnvOrDefault("ROUTER_AGENT_USER_CACHE_TTL_SECONDS", defaultRouterAgentUserIDTTLSecond)
	if ttlSeconds <= 0 {
		ttlSeconds = defaultRouterAgentUserIDTTLSecond
	}
	_ = cache.SetWithTTL(cacheKey, agentUserID, time.Duration(ttlSeconds)*time.Second)
	return agentUserID, nil
}

// mirrorConsumeQuotaToAgent 在请求实际扣费时，对关联代理执行同额度扣费/返还（不写消费日志）。
// 仅用于无 BillingSession 的旧回退路径；新路径应走「预扣 -> 差额结算 -> 失败退回」。
func mirrorConsumeQuotaToAgent(ctx context.Context, userId int, consumeQuota int) error {
	if consumeQuota == 0 {
		return nil
	}
	agentUserId, err := getAgentUserIDByUserID(userId)
	if err != nil || agentUserId <= 0 || agentUserId == userId {
		return err
	}
	if consumeQuota > 0 {
		if err := model.DecreaseUserQuota(agentUserId, consumeQuota); err != nil {
			return err
		}
		logger.LogInfo(ctx, fmt.Sprintf("代理同步扣费成功（sourceUserId=%d, agentUserId=%d, quota=%d）", userId, agentUserId, consumeQuota))
		return nil
	}
	if err := model.IncreaseUserQuota(agentUserId, -consumeQuota, false); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("代理同步返还成功（sourceUserId=%d, agentUserId=%d, quota=%d）", userId, agentUserId, -consumeQuota))
	return nil
}

// preConsumeQuotaToAgent 对关联代理执行预扣，返回命中的代理用户和实际预扣额度。
func preConsumeQuotaToAgent(ctx context.Context, userId int, preConsumedQuota int) (int, int, error) {
	if preConsumedQuota <= 0 {
		return 0, 0, nil
	}
	agentUserId, err := getAgentUserIDByUserID(userId)
	if err != nil || agentUserId <= 0 || agentUserId == userId {
		return 0, 0, err
	}
	if err := model.DecreaseUserQuota(agentUserId, preConsumedQuota); err != nil {
		return 0, 0, err
	}
	logger.LogInfo(ctx, fmt.Sprintf("代理预扣费成功（sourceUserId=%d, agentUserId=%d, preConsumedQuota=%d）", userId, agentUserId, preConsumedQuota))
	return agentUserId, preConsumedQuota, nil
}

// settlePreConsumedQuotaToAgent 按 actualQuota 与预扣额度的差额，对代理做补扣/返还。
func settlePreConsumedQuotaToAgent(ctx context.Context, sourceUserId int, agentUserId int, preConsumedQuota int, actualQuota int) error {
	if agentUserId <= 0 || agentUserId == sourceUserId {
		return nil
	}
	delta := actualQuota - preConsumedQuota
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if err := model.DecreaseUserQuota(agentUserId, delta); err != nil {
			return err
		}
		logger.LogInfo(ctx, fmt.Sprintf("代理补扣费成功（sourceUserId=%d, agentUserId=%d, delta=%d, actualQuota=%d, preConsumedQuota=%d）", sourceUserId, agentUserId, delta, actualQuota, preConsumedQuota))
		return nil
	}
	refundQuota := -delta
	if err := model.IncreaseUserQuota(agentUserId, refundQuota, false); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("代理差额返还成功（sourceUserId=%d, agentUserId=%d, refundQuota=%d, actualQuota=%d, preConsumedQuota=%d）", sourceUserId, agentUserId, refundQuota, actualQuota, preConsumedQuota))
	return nil
}

// refundPreConsumedQuotaToAgent 在请求失败时退还代理已预扣额度。
func refundPreConsumedQuotaToAgent(ctx context.Context, sourceUserId int, agentUserId int, preConsumedQuota int) error {
	if preConsumedQuota <= 0 || agentUserId <= 0 || agentUserId == sourceUserId {
		return nil
	}
	if err := model.IncreaseUserQuota(agentUserId, preConsumedQuota, false); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("代理预扣费退还成功（sourceUserId=%d, agentUserId=%d, refundQuota=%d）", sourceUserId, agentUserId, preConsumedQuota))
	return nil
}

// ResolveAgentActualQuotaByPlatformDiscount 使用平台给代理的折扣，基于折前额度计算代理实际扣费。
// 若无法解析 appid 或未命中折扣配置，则回退到 fallbackActualQuota。
func ResolveAgentActualQuotaByPlatformDiscount(relayInfo *relaycommon.RelayInfo, username string, preDiscountQuota decimal.Decimal, fallbackActualQuota int) int {
	agentActualQuota := fallbackActualQuota
	if relayInfo == nil {
		return agentActualQuota
	}
	if appid, ok := DiscountAppIDFromUsername(username); ok {
		platformMult := GetPlatformToAgentDiscountMultiplier(appid, relayInfo.UsingGroup, relayInfo.OriginModelName)
		agentActualQuota = QuotaAfterUserDiscountMultiplier(preDiscountQuota, platformMult)
	}
	relayInfo.AgentActualQuota = agentActualQuota
	relayInfo.AgentActualQuotaReady = true
	return agentActualQuota
}

func resolveAgentActualQuotaFromInt(relayInfo *relaycommon.RelayInfo, username string, preDiscountQuota int, fallbackActualQuota int) int {
	if preDiscountQuota <= 0 {
		if relayInfo != nil {
			relayInfo.AgentActualQuota = fallbackActualQuota
			relayInfo.AgentActualQuotaReady = true
		}
		return fallbackActualQuota
	}
	return ResolveAgentActualQuotaByPlatformDiscount(relayInfo, username, decimal.NewFromInt(int64(preDiscountQuota)), fallbackActualQuota)
}

func getAgentActualQuotaForSettle(relayInfo *relaycommon.RelayInfo, fallbackActualQuota int) int {
	if relayInfo != nil && relayInfo.AgentActualQuotaReady {
		return relayInfo.AgentActualQuota
	}
	return fallbackActualQuota
}

// updateAgentUsedQuotaStats 在请求成功后，同步代理的已用额度与请求次数统计（不写消费日志）。
func updateAgentUsedQuotaStats(ctx context.Context, sourceUserId int, actualQuota int) error {
	if actualQuota <= 0 {
		return nil
	}
	agentUserId, err := getAgentUserIDByUserID(sourceUserId)
	if err != nil || agentUserId <= 0 || agentUserId == sourceUserId {
		return err
	}
	model.UpdateUserUsedQuotaAndRequestCount(agentUserId, actualQuota)
	logger.LogInfo(ctx, fmt.Sprintf("代理已用额度统计更新成功（sourceUserId=%d, agentUserId=%d, actualQuota=%d）", sourceUserId, agentUserId, actualQuota))
	return nil
}

// updateAgentUsedQuotaStatsRefund 异步任务失败退费后，回退代理侧已用额度统计（与 updateAgentUsedQuotaStats 对称）。
func updateAgentUsedQuotaStatsRefund(ctx context.Context, sourceUserId int, refundQuota int) error {
	if refundQuota <= 0 {
		return nil
	}
	agentUserId, err := getAgentUserIDByUserID(sourceUserId)
	if err != nil || agentUserId <= 0 || agentUserId == sourceUserId {
		return err
	}
	model.UpdateUserUsedQuotaAndRequestCount(agentUserId, -refundQuota)
	logger.LogInfo(ctx, fmt.Sprintf("代理已用额度统计回退成功（sourceUserId=%d, agentUserId=%d, refundQuota=%d）", sourceUserId, agentUserId, refundQuota))
	return nil
}

// checkRouterUserAgentQuotaForPreConsume 在 new-api 侧预扣前进行 router_user 关联校验：
// 1) 当前用户 -> app_id；2) app_id + role=AGENT -> AGENT 的 new_user_id；
// 3) 用 new-api 现有用户逻辑判断：若 AGENT 用户被禁用，或额度不足以覆盖预扣，则返回 429。
// 任一步骤查不到记录（ErrRecordNotFound / 空字段）都直接跳过本次校验。
func checkRouterUserAgentQuotaForPreConsume(userId int, preConsumedQuota int) *types.NewAPIError {
	if !routerUserQuotaCheckEnabled() || preConsumedQuota <= 0 || userId <= 0 {
		return nil
	}
	agentUserId, err := getAgentUserIDByUserID(userId)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError)
	}
	if agentUserId <= 0 {
		return nil
	}

	agentUser, err := model.GetUserById(agentUserId, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return types.NewError(err, types.ErrorCodeQueryDataError)
	}
	if agentUser == nil {
		return nil
	}
	if agentUser.Status != common.UserStatusEnabled {
		err := fmt.Errorf("关联AGENT用户已禁用 (agent_user_id=%d)", agentUserId)
		return types.NewErrorWithStatusCode(err, types.ErrorCodeRouterAgentQuotaExceeded, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	agentUserQuota, err := model.GetUserQuota(agentUserId, false)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return types.NewError(err, types.ErrorCodeQueryDataError)
	}
	if agentUserQuota <= 0 || agentUserQuota-preConsumedQuota < 0 {
		err := fmt.Errorf("关联AGENT用户额度不足 (agent_user_id=%d, quota=%d, pre=%d)", agentUserId, agentUserQuota, preConsumedQuota)
		return types.NewErrorWithStatusCode(err, types.ErrorCodeRouterAgentQuotaExceeded, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	return nil
}

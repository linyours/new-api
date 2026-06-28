package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"golang.org/x/sync/singleflight"
)

const (
	// groupFallbackRedisKeyPrefix 是“分组兜底配置缓存”的 Redis key 前缀。
	groupFallbackRedisKeyPrefix = "group_fallback:enabled"

	// groupFallbackConfigCacheTTL 是“已命中配置”的缓存时长。
	//
	// 说明：
	// - 本缓存存于 Redis，所有容器共享；
	// - 兜底逻辑只在“主重试耗尽后”触发，故障风暴期会高并发命中该 key；
	// - 配置变更后会主动失效（见 InvalidateGroupFallbackCache），不必等 TTL 到期。
	groupFallbackConfigCacheTTL = 10 * time.Second

	// groupFallbackMissCacheTTL 是“未命中配置(nil)”的缓存时长。
	//
	// 说明：
	// - 当某个 group+type 没有配置时，失败风暴下同样可能反复触发查询；
	// - 对“未命中”写入 Redis 短 TTL，减少无效 DB 回源；
	// - TTL 设得更短，避免刚新增配置后“未命中缓存”持续太久。
	groupFallbackMissCacheTTL = 3 * time.Second
)

var (
	// groupFallbackLookupSF 仅用于“单容器内”合并同 key 的并发回源；
	// 集群一致性由 Redis 共享缓存保证。
	groupFallbackLookupSF singleflight.Group
)

type groupFallbackRedisPayload struct {
	// Found=true 且 Config!=nil 表示命中启用配置；
	// Found=false 表示“明确无配置”的 miss 缓存。
	Found  bool                        `json:"found"`
	Config *model.GroupChannelFallback `json:"config,omitempty"`
}

func makeGroupFallbackCacheKey(groupName string, channelType int) string {
	return fmt.Sprintf("%s:%s:%d", groupFallbackRedisKeyPrefix, strings.TrimSpace(groupName), channelType)
}

func cloneGroupFallbackConfig(cfg *model.GroupChannelFallback) *model.GroupChannelFallback {
	if cfg == nil {
		return nil
	}
	copied := *cfg
	return &copied
}

// InvalidateGroupFallbackCache 精确失效某个“分组+渠道类型”的兜底缓存。
//
// 该函数应在配置写操作成功后调用（新增/编辑/删除）。
func InvalidateGroupFallbackCache(groupName string, channelType int) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || channelType < 0 {
		return
	}
	key := makeGroupFallbackCacheKey(groupName, channelType)
	groupFallbackLookupSF.Forget(key)
	if !common.RedisEnabled {
		return
	}
	if err := common.RedisDel(key); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate group fallback redis cache key=%s: %v", key, err))
	}
}

func setGroupFallbackCacheToRedis(key string, cfg *model.GroupChannelFallback) error {
	if !common.RedisEnabled {
		return nil
	}
	payload := groupFallbackRedisPayload{}
	ttl := groupFallbackMissCacheTTL
	if cfg != nil {
		payload.Found = true
		payload.Config = cloneGroupFallbackConfig(cfg)
		ttl = groupFallbackConfigCacheTTL
	}
	serialized, err := common.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal group fallback redis payload failed: %w", err)
	}
	if err := common.RedisSet(key, string(serialized), ttl); err != nil {
		return fmt.Errorf("set group fallback redis cache failed: %w", err)
	}
	return nil
}

func getGroupFallbackCacheFromRedis(key string) (hit bool, cfg *model.GroupChannelFallback, err error) {
	if !common.RedisEnabled {
		return false, nil, nil
	}
	raw, err := common.RedisGet(key)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("get group fallback redis cache failed: %w", err)
	}
	var payload groupFallbackRedisPayload
	if err := common.Unmarshal([]byte(raw), &payload); err != nil {
		// 避免脏数据反复触发反序列化错误，尝试删除该 key。
		_ = common.RedisDel(key)
		return false, nil, fmt.Errorf("unmarshal group fallback redis payload failed: %w", err)
	}
	if !payload.Found {
		return true, nil, nil
	}
	return true, cloneGroupFallbackConfig(payload.Config), nil
}

func getEnabledGroupFallbackWithCache(groupName string, channelType int) (*model.GroupChannelFallback, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || channelType < 0 {
		return nil, nil
	}
	key := makeGroupFallbackCacheKey(groupName, channelType)

	// 1) 先读 Redis 共享缓存（集群稳态：所有容器读取同一份数据）。
	if hit, cfg, err := getGroupFallbackCacheFromRedis(key); err == nil {
		if hit {
			return cfg, nil
		}
	} else {
		// Redis 异常时降级 DB，保证业务可用性。
		common.SysLog(fmt.Sprintf("group fallback redis read failed, fallback to DB. key=%s err=%v", key, err))
	}

	// 2) 未命中时，使用 singleflight 合并单实例内并发回源，避免 DB 击穿。
	result, err, _ := groupFallbackLookupSF.Do(key, func() (interface{}, error) {
		// 2.1) 再次检查 Redis，避免等待期间其他请求已回填缓存后重复查 DB。
		if hit, cfg, err := getGroupFallbackCacheFromRedis(key); err == nil {
			if hit {
				return cfg, nil
			}
		} else {
			common.SysLog(fmt.Sprintf("group fallback redis recheck failed, fallback to DB. key=%s err=%v", key, err))
		}

		// 2.2) 回源 DB。
		cfg, err := model.GetEnabledGroupChannelFallback(groupName, channelType)
		if err != nil {
			return nil, err
		}

		// 2.3) 回填 Redis（命中和未命中都缓存），让所有容器共享结果。
		if err := setGroupFallbackCacheToRedis(key, cfg); err != nil {
			// 缓存写失败不影响主流程；仅记录日志，返回 DB 结果。
			common.SysLog(fmt.Sprintf("set group fallback redis cache failed, key=%s err=%v", key, err))
		}
		return cfg, nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	cfg, _ := result.(*model.GroupChannelFallback)
	return cloneGroupFallbackConfig(cfg), nil
}

// ResolveFallbackLookupGroup 解析当前请求“应查询兜底配置”的分组。
//
// 规则：
// 1) auto 分组优先使用当前请求上下文中“实际命中的 auto_group”；
// 2) 其次使用 using_group；
// 3) 最后回退 token_group；
// 4) 若都不可用返回空字符串（表示不触发兜底）。
func ResolveFallbackLookupGroup(c *gin.Context, retryParam *RetryParam) string {
	if c == nil {
		return ""
	}

	tokenGroup := ""
	if retryParam != nil {
		tokenGroup = strings.TrimSpace(retryParam.TokenGroup)
	}
	usingGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))

	// auto 场景：兜底绑定应该跟随“当前真实命中的分组”，而不是字面量 "auto"。
	if tokenGroup == "auto" || usingGroup == "auto" {
		autoGroup := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
		if autoGroup != "" {
			return autoGroup
		}
	}
	if usingGroup != "" && usingGroup != "auto" {
		return usingGroup
	}
	if tokenGroup != "" && tokenGroup != "auto" {
		return tokenGroup
	}
	return ""
}

// ValidateGroupFallbackBinding 校验“分组兜底绑定”是否合法。
//
// 约束：
// - fallback_channel_id 必须存在；
// - fallback channel 的 type 必须与配置维度 channel_type 一致；
// - 允许 fallback channel 来自任意分组（只校验类型一致）。
func ValidateGroupFallbackBinding(groupName string, channelType int, fallbackChannelId int) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return errors.New("group_name is required")
	}
	if channelType < 0 {
		return errors.New("channel_type is invalid")
	}
	if fallbackChannelId <= 0 {
		return errors.New("fallback_channel_id is invalid")
	}

	channel, err := model.GetChannelById(fallbackChannelId, true)
	if err != nil {
		return fmt.Errorf("fallback channel not found: %w", err)
	}
	if channel.Type != channelType {
		return fmt.Errorf("fallback channel type mismatch: expect %d, got %d", channelType, channel.Type)
	}
	return nil
}

// ResolveGroupFallbackChannel 获取“可执行的兜底渠道”。
//
// 这里会进行运行期防御校验，避免配置变更/渠道状态变化导致异常：
// - 规则存在且启用；
// - 渠道存在；
// - 渠道类型匹配；
// - 渠道状态为启用。
func ResolveGroupFallbackChannel(groupName string, channelType int) (*model.Channel, error) {
	cfg, err := getEnabledGroupFallbackWithCache(groupName, channelType)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}

	// 说明：
	// - 这里优先复用现有渠道缓存（CacheGetChannel）：
	//   - 开启 MemoryCache 时：从 channelsIDM 读，且渠道状态变更会同步更新缓存；
	//   - 关闭 MemoryCache 时：自动回退到 DB 查询（与原行为一致）。
	// - 这样可把失败风暴期的“兜底渠道详情查询”从 DB 读尽可能转为内存读。
	channel, err := model.CacheGetChannel(cfg.FallbackChannelId)
	if err != nil {
		return nil, fmt.Errorf("fallback channel #%d not found: %w", cfg.FallbackChannelId, err)
	}
	if channel.Type != channelType {
		return nil, fmt.Errorf("fallback channel type mismatch: expect %d, got %d", channelType, channel.Type)
	}
	if channel.Status != common.ChannelStatusEnabled {
		return nil, fmt.Errorf("fallback channel #%d is not enabled", channel.Id)
	}
	return channel, nil
}

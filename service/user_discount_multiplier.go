package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/samber/hot"
	"github.com/shopspring/decimal"
)

type discountConfigRow struct {
	Account      *string `json:"account"`
	DiscountInfos string  `json:"discount_infos"`
}

type discountInfoJSON struct {
	// 与 DB/前端 JSON 一致：分组字段名为 group（Java 侧序列化可能为 rule，本库按 group 解析）
	Group     string `json:"group"`
	ModelRule string `json:"modelRule"`
	// Java: DiscountInfo.getDiscount()  => JSON: "discount"
	// 可能是 number(0.8) 或 string("0.8")
	Discount json.RawMessage `json:"discount"`
	// Java 里还会有 type: "GROUP"（本实现忽略它）
}

const (
	// 与 ai-router OptionsManager.getUserDiscountConfig 一致：仅使用 type=AGENT 的 discount_config（见 DiscountConfigManager.getDiscountConfigForAccount）
	discountConfigTypeAgent = "AGENT"
	// 平台给代理的折扣：DiscountConfigManager.getDiscountConfigForAgent(appId, PLATFORM)
	discountConfigTypePlatform = "PLATFORM"
	// v2-agent：此前未过滤 type，升级命名空间避免误用旧缓存
	discountMultiplierCacheNamespace = "new-api:user_discount_multiplier:v2-agent"
	platformDiscountCacheNamespace   = "new-api:platform_discount_multiplier:v1"
	defaultCacheTTLSeconds           = 60
	defaultCacheCapacity            = 100000
)

var (
	discountMultiplierCacheOnce sync.Once
	discountMultiplierCache     *cachex.HybridCache[string] // value: decimal string

	platformDiscountCacheOnce sync.Once
	platformDiscountCache     *cachex.HybridCache[string]
)

func getDiscountMultiplierCache() *cachex.HybridCache[string] {
	discountMultiplierCacheOnce.Do(func() {
		ttlSeconds := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_TTL_SECONDS", defaultCacheTTLSeconds)
		if ttlSeconds <= 0 {
			ttlSeconds = defaultCacheTTLSeconds
		}
		capacity := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_CAP", defaultCacheCapacity)
		if capacity <= 0 {
			capacity = defaultCacheCapacity
		}
		ttl := time.Duration(ttlSeconds) * time.Second

		discountMultiplierCache = cachex.NewHybridCache[string](cachex.HybridCacheConfig[string]{
			Namespace: cachex.Namespace(discountMultiplierCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.StringCodec{},
			Memory: func() *hot.HotCache[string, string] {
				return hot.NewHotCache[string, string](hot.LRU, capacity).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return discountMultiplierCache
}

func getPlatformDiscountCache() *cachex.HybridCache[string] {
	platformDiscountCacheOnce.Do(func() {
		ttlSeconds := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_TTL_SECONDS", defaultCacheTTLSeconds)
		if ttlSeconds <= 0 {
			ttlSeconds = defaultCacheTTLSeconds
		}
		capacity := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_CAP", defaultCacheCapacity)
		if capacity <= 0 {
			capacity = defaultCacheCapacity
		}
		ttl := time.Duration(ttlSeconds) * time.Second
		platformDiscountCache = cachex.NewHybridCache[string](cachex.HybridCacheConfig[string]{
			Namespace: cachex.Namespace(platformDiscountCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.StringCodec{},
			Memory: func() *hot.HotCache[string, string] {
				return hot.NewHotCache[string, string](hot.LRU, capacity).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		})
	})
	return platformDiscountCache
}

// DiscountAppIDFromUsername 从 username（appid-account）解析 app_id，供平台折扣查询与落表。
func DiscountAppIDFromUsername(username string) (appid string, ok bool) {
	appid, _, ok = parseUsernameToAppIdAccount(username)
	return appid, ok
}

// parseUsernameToAppIdAccount：只按第一个 '-' 切分为 appid/account
// username: bluecat-root1111116 => appid=bluecat, account=root1111116
func parseUsernameToAppIdAccount(username string) (appid, account string, ok bool) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", "", false
	}

	parts := strings.SplitN(username, "-", 2) // 只用第一个 '-'
	if len(parts) != 2 {
		return "", "", false
	}

	appid = strings.TrimSpace(parts[0])
	account = strings.TrimSpace(parts[1])
	if appid == "" || account == "" {
		return "", "", false
	}
	return appid, account, true
}

func parseDiscountInfos(discountInfosJSON string) []discountInfoJSON {
	discountInfosJSON = strings.TrimSpace(discountInfosJSON)
	if discountInfosJSON == "" {
		return nil
	}
	var infos []discountInfoJSON
	if err := json.Unmarshal([]byte(discountInfosJSON), &infos); err != nil {
		return nil
	}
	return infos
}

// parseDiscountMultiplier：discount 可能是 JSON number(0.8) 或 string("0.8")
// 未解析/无效则回退 one。
func parseDiscountMultiplier(raw json.RawMessage, one decimal.Decimal) decimal.Decimal {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return one
	}

	// "0.8"
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return one
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return one
		}
		d, err := decimal.NewFromString(s)
		if err != nil || d.LessThanOrEqual(decimal.Zero) {
			return one
		}
		return d
	}

	// 0.8
	d, err := decimal.NewFromString(strings.TrimSpace(string(raw)))
	if err != nil || d.LessThanOrEqual(decimal.Zero) {
		return one
	}
	return d
}

// GetUserDiscountMultiplier：与 ai-router OptionsManager.getUserDiscountConfig 数据源一致——仅查询 discount_config.type=AGENT（见 DiscountConfigManager.getDiscountConfigForAccount）；
// 匹配顺序复刻 DiscountConfigManager.getDiscountConfigForUser（113-149）：先 account 精确，再 account IS NULL。
// - 命中返回 discountInfo.discount（乘子：0.8 直接 Mul）
// - 未命中返回 1（并写缓存）
// - group 使用 relayInfo.UsingGroup
func GetUserDiscountMultiplier(username, group, modelName string) decimal.Decimal {
	one := decimal.NewFromInt(1)

	appid, account, ok := parseUsernameToAppIdAccount(username)
	if !ok {
		common.SysLog(fmt.Sprintf("折扣乘子：用户名切分失败（username=%q）", username))
		return one
	}

	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	if group == "" || modelName == "" {
		common.SysLog(fmt.Sprintf("折扣乘子：group 或 modelName 为空（appid=%q, account=%q, group=%q, model=%q）=> 1", appid, account, group, modelName))
		return one
	}

	cacheKey := strings.Join([]string{appid, account, group, modelName}, ":")
	cache := getDiscountMultiplierCache()

	// 1) cache hit
	if v, found, err := cache.Get(cacheKey); err == nil && found {
		d, parseErr := decimal.NewFromString(strings.TrimSpace(v))
		if parseErr == nil && d.GreaterThan(decimal.Zero) {
			common.SysLog(fmt.Sprintf("折扣乘子：缓存命中（appid=%q, account=%q, group=%q, model=%q）=> %s", appid, account, group, modelName, d.String()))
			return d
		}
		common.SysLog(fmt.Sprintf("折扣乘子：缓存命中但值无效（appid=%q, account=%q, group=%q, model=%q）raw=%q => 1", appid, account, group, modelName, v))
		return one
	}

	common.SysLog(fmt.Sprintf("折扣乘子：缓存未命中（appid=%q, account=%q, group=%q, model=%q），查询 DB", appid, account, group, modelName))

	// 2) DB + 匹配（两段：account 精确 -> account==null 兜底）
	multiplier := getUserDiscountMultiplierFromDB(appid, account, group, modelName, one)

	// 3) cache write（未命中=1也写）
	ttlSeconds := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_TTL_SECONDS", defaultCacheTTLSeconds)
	if ttlSeconds <= 0 {
		ttlSeconds = defaultCacheTTLSeconds
	}
	_ = cache.SetWithTTL(cacheKey, multiplier.String(), time.Duration(ttlSeconds)*time.Second)

	return multiplier
}

func getUserDiscountMultiplierFromDB(appid, account, group, modelName string, one decimal.Decimal) decimal.Decimal {
	// pass #1：discount_config.account != null && equals(account)
	var rowsAccount []discountConfigRow
	if err := model.DB.Table("discount_config").
		Select("account, discount_infos").
		Where("app_id = ? AND account = ? AND type = ? AND delete_time IS NULL", appid, account, discountConfigTypeAgent).
		Order("id asc").
		Find(&rowsAccount).Error; err != nil {
		common.SysLog(fmt.Sprintf("折扣乘子：DB 查询 pass#1 失败（appid=%q, account=%q）：%v => 1", appid, account, err))
		return one
	}

	for _, cfg := range rowsAccount {
		infos := parseDiscountInfos(cfg.DiscountInfos)
		if len(infos) == 0 {
			continue
		}
		for _, info := range infos {
			if !strings.EqualFold(group, strings.TrimSpace(info.Group)) {
				continue
			}

			rule := strings.TrimSpace(info.ModelRule)
			if rule == "*" || strings.Contains(modelName, rule) {
				m := parseDiscountMultiplier(info.Discount, one)
				common.SysLog(fmt.Sprintf("折扣乘子：命中 pass#1（appid=%q, account=%q, group=%q, modelRule=%q, model=%q）=> %s",
					appid, account, group, rule, modelName, m.String()))
				return m
			}
		}
	}

	// pass #2：discount_config.account == null（兜底）
	var rowsNull []discountConfigRow
	if err := model.DB.Table("discount_config").
		Select("account, discount_infos").
		Where("app_id = ? AND account IS NULL AND type = ? AND delete_time IS NULL", appid, discountConfigTypeAgent).
		Order("id asc").
		Find(&rowsNull).Error; err != nil {
		common.SysLog(fmt.Sprintf("折扣乘子：DB 查询 pass#2 失败（appid=%q）：%v => 1", appid, err))
		return one
	}

	for _, cfg := range rowsNull {
		infos := parseDiscountInfos(cfg.DiscountInfos)
		if len(infos) == 0 {
			continue
		}
		for _, info := range infos {
			if !strings.EqualFold(group, strings.TrimSpace(info.Group)) {
				continue
			}

			rule := strings.TrimSpace(info.ModelRule)
			if rule == "*" || strings.Contains(modelName, rule) {
				m := parseDiscountMultiplier(info.Discount, one)
				common.SysLog(fmt.Sprintf("折扣乘子：命中 pass#2（appid=%q, account=%q, group=%q, modelRule=%q, model=%q）=> %s",
					appid, account, group, rule, modelName, m.String()))
				return m
			}
		}
	}

	common.SysLog(fmt.Sprintf("折扣乘子：未命中 => 1（appid=%q, account=%q, group=%q, model=%q）", appid, account, group, modelName))
	return one
}

// matchPlatformToAgentDiscount 复刻 DiscountConfigManager.getDiscountConfigForAgent(List, group, model)（154-170）：
// (group 与 rule 一致或 rule 为空) 且 (modelRule 为 * 或 model 包含 modelRule)，取第一条命中。
func matchPlatformToAgentDiscount(infos []discountInfoJSON, group, modelName string, one decimal.Decimal) (decimal.Decimal, bool) {
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	for _, info := range infos {
		infoGroup := strings.TrimSpace(info.Group)
		if infoGroup != "" && !strings.EqualFold(group, infoGroup) {
			continue
		}
		rule := strings.TrimSpace(info.ModelRule)
		if rule == "" {
			continue
		}
		if rule == "*" || strings.Contains(modelName, rule) {
			m := parseDiscountMultiplier(info.Discount, one)
			return m, true
		}
	}
	return one, false
}

func getPlatformToAgentDiscountMultiplierFromDB(appid, group, modelName string, one decimal.Decimal) decimal.Decimal {
	var rows []discountConfigRow
	if err := model.DB.Table("discount_config").
		Select("discount_infos").
		Where("app_id = ? AND type = ? AND delete_time IS NULL", appid, discountConfigTypePlatform).
		Order("id asc").
		Find(&rows).Error; err != nil {
		common.SysLog(fmt.Sprintf("平台折扣乘子：DB 查询失败（appid=%q）：%v => 1", appid, err))
		return one
	}
	for _, cfg := range rows {
		infos := parseDiscountInfos(cfg.DiscountInfos)
		if len(infos) == 0 {
			continue
		}
		if m, ok := matchPlatformToAgentDiscount(infos, group, modelName, one); ok {
			common.SysLog(fmt.Sprintf("平台折扣乘子：命中（appid=%q, group=%q, model=%q）=> %s", appid, group, modelName, m.String()))
			return m
		}
	}
	common.SysLog(fmt.Sprintf("平台折扣乘子：未命中 => 1（appid=%q, group=%q, model=%q）", appid, group, modelName))
	return one
}

// GetPlatformToAgentDiscountMultiplier 查询平台给代理的折扣（discount_config.type=PLATFORM），用于消费日志落表，不参与结算乘子。
// 匹配规则与 Java getDiscountConfigForAgent(discountInfos, group, model) 一致。
func GetPlatformToAgentDiscountMultiplier(appid, group, modelName string) decimal.Decimal {
	one := decimal.NewFromInt(1)
	appid = strings.TrimSpace(appid)
	group = strings.TrimSpace(group)
	modelName = strings.TrimSpace(modelName)
	if appid == "" || group == "" || modelName == "" {
		return one
	}

	cacheKey := strings.Join([]string{appid, group, modelName}, ":")
	cache := getPlatformDiscountCache()

	if v, found, err := cache.Get(cacheKey); err == nil && found {
		d, parseErr := decimal.NewFromString(strings.TrimSpace(v))
		if parseErr == nil && d.GreaterThan(decimal.Zero) {
			return d
		}
		return one
	}

	multiplier := getPlatformToAgentDiscountMultiplierFromDB(appid, group, modelName, one)
	ttlSeconds := common.GetEnvOrDefault("DISCOUNT_MULTIPLIER_CACHE_TTL_SECONDS", defaultCacheTTLSeconds)
	if ttlSeconds <= 0 {
		ttlSeconds = defaultCacheTTLSeconds
	}
	_ = cache.SetWithTTL(cacheKey, multiplier.String(), time.Duration(ttlSeconds)*time.Second)
	return multiplier
}


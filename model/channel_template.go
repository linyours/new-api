package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

const (
	DefaultChannelTemplateNameSuffixLength = 6
	MinChannelTemplateNameSuffixLength     = 4
	MaxChannelTemplateNameSuffixLength     = 16
	MaxChannelTemplateNameLength           = 64
	MaxChannelTemplateApplyKeys            = 500
	maxChannelTemplateNameCollisionTries   = 10000
)

// ChannelTemplate stores a reusable channel configuration without credentials.
type ChannelTemplate struct {
	Id               int    `json:"id"`
	Name             string `json:"name" gorm:"type:varchar(64);uniqueIndex;not null"`
	Description      string `json:"description" gorm:"type:varchar(255)"`
	ChannelType      int    `json:"channel_type"`
	Config           string `json:"config" gorm:"type:text;not null"`
	NameSuffixLength int    `json:"name_suffix_length"`
	CreatedTime      int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime      int64  `json:"updated_time" gorm:"bigint"`
}

// ChannelTemplateConfig is the persisted channel snapshot. It intentionally
// omits Key and usage counters so templates cannot become a secret store.
type ChannelTemplateConfig struct {
	Type               int                      `json:"type"`
	OpenAIOrganization *string                  `json:"openai_organization,omitempty"`
	TestModel          *string                  `json:"test_model,omitempty"`
	Status             int                      `json:"status"`
	Weight             *uint                    `json:"weight,omitempty"`
	BaseURL            *string                  `json:"base_url,omitempty"`
	Other              string                   `json:"other,omitempty"`
	Models             string                   `json:"models"`
	Group              string                   `json:"group"`
	KeyRpmLimit        int                      `json:"key_rpm_limit"`
	KeyQuotaLimit      int64                    `json:"key_quota_limit"`
	ModelMapping       *string                  `json:"model_mapping,omitempty"`
	StatusCodeMapping  *string                  `json:"status_code_mapping,omitempty"`
	Priority           *int64                   `json:"priority,omitempty"`
	AutoBan            *int                     `json:"auto_ban,omitempty"`
	Tag                *string                  `json:"tag,omitempty"`
	Setting            *string                  `json:"setting,omitempty"`
	ParamOverride      *string                  `json:"param_override,omitempty"`
	HeaderOverride     *string                  `json:"header_override,omitempty"`
	Remark             *string                  `json:"remark,omitempty"`
	OtherSettings      string                   `json:"settings,omitempty"`
	ModelRpmLimits     ChannelKeyModelRpmLimits `json:"model_rpm_limits,omitempty"`
}

func NormalizeChannelTemplateNameSuffixLength(n int) int {
	if n <= 0 {
		return DefaultChannelTemplateNameSuffixLength
	}
	if n < MinChannelTemplateNameSuffixLength {
		return MinChannelTemplateNameSuffixLength
	}
	if n > MaxChannelTemplateNameSuffixLength {
		return MaxChannelTemplateNameSuffixLength
	}
	return n
}

func NormalizeChannelTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("template name cannot be empty")
	}
	if len(name) > MaxChannelTemplateNameLength {
		return "", fmt.Errorf("template name cannot exceed %d characters", MaxChannelTemplateNameLength)
	}
	return name, nil
}

func ChannelTemplateConfigFromChannel(channel *Channel) (ChannelTemplateConfig, error) {
	if channel == nil {
		return ChannelTemplateConfig{}, errors.New("channel cannot be empty")
	}
	cfg := ChannelTemplateConfig{
		Type:               channel.Type,
		OpenAIOrganization: cloneStringPtr(channel.OpenAIOrganization),
		TestModel:          cloneStringPtr(channel.TestModel),
		Status:             channel.Status,
		Weight:             cloneUintPtr(channel.Weight),
		BaseURL:            cloneStringPtr(channel.BaseURL),
		Other:              channel.Other,
		Models:             channel.Models,
		Group:              channel.Group,
		KeyRpmLimit:        channel.KeyRpmLimit,
		KeyQuotaLimit:      channel.KeyQuotaLimit,
		ModelMapping:       cloneStringPtr(channel.ModelMapping),
		StatusCodeMapping:  cloneStringPtr(channel.StatusCodeMapping),
		Priority:           cloneInt64Ptr(channel.Priority),
		AutoBan:            cloneIntPtr(channel.AutoBan),
		Tag:                cloneStringPtr(channel.Tag),
		Setting:            cloneStringPtr(channel.Setting),
		ParamOverride:      cloneStringPtr(channel.ParamOverride),
		HeaderOverride:     cloneStringPtr(channel.HeaderOverride),
		Remark:             cloneStringPtr(channel.Remark),
		OtherSettings:      channel.OtherSettings,
	}
	if cfg.Status == 0 {
		cfg.Status = common.ChannelStatusEnabled
	}
	if strings.TrimSpace(cfg.Group) == "" {
		cfg.Group = "default"
	}

	var keys []*ChannelKey
	if err := DB.
		Where("channel_id = ? AND status <> ?", channel.Id, ChannelKeyStatusArchived).
		Order("position ASC").
		Find(&keys).Error; err != nil {
		return ChannelTemplateConfig{}, err
	}
	if len(keys) == 1 {
		cfg.ModelRpmLimits = cloneChannelKeyModelRpmLimits(keys[0].ModelRpmLimits)
	}
	return cfg, nil
}

func (cfg ChannelTemplateConfig) ToChannel() *Channel {
	status := cfg.Status
	if status == 0 {
		status = common.ChannelStatusEnabled
	}
	group := strings.TrimSpace(cfg.Group)
	if group == "" {
		group = "default"
	}
	return &Channel{
		Type:               cfg.Type,
		OpenAIOrganization: cloneStringPtr(cfg.OpenAIOrganization),
		TestModel:          cloneStringPtr(cfg.TestModel),
		Status:             status,
		Weight:             cloneUintPtr(cfg.Weight),
		BaseURL:            cloneStringPtr(cfg.BaseURL),
		Other:              cfg.Other,
		Models:             cfg.Models,
		Group:              group,
		KeyRpmLimit:        cfg.KeyRpmLimit,
		KeyQuotaLimit:      cfg.KeyQuotaLimit,
		ModelMapping:       cloneStringPtr(cfg.ModelMapping),
		StatusCodeMapping:  cloneStringPtr(cfg.StatusCodeMapping),
		Priority:           cloneInt64Ptr(cfg.Priority),
		AutoBan:            cloneIntPtr(cfg.AutoBan),
		Tag:                cloneStringPtr(cfg.Tag),
		Setting:            cloneStringPtr(cfg.Setting),
		ParamOverride:      cloneStringPtr(cfg.ParamOverride),
		HeaderOverride:     cloneStringPtr(cfg.HeaderOverride),
		Remark:             cloneStringPtr(cfg.Remark),
		OtherSettings:      cfg.OtherSettings,
	}
}

func (cfg ChannelTemplateConfig) MarshalConfig() (string, error) {
	data, err := common.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func ParseChannelTemplateConfig(raw string) (ChannelTemplateConfig, error) {
	var cfg ChannelTemplateConfig
	if strings.TrimSpace(raw) == "" {
		return cfg, errors.New("template config cannot be empty")
	}
	if err := common.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("invalid template config: %w", err)
	}
	if cfg.ModelRpmLimits != nil {
		normalized, err := cfg.ModelRpmLimits.Normalize()
		if err != nil {
			return cfg, err
		}
		cfg.ModelRpmLimits = normalized
	}
	if cfg.Status == 0 {
		cfg.Status = common.ChannelStatusEnabled
	}
	if strings.TrimSpace(cfg.Group) == "" {
		cfg.Group = "default"
	}
	return cfg, nil
}

func (template *ChannelTemplate) ParsedConfig() (ChannelTemplateConfig, error) {
	if template == nil {
		return ChannelTemplateConfig{}, errors.New("invalid template")
	}
	return ParseChannelTemplateConfig(template.Config)
}

func ChannelTemplateKeyIdentity(rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "{") {
		var obj map[string]any
		if common.Unmarshal([]byte(key), &obj) == nil {
			for _, field := range []string{"client_email", "account_id", "project_id"} {
				value := strings.TrimSpace(fmt.Sprintf("%v", obj[field]))
				if value == "" || value == "<nil>" {
					continue
				}
				if local, _, found := strings.Cut(value, "@"); found && strings.TrimSpace(local) != "" {
					return strings.TrimSpace(local)
				}
				return value
			}
		}
	}
	if index := strings.IndexByte(key, '|'); index > 0 {
		return strings.TrimSpace(key[:index])
	}
	return key
}

func ChannelTemplateNameSuffix(rawKey string, suffixLength int) string {
	suffixLength = NormalizeChannelTemplateNameSuffixLength(suffixLength)
	identity := ChannelTemplateKeyIdentity(rawKey)
	alnum := make([]rune, 0, len(identity))
	for _, r := range identity {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			alnum = append(alnum, r)
		}
	}
	if len(alnum) == 0 {
		sum := sha256.Sum256([]byte(identity))
		encoded := hex.EncodeToString(sum[:])
		if suffixLength > len(encoded) {
			suffixLength = len(encoded)
		}
		return encoded[:suffixLength]
	}
	if len(alnum) <= suffixLength {
		return string(alnum)
	}
	return string(alnum[len(alnum)-suffixLength:])
}

func BuildChannelTemplateChannelName(baseName, rawKey string, suffixLength int) string {
	baseName = strings.TrimSpace(baseName)
	suffix := ChannelTemplateNameSuffix(rawKey, suffixLength)
	if baseName == "" {
		return suffix
	}
	if suffix == "" {
		return baseName
	}
	return baseName + "-" + suffix
}

func AllocateUniqueChannelName(baseName string, reserved map[string]struct{}) (string, error) {
	if reserved == nil {
		reserved = make(map[string]struct{})
	}
	name := strings.TrimSpace(baseName)
	if name == "" {
		return "", errors.New("channel name cannot be empty")
	}
	for i := 0; i < maxChannelTemplateNameCollisionTries; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", name, i+1)
		}
		if _, exists := reserved[candidate]; exists {
			continue
		}
		var count int64
		if err := DB.Model(&Channel{}).Where("name = ?", candidate).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			reserved[candidate] = struct{}{}
			return candidate, nil
		}
	}
	return "", errors.New("unable to allocate a unique channel name")
}

func GetAllChannelTemplates() ([]*ChannelTemplate, error) {
	var templates []*ChannelTemplate
	err := DB.Order("updated_time desc").Find(&templates).Error
	return templates, err
}

func GetChannelTemplateById(id int) (*ChannelTemplate, error) {
	if id <= 0 {
		return nil, errors.New("invalid template id")
	}
	template := &ChannelTemplate{}
	if err := DB.First(template, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return template, nil
}

func IsChannelTemplateNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	query := DB.Model(&ChannelTemplate{}).Where("name = ?", name)
	if id > 0 {
		query = query.Where("id <> ?", id)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (template *ChannelTemplate) Insert() error {
	if template == nil {
		return errors.New("invalid template")
	}
	now := common.GetTimestamp()
	template.CreatedTime = now
	template.UpdatedTime = now
	template.NameSuffixLength = NormalizeChannelTemplateNameSuffixLength(template.NameSuffixLength)
	return DB.Create(template).Error
}

func (template *ChannelTemplate) Update() error {
	if template == nil || template.Id <= 0 {
		return errors.New("invalid template")
	}
	template.UpdatedTime = common.GetTimestamp()
	template.NameSuffixLength = NormalizeChannelTemplateNameSuffixLength(template.NameSuffixLength)
	return DB.Save(template).Error
}

func DeleteChannelTemplateById(id int) error {
	if id <= 0 {
		return errors.New("invalid template id")
	}
	return DB.Delete(&ChannelTemplate{}, id).Error
}

func ApplyChannelKeyModelRpmLimits(channelId int, limits ChannelKeyModelRpmLimits) error {
	if channelId <= 0 {
		return errors.New("invalid channel")
	}
	if len(limits) == 0 {
		return nil
	}
	normalized, err := limits.Normalize()
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	if err := DB.Model(&ChannelKey{}).
		Where("channel_id = ? AND status <> ?", channelId, ChannelKeyStatusArchived).
		Updates(map[string]any{
			"model_rpm_limits": normalized,
			"updated_at":       now,
		}).Error; err != nil {
		return err
	}
	return ReloadChannelKeyCache(channelId)
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneUintPtr(value *uint) *uint {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneChannelKeyModelRpmLimits(limits ChannelKeyModelRpmLimits) ChannelKeyModelRpmLimits {
	if len(limits) == 0 {
		return nil
	}
	copied := make(ChannelKeyModelRpmLimits, len(limits))
	for modelName, rpm := range limits {
		copied[modelName] = rpm
	}
	return copied
}

package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupChannelFallback 定义“分组+渠道类型 -> 兜底渠道”的绑定关系。
//
// 设计要点：
// 1) 只在主重试耗尽后才会使用，不参与正常选渠主路径；
// 2) 维度为 group_name + channel_type（唯一）；
// 3) fallback_channel_id 指向已存在渠道。
type GroupChannelFallback struct {
	Id                int    `json:"id"`
	GroupName         string `json:"group_name" gorm:"type:varchar(64);not null;uniqueIndex:uk_group_channel_type"`
	ChannelType       int    `json:"channel_type" gorm:"not null;uniqueIndex:uk_group_channel_type"`
	FallbackChannelId int    `json:"fallback_channel_id" gorm:"not null;index"`
	Enabled           bool   `json:"enabled" gorm:"not null;default:true"`
	Remark            string `json:"remark" gorm:"type:varchar(255);default:''"`
	CreatedTime       int64  `json:"created_time" gorm:"bigint;not null;default:0"`
	UpdatedTime       int64  `json:"updated_time" gorm:"bigint;not null;default:0"`
}

func (GroupChannelFallback) TableName() string {
	return "group_channel_fallbacks"
}

func (f *GroupChannelFallback) normalize() {
	f.GroupName = strings.TrimSpace(f.GroupName)
	f.Remark = strings.TrimSpace(f.Remark)
}

func (f *GroupChannelFallback) validate() error {
	if f == nil {
		return errors.New("group fallback config is nil")
	}
	f.normalize()
	if f.GroupName == "" {
		return errors.New("group_name is required")
	}
	if f.ChannelType < 0 {
		return errors.New("channel_type is invalid")
	}
	if f.FallbackChannelId <= 0 {
		return errors.New("fallback_channel_id is invalid")
	}
	if len(f.Remark) > 255 {
		return errors.New("remark length must be <= 255")
	}
	return nil
}

// UpsertGroupChannelFallback 按 group_name + channel_type 幂等写入配置。
//
// 注意：这里仅做结构层校验，业务层（渠道是否存在、类型是否匹配等）
// 在 service 层统一校验，避免 model 层耦合业务规则。
func UpsertGroupChannelFallback(f *GroupChannelFallback) error {
	if err := f.validate(); err != nil {
		return err
	}
	now := common.GetTimestamp()
	if f.CreatedTime == 0 {
		f.CreatedTime = now
	}
	f.UpdatedTime = now

	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "group_name"},
			{Name: "channel_type"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"fallback_channel_id": f.FallbackChannelId,
			"enabled":             f.Enabled,
			"remark":              f.Remark,
			"updated_time":        f.UpdatedTime,
		}),
	}).Create(f).Error
}

// GetEnabledGroupChannelFallback 读取启用状态的兜底配置。
func GetEnabledGroupChannelFallback(groupName string, channelType int) (*GroupChannelFallback, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || channelType < 0 {
		return nil, nil
	}
	var cfg GroupChannelFallback
	err := DB.Where("group_name = ? AND channel_type = ? AND enabled = ?", groupName, channelType, true).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("query group fallback failed: %w", err)
	}
	return &cfg, nil
}

// GetGroupChannelFallback 按“分组 + 渠道类型”查询配置（不区分启用状态）。
func GetGroupChannelFallback(groupName string, channelType int) (*GroupChannelFallback, error) {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" || channelType < 0 {
		return nil, nil
	}
	var cfg GroupChannelFallback
	err := DB.Where("group_name = ? AND channel_type = ?", groupName, channelType).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("query group fallback failed: %w", err)
	}
	return &cfg, nil
}

// GetGroupChannelFallbackByID 按主键查询兜底配置。
func GetGroupChannelFallbackByID(id int) (*GroupChannelFallback, error) {
	if id <= 0 {
		return nil, nil
	}
	var cfg GroupChannelFallback
	err := DB.Where("id = ?", id).First(&cfg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("query group fallback by id failed: %w", err)
	}
	return &cfg, nil
}

func GetAllGroupChannelFallbacks() ([]*GroupChannelFallback, error) {
	list := make([]*GroupChannelFallback, 0)
	err := DB.Order("group_name asc").Order("channel_type asc").Find(&list).Error
	return list, err
}

func DeleteGroupChannelFallback(groupName string, channelType int) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return errors.New("group_name is required")
	}
	if channelType < 0 {
		return errors.New("channel_type is invalid")
	}
	return DB.Where("group_name = ? AND channel_type = ?", groupName, channelType).Delete(&GroupChannelFallback{}).Error
}

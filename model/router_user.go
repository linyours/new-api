package model

import (
	"time"
)

// RouterUser 对应业务库表 router_user（与 new_user_id 关联 new-api 用户，用于预扣前校验 AGENT 侧额度）
type RouterUser struct {
	Id        int64      `gorm:"column:id;primaryKey;autoIncrement"`
	CreateTime *time.Time `gorm:"column:create_time"`
	UpdateTime *time.Time `gorm:"column:update_time"`
	AppId     *string    `gorm:"column:app_id"`
	Username  *string    `gorm:"column:username"`
	Role      *string    `gorm:"column:role"`
	Quota     *int64     `gorm:"column:quota"`
	UsedQuota *int64     `gorm:"column:used_quota"`
	NewUserId *int64     `gorm:"column:new_user_id;uniqueIndex:uk_router_user_new_user_id"`
	DeleteTime *time.Time `gorm:"column:delete_time"`
}

func (RouterUser) TableName() string {
	return "router_user"
}

// GetRouterUserByNewUserId 按 new_user_id 取有效行（已软删 delete_time 非空则忽略）
func GetRouterUserByNewUserId(newUserId int64) (*RouterUser, error) {
	var u RouterUser
	err := DB.Where("new_user_id = ? AND delete_time IS NULL", newUserId).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetAgentRouterUserByAppId 取该 app 下 role=AGENT 的有效行（无则 ErrRecordNotFound）
func GetAgentRouterUserByAppId(appId string) (*RouterUser, error) {
	var u RouterUser
	err := DB.Where("app_id = ? AND role = ? AND delete_time IS NULL", appId, "AGENT").First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

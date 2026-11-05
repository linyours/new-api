package model

import (
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	MaxStandbyPromoteThreshold = 100000
	MaxStandbyPromoteBatch     = 500
	maxStandbyPromoteRounds    = 10000
)

var standbyPromoteLock sync.Mutex

const standbyPoolSettingID = 1

// StandbyPoolSetting is the single promotion rule for every standby channel.
type StandbyPoolSetting struct {
	Id               int  `json:"-" gorm:"primaryKey"`
	PromoteEnabled   bool `json:"promote_enabled"`
	PromoteThreshold int  `json:"promote_threshold"`
	PromoteBatch     int  `json:"promote_batch"`
}

// StandbyPool is the one pool: one rule and the totals of every template-created channel.
type StandbyPool struct {
	PromoteEnabled   bool `json:"promote_enabled"`
	PromoteThreshold int  `json:"promote_threshold"`
	PromoteBatch     int  `json:"promote_batch"`
	EnabledCount     int  `json:"enabled_count"`
	StandbyCount     int  `json:"standby_count"`
	UnavailableCount int  `json:"unavailable_count"`
}

// NormalizeStandbyPromoteSettings checks a template's standby-pool rule.
// A disabled switch may keep a batch of zero. An enabled switch must promote
// at least one channel per trigger.
func NormalizeStandbyPromoteSettings(enabled bool, threshold, batch int) (bool, int, int, error) {
	if threshold < 0 || threshold > MaxStandbyPromoteThreshold {
		return false, 0, 0, fmt.Errorf("standby promote threshold must be between 0 and %d", MaxStandbyPromoteThreshold)
	}
	if batch < 0 || batch > MaxStandbyPromoteBatch {
		return false, 0, 0, fmt.Errorf("standby promote batch must be between 0 and %d", MaxStandbyPromoteBatch)
	}
	if enabled && batch < 1 {
		return false, 0, 0, errors.New("standby promote batch must be at least 1 when promotion is enabled")
	}
	return enabled, threshold, batch, nil
}

// GetStandbyPool returns the shared rule and counts across every channel
// created from a template.
func GetStandbyPool() (*StandbyPool, error) {
	setting, err := readStandbyPoolSetting()
	if err != nil {
		return nil, err
	}
	pool := standbyPoolFromSetting(setting)
	if err := attachStandbyPoolCounts(DB, &pool); err != nil {
		return nil, err
	}
	return &pool, nil
}

// SaveStandbyPoolSettings stores the one promotion rule, then fills the pool
// if the new rule says it is already below the line.
func SaveStandbyPoolSettings(enabled bool, threshold, batch int) (int, error) {
	enabled, threshold, batch, err := NormalizeStandbyPromoteSettings(enabled, threshold, batch)
	if err != nil {
		return 0, err
	}
	err = func() error {
		standbyPromoteLock.Lock()
		defer standbyPromoteLock.Unlock()
		return DB.Transaction(func(tx *gorm.DB) error {
			if _, err := loadStandbyPoolSetting(tx); err != nil {
				return err
			}
			return tx.Model(&StandbyPoolSetting{}).Where("id = ?", standbyPoolSettingID).Updates(map[string]any{
				"promote_enabled":   enabled,
				"promote_threshold": threshold,
				"promote_batch":     batch,
			}).Error
		})
	}()
	if err != nil {
		return 0, err
	}
	return PromoteStandbyPool()
}

// PromoteStandbyPool enables the oldest standby channels in the one pool until
// the total enabled count is above the watermark. Template is only how a
// channel was created; it does not split the pool.
func PromoteStandbyPool() (int, error) {
	standbyPromoteLock.Lock()
	defer standbyPromoteLock.Unlock()

	var promoted []Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		setting, err := loadStandbyPoolSetting(tx)
		if err != nil {
			return err
		}
		if !setting.PromoteEnabled || setting.PromoteBatch < 1 {
			return nil
		}
		threshold := setting.PromoteThreshold
		if threshold < 0 {
			threshold = 0
		}

		var enabled int64
		if err := tx.Model(&Channel{}).
			Where("template_id > 0 AND status = ?", common.ChannelStatusEnabled).
			Count(&enabled).Error; err != nil {
			return err
		}

		for round := 0; round < maxStandbyPromoteRounds && enabled <= int64(threshold); round++ {
			var standby []Channel
			if err := lockForUpdate(tx).
				Where("template_id > 0 AND status = ?", common.ChannelStatusStandby).
				Order("id ASC").
				Limit(setting.PromoteBatch).
				Find(&standby).Error; err != nil {
				return err
			}
			if len(standby) == 0 {
				common.SysLog(fmt.Sprintf(
					"standby pool empty: enabled=%d, threshold=%d",
					enabled, threshold,
				))
				return nil
			}

			ids := make([]int, 0, len(standby))
			for _, channel := range standby {
				if channel.Id > 0 {
					ids = append(ids, channel.Id)
				}
			}
			if len(ids) == 0 {
				return nil
			}
			result := tx.Model(&Channel{}).
				Where("id IN ? AND status = ?", ids, common.ChannelStatusStandby).
				Update("status", common.ChannelStatusEnabled)
			if result.Error != nil {
				return result.Error
			}
			if err := tx.Model(&Ability{}).
				Where("channel_id IN ?", ids).
				Update("enabled", true).Error; err != nil {
				return err
			}

			var fresh []Channel
			if err := tx.Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).
				Find(&fresh).Error; err != nil {
				return err
			}
			if len(fresh) == 0 {
				return errors.New("standby promotion updated no channels")
			}
			promoted = append(promoted, fresh...)
			enabled += int64(len(fresh))
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if len(promoted) > 0 {
		common.SysLog(fmt.Sprintf("standby pool promoted: count=%d", len(promoted)))
		cacheEnableChannels(promoted)
	}
	return len(promoted), nil
}

func standbyPoolFromSetting(setting StandbyPoolSetting) StandbyPool {
	return StandbyPool{
		PromoteEnabled:   setting.PromoteEnabled,
		PromoteThreshold: setting.PromoteThreshold,
		PromoteBatch:     setting.PromoteBatch,
	}
}

func readStandbyPoolSetting() (StandbyPoolSetting, error) {
	var setting StandbyPoolSetting
	err := DB.First(&setting, standbyPoolSettingID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return StandbyPoolSetting{Id: standbyPoolSettingID}, nil
	}
	return setting, err
}

func loadStandbyPoolSetting(tx *gorm.DB) (StandbyPoolSetting, error) {
	var setting StandbyPoolSetting
	err := lockForUpdate(tx).First(&setting, standbyPoolSettingID).Error
	if err == nil {
		return setting, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return setting, err
	}
	setting = StandbyPoolSetting{Id: standbyPoolSettingID}
	if createErr := tx.Create(&setting).Error; createErr != nil {
		if readErr := lockForUpdate(tx).First(&setting, standbyPoolSettingID).Error; readErr != nil {
			return setting, createErr
		}
	}
	return setting, nil
}

type standbyPoolStatusCount struct {
	Status int   `gorm:"column:status"`
	Total  int64 `gorm:"column:total"`
}

// StandbyPoolChannel is one channel created from a template and kept in the
// single standby pool. It omits credentials.
type StandbyPoolChannel struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Type         int    `json:"type"`
	Status       int    `json:"status"`
	Models       string `json:"models"`
	Group        string `json:"group"`
	TemplateId   int    `json:"template_id"`
	TemplateName string `json:"template_name"`
}

type standbyPoolChannelRow struct {
	Id         int    `gorm:"column:id"`
	Name       string `gorm:"column:name"`
	Type       int    `gorm:"column:type"`
	Status     int    `gorm:"column:status"`
	Models     string `gorm:"column:models"`
	Group      string `gorm:"column:group"`
	TemplateId int    `gorm:"column:template_id"`
}

// ListStandbyPoolChannels returns channels created from any template, newest first.
func ListStandbyPoolChannels(offset, limit int) ([]StandbyPoolChannel, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var total int64
	if err := DB.Model(&Channel{}).Where("template_id > 0").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	groupCol := "`group`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		groupCol = `"group"`
	}
	var rows []standbyPoolChannelRow
	if err := DB.Model(&Channel{}).
		Select("id", "name", "type", "status", "models", groupCol, "template_id").
		Where("template_id > 0").
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	names := map[int]string{}
	if len(rows) > 0 {
		ids := make([]int, 0, len(rows))
		seen := map[int]struct{}{}
		for _, row := range rows {
			if row.TemplateId <= 0 {
				continue
			}
			if _, ok := seen[row.TemplateId]; ok {
				continue
			}
			seen[row.TemplateId] = struct{}{}
			ids = append(ids, row.TemplateId)
		}
		if len(ids) > 0 {
			var templates []ChannelTemplate
			if err := DB.Select("id", "name").Where("id IN ?", ids).Find(&templates).Error; err != nil {
				return nil, 0, err
			}
			for _, template := range templates {
				names[template.Id] = template.Name
			}
		}
	}

	items := make([]StandbyPoolChannel, 0, len(rows))
	for _, row := range rows {
		items = append(items, StandbyPoolChannel{
			Id:           row.Id,
			Name:         row.Name,
			Type:         row.Type,
			Status:       row.Status,
			Models:       row.Models,
			Group:        row.Group,
			TemplateId:   row.TemplateId,
			TemplateName: names[row.TemplateId],
		})
	}
	return items, total, nil
}

func attachStandbyPoolCounts(db *gorm.DB, pool *StandbyPool) error {
	var rows []standbyPoolStatusCount
	if err := db.Model(&Channel{}).
		Select("status, COUNT(*) as total").
		Where("template_id > 0").
		Group("status").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		switch row.Status {
		case common.ChannelStatusEnabled:
			pool.EnabledCount = int(row.Total)
		case common.ChannelStatusStandby:
			pool.StandbyCount = int(row.Total)
		case common.ChannelStatusManuallyDisabled, common.ChannelStatusAutoDisabled:
			pool.UnavailableCount += int(row.Total)
		}
	}
	return nil
}

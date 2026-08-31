package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ChannelCreatorUsageRow is one creator's aggregated lifetime channel usage.
type ChannelCreatorUsageRow struct {
	CreatedBy     int    `json:"created_by" gorm:"column:created_by"`
	Username      string `json:"username" gorm:"column:username"`
	ChannelCount  int64  `json:"channel_count" gorm:"column:channel_count"`
	EnabledCount  int64  `json:"enabled_count" gorm:"column:enabled_count"`
	DisabledCount int64  `json:"disabled_count" gorm:"column:disabled_count"`
	UsedQuota     int64  `json:"used_quota" gorm:"column:used_quota"`
}

// ChannelCreatorUsageSummaryTotals is page-level rollup across all creators.
type ChannelCreatorUsageSummaryTotals struct {
	TotalUsedQuota         int64 `json:"total_used_quota"`
	TotalChannels          int64 `json:"total_channels"`
	UnknownCreatorChannels int64 `json:"unknown_creator_channels"`
	CreatorCount           int64 `json:"creator_count"`
}

// ChannelCreatorUsageChannel is a compact channel row for creator detail views.
type ChannelCreatorUsageChannel struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	Status      int    `json:"status"`
	Group       string `json:"group"`
	UsedQuota   int64  `json:"used_quota"`
	CreatedTime int64  `json:"created_time"`
	CreatedBy   int    `json:"created_by"`
}

func channelCreatorUsageBaseQuery() *gorm.DB {
	return DB.Table("channels AS c").
		Select(`c.created_by AS created_by,
			COALESCE(u.username, '') AS username,
			COUNT(*) AS channel_count,
			SUM(CASE WHEN c.status = ? THEN 1 ELSE 0 END) AS enabled_count,
			SUM(CASE WHEN c.status <> ? THEN 1 ELSE 0 END) AS disabled_count,
			COALESCE(SUM(c.used_quota), 0) AS used_quota`,
			common.ChannelStatusEnabled,
			common.ChannelStatusEnabled,
		).
		Joins("LEFT JOIN users AS u ON u.id = c.created_by").
		Group("c.created_by, u.username")
}

// ListChannelCreatorUsage returns every creator aggregate, highest spend first.
func ListChannelCreatorUsage() ([]ChannelCreatorUsageRow, error) {
	rows := make([]ChannelCreatorUsageRow, 0)
	err := channelCreatorUsageBaseQuery().
		Order("used_quota DESC").
		Scan(&rows).Error
	return rows, err
}

// FilterChannelCreatorUsage applies optional username / id keyword filtering.
func FilterChannelCreatorUsage(rows []ChannelCreatorUsageRow, keyword string) []ChannelCreatorUsageRow {
	keyword = strings.TrimSpace(strings.ToLower(keyword))
	if keyword == "" {
		return rows
	}
	filtered := make([]ChannelCreatorUsageRow, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Username), keyword) {
			filtered = append(filtered, row)
			continue
		}
		if strconv.Itoa(row.CreatedBy) == keyword {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// SummarizeChannelCreatorUsage computes totals for the (optionally filtered) rows.
func SummarizeChannelCreatorUsage(rows []ChannelCreatorUsageRow) ChannelCreatorUsageSummaryTotals {
	var totals ChannelCreatorUsageSummaryTotals
	totals.CreatorCount = int64(len(rows))
	for _, row := range rows {
		totals.TotalUsedQuota += row.UsedQuota
		totals.TotalChannels += row.ChannelCount
		if row.CreatedBy == 0 {
			totals.UnknownCreatorChannels += row.ChannelCount
		}
	}
	return totals
}

// PaginateChannelCreatorUsage slices rows for page/pageSize (1-based page).
func PaginateChannelCreatorUsage(rows []ChannelCreatorUsageRow, page, pageSize int) []ChannelCreatorUsageRow {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	start := (page - 1) * pageSize
	if start >= len(rows) {
		return []ChannelCreatorUsageRow{}
	}
	end := start + pageSize
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

// ListChannelsByCreator returns all channels created by the given user id (0 = unknown).
func ListChannelsByCreator(createdBy int) ([]ChannelCreatorUsageChannel, error) {
	channels := make([]Channel, 0)
	err := DB.Model(&Channel{}).
		Omit("key").
		Where("created_by = ?", createdBy).
		Order("used_quota DESC, id ASC").
		Find(&channels).Error
	if err != nil {
		return nil, err
	}
	rows := make([]ChannelCreatorUsageChannel, 0, len(channels))
	for _, channel := range channels {
		rows = append(rows, ChannelCreatorUsageChannel{
			Id:          channel.Id,
			Name:        channel.Name,
			Type:        channel.Type,
			Status:      channel.Status,
			Group:       channel.Group,
			UsedQuota:   channel.UsedQuota,
			CreatedTime: channel.CreatedTime,
			CreatedBy:   channel.CreatedBy,
		})
	}
	return rows, nil
}

// GetChannelCreatorUsername resolves a display username for a creator id.
func GetChannelCreatorUsername(createdBy int) (string, error) {
	if createdBy <= 0 {
		return "", nil
	}
	var username string
	err := DB.Model(&User{}).Select("username").Where("id = ?", createdBy).Limit(1).Pluck("username", &username).Error
	if err != nil {
		return "", err
	}
	return username, nil
}

package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

type channelCreatorUsageItem struct {
	CreatedBy     int     `json:"created_by"`
	Username      string  `json:"username"`
	ChannelCount  int64   `json:"channel_count"`
	EnabledCount  int64   `json:"enabled_count"`
	DisabledCount int64   `json:"disabled_count"`
	UsedQuota     int64   `json:"used_quota"`
	QuotaRatio    float64 `json:"quota_ratio"`
}

type channelCreatorUsageChannelItem struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Type        int    `json:"type"`
	TypeName    string `json:"type_name"`
	Status      int    `json:"status"`
	Group       string `json:"group"`
	UsedQuota   int64  `json:"used_quota"`
	CreatedTime int64  `json:"created_time"`
	CreatedBy   int    `json:"created_by"`
}

// GetChannelCreatorUsage returns lifetime used_quota aggregates keyed by channel creator.
func GetChannelCreatorUsage(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	page, _ := strconv.Atoi(c.DefaultQuery("p", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}

	rows, err := model.ListChannelCreatorUsage()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	filtered := model.FilterChannelCreatorUsage(rows, keyword)
	totals := model.SummarizeChannelCreatorUsage(filtered)
	pageRows := model.PaginateChannelCreatorUsage(filtered, page, pageSize)

	items := make([]channelCreatorUsageItem, 0, len(pageRows))
	for _, row := range pageRows {
		ratio := 0.0
		if totals.TotalUsedQuota > 0 {
			ratio = float64(row.UsedQuota) / float64(totals.TotalUsedQuota)
		}
		items = append(items, channelCreatorUsageItem{
			CreatedBy:     row.CreatedBy,
			Username:      row.Username,
			ChannelCount:  row.ChannelCount,
			EnabledCount:  row.EnabledCount,
			DisabledCount: row.DisabledCount,
			UsedQuota:     row.UsedQuota,
			QuotaRatio:    ratio,
		})
	}

	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     len(filtered),
		"page":      page,
		"page_size": pageSize,
		"summary":   totals,
	})
}

// GetChannelCreatorUsageChannels returns every channel created by a user for the detail dialog.
func GetChannelCreatorUsageChannels(c *gin.Context) {
	createdBy, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || createdBy < 0 {
		common.ApiErrorMsg(c, "invalid user_id")
		return
	}

	channels, err := model.ListChannelsByCreator(createdBy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	username, err := model.GetChannelCreatorUsername(createdBy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var totalQuota int64
	items := make([]channelCreatorUsageChannelItem, 0, len(channels))
	for _, channel := range channels {
		totalQuota += channel.UsedQuota
		items = append(items, channelCreatorUsageChannelItem{
			Id:          channel.Id,
			Name:        channel.Name,
			Type:        channel.Type,
			TypeName:    constant.GetChannelTypeName(channel.Type),
			Status:      channel.Status,
			Group:       channel.Group,
			UsedQuota:   channel.UsedQuota,
			CreatedTime: channel.CreatedTime,
			CreatedBy:   channel.CreatedBy,
		})
	}

	common.ApiSuccess(c, gin.H{
		"created_by":    createdBy,
		"username":      username,
		"channel_count": len(items),
		"used_quota":    totalQuota,
		"channels":      items,
	})
}

// ExportChannelCreatorUsage exports a creator's channel usage as an .xlsx workbook.
func ExportChannelCreatorUsage(c *gin.Context) {
	createdBy, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || createdBy < 0 {
		common.ApiErrorMsg(c, "invalid user_id")
		return
	}

	channels, err := model.ListChannelsByCreator(createdBy)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	username, err := model.GetChannelCreatorUsername(createdBy)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	file, err := buildChannelCreatorUsageWorkbook(createdBy, username, c.GetString("username"), channels)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer func() { _ = file.Close() }()

	displayName := username
	if displayName == "" {
		if createdBy == 0 {
			displayName = "unknown"
		} else {
			displayName = fmt.Sprintf("user-%d", createdBy)
		}
	}
	filename := fmt.Sprintf(
		"channel-creator-usage-%s-%s.xlsx",
		sanitizeFilename(displayName),
		time.Now().Format("20060102-150405"),
	)

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Cache-Control", "no-store")
	if err := file.Write(c.Writer); err != nil {
		common.SysError("failed to write channel creator usage xlsx: " + err.Error())
		c.Status(http.StatusInternalServerError)
	}
}

func buildChannelCreatorUsageWorkbook(
	createdBy int,
	creatorUsername string,
	exportedBy string,
	channels []model.ChannelCreatorUsageChannel,
) (*excelize.File, error) {
	file := excelize.NewFile()
	channelsSheet := "Channels"
	summarySheet := "Summary"
	if err := file.SetSheetName("Sheet1", channelsSheet); err != nil {
		return nil, err
	}
	if _, err := file.NewSheet(summarySheet); err != nil {
		return nil, err
	}

	channelHeaders := []string{
		"Channel ID", "Channel Name", "Type", "Type Name", "Status", "Group",
		"Used Quota", "Amount (USD)", "Created At", "Created By ID", "Created By",
	}
	for i, header := range channelHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := file.SetCellValue(channelsSheet, cell, header); err != nil {
			return nil, err
		}
	}

	var totalQuota int64
	quotaPerUnit := common.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = 500000
	}
	for idx, channel := range channels {
		totalQuota += channel.UsedQuota
		row := idx + 2
		values := []any{
			channel.Id,
			channel.Name,
			channel.Type,
			constant.GetChannelTypeName(channel.Type),
			channelStatusLabel(channel.Status),
			channel.Group,
			channel.UsedQuota,
			float64(channel.UsedQuota) / quotaPerUnit,
			formatUnixTime(channel.CreatedTime),
			createdBy,
			creatorUsername,
		}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row)
			if err := file.SetCellValue(channelsSheet, cell, value); err != nil {
				return nil, err
			}
		}
	}

	summaryRows := [][]any{
		{"Creator ID", createdBy},
		{"Creator Username", creatorUsername},
		{"Channel Count", len(channels)},
		{"Total Used Quota", totalQuota},
		{"Total Amount (USD)", float64(totalQuota) / quotaPerUnit},
		{"Exported At", time.Now().Format(time.RFC3339)},
		{"Exported By", exportedBy},
	}
	for i, row := range summaryRows {
		if err := file.SetCellValue(summarySheet, fmt.Sprintf("A%d", i+1), row[0]); err != nil {
			return nil, err
		}
		if err := file.SetCellValue(summarySheet, fmt.Sprintf("B%d", i+1), row[1]); err != nil {
			return nil, err
		}
	}

	file.SetActiveSheet(0)
	return file, nil
}

func channelStatusLabel(status int) string {
	switch status {
	case common.ChannelStatusEnabled:
		return "enabled"
	case common.ChannelStatusManuallyDisabled:
		return "manually_disabled"
	case common.ChannelStatusAutoDisabled:
		return "auto_disabled"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}

func formatUnixTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "export"
	}
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", " ", "_",
	)
	return replacer.Replace(name)
}

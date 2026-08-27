package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type globalArchivedKeyStatus struct {
	ArchiveId      int64  `json:"archive_id"`
	OriginalKeyId  int64  `json:"original_key_id"`
	ChannelId      int    `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	Index          int    `json:"index"`
	KeyPreview     string `json:"key_preview"`
	RpmLimit       *int   `json:"rpm_limit"`
	ModelRpmLimits model.ChannelKeyModelRpmLimits `json:"model_rpm_limits"`
	QuotaLimit     *int64 `json:"quota_limit"`
	EffectiveQuota int64  `json:"effective_quota"`
	QuotaUsed      int64  `json:"quota_used"`
	LifetimeQuota  int64  `json:"lifetime_quota"`
	Reason         string `json:"reason"`
	ExhaustedTime  int64  `json:"exhausted_time"`
	ArchivedTime   int64  `json:"archived_time"`
}

// GetGlobalChannelKeyArchives returns the active exhausted-key library across
// all channels. Credentials and fingerprints never leave the model layer; the
// response contains only a masked preview.
func GetGlobalChannelKeyArchives(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}

	var channelId *int
	if value := c.Query("channel_id"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel_id"})
			return
		}
		channelId = &parsed
	}

	archives, total, err := model.GetChannelKeyArchivesGlobal(
		channelId,
		(page-1)*pageSize,
		pageSize,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	channelIds := make([]int, 0, len(archives))
	for _, archive := range archives {
		channelIds = append(channelIds, archive.ChannelId)
	}
	var channels []model.Channel
	if len(channelIds) > 0 {
		if err := model.DB.Select("id", "name").Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}
	channelNames := make(map[int]string, len(channels))
	for _, channel := range channels {
		channelNames[channel.Id] = channel.Name
	}

	keys := make([]globalArchivedKeyStatus, 0, len(archives))
	for _, archive := range archives {
		keys = append(keys, globalArchivedKeyStatus{
			ArchiveId:      archive.Id,
			OriginalKeyId:  archive.OriginalKeyId,
			ChannelId:      archive.ChannelId,
			ChannelName:    channelNames[archive.ChannelId],
			Index:          archive.Position,
			KeyPreview:     archive.Preview(),
			RpmLimit:       archive.RpmLimit,
			ModelRpmLimits: archive.ModelRpmLimits,
			QuotaLimit:     archive.QuotaLimit,
			EffectiveQuota: archive.EffectiveQuotaLimit,
			QuotaUsed:      archive.QuotaUsed,
			LifetimeQuota:  archive.LifetimeQuota,
			Reason:         archive.Reason,
			ExhaustedTime:  archive.ExhaustedAt,
			ArchivedTime:   archive.ArchivedAt,
		})
	}

	totalPages := (int(total) + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"keys":        keys,
			"total":       int(total),
			"page":        page,
			"page_size":   pageSize,
			"total_pages": totalPages,
		},
	})
}

func RestoreGlobalChannelKeyArchive(c *gin.Context) {
	archiveId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || archiveId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid archive id"})
		return
	}
	if err := model.RestoreChannelKeyArchiveById(archiveId); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.key_archive_restore", map[string]interface{}{
		"archive_id": archiveId,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "密钥已从耗尽密钥库恢复"})
}

func GetGlobalChannelKeyArchiveSecret(c *gin.Context) {
	archiveId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || archiveId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid archive id"})
		return
	}
	key, err := model.GetChannelKeyArchiveSecret(archiveId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.key_archive_secret_view", map[string]interface{}{
		"archive_id": archiveId,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "获取成功",
		"data":    gin.H{"key": key},
	})
}

func DeleteGlobalChannelKeyArchive(c *gin.Context) {
	archiveId, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || archiveId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid archive id"})
		return
	}
	if err := model.DeleteChannelKeyArchive(archiveId); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.key_archive_delete", map[string]interface{}{
		"archive_id": archiveId,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "密钥已永久删除"})
}

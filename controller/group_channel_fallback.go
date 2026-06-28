package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type UpsertGroupChannelFallbackRequest struct {
	Id                int    `json:"id"`
	GroupName         string `json:"group_name"`
	ChannelType       int    `json:"channel_type"`
	FallbackChannelId int    `json:"fallback_channel_id"`
	Enabled           *bool  `json:"enabled"`
	Remark            string `json:"remark"`
	// Deprecated: 仅为兼容旧前端保留，后端不再信任此字段判断编辑态。
	IsEdit *bool `json:"is_edit"`
}

type DeleteGroupChannelFallbackRequest struct {
	GroupName   string `json:"group_name"`
	ChannelType int    `json:"channel_type"`
}

// GetGroupChannelFallbacks 查询所有分组兜底配置。
func GetGroupChannelFallbacks(c *gin.Context) {
	list, err := model.GetAllGroupChannelFallbacks()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    list,
	})
}

// UpsertGroupChannelFallback 新增或更新“分组+渠道类型 -> 兜底渠道”配置。
func UpsertGroupChannelFallback(c *gin.Context) {
	var req UpsertGroupChannelFallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	req.Remark = strings.TrimSpace(req.Remark)

	targetGroup := req.GroupName
	targetChannelType := req.ChannelType

	// 仅信任 id 判定编辑态：id>0 视为编辑，id<=0 视为新增。
	if req.Id > 0 {
		existingByID, err := model.GetGroupChannelFallbackByID(req.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if existingByID == nil {
			common.ApiErrorMsg(c, "要编辑的兜底配置不存在")
			return
		}
		// 稳态策略：编辑不允许修改唯一维度（group_name + channel_type），避免语义混乱。
		if req.GroupName != existingByID.GroupName || req.ChannelType != existingByID.ChannelType {
			common.ApiErrorMsg(c, "编辑不允许修改分组或渠道类型，请删除后重新新增")
			return
		}
		targetGroup = existingByID.GroupName
		targetChannelType = existingByID.ChannelType
	} else {
		// 新增场景下，禁止同“分组 + 渠道类型”重复添加。
		existed, err := model.GetGroupChannelFallback(req.GroupName, req.ChannelType)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if existed != nil {
			common.ApiErrorMsg(c, "当前分组+渠道类型已存在兜底配置，请勿重复添加")
			return
		}
	}

	if err := service.ValidateGroupFallbackBinding(targetGroup, targetChannelType, req.FallbackChannelId); err != nil {
		common.ApiError(c, err)
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	cfg := &model.GroupChannelFallback{
		Id:                req.Id,
		GroupName:         targetGroup,
		ChannelType:       targetChannelType,
		FallbackChannelId: req.FallbackChannelId,
		Enabled:           enabled,
		Remark:            req.Remark,
	}
	if err := model.UpsertGroupChannelFallback(cfg); err != nil {
		common.ApiError(c, err)
		return
	}
	// 写成功后精确失效缓存，保证后续读请求立即看到最新配置。
	service.InvalidateGroupFallbackCache(targetGroup, targetChannelType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteGroupChannelFallback 删除兜底配置。
//
// 为了兼容不同客户端，这里同时支持：
// 1) JSON body: {group_name, channel_type}
// 2) query: ?group_name=xxx&channel_type=14
func DeleteGroupChannelFallback(c *gin.Context) {
	var req DeleteGroupChannelFallbackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		// body 解析失败时尝试 query 参数，避免客户端未传 body 导致删除失败。
		req.GroupName = c.Query("group_name")
		if req.GroupName == "" {
			req.GroupName = c.Query("group")
		}
		channelTypeStr := c.Query("channel_type")
		if channelTypeStr != "" {
			if parsed, parseErr := strconv.Atoi(channelTypeStr); parseErr == nil {
				req.ChannelType = parsed
			}
		}
	}
	req.GroupName = strings.TrimSpace(req.GroupName)
	if err := model.DeleteGroupChannelFallback(req.GroupName, req.ChannelType); err != nil {
		common.ApiError(c, err)
		return
	}
	// 删除成功后精确失效缓存，避免短时间内读到旧配置。
	service.InvalidateGroupFallbackCache(req.GroupName, req.ChannelType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

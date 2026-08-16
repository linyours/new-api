package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const supplierChannelMaxPageSize = 100

type supplierChannelCreateRequest struct {
	Name         string  `json:"name"`
	Type         int     `json:"type"`
	Key          string  `json:"key"`
	BaseURL      *string `json:"base_url"`
	Models       string  `json:"models"`
	Group        string  `json:"group"`
	ModelMapping *string `json:"model_mapping"`
	TestModel    *string `json:"test_model"`
	AutoBan      *int    `json:"auto_ban"`
}

type supplierChannelUpdateRequest struct {
	Name         *string `json:"name"`
	Type         *int    `json:"type"`
	Key          *string `json:"key"`
	BaseURL      *string `json:"base_url"`
	Models       *string `json:"models"`
	Group        *string `json:"group"`
	ModelMapping *string `json:"model_mapping"`
	TestModel    *string `json:"test_model"`
	AutoBan      *int    `json:"auto_ban"`
}

type supplierChannelResponse struct {
	ID           int     `json:"id"`
	OwnerUserID  int     `json:"owner_user_id"`
	Name         string  `json:"name"`
	Type         int     `json:"type"`
	BaseURL      *string `json:"base_url"`
	Models       string  `json:"models"`
	Group        string  `json:"group"`
	ModelMapping *string `json:"model_mapping"`
	TestModel    *string `json:"test_model"`
	AutoBan      *int    `json:"auto_ban"`
	Status       int     `json:"status"`
	CreatedTime  int64   `json:"created_time"`
}

func supplierChannelView(channel *model.Channel) supplierChannelResponse {
	return supplierChannelResponse{
		ID:           channel.Id,
		OwnerUserID:  channel.OwnerUserId,
		Name:         channel.Name,
		Type:         channel.Type,
		BaseURL:      channel.BaseURL,
		Models:       channel.Models,
		Group:        channel.Group,
		ModelMapping: channel.ModelMapping,
		TestModel:    channel.TestModel,
		AutoBan:      channel.AutoBan,
		Status:       channel.Status,
		CreatedTime:  channel.CreatedTime,
	}
}

// ListSupplierChannels lists only channels owned by X-Owner-User-Id.
func ListSupplierChannels(c *gin.Context) {
	ownerUserId := middleware.GetSupplierOwnerUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > supplierChannelMaxPageSize {
		pageSize = supplierChannelMaxPageSize
	}

	channels, total, err := model.GetChannelsByOwner(ownerUserId, (page-1)*pageSize, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	items := make([]supplierChannelResponse, 0, len(channels))
	for _, channel := range channels {
		items = append(items, supplierChannelView(channel))
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}

// GetSupplierChannel returns one channel owned by X-Owner-User-Id.
func GetSupplierChannel(c *gin.Context) {
	channel, ok := loadSupplierOwnedChannel(c, false)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    supplierChannelView(channel),
	})
}

// CreateSupplierChannel creates a single supplier-owned channel.
func CreateSupplierChannel(c *gin.Context) {
	var req supplierChannelCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" ||
		req.Type <= 0 ||
		strings.TrimSpace(req.Key) == "" ||
		strings.TrimSpace(req.Models) == "" ||
		strings.TrimSpace(req.Group) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name, type, key, models and group are required"})
		return
	}
	autoBan := req.AutoBan
	if autoBan == nil {
		defaultAutoBan := 1
		autoBan = &defaultAutoBan
	}
	channel := model.Channel{
		OwnerUserId:  middleware.GetSupplierOwnerUserID(c),
		Name:         strings.TrimSpace(req.Name),
		Type:         req.Type,
		Key:          strings.TrimSpace(req.Key),
		BaseURL:      req.BaseURL,
		Models:       strings.TrimSpace(req.Models),
		Group:        strings.TrimSpace(req.Group),
		ModelMapping: req.ModelMapping,
		TestModel:    req.TestModel,
		AutoBan:      autoBan,
		Status:       common.ChannelStatusEnabled,
		CreatedTime:  common.GetTimestamp(),
	}
	if err := validateChannel(&channel, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	channels := []model.Channel{channel}
	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "",
		"data":    supplierChannelView(&channels[0]),
	})
}

// UpdateSupplierChannel updates only explicitly whitelisted fields.
func UpdateSupplierChannel(c *gin.Context) {
	channel, ok := loadSupplierOwnedChannel(c, true)
	if !ok {
		return
	}
	var req supplierChannelUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "name cannot be empty"})
			return
		}
		channel.Name = strings.TrimSpace(*req.Name)
	}
	if req.Type != nil {
		if *req.Type <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "type must be greater than 0"})
			return
		}
		channel.Type = *req.Type
	}
	if req.Key != nil {
		if strings.TrimSpace(*req.Key) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "key cannot be empty"})
			return
		}
		channel.Key = strings.TrimSpace(*req.Key)
	}
	if req.BaseURL != nil {
		channel.BaseURL = req.BaseURL
	}
	if req.Models != nil {
		if strings.TrimSpace(*req.Models) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "models cannot be empty"})
			return
		}
		channel.Models = strings.TrimSpace(*req.Models)
	}
	if req.Group != nil {
		if strings.TrimSpace(*req.Group) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "group cannot be empty"})
			return
		}
		channel.Group = strings.TrimSpace(*req.Group)
	}
	if req.ModelMapping != nil {
		channel.ModelMapping = req.ModelMapping
	}
	if req.TestModel != nil {
		channel.TestModel = req.TestModel
	}
	if req.AutoBan != nil {
		channel.AutoBan = req.AutoBan
	}
	if err := validateChannel(channel, false); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.UpdateSupplierOwnedChannel(channel, middleware.GetSupplierOwnerUserID(c)); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	channel.Key = ""
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    supplierChannelView(channel),
	})
}

// UpdateSupplierChannelStatus enables or manually disables an owned channel.
func UpdateSupplierChannelStatus(c *gin.Context) {
	channel, ok := loadSupplierOwnedChannel(c, false)
	if !ok {
		return
	}
	var req ChannelStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil || !isManageableChannelStatus(req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid status"})
		return
	}
	changed, err := model.UpdateSupplierOwnedChannelStatus(
		channel.Id,
		middleware.GetSupplierOwnerUserID(c),
		req.Status,
		"supplier internal operation",
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if changed {
		model.InitChannelCache()
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":      channel.Id,
			"status":  req.Status,
			"changed": changed,
		},
	})
}

func loadSupplierOwnedChannel(c *gin.Context, selectAll bool) (*model.Channel, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel id"})
		return nil, false
	}
	channel, err := model.GetChannelByIdAndOwner(id, middleware.GetSupplierOwnerUserID(c), selectAll)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return nil, false
	}
	return channel, true
}

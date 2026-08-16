package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type channelCostPriceRequest struct {
	// CostPrice sets the channel multiplier. null clears (unset).
	CostPrice *float64 `json:"cost_price"`
}

type channelCostPriceBatchRequest struct {
	Ids       []int    `json:"ids"`
	CostPrice *float64 `json:"cost_price"`
}

// GetChannelCostPrice returns a channel's cost_price.
// GET /api/v1/channel/:id/cost-price
func GetChannelCostPrice(c *gin.Context) {
	channel, ok := loadChannelByParam(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": channel.Id,
			"cost_price": channel.CostPrice,
		},
	})
}

// UpdateChannelCostPrice sets or clears a channel's cost_price.
// PUT /api/v1/channel/:id/cost-price
func UpdateChannelCostPrice(c *gin.Context) {
	channel, ok := loadChannelByParam(c)
	if !ok {
		return
	}
	var req channelCostPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if req.CostPrice != nil && *req.CostPrice < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "cost_price must be >= 0",
		})
		return
	}
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).UpdateColumn("cost_price", req.CostPrice).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	channel.CostPrice = req.CostPrice
	model.InitChannelCache()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": channel.Id,
			"cost_price": channel.CostPrice,
		},
	})
}

// BatchUpdateChannelCostPrice sets or clears cost_price for multiple channels.
// PUT /api/v1/channel/cost-price/batch
func BatchUpdateChannelCostPrice(c *gin.Context) {
	var req channelCostPriceBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(req.Ids) > maxV1BatchIDs {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": maxV1BatchIDs})
		return
	}
	if req.CostPrice != nil && *req.CostPrice < 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "cost_price must be >= 0",
		})
		return
	}

	updated := make([]int, 0, len(req.Ids))
	failed := make([]v1BatchFailItem, 0)
	seen := make(map[int]struct{}, len(req.Ids))
	anyUpdated := false

	for _, id := range req.Ids {
		if id <= 0 {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: "invalid id"})
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		channel, err := model.GetChannelById(id, true)
		if err != nil || channel == nil {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: "not found"})
			continue
		}
		if err := model.DB.Model(&model.Channel{}).Where("id = ?", id).UpdateColumn("cost_price", req.CostPrice).Error; err != nil {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: err.Error()})
			continue
		}
		updated = append(updated, id)
		anyUpdated = true
	}

	if anyUpdated {
		model.InitChannelCache()
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"updated":    updated,
			"failed":     failed,
			"count":      len(updated),
			"cost_price": req.CostPrice,
		},
	})
}

func loadChannelByParam(c *gin.Context) (*model.Channel, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return nil, false
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgChannelNotExists)
		return nil, false
	}
	return channel, true
}

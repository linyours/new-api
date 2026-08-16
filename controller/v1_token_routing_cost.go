package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	chselector "github.com/QuantumNous/new-api/pkg/channel_selector"
	"github.com/gin-gonic/gin"
)

const maxV1BatchIDs = 100

type tokenRoutingMaxCostPriceRequest struct {
	// RoutingMaxCostPriceByType replaces the token caps. Empty object / null clears.
	RoutingMaxCostPriceByType map[string]float64 `json:"routing_max_cost_price_by_type"`
}

type tokenRoutingMaxCostPriceBatchRequest struct {
	Ids                       []int              `json:"ids"`
	RoutingMaxCostPriceByType map[string]float64 `json:"routing_max_cost_price_by_type"`
}

type v1BatchFailItem struct {
	ID     int    `json:"id"`
	Reason string `json:"reason"`
}

// GetTokenRoutingMaxCostPrice returns the caller's own token routing cost caps.
// GET /api/v1/token/:id/routing-max-cost-price
func GetTokenRoutingMaxCostPrice(c *gin.Context) {
	token, ok := loadOwnedToken(c)
	if !ok {
		return
	}
	byType := token.GetRoutingMaxCostPriceByType()
	if byType == nil {
		byType = map[string]float64{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"token_id":                       token.Id,
			"routing_max_cost_price_by_type": byType,
		},
	})
}

// UpdateTokenRoutingMaxCostPrice replaces the caller's own token routing cost caps.
// PUT /api/v1/token/:id/routing-max-cost-price
func UpdateTokenRoutingMaxCostPrice(c *gin.Context) {
	token, ok := loadOwnedToken(c)
	if !ok {
		return
	}
	var req tokenRoutingMaxCostPriceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	normalized, err := chselector.NormalizeRoutingMaxCostPriceByType(req.RoutingMaxCostPriceByType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	if err := token.SetRoutingMaxCostPriceByType(normalized); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := token.UpdateRoutingMaxCostPriceByType(); err != nil {
		common.ApiError(c, err)
		return
	}
	out := token.GetRoutingMaxCostPriceByType()
	if out == nil {
		out = map[string]float64{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"token_id":                       token.Id,
			"routing_max_cost_price_by_type": out,
		},
	})
}

// BatchUpdateTokenRoutingMaxCostPrice applies the same caps to multiple owned tokens.
// PUT /api/v1/token/routing-max-cost-price/batch
func BatchUpdateTokenRoutingMaxCostPrice(c *gin.Context) {
	var req tokenRoutingMaxCostPriceBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(req.Ids) > maxV1BatchIDs {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": maxV1BatchIDs})
		return
	}
	normalized, err := chselector.NormalizeRoutingMaxCostPriceByType(req.RoutingMaxCostPriceByType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	userId := c.GetInt("id")
	updated := make([]int, 0, len(req.Ids))
	failed := make([]v1BatchFailItem, 0)
	seen := make(map[int]struct{}, len(req.Ids))

	for _, id := range req.Ids {
		if id <= 0 {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: "invalid id"})
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}

		token, err := model.GetTokenByIds(id, userId)
		if err != nil {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: "not found"})
			continue
		}
		if err := token.SetRoutingMaxCostPriceByType(normalized); err != nil {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: err.Error()})
			continue
		}
		if err := token.UpdateRoutingMaxCostPriceByType(); err != nil {
			failed = append(failed, v1BatchFailItem{ID: id, Reason: err.Error()})
			continue
		}
		updated = append(updated, id)
	}

	out := normalized
	if out == nil {
		out = map[string]float64{}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"updated":                        updated,
			"failed":                         failed,
			"count":                          len(updated),
			"routing_max_cost_price_by_type": out,
		},
	})
}

func loadOwnedToken(c *gin.Context) (*model.Token, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return nil, false
	}
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return token, true
}

package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

type updateStandbyPoolRequest struct {
	PromoteEnabled   bool `json:"promote_enabled"`
	PromoteThreshold int  `json:"promote_threshold"`
	PromoteBatch     int  `json:"promote_batch"`
}

func GetStandbyPoolChannels(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListStandbyPoolChannels(pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if items == nil {
		items = []model.StandbyPoolChannel{}
	}
	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      pageInfo.GetPage(),
		"page_size": pageInfo.GetPageSize(),
	})
}

func GetStandbyPool(c *gin.Context) {
	pool, err := model.GetStandbyPool()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, pool)
}

func UpdateStandbyPool(c *gin.Context) {
	var req updateStandbyPoolRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	promoted, err := model.SaveStandbyPoolSettings(req.PromoteEnabled, req.PromoteThreshold, req.PromoteBatch)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if promoted > 0 {
		model.InitChannelCache()
	}
	pool, err := model.GetStandbyPool()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "channel.standby_pool.update", map[string]interface{}{
		"promote_enabled":   pool.PromoteEnabled,
		"promote_threshold": pool.PromoteThreshold,
		"promote_batch":     pool.PromoteBatch,
		"promoted":          promoted,
	})
	common.ApiSuccess(c, pool)
}

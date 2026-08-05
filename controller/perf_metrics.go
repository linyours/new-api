package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto")
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}

func GetPerfMetricsLayered(c *gin.Context) {
	dim := perfmetrics.LayeredDimension(c.DefaultQuery("dimension", "all"))

	var (
		result perfmetrics.LayeredResult
		err    error
	)

	switch dim {
	case perfmetrics.LayeredDimAll:
		result, err = perfmetrics.QueryLayered(dim, 0)
	case perfmetrics.LayeredDimChannelType:
		parsed, parseErr := strconv.Atoi(c.Query("channel_type"))
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "channel_type is required",
			})
			return
		}
		result, err = perfmetrics.QueryLayered(dim, parsed)
	case perfmetrics.LayeredDimChannel:
		parsed, parseErr := strconv.Atoi(c.Query("channel_id"))
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "channel_id is required",
			})
			return
		}
		result, err = perfmetrics.QueryLayered(dim, parsed)
	case perfmetrics.LayeredDimModel:
		result, err = perfmetrics.QueryLayeredModel(c.Query("model"))
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "dimension must be all, channel_type, channel, or model",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetricsLayeredChannels(c *gin.Context) {
	ids, err := parsePositiveIntCSV(c.Query("ids"))
	if err != nil || len(ids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "ids is required (comma-separated channel ids)",
		})
		return
	}
	if len(ids) > perfmetrics.MaxLayeredChannelBatch {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "too many channel ids (max " + strconv.Itoa(perfmetrics.MaxLayeredChannelBatch) + ")",
		})
		return
	}

	windowSeconds := int64(3600)
	if raw := strings.TrimSpace(c.Query("window")); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "window must be one of 60,300,900,1800,3600",
			})
			return
		}
		windowSeconds = parsed
	}

	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	enabledIDs := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Status == common.ChannelStatusEnabled {
			enabledIDs = append(enabledIDs, channel.Id)
		}
	}

	result, err := perfmetrics.QueryLayeredChannels(enabledIDs, windowSeconds)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func parsePositiveIntCSV(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, strconv.ErrSyntax
	}
	parts := strings.Split(raw, ",")
	ids := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.Atoi(part)
		if err != nil {
			return nil, err
		}
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, strconv.ErrSyntax
	}
	return ids, nil
}

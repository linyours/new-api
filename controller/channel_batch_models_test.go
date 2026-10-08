package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchAddChannelModelsRejectsEmptySelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody, err := common.Marshal(ChannelBatchAddModelsRequest{
		Models: []string{"gpt-4o"},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/batch/models", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	BatchAddChannelModels(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "no channels selected", response.Message)
}

func TestBatchAddChannelModelRpmLimitsRejectsEmptyLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requestBody, err := common.Marshal(ChannelBatchAddModelRpmLimitsRequest{
		Ids:            []int{1},
		ModelRpmLimits: model.ChannelKeyModelRpmLimits{},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/batch/model_rpm_limits", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	BatchAddChannelModelRpmLimits(ctx)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	assert.Equal(t, "no model RPM limits to add", response.Message)
}

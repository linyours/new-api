package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelTemplateControllerTestDB(t *testing.T) *model.Channel {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelKey{},
		&model.ChannelKeyQuotaReservation{},
		&model.ChannelKeyEvent{},
		&model.ChannelKeyArchive{},
		&model.ChannelTemplate{},
	))
	model.InitChannelKeyCache(nil)
	t.Cleanup(func() { model.InitChannelKeyCache(nil) })

	origin := &model.Channel{
		Type:        constant.ChannelTypeOpenAI,
		Name:        "openai",
		Key:         "sk-origin-secret",
		Models:      "gpt-4o",
		Group:       "default",
		Status:      common.ChannelStatusEnabled,
		KeyRpmLimit: 15,
	}
	require.NoError(t, origin.Insert())
	key := model.CacheGetChannelKeys(origin.Id)[0]
	limits := model.ChannelKeyModelRpmLimits{"gpt-4o": 7}
	require.NoError(t, model.UpdateChannelKeyLimits(origin.Id, key.Id, nil, nil, &limits))
	return origin
}

func TestCreateAndApplyChannelTemplate(t *testing.T) {
	origin := setupChannelTemplateControllerTestDB(t)

	createRecorder := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRecorder)
	createCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	createBody, err := common.Marshal(map[string]any{
		"name":               "openai-prod",
		"name_suffix_length": 6,
	})
	require.NoError(t, err)
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/templates/from_channel", bytes.NewReader(createBody))
	createCtx.Request.Header.Set("Content-Type", "application/json")

	CreateChannelTemplateFromChannel(createCtx)

	var createResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Id     int    `json:"id"`
			Config string `json:"config"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &createResponse))
	require.True(t, createResponse.Success, createRecorder.Body.String())
	assert.NotContains(t, createResponse.Data.Config, "sk-origin-secret")

	applyBody, err := common.Marshal(map[string]any{
		"keys": "sk-aaaabcdef\nsk-bbbxyz123",
	})
	require.NoError(t, err)
	applyRecorder := httptest.NewRecorder()
	applyCtx, _ := gin.CreateTestContext(applyRecorder)
	applyCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", createResponse.Data.Id)}}
	applyCtx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/templates/apply", bytes.NewReader(applyBody))
	applyCtx.Request.Header.Set("Content-Type", "application/json")

	ApplyChannelTemplate(applyCtx)

	var applyResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Count    int `json:"count"`
			Channels []struct {
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"channels"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(applyRecorder.Body.Bytes(), &applyResponse))
	require.True(t, applyResponse.Success, applyRecorder.Body.String())
	require.Equal(t, 2, applyResponse.Data.Count)
	assert.Equal(t, "openai-prod-abcdef", applyResponse.Data.Channels[0].Name)
	assert.Equal(t, "openai-prod-xyz123", applyResponse.Data.Channels[1].Name)

	var stored model.Channel
	require.NoError(t, model.DB.First(&stored, applyResponse.Data.Channels[0].Id).Error)
	assert.Equal(t, "sk-aaaabcdef", stored.Key)
	assert.Equal(t, origin.Models, stored.Models)
	assert.Equal(t, origin.KeyRpmLimit, stored.KeyRpmLimit)
	assert.Empty(t, stored.UsedQuota)

	copiedKey := model.CacheGetChannelKeys(applyResponse.Data.Channels[0].Id)[0]
	assert.Equal(t, model.ChannelKeyModelRpmLimits{"gpt-4o": 7}, copiedKey.ModelRpmLimits)
}

func TestApplyChannelTemplateItemsOptionalProxy(t *testing.T) {
	origin := setupChannelTemplateControllerTestDB(t)
	origin.SetSetting(dto.ChannelSettings{Proxy: "socks5://template-proxy.example:1080"})
	require.NoError(t, origin.Update())

	createRecorder := httptest.NewRecorder()
	createCtx, _ := gin.CreateTestContext(createRecorder)
	createCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	createBody, err := common.Marshal(map[string]any{
		"name": "openai-proxy-tpl",
	})
	require.NoError(t, err)
	createCtx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/templates/from_channel", bytes.NewReader(createBody))
	createCtx.Request.Header.Set("Content-Type", "application/json")
	CreateChannelTemplateFromChannel(createCtx)

	var createResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Id int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(createRecorder.Body.Bytes(), &createResponse))
	require.True(t, createResponse.Success, createRecorder.Body.String())

	applyBody, err := common.Marshal(map[string]any{
		"items": []map[string]any{
			{"key": "sk-withproxyabcdef", "proxy": "socks5://user:pass@proxy1.example:1080"},
			{"key": "sk-noproxyxyz123", "proxy": ""},
		},
	})
	require.NoError(t, err)
	applyRecorder := httptest.NewRecorder()
	applyCtx, _ := gin.CreateTestContext(applyRecorder)
	applyCtx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", createResponse.Data.Id)}}
	applyCtx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/templates/apply", bytes.NewReader(applyBody))
	applyCtx.Request.Header.Set("Content-Type", "application/json")
	ApplyChannelTemplate(applyCtx)

	var applyResponse struct {
		Success bool `json:"success"`
		Data    struct {
			Count    int `json:"count"`
			Channels []struct {
				Id   int    `json:"id"`
				Name string `json:"name"`
			} `json:"channels"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(applyRecorder.Body.Bytes(), &applyResponse))
	require.True(t, applyResponse.Success, applyRecorder.Body.String())
	require.Equal(t, 2, applyResponse.Data.Count)

	var withProxy model.Channel
	require.NoError(t, model.DB.First(&withProxy, applyResponse.Data.Channels[0].Id).Error)
	assert.Equal(t, "socks5://user:pass@proxy1.example:1080", withProxy.GetSetting().Proxy)

	var withoutProxy model.Channel
	require.NoError(t, model.DB.First(&withoutProxy, applyResponse.Data.Channels[1].Id).Error)
	assert.Empty(t, withoutProxy.GetSetting().Proxy)
}

func TestApplyChannelTemplateDoesNotChangeExistingAddChannel(t *testing.T) {
	origin := setupChannelTemplateControllerTestDB(t)
	body, err := common.Marshal(AddChannelRequest{
		Mode: "batch",
		Channel: &model.Channel{
			Type:   constant.ChannelTypeOpenAI,
			Name:   "batch-plain",
			Key:    "sk-one\nsk-two",
			Models: "gpt-4o",
			Group:  "default",
			Status: common.ChannelStatusEnabled,
		},
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	AddChannel(ctx)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())

	var names []string
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id <> ?", origin.Id).Pluck("name", &names).Error)
	assert.ElementsMatch(t, []string{"batch-plain", "batch-plain"}, names)
}

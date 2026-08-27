package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReserveChannelKeyRPMLimitsEachKeyIndependently(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].Lock()
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
		localChannelKeyRPMShards[i].Unlock()
	}
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	limit := 2
	channel := &model.Channel{Id: 7}
	keys := []*model.ChannelKey{
		{Id: 101, ChannelId: channel.Id, RpmLimit: &limit, Status: model.ChannelKeyStatusEnabled},
		{Id: 102, ChannelId: channel.Id, RpmLimit: &limit, Status: model.ChannelKeyStatusEnabled},
	}

	first, firstIndex, err := reserveChannelKeyRPM(context.Background(), channel, keys, "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 0, firstIndex)

	second, secondIndex, err := reserveChannelKeyRPM(context.Background(), channel, keys, "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 0, secondIndex)

	third, thirdIndex, err := reserveChannelKeyRPM(context.Background(), channel, keys, "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, 1, thirdIndex)

	fourth, fourthIndex, err := reserveChannelKeyRPM(context.Background(), channel, keys[1:], "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, fourth)
	assert.Equal(t, 0, fourthIndex)

	exhausted, exhaustedIndex, err := reserveChannelKeyRPM(context.Background(), channel, keys, "gpt-4o")
	require.NoError(t, err)
	assert.Nil(t, exhausted)
	assert.Equal(t, -1, exhaustedIndex)
}

func TestReserveChannelKeyRPMCombinesDefaultAndModelLimits(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].Lock()
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
		localChannelKeyRPMShards[i].Unlock()
	}
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	defaultLimit := 5
	channel := &model.Channel{Id: 8}
	key := &model.ChannelKey{
		Id:        201,
		ChannelId: channel.Id,
		RpmLimit:  &defaultLimit,
		Status:    model.ChannelKeyStatusEnabled,
		ModelRpmLimits: model.ChannelKeyModelRpmLimits{
			"gpt-4o": 2,
		},
	}
	candidates := []*model.ChannelKey{key}

	for i := 0; i < 2; i++ {
		lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, 0, index)
	}
	lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
	require.NoError(t, err)
	assert.Nil(t, lease)
	assert.Equal(t, -1, index)

	for i := 0; i < 3; i++ {
		lease, index, err = reserveChannelKeyRPM(context.Background(), channel, candidates, "claude-3-5-sonnet")
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, 0, index)
	}
	lease, index, err = reserveChannelKeyRPM(context.Background(), channel, candidates, "gemini-2.5-pro")
	require.NoError(t, err)
	assert.Nil(t, lease)
	assert.Equal(t, -1, index)
}

func TestReserveChannelKeyRPMDefaultWindowBlocksConfiguredModel(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].Lock()
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
		localChannelKeyRPMShards[i].Unlock()
	}
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	defaultLimit := 2
	channel := &model.Channel{Id: 9}
	key := &model.ChannelKey{
		Id:        202,
		ChannelId: channel.Id,
		RpmLimit:  &defaultLimit,
		Status:    model.ChannelKeyStatusEnabled,
		ModelRpmLimits: model.ChannelKeyModelRpmLimits{
			"gpt-4o": 5,
		},
	}
	candidates := []*model.ChannelKey{key}

	for i := 0; i < 2; i++ {
		lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "claude-3-5-sonnet")
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, 0, index)
	}
	lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
	require.NoError(t, err)
	assert.Nil(t, lease)
	assert.Equal(t, -1, index)
}

func TestReserveChannelKeyRPMZeroModelCapStillUsesDefault(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].Lock()
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
		localChannelKeyRPMShards[i].Unlock()
	}
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	defaultLimit := 2
	channel := &model.Channel{Id: 10}
	key := &model.ChannelKey{
		Id:        203,
		ChannelId: channel.Id,
		RpmLimit:  &defaultLimit,
		Status:    model.ChannelKeyStatusEnabled,
		ModelRpmLimits: model.ChannelKeyModelRpmLimits{
			"gpt-4o": 0,
		},
	}
	candidates := []*model.ChannelKey{key}

	for i := 0; i < 2; i++ {
		lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
		require.NoError(t, err)
		require.NotNil(t, lease)
		assert.Equal(t, 0, index)
	}
	lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
	require.NoError(t, err)
	assert.Nil(t, lease)
	assert.Equal(t, -1, index)
}

func TestReleaseChannelKeyRPMRestoresDefaultAndModelWindows(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	for i := range localChannelKeyRPMShards {
		localChannelKeyRPMShards[i].Lock()
		localChannelKeyRPMShards[i].States = make(map[channelKeyRPMIdentity]localChannelKeyRPMState)
		localChannelKeyRPMShards[i].Unlock()
	}
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	defaultLimit := 1
	channel := &model.Channel{Id: 11}
	key := &model.ChannelKey{
		Id:        204,
		ChannelId: channel.Id,
		RpmLimit:  &defaultLimit,
		Status:    model.ChannelKeyStatusEnabled,
		ModelRpmLimits: model.ChannelKeyModelRpmLimits{
			"gpt-4o": 1,
		},
	}
	candidates := []*model.ChannelKey{key}

	lease, index, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "gpt-4o")
	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, 0, index)

	blocked, blockedIndex, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "claude-3-5-sonnet")
	require.NoError(t, err)
	assert.Nil(t, blocked)
	assert.Equal(t, -1, blockedIndex)

	releaseChannelKeyRPM(context.Background(), lease)
	restored, restoredIndex, err := reserveChannelKeyRPM(context.Background(), channel, candidates, "claude-3-5-sonnet")
	require.NoError(t, err)
	require.NotNil(t, restored)
	assert.Equal(t, 0, restoredIndex)
}

func TestRPMCapacityExhaustionReturnsResourceExhausted(t *testing.T) {
	apiErr := NewChannelKeyRPMExhaustedError()

	assert.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCodeChannelKeyRateLimited, apiErr.GetErrorCode())
	assert.Equal(t, "Resource exhausted. Please try again later...", apiErr.Error())
}

func TestRetryParamReportsRPMOnlyWhenEveryCapacityFailureIsRateLimited(t *testing.T) {
	param := &RetryParam{}
	param.RecordCapacityFailure(types.ErrorCodeChannelKeyRateLimited)
	param.RecordCapacityFailure(types.ErrorCodeChannelKeyRateLimited)
	assert.True(t, param.AllCapacityFailuresAreRPM())

	param.RecordCapacityFailure(types.ErrorCodeChannelKeyQuotaInsufficient)
	assert.False(t, param.AllCapacityFailuresAreRPM())
}

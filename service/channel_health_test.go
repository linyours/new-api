package service

import (
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyChannelHealthOutcome(t *testing.T) {
	total, success, bad := ClassifyChannelHealthOutcome(nil)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), success)
	assert.Equal(t, int64(0), bad)

	localErr := types.NewError(errors.New("quota"), types.ErrorCodeInsufficientUserQuota, types.ErrOptionWithNoRecordErrorLog())
	total, success, bad = ClassifyChannelHealthOutcome(localErr)
	assert.Equal(t, int64(0), total)
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(0), bad)

	badRequest := types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	total, success, bad = ClassifyChannelHealthOutcome(badRequest)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(0), bad)

	timeout := types.NewOpenAIError(errors.New("context deadline exceeded"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	total, success, bad = ClassifyChannelHealthOutcome(timeout)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(1), bad)

	upstream := types.NewOpenAIError(errors.New("upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	total, success, bad = ClassifyChannelHealthOutcome(upstream)
	assert.Equal(t, int64(1), bad)
	assert.Equal(t, int64(1), total)

	doRequestFailed := types.NewError(errors.New("upstream error: do request failed"), types.ErrorCodeDoRequestFailed)
	total, success, bad = ClassifyChannelHealthOutcome(doRequestFailed)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(1), bad)

	rateLimited := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	total, success, bad = ClassifyChannelHealthOutcome(rateLimited)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), success)
	assert.Equal(t, int64(0), bad)
}

func TestClassifyChannelHealthOutcomeUsesConfiguredStatusCodes(t *testing.T) {
	orig := operation_setting.ChannelHealthErrorStatusCodeRanges
	t.Cleanup(func() {
		operation_setting.ChannelHealthErrorStatusCodeRanges = orig
	})
	require.NoError(t, operation_setting.ChannelHealthErrorStatusCodesFromString("429"))

	rateLimited := types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests)
	total, success, bad := ClassifyChannelHealthOutcome(rateLimited)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), bad)
	assert.Equal(t, int64(0), success)

	clientErr := types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
	total, success, bad = ClassifyChannelHealthOutcome(clientErr)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), bad)
	assert.Equal(t, int64(0), success)

	require.NoError(t, operation_setting.ChannelHealthErrorStatusCodesFromString(""))
	upstream := types.NewOpenAIError(errors.New("upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	total, success, bad = ClassifyChannelHealthOutcome(upstream)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(0), bad)
	assert.Equal(t, int64(0), success)

	timeout := types.NewOpenAIError(errors.New("context deadline exceeded"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	total, success, bad = ClassifyChannelHealthOutcome(timeout)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, int64(1), bad)
	assert.Equal(t, int64(0), success)
}

func TestObserveChannelHealthIgnoresSmallSamplesAndAlertsOnce(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	var alerts atomic.Int32
	prevNotify := notifyChannelHealthFn
	notifyChannelHealthFn = func(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
		alerts.Add(1)
		assert.Equal(t, 7, channelID)
		assert.Equal(t, "unstable", channelName)
		assert.True(t, snap.SampleReady)
		assert.True(t, shouldAlertChannelHealth(snap))
		return true
	}
	t.Cleanup(func() { notifyChannelHealthFn = prevNotify })

	minTotal := operation_setting.GetChannelHealthMinTotal()
	for i := 0; i < minTotal-1; i++ {
		ObserveChannelHealth(7, "unstable", 1, 0, 1)
	}
	assert.Equal(t, int32(0), alerts.Load())
	snap := GetChannelHealthStats([]int{7})["7"]
	assert.False(t, snap.SampleReady)
	assert.Equal(t, int64(minTotal-1), snap.Bad)

	ObserveChannelHealth(7, "unstable", 1, 0, 1)
	require.Eventually(t, func() bool {
		return alerts.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	ObserveChannelHealth(7, "unstable", 1, 0, 1)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), alerts.Load())

	ready := GetChannelHealthStats([]int{7})["7"]
	assert.True(t, ready.SampleReady)
	assert.Equal(t, int64(minTotal+1), ready.Total)
}

func TestObserveChannelHealthAlertsWhenSampleBecomesReadyWithoutNewBad(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	origMin := operation_setting.ChannelHealthMinTotal
	origBelow := operation_setting.ChannelHealthAlertBelowPercent
	t.Cleanup(func() {
		operation_setting.ChannelHealthMinTotal = origMin
		operation_setting.ChannelHealthAlertBelowPercent = origBelow
	})
	operation_setting.ChannelHealthMinTotal = 10
	operation_setting.ChannelHealthAlertBelowPercent = 50

	var alerts atomic.Int32
	prevNotify := notifyChannelHealthFn
	notifyChannelHealthFn = func(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
		alerts.Add(1)
		assert.Equal(t, 8, channelID)
		assert.True(t, snap.SampleReady)
		assert.Greater(t, snap.Bad, int64(0))
		return true
	}
	t.Cleanup(func() { notifyChannelHealthFn = prevNotify })

	for i := 0; i < 9; i++ {
		ObserveChannelHealth(8, "openai", 1, 0, 1)
	}
	assert.Equal(t, int32(0), alerts.Load())

	ObserveChannelHealth(8, "openai", 1, 0, 0)
	require.Eventually(t, func() bool {
		return alerts.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	resetChannelHealthForTest()
	alerts.Store(0)
	for i := 0; i < 10; i++ {
		ObserveChannelHealth(8, "openai", 1, 0, 0)
	}
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), alerts.Load())
}

func TestFinalizeHealthSnapshotUsesConfiguredMinTotal(t *testing.T) {
	orig := operation_setting.ChannelHealthMinTotal
	t.Cleanup(func() { operation_setting.ChannelHealthMinTotal = orig })
	operation_setting.ChannelHealthMinTotal = 2

	snap := finalizeHealthSnapshot(channelHealthCounts{Total: 1, Success: 0, Bad: 1})
	assert.False(t, snap.SampleReady)
	assert.Equal(t, 2, snap.MinTotal)

	snap = finalizeHealthSnapshot(channelHealthCounts{Total: 2, Success: 1, Bad: 1})
	assert.True(t, snap.SampleReady)
	assert.Equal(t, 2, snap.MinTotal)
}

func TestShouldAlertChannelHealthUsesSuccessRateBelow(t *testing.T) {
	origMin := operation_setting.ChannelHealthMinTotal
	origBelow := operation_setting.ChannelHealthAlertBelowPercent
	t.Cleanup(func() {
		operation_setting.ChannelHealthMinTotal = origMin
		operation_setting.ChannelHealthAlertBelowPercent = origBelow
	})
	operation_setting.ChannelHealthMinTotal = 10
	operation_setting.ChannelHealthAlertBelowPercent = 50

	highSuccess := finalizeHealthSnapshot(channelHealthCounts{Total: 10, Success: 8, Bad: 2})
	assert.False(t, shouldAlertChannelHealth(highSuccess))

	equalLine := finalizeHealthSnapshot(channelHealthCounts{Total: 10, Success: 5, Bad: 5})
	assert.False(t, shouldAlertChannelHealth(equalLine))

	lowSuccess := finalizeHealthSnapshot(channelHealthCounts{Total: 10, Success: 4, Bad: 6})
	assert.True(t, shouldAlertChannelHealth(lowSuccess))
	assert.Equal(t, 0.5, lowSuccess.AlertBelow)
}

func TestMaybeAlertChannelHealthFromExistingSnapshot(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	origBelow := operation_setting.ChannelHealthAlertBelowPercent
	t.Cleanup(func() { operation_setting.ChannelHealthAlertBelowPercent = origBelow })
	operation_setting.ChannelHealthAlertBelowPercent = 50

	var alerts atomic.Int32
	prevNotify := notifyChannelHealthFn
	notifyChannelHealthFn = func(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
		alerts.Add(1)
		assert.Equal(t, 24, channelID)
		assert.Equal(t, "openai", channelName)
		return true
	}
	t.Cleanup(func() { notifyChannelHealthFn = prevNotify })

	snap := ChannelHealthSnapshot{
		Total:       5,
		Success:     0,
		Bad:         5,
		SuccessRate: 0,
		SampleReady: true,
		AlertBelow:  0.5,
	}
	MaybeAlertChannelHealth(24, "openai", snap)
	require.Eventually(t, func() bool {
		return alerts.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	MaybeAlertChannelHealth(24, "openai", snap)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), alerts.Load())
}

func TestSendDedicatedChannelHealthWebhookUsesConfiguredURL(t *testing.T) {
	origURL := operation_setting.ChannelHealthWebhookUrl
	origSecret := operation_setting.ChannelHealthWebhookSecret
	origSend := sendChannelHealthWebhookFn
	t.Cleanup(func() {
		operation_setting.ChannelHealthWebhookUrl = origURL
		operation_setting.ChannelHealthWebhookSecret = origSecret
		sendChannelHealthWebhookFn = origSend
	})

	var called int
	sendChannelHealthWebhookFn = func(webhookURL string, secret string, data dto.Notify) error {
		called++
		assert.Equal(t, "https://hooks.example.com/channel-health", webhookURL)
		assert.Equal(t, "s3cret", secret)
		assert.Equal(t, "channel_health_9", data.Type)
		assert.Equal(t, "渠道「bad」（#9）近5分钟错误率过高", data.Title)
		assert.Equal(t, "body", data.Content)
		return nil
	}

	sendDedicatedChannelHealthWebhook("channel_health_9", "title", "body")
	assert.Equal(t, 0, called)

	operation_setting.ChannelHealthWebhookUrl = "https://hooks.example.com/channel-health"
	operation_setting.ChannelHealthWebhookSecret = "s3cret"
	sendDedicatedChannelHealthWebhook(
		"channel_health_9",
		"渠道「bad」（#9）近5分钟错误率过高",
		"body",
	)
	assert.Equal(t, 1, called)
}

func TestGetChannelHealthStatsOnlyUsesRequestedIDs(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	ObserveChannelHealthSuccess(11, "a")
	ObserveChannelHealth(12, "b", 1, 0, 1)
	stats := GetChannelHealthStats([]int{11, 11, 0, 12})
	require.Len(t, stats, 2)
	assert.Equal(t, int64(1), stats["11"].Success)
	assert.Equal(t, int64(1), stats["12"].Bad)
	_, hasOther := stats["99"]
	assert.False(t, hasOther)
}

func TestMaybeAlertChannelHealthSkipsWhenNoClassifiedErrors(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	var alerts atomic.Int32
	prevNotify := notifyChannelHealthFn
	notifyChannelHealthFn = func(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
		alerts.Add(1)
		return true
	}
	t.Cleanup(func() { notifyChannelHealthFn = prevNotify })

	MaybeAlertChannelHealth(24, "openai", ChannelHealthSnapshot{
		Total:       5,
		Success:     0,
		Bad:         0,
		SuccessRate: 0,
		SampleReady: true,
		AlertBelow:  0.5,
	})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), alerts.Load())
}

func TestMaybeAlertChannelHealthReleasesLockWhenNotifyFails(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	var alerts atomic.Int32
	prevNotify := notifyChannelHealthFn
	notifyChannelHealthFn = func(channelID int, channelName string, snap ChannelHealthSnapshot) bool {
		alerts.Add(1)
		return false
	}
	t.Cleanup(func() { notifyChannelHealthFn = prevNotify })

	snap := ChannelHealthSnapshot{
		Total:       5,
		Success:     0,
		Bad:         5,
		SuccessRate: 0,
		SampleReady: true,
		AlertBelow:  0.5,
	}
	MaybeAlertChannelHealth(31, "openai", snap)
	require.Eventually(t, func() bool {
		return alerts.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		_, loaded := channelHealthAlertUntil.Load(31)
		return !loaded
	}, 2*time.Second, 10*time.Millisecond)

	MaybeAlertChannelHealth(31, "openai", snap)
	require.Eventually(t, func() bool {
		return alerts.Load() == 2
	}, 2*time.Second, 10*time.Millisecond)
}

func TestFormatChannelHealthAlertIncludesRecentErrors(t *testing.T) {
	origBelow := operation_setting.ChannelHealthAlertBelowPercent
	t.Cleanup(func() { operation_setting.ChannelHealthAlertBelowPercent = origBelow })
	operation_setting.ChannelHealthAlertBelowPercent = 50

	snap := ChannelHealthSnapshot{SuccessRate: 0, Success: 0, Total: 3, Bad: 3, SampleReady: true}
	subject, content := formatChannelHealthAlert("模版-123", 24, snap, []*model.Log{
		{
			CreatedAt: 1757268420,
			ModelName: "gpt-4",
			Content:   "upstream error: do request failed",
			Other:     `{"status_code":500}`,
		},
	})
	assert.Equal(t, "渠道「模版-123」（#24）近5分钟成功率过低", subject)
	assert.Contains(t, content, "**渠道** 模版-123 `#24`")
	assert.Contains(t, content, "**近5分钟成功率** 0%（0 / 3）")
	assert.Contains(t, content, "**告警线** 低于 50%")
	assert.Contains(t, content, "gpt-4")
	assert.Contains(t, content, "HTTP 500")
	assert.Contains(t, content, "upstream error: do request failed")

	_, emptyContent := formatChannelHealthAlert("模版-123", 24, snap, nil)
	assert.Contains(t, emptyContent, "暂无")
}

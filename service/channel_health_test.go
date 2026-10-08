package service

import (
	"errors"
	"net/http"
	"strings"
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
	assert.Contains(t, content, "**处理** 未自动禁用")
}

func TestObserveChannelHealthDisablesWhenSampleReadyAtZeroSuccess(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	origMin := operation_setting.ChannelHealthMinTotal
	prevEnabled := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() {
		operation_setting.ChannelHealthMinTotal = origMin
		common.AutomaticDisableChannelEnabled = prevEnabled
	})
	operation_setting.ChannelHealthMinTotal = 2
	common.AutomaticDisableChannelEnabled = true

	var disabled atomic.Int32
	prevLookup := lookupChannelForHealthDisableFn
	prevDisable := disableChannelHealthFn
	lookupChannelForHealthDisableFn = func(channelID int) (*channelHealthDisableTarget, error) {
		return &channelHealthDisableTarget{
			ID:      channelID,
			Name:    "unstable",
			AutoBan: true,
			Status:  common.ChannelStatusEnabled,
		}, nil
	}
	disableChannelHealthFn = func(channelError types.ChannelError, reason string) {
		disabled.Add(1)
		assert.Equal(t, 9, channelError.ChannelId)
	}
	t.Cleanup(func() {
		lookupChannelForHealthDisableFn = prevLookup
		disableChannelHealthFn = prevDisable
	})

	ObserveChannelHealth(9, "unstable", 1, 0, 1)
	assert.Equal(t, int32(0), disabled.Load())
	ObserveChannelHealth(9, "unstable", 1, 0, 1)
	require.Eventually(t, func() bool {
		return disabled.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestShouldDisableChannelByHealth(t *testing.T) {
	assert.False(t, shouldDisableChannelByHealth(ChannelHealthSnapshot{SampleReady: false, Total: 5, Success: 0, Bad: 5}))
	assert.False(t, shouldDisableChannelByHealth(ChannelHealthSnapshot{SampleReady: true, Total: 0, Success: 0, Bad: 0}))
	assert.False(t, shouldDisableChannelByHealth(ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 0}))
	assert.False(t, shouldDisableChannelByHealth(ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 1, Bad: 29}))
	assert.True(t, shouldDisableChannelByHealth(ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 12}))
}

func TestMaybeDisableChannelByHealthDisablesOnceAtZeroSuccess(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	prevEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = prevEnabled })

	var disabled atomic.Int32
	prevLookup := lookupChannelForHealthDisableFn
	prevDisable := disableChannelHealthFn
	lookupChannelForHealthDisableFn = func(channelID int) (*channelHealthDisableTarget, error) {
		return &channelHealthDisableTarget{
			ID:      channelID,
			Name:    "模版-123",
			AutoBan: true,
			Status:  common.ChannelStatusEnabled,
		}, nil
	}
	disableChannelHealthFn = func(channelError types.ChannelError, reason string) {
		disabled.Add(1)
		assert.Equal(t, 24, channelError.ChannelId)
		assert.True(t, channelError.AutoBan)
		assert.Empty(t, channelError.UsingKey)
		assert.Contains(t, reason, "0%")
		assert.Contains(t, reason, "30")
		assert.Contains(t, reason, "12")
	}
	t.Cleanup(func() {
		lookupChannelForHealthDisableFn = prevLookup
		disableChannelHealthFn = prevDisable
	})

	snap := ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 12, SuccessRate: 0}
	MaybeDisableChannelByHealth(24, "模版-123", snap)
	require.Eventually(t, func() bool {
		return disabled.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)

	MaybeDisableChannelByHealth(24, "模版-123", snap)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(1), disabled.Load())
}

func TestMaybeDisableChannelByHealthSkipsWhenAutoBanOffOrAlreadyDisabled(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	prevEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = prevEnabled })

	var disabled atomic.Int32
	prevLookup := lookupChannelForHealthDisableFn
	prevDisable := disableChannelHealthFn
	disableChannelHealthFn = func(channelError types.ChannelError, reason string) {
		disabled.Add(1)
	}
	t.Cleanup(func() {
		lookupChannelForHealthDisableFn = prevLookup
		disableChannelHealthFn = prevDisable
	})

	snap := ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 12}

	lookupChannelForHealthDisableFn = func(channelID int) (*channelHealthDisableTarget, error) {
		return &channelHealthDisableTarget{ID: channelID, AutoBan: false, Status: common.ChannelStatusEnabled}, nil
	}
	MaybeDisableChannelByHealth(41, "no-ban", snap)
	require.Eventually(t, func() bool {
		_, loaded := channelHealthDisableUntil.Load(41)
		return !loaded
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(0), disabled.Load())

	resetChannelHealthForTest()
	lookupChannelForHealthDisableFn = func(channelID int) (*channelHealthDisableTarget, error) {
		return &channelHealthDisableTarget{ID: channelID, AutoBan: true, Status: common.ChannelStatusAutoDisabled}, nil
	}
	MaybeDisableChannelByHealth(42, "already-off", snap)
	require.Eventually(t, func() bool {
		_, loaded := channelHealthDisableUntil.Load(42)
		return loaded
	}, 2*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(0), disabled.Load())
}

func TestMaybeDisableChannelByHealthRequiresGlobalSwitchAndClassifiedErrors(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	prevEnabled := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = prevEnabled })

	var disabled atomic.Int32
	prevDisable := disableChannelHealthFn
	disableChannelHealthFn = func(channelError types.ChannelError, reason string) {
		disabled.Add(1)
	}
	t.Cleanup(func() { disableChannelHealthFn = prevDisable })

	snap := ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 12}
	common.AutomaticDisableChannelEnabled = false
	MaybeDisableChannelByHealth(51, "off", snap)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), disabled.Load())

	common.AutomaticDisableChannelEnabled = true
	MaybeDisableChannelByHealth(52, "no-bad", ChannelHealthSnapshot{SampleReady: true, Total: 30, Success: 0, Bad: 0})
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), disabled.Load())
}

func TestFormatChannelHealthAlertMentionsAutoDisableWhenZeroSuccess(t *testing.T) {
	prevEnabled := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = prevEnabled })

	_, content := formatChannelHealthAlert("模版-123", 24, ChannelHealthSnapshot{
		SampleReady: true,
		Total:       30,
		Success:     0,
		Bad:         12,
		SuccessRate: 0,
	}, nil)
	assert.Contains(t, content, "将自动禁用该渠道")
}

func TestChannelRecentErrorItemsOmitSecretsAndTruncateContent(t *testing.T) {
	assert.Empty(t, channelRecentErrorItemsFromLogs(nil))
	assert.Empty(t, channelRecentErrorItemsFromLogs([]*model.Log{nil}))

	longContent := strings.Repeat("失败 ", 120)
	items := channelRecentErrorItemsFromLogs([]*model.Log{
		{
			Id:        9,
			CreatedAt: 1_700_000_000,
			ModelName: " gpt-4 ",
			Content:   longContent,
			Other:     `{"status_code":502,"admin_info":{"use_channel":[1],"secret":"sk-secret"}}`,
			Username:  "alice",
			TokenName: "sk-secret",
			Ip:        "10.0.0.1",
		},
		{
			Id:      10,
			Content: "plain timeout",
		},
	})
	require.Len(t, items, 2)
	assert.Equal(t, 9, items[0].Id)
	assert.Equal(t, "gpt-4", items[0].ModelName)
	assert.Equal(t, 502, items[0].StatusCode)
	assert.LessOrEqual(t, len([]rune(items[0].Content)), channelHealthErrorPreviewRunes+1)
	assert.True(t, strings.HasSuffix(items[0].Content, "…"))
	assert.Equal(t, 0, items[1].StatusCode)
	assert.Equal(t, "plain timeout", items[1].Content)

	encoded, err := common.Marshal(items[0])
	require.NoError(t, err)
	payload := string(encoded)
	assert.NotContains(t, payload, "alice")
	assert.NotContains(t, payload, "sk-secret")
	assert.NotContains(t, payload, "admin_info")
	assert.NotContains(t, payload, "10.0.0.1")
}

func TestListRecentChannelErrorsRejectsInvalidChannel(t *testing.T) {
	result, err := ListRecentChannelErrors(0)
	require.NoError(t, err)
	assert.Empty(t, result.Items)
	assert.Equal(t, channelHealthWindowMinutes*60, result.WindowSeconds)
	assert.NotNil(t, result.Items)
}

func TestChannelHealthLayerMinTotalScalesFromFiveMinuteBase(t *testing.T) {
	assert.Equal(t, 6, channelHealthLayerMinTotal(1, 30))
	assert.Equal(t, 30, channelHealthLayerMinTotal(5, 30))
	assert.Equal(t, 90, channelHealthLayerMinTotal(15, 30))
	assert.Equal(t, 180, channelHealthLayerMinTotal(30, 30))
	assert.Equal(t, 360, channelHealthLayerMinTotal(60, 30))
	assert.Equal(t, 1, channelHealthLayerMinTotal(1, 1))
}

func TestChannelHealthLayersAggregateWindowsAndExpireOldBuckets(t *testing.T) {
	prevRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = prevRedis })
	resetChannelHealthForTest()

	now := time.Now()
	require.NoError(t, incrChannelHealth(3, now, 1, 1, 0))
	require.NoError(t, incrChannelHealth(3, now.Add(-2*time.Minute), 1, 0, 1))
	require.NoError(t, incrChannelHealth(3, now.Add(-10*time.Minute), 1, 0, 1))
	require.NoError(t, incrChannelHealth(3, now.Add(-40*time.Minute), 2, 2, 0))
	require.NoError(t, incrChannelHealth(3, now.Add(-70*time.Minute), 5, 0, 5))

	snap := GetChannelHealthStats([]int{3})["3"]
	assert.Equal(t, int64(2), snap.Total)
	assert.Equal(t, int64(1), snap.Success)
	assert.Equal(t, int64(1), snap.Bad)
	require.Len(t, snap.Layers, len(channelHealthLayerMinutes))

	byMinutes := map[int]ChannelHealthLayer{}
	for _, layer := range snap.Layers {
		byMinutes[layer.Minutes] = layer
	}
	assert.Equal(t, int64(1), byMinutes[1].Total)
	assert.Equal(t, int64(1), byMinutes[1].Success)
	assert.Equal(t, int64(2), byMinutes[5].Total)
	assert.Equal(t, snap.Total, byMinutes[5].Total)
	assert.Equal(t, snap.SuccessRate, byMinutes[5].SuccessRate)
	assert.Equal(t, int64(3), byMinutes[15].Total)
	assert.Equal(t, int64(3), byMinutes[30].Total)
	assert.Equal(t, int64(5), byMinutes[60].Total)
	assert.Equal(t, int64(3), byMinutes[60].Success)
}

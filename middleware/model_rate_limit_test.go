package middleware

import (
	"bytes"
	"context"
	"fmt"
	"hash/maphash"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

var modelRateLimitTestUsers atomic.Int64

func TestModelRateLimitStreamFailuresDoNotConsumeSuccessLimit(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, backend := range []string{"memory", "redis"} {
		for _, totalLimit := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/total=%d", backend, totalLimit), func(t *testing.T) {
				subject := strconv.Itoa(7200000 + int(modelRateLimitTestUsers.Add(1)))
				handler := memoryRateLimitHandler(subject, 60, totalLimit, 1)
				if backend == "redis" {
					useRateLimitMiniRedis(t)
					handler = redisRateLimitHandler(subject, 60, totalLimit, 1)
				}
				router := gin.New()
				router.GET("/:outcome", handler, func(c *gin.Context) {
					status := relaycommon.NewStreamStatus()
					if c.Param("outcome") == "failed" {
						status.MarkFailed("server_error", "", 0)
					} else {
						status.MarkCompleted()
					}
					common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
					c.Status(http.StatusOK)
				})
				assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				if totalLimit > 0 {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				} else {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
				}
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
			})
		}
	}
}

func TestModelMemoryRateLimitReservesConcurrentSuccessAdmission(t *testing.T) {
	require.NoError(t, i18n.Init())
	subject := strconv.Itoa(7200000 + int(modelRateLimitTestUsers.Add(1)))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	router := gin.New()
	router.GET("/:outcome", memoryRateLimitHandler(subject, 60, 0, 1), func(c *gin.Context) {
		if c.Param("outcome") == "failed" {
			close(entered)
			<-release
			status := relaycommon.NewStreamStatus()
			status.MarkFailed("server_error", "", 0)
			common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
		}
		c.Status(http.StatusOK)
	})
	go func() {
		defer close(finished)
		assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
	}()
	<-entered
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	close(release)
	<-finished
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
}

// The memory limiter answers like the Redis limiter: an OpenAI error body
// naming the limit that was reached, not an empty 429.
func TestModelMemoryRateLimitRejectionsReturnOpenAIError(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, tc := range []struct {
		name                     string
		totalLimit, successLimit int
		messageKey               string
	}{
		{name: "total", totalLimit: 1, messageKey: i18n.MsgRateLimitTotalReached},
		{name: "success", successLimit: 1, messageKey: i18n.MsgRateLimitReached},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subject := strconv.Itoa(7200000 + int(modelRateLimitTestUsers.Add(1)))
			router := gin.New()
			router.GET("/", func(c *gin.Context) {
				c.Set(common.RequestIdKey, "req-limited")
			}, memoryRateLimitHandler(subject, 60, tc.totalLimit, tc.successLimit), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})
			require.Equal(t, http.StatusOK, performRateLimitRequest(router, "/", "127.0.0.1:1000").Code)

			resp := performRateLimitRequest(router, "/", "127.0.0.1:1000")
			assert.Equal(t, http.StatusTooManyRequests, resp.Code)
			message := i18n.Translate("", tc.messageKey, map[string]any{"Minutes": setting.ModelRequestRateLimitDurationMinutes, "Max": 1})
			want, err := common.Marshal(gin.H{"error": gin.H{
				"message": common.MessageWithRequestId(message, "req-limited"),
				"type":    "new_api_error",
				"code":    "",
			}})
			require.NoError(t, err)
			assert.JSONEq(t, string(want), resp.Body.String())
		})
	}
}

// A group with its own limit counts each user's requests on its own; groups
// without one keep sharing the user's global limit.
func TestModelRateLimitCountsConfiguredGroupsSeparately(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousEnabled, previousMinutes := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes
	previousTotal, previousSuccess := setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount
	previousGroups, previousRedisEnabled := setting.ModelRequestRateLimitGroup, common.RedisEnabled
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = previousEnabled, previousMinutes
		setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = previousTotal, previousSuccess
		setting.ModelRequestRateLimitGroup, common.RedisEnabled = previousGroups, previousRedisEnabled
	})
	setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes = true, 1
	common.RedisEnabled = false

	// Groups a and b have their own limits; c and d have none and share the global one.
	steps := []struct {
		group string
		want  int
	}{
		{"a", http.StatusOK}, {"a", http.StatusTooManyRequests},
		{"b", http.StatusOK}, {"b", http.StatusTooManyRequests},
		{"c", http.StatusOK}, {"d", http.StatusTooManyRequests},
	}
	// The Redis token bucket binds to the first Redis client it sees and another
	// test already drives it, so the total limit runs on the memory backend only.
	for _, tc := range []struct {
		backend, limit string
		counts         [2]int // [total, success] for groups a and b and for the global limit
	}{
		{"memory", "success", [2]int{0, 1}},
		{"redis", "success", [2]int{0, 1}},
		{"memory", "total", [2]int{1, 1000}},
	} {
		t.Run(tc.backend+"/"+tc.limit, func(t *testing.T) {
			if tc.backend == "redis" {
				useRateLimitMiniRedis(t)
			}
			setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount = tc.counts[0], tc.counts[1]
			setting.ModelRequestRateLimitGroup = map[string][2]int{"a": tc.counts, "b": tc.counts}
			userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
			router := gin.New()
			router.GET("/:group", func(c *gin.Context) {
				c.Set("id", userID)
				common.SetContextKey(c, constant.ContextKeyTokenGroup, c.Param("group"))
			}, ModelRequestRateLimit(), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			for _, step := range steps {
				assert.Equal(t, step.want, performRateLimitRequest(router, "/"+step.group, "127.0.0.1:1000").Code, "group %s", step.group)
			}
		})
	}
}

func TestGlobalModelRPMConfigurationRejectsAmbiguousAndUnboundedLimits(t *testing.T) {
	previous := setting.GlobalModelRateLimitJSON()
	t.Cleanup(func() { require.NoError(t, setting.UpdateGlobalModelRateLimit(previous)) })
	require.NoError(t, setting.UpdateGlobalModelRateLimit(`{"enabled":true,"limits":{"global-config-model":3}}`))
	for _, value := range []string{
		`null`, `{}`, `{"enabled":true,"limits":null}`, `{"enabled":true,"limits":{"m":null}}`,
		`{"enabled":true,"limits":{"m":-1}}`, `{"enabled":true,"limits":{"m":1.5}}`,
		`{"enabled":true,"limits":{"m":100001}}`, `{"enabled":true,"limits":{" ":1}}`,
		`{"enabled":true,"limits":{"m":1,"m":2}}`, `{"enabled":true,"limits":{"m":1," m ":2}}`,
		`{"enabled":false,"enabled":true,"limits":{}}`, `{"enabled":true,"limits":{},"unexpected":1}`,
	} {
		assert.Error(t, setting.UpdateGlobalModelRateLimit(value), value)
		assert.Equal(t, 3, setting.GlobalModelRPM("global-config-model"), "invalid updates must preserve the active configuration")
	}
	large := make(map[string]int)
	for index := range 1001 {
		large[fmt.Sprintf("model-%d", index)] = 1
	}
	data, err := common.Marshal(map[string]any{"enabled": true, "limits": large})
	require.NoError(t, err)
	assert.Error(t, setting.UpdateGlobalModelRateLimit(string(data)))
	config, err := setting.ParseGlobalModelRateLimit(`{"enabled":true,"limits":{" m ":0,"other":100000}}`)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"m": 0, "other": 100000}, config.Limits)
}

func TestGlobalModelRPMSharedAdmissionAndFailureCounting(t *testing.T) {
	require.NoError(t, i18n.Init())
	previous := setting.GlobalModelRateLimitJSON()
	previousRedis := common.RedisEnabled
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateGlobalModelRateLimit(previous))
		common.RedisEnabled = previousRedis
	})
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			common.RedisEnabled = false
			if backend == "redis" {
				useRateLimitMiniRedis(t)
			}
			name := fmt.Sprintf("global-shared-%s-%d", backend, modelRateLimitTestUsers.Add(1))
			other := name + "-other"
			require.NoError(t, setting.UpdateGlobalModelRateLimit(fmt.Sprintf(`{"enabled":true,"limits":{%q:2,%q:1}}`, name, other)))
			router := gin.New()
			router.GET("/:model/:user/:channel/:outcome", func(c *gin.Context) {
				userID, err := strconv.Atoi(c.Param("user"))
				require.NoError(t, err)
				channelID, err := strconv.Atoi(c.Param("channel"))
				require.NoError(t, err)
				c.Set("id", userID)
				c.Set("token_id", userID*100)
				c.Set("channel_id", channelID)
				common.SetContextKey(c, constant.ContextKeyUsingGroup, c.Param("user"))
				if apiErr := CheckGlobalModelRPM(c, c.Param("model")); apiErr != nil {
					assert.Equal(t, "status_code=429, Resource exhausted. Please try again later...", apiErr.ErrorWithStatusCode())
					abortWithOpenAiMessage(c, apiErr.StatusCode, apiErr.Error(), apiErr.GetErrorCode())
					return
				}
				// Repeated checks/retries within one request must not consume twice.
				require.Nil(t, CheckGlobalModelRPM(c, c.Param("model")))
				if c.Param("outcome") == "failed" {
					c.Status(http.StatusBadGateway)
				} else {
					c.Status(http.StatusOK)
				}
			})
			assert.Equal(t, http.StatusBadGateway, performRateLimitRequest(router, "/"+name+"/1/11/failed", "127.0.0.1:1000").Code)
			assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/"+name+"/2/22/ok", "127.0.0.1:1000").Code)
			for _, language := range []string{"", "en", "zh-CN", "zh-TW"} {
				request := httptest.NewRequest(http.MethodGet, "/"+name+"/3/33/ok", nil)
				request.Header.Set("Accept-Language", language)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				assert.Equal(t, http.StatusTooManyRequests, response.Code)
				assert.Equal(t, "60", response.Header().Get("Retry-After"))
				assert.JSONEq(t, `{"error":{"message":"Resource exhausted. Please try again later... (request id: )","type":"new_api_error","code":"global_model_rate_limit_exceeded"}}`, response.Body.String(), "language %q", language)
			}
			assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/"+other+"/3/33/ok", "127.0.0.1:1000").Code)
			assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/unconfigured/3/33/ok", "127.0.0.1:1000").Code)
		})
	}
}

func TestGlobalModelRPMMemorySlidingWindowAndLimitDecrease(t *testing.T) {
	limiter := globalModelRPMMemoryLimiter{seed: maphash.MakeSeed()}
	now := time.Now()
	assert.Zero(t, limiter.take("m", 3, now))
	assert.Zero(t, limiter.take("m", 3, now.Add(10*time.Second)))
	assert.Zero(t, limiter.take("m", 3, now.Add(20*time.Second)))
	assert.Equal(t, 30, limiter.take("m", 3, now.Add(30*time.Second)))
	assert.Equal(t, 50, limiter.take("m", 1, now.Add(30*time.Second)), "lowering the cap retains all accepted traffic")
	assert.Equal(t, 1, limiter.take("m", 3, now.Add(60*time.Second-time.Nanosecond)))
	assert.Zero(t, limiter.take("m", 3, now.Add(60*time.Second)))
	assert.Equal(t, 10, limiter.take("m", 3, now.Add(60*time.Second)))
	assert.Zero(t, limiter.take("m", 1, now.Add(121*time.Second)))
}

func TestGlobalModelRPMConcurrentAdmission(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			limiter := globalModelRPMMemoryLimiter{seed: maphash.MakeSeed()}
			var client *redis.Client
			if backend == "redis" {
				_, client = useRateLimitMiniRedis(t)
			}
			const requests, limit = 24, 7
			var admitted atomic.Int64
			var wg sync.WaitGroup
			start := make(chan struct{})
			for index := range requests {
				wg.Go(func() {
					<-start
					var retry int
					if client != nil {
						var err error
						retry, err = globalModelRPMScript.Run(context.Background(), client, []string{"test:global-rpm-concurrent"}, limit, strconv.Itoa(index)).Int()
						if !assert.NoError(t, err) {
							return
						}
					} else {
						retry = limiter.take("m", limit, time.Now())
					}
					if retry == 0 {
						admitted.Add(1)
					}
				})
			}
			close(start)
			wg.Wait()
			assert.EqualValues(t, limit, admitted.Load())
		})
	}
}

func TestGlobalModelRPMRedisWindowDoesNotExtendOnRejection(t *testing.T) {
	server, client := useRateLimitMiniRedis(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	server.SetTime(now)
	ctx := context.Background()
	key := "test:global-rpm-expiry"
	first, err := globalModelRPMScript.Run(ctx, client, []string{key}, 1, "first").Int()
	require.NoError(t, err)
	assert.Zero(t, first)
	duplicate, err := globalModelRPMScript.Run(ctx, client, []string{key}, 1, "first").Int()
	require.NoError(t, err)
	assert.Zero(t, duplicate, "a Redis transport retry must be idempotent")
	server.FastForward(30 * time.Second)
	server.SetTime(now.Add(30 * time.Second))
	blocked, err := globalModelRPMScript.Run(ctx, client, []string{key}, 1, "blocked").Int()
	require.NoError(t, err)
	assert.Equal(t, 30, blocked)
	assert.Equal(t, 30*time.Second, server.TTL(key))
	assert.EqualValues(t, 1, client.ZCard(ctx, key).Val())
	server.SetTime(now.Add(time.Minute)) // Exercise pruning, independently of Redis key expiry.
	allowed, err := globalModelRPMScript.Run(ctx, client, []string{key}, 1, "next").Int()
	require.NoError(t, err)
	assert.Zero(t, allowed)
	assert.EqualValues(t, 1, client.ZCard(ctx, key).Val())
}

func TestGlobalModelRPMRedisFailureIsClosedAndDisabledModelsBypass(t *testing.T) {
	require.NoError(t, i18n.Init())
	_, client := useRateLimitMiniRedis(t)
	require.NoError(t, client.Close())
	previous := setting.GlobalModelRateLimitJSON()
	t.Cleanup(func() { require.NoError(t, setting.UpdateGlobalModelRateLimit(previous)) })
	require.NoError(t, setting.UpdateGlobalModelRateLimit(`{"enabled":true,"limits":{"global-failure":1,"zero":0}}`))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := CheckGlobalModelRPM(c, "global-failure")
	require.NotNil(t, apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.Equal(t, "global_model_rate_limit_unavailable", string(apiErr.GetErrorCode()))
	assert.Nil(t, CheckGlobalModelRPM(c, "zero"))
	assert.Nil(t, CheckGlobalModelRPM(c, "unconfigured"))
	require.NoError(t, setting.UpdateGlobalModelRateLimit(`{"enabled":false,"limits":{"global-failure":1}}`))
	assert.Nil(t, CheckGlobalModelRPM(c, "global-failure"))
}

func TestGlobalModelRPMDistributeCoversRequestFormatsBeforeSelection(t *testing.T) {
	require.NoError(t, i18n.Init())
	useRateLimitMiniRedis(t)
	previous := setting.GlobalModelRateLimitJSON()
	t.Cleanup(func() { require.NoError(t, setting.UpdateGlobalModelRateLimit(previous)) })
	require.NoError(t, setting.UpdateGlobalModelRateLimit(`{"enabled":true,"limits":{"global-format":1}}`))
	admitted, _ := gin.CreateTestContext(httptest.NewRecorder())
	admitted.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	require.Nil(t, CheckGlobalModelRPM(admitted, "global-format"))
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/images/generations", "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/images/edits", "/v1/videos", "/v1beta/models/global-format:generateContent", "/v1/realtime?model=global-format", "/pg/chat/completions", "/tasks/submit/example"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"global-format"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			if path == "/v1/audio/transcriptions" || path == "/v1/images/edits" || path == "/v1/videos" {
				var body bytes.Buffer
				form := multipart.NewWriter(&body)
				require.NoError(t, form.WriteField("model", "global-format"))
				require.NoError(t, form.Close())
				c.Request = httptest.NewRequest(http.MethodPost, path, &body)
				c.Request.Header.Set("Content-Type", form.FormDataContentType())
			}
			if strings.HasPrefix(path, "/tasks/") {
				c.Set("resolved_task_model", "global-format")
			}
			Distribute()(c)
			assert.Equal(t, http.StatusTooManyRequests, response.Code)
			assert.Contains(t, response.Body.String(), "global_model_rate_limit_exceeded")
		})
	}
	t.Run("pinned channel still shares the global limit", func(t *testing.T) {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"global-format"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		service.GetChannelConstraints(c).AddPin(dto.ChannelPin{
			ChannelId: 1,
			Source:    dto.PinSourceToken,
			Rank:      dto.PinRankToken,
			RetryMode: dto.PinRetrySingleAttempt,
		})
		Distribute()(c)
		assert.Equal(t, http.StatusTooManyRequests, response.Code)
		assert.Contains(t, response.Body.String(), "global_model_rate_limit_exceeded")
	})
	for _, path := range []string{"/v1/videos/task_id", "/v1/video/generations/task_id", "/mj/task/task_id/fetch"} {
		t.Run("poll "+path, func(t *testing.T) {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodGet, path, nil)
			Distribute()(c)
			assert.False(t, c.IsAborted(), "task polling must not be blocked by submission RPM")
			assert.Empty(t, response.Header().Get("Retry-After"))
		})
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"global-format"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	Distribute()(c)
	assert.Equal(t, http.StatusForbidden, response.Code, "model authorization must precede RPM admission")
}

func TestGlobalModelRPMRealRedisSharedClients(t *testing.T) {
	address := os.Getenv("TEST_GLOBAL_RPM_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_GLOBAL_RPM_REDIS_ADDR is not configured")
	}
	ctx := context.Background()
	first := redis.NewClient(&redis.Options{Addr: address})
	second := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	key := "test:global-model-rpm:" + t.Name()
	require.NoError(t, first.Del(ctx, key).Err())
	t.Cleanup(func() { _ = first.Del(ctx, key).Err() })
	result, err := globalModelRPMScript.Run(ctx, first, []string{key}, 1, "first-instance").Int()
	require.NoError(t, err)
	assert.Zero(t, result)
	result, err = globalModelRPMScript.Run(ctx, second, []string{key}, 1, "second-instance").Int()
	require.NoError(t, err)
	assert.Positive(t, result)
	assert.EqualValues(t, 1, second.ZCard(ctx, key).Val())

	concurrentKey := key + ":concurrent"
	require.NoError(t, first.Del(ctx, concurrentKey).Err())
	t.Cleanup(func() { _ = first.Del(ctx, concurrentKey).Err() })
	const requests, limit = 24, 7
	var admitted atomic.Int64
	var workers sync.WaitGroup
	start := make(chan struct{})
	clients := []*redis.Client{first, second}
	for index := range requests {
		workers.Go(func() {
			<-start
			retry, err := globalModelRPMScript.Run(ctx, clients[index%len(clients)], []string{concurrentKey}, limit, strconv.Itoa(index)).Int()
			if !assert.NoError(t, err) {
				return
			}
			if retry == 0 {
				admitted.Add(1)
			}
		})
	}
	close(start)
	workers.Wait()
	assert.EqualValues(t, limit, admitted.Load(), "two clients must not admit beyond the shared limit")
	assert.EqualValues(t, limit, second.ZCard(ctx, concurrentKey).Val())
}

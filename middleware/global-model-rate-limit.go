package middleware

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash/maphash"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

// Redis time avoids clock skew between gateway instances. Admission and expiry
// are atomic; rejected requests neither grow the log nor extend its lifetime.
var globalModelRPMScript = redis.NewScript(`
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000000 + tonumber(clock[2])
local window = 60000000
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
if redis.call('ZSCORE', KEYS[1], ARGV[2]) then return 0 end
local count = redis.call('ZCARD', KEYS[1])
if count >= tonumber(ARGV[1]) then
  local rank = count - tonumber(ARGV[1])
  local oldest = redis.call('ZRANGE', KEYS[1], rank, rank, 'WITHSCORES')
  return math.max(1, math.ceil((tonumber(oldest[2]) + window - now) / 1000000))
end
redis.call('ZADD', KEYS[1], now, ARGV[2])
redis.call('PEXPIRE', KEYS[1], 60000)
return 0
`)

type globalModelRPMWindow struct {
	requests []time.Time
	head     int
}

type globalModelRPMShard struct {
	sync.Mutex
	windows     map[string]*globalModelRPMWindow
	nextCleanup time.Time
}

type globalModelRPMMemoryLimiter struct {
	seed   maphash.Seed
	shards [64]globalModelRPMShard
}

var globalModelRPMMemory = globalModelRPMMemoryLimiter{seed: maphash.MakeSeed()}

// take returns the seconds until the oldest accepted request leaves the
// sliding window, or zero for admission. No allocation occurs on rejection.
func (l *globalModelRPMMemoryLimiter) take(model string, limit int, now time.Time) int {
	shard := &l.shards[maphash.String(l.seed, model)%uint64(len(l.shards))]
	shard.Lock()
	defer shard.Unlock()
	if shard.windows == nil {
		shard.windows = make(map[string]*globalModelRPMWindow)
	}
	if !now.Before(shard.nextCleanup) {
		for key, window := range shard.windows {
			if now.Sub(window.requests[len(window.requests)-1]) >= time.Minute {
				delete(shard.windows, key)
			}
		}
		shard.nextCleanup = now.Add(time.Minute)
	}
	window := shard.windows[model]
	if window == nil {
		window = &globalModelRPMWindow{}
		shard.windows[model] = window
	}
	// Callers can acquire the lock in a different order from their timestamps.
	if n := len(window.requests); n > 0 && now.Before(window.requests[n-1]) {
		now = window.requests[n-1]
	}
	for window.head < len(window.requests) && now.Sub(window.requests[window.head]) >= time.Minute {
		window.head++
	}
	if len(window.requests)-window.head >= limit {
		remaining := time.Minute - now.Sub(window.requests[len(window.requests)-limit])
		return max(1, int((remaining+time.Second-1)/time.Second))
	}
	if window.head == len(window.requests) {
		window.requests = nil
		window.head = 0
	} else if window.head > 0 && window.head >= len(window.requests)/2 {
		window.requests = append([]time.Time(nil), window.requests[window.head:]...)
		window.head = 0
	}
	window.requests = append(window.requests, now)
	return 0
}

// CheckGlobalModelRPM must run after authentication/model authorization and
// before billing or dispatch. A client request consumes one slot even if the
// upstream fails; repeated admission checks within that request are idempotent.
func CheckGlobalModelRPM(c *gin.Context, model string) *types.NewAPIError {
	limit := setting.GlobalModelRPM(model)
	if limit <= 0 || c.GetString("global_model_rpm_admitted") == model {
		return nil
	}
	var retryAfter int
	if common.RedisEnabled {
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		var err error
		if common.RDB == nil {
			err = errors.New("Redis client is not initialized")
		} else {
			key := fmt.Sprintf("new-api:global-model-rpm:v1:%x", sha256.Sum256([]byte(model)))
			retryAfter, err = globalModelRPMScript.Run(ctx, common.RDB, []string{key}, limit, uuid.NewString()).Int()
			if err == nil && (retryAfter < 0 || retryAfter > 60) {
				err = errors.New("invalid global model RPM response")
			}
		}
		if err != nil {
			logger.LogWarn(c, common.LogText("failed to check global model RPM: %s", err.Error()))
			return types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgGlobalModelRPMUnavailable)), "global_model_rate_limit_unavailable", http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
	} else {
		retryAfter = globalModelRPMMemory.take(model, limit, time.Now())
	}
	if retryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		message := i18n.T(c, i18n.MsgGlobalModelRPMReached)
		return types.NewErrorWithStatusCode(errors.New(message), "global_model_rate_limit_exceeded", http.StatusTooManyRequests, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	c.Set("global_model_rpm_admitted", model)
	return nil
}

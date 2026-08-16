package channel_selector

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// AttemptOutcome classifies one upstream attempt (not the final user request).
type AttemptOutcome int

const (
	// OutcomeSuccess counts toward success rate as success.
	OutcomeSuccess AttemptOutcome = iota
	// OutcomeChannelFault counts toward success rate as failure (5xx, timeout, channel: errors, etc.).
	OutcomeChannelFault
	// OutcomeIgnored is not recorded (e.g. HTTP 400 client errors).
	OutcomeIgnored
)

type minuteCounters struct {
	eligible   atomic.Int64
	ok         atomic.Int64
	latencySum atomic.Int64
	latencyN   atomic.Int64
}

type store struct {
	mu   sync.RWMutex
	data map[string]map[int64]*minuteCounters
}

var globalStore = &store{data: map[string]map[int64]*minuteCounters{}}

func metricKey(channelID int, model string) string {
	return fmt.Sprintf("%d|%s", channelID, model)
}

func minuteBucket(ts int64) int64 {
	return ts - (ts % 60)
}

// RecordAttempt records one upstream attempt for channel×model stats.
// Call after every attempt ends (success or failure). Do not call only on final outcome.
func RecordAttempt(channelID int, model string, outcome AttemptOutcome, latencyMs int64) {
	if channelID <= 0 || model == "" || outcome == OutcomeIgnored {
		return
	}
	key := metricKey(channelID, model)
	min := minuteBucket(time.Now().Unix())

	globalStore.mu.Lock()
	buckets, ok := globalStore.data[key]
	if !ok {
		buckets = map[int64]*minuteCounters{}
		globalStore.data[key] = buckets
	}
	c, ok := buckets[min]
	if !ok {
		c = &minuteCounters{}
		buckets[min] = c
	}
	globalStore.mu.Unlock()

	c.eligible.Add(1)
	if outcome == OutcomeSuccess {
		c.ok.Add(1)
	}
	if latencyMs >= 0 {
		c.latencySum.Add(latencyMs)
		c.latencyN.Add(1)
	}
}

// Stats is aggregated channel×model health in a rolling window.
type Stats struct {
	Eligible int64
	OK       int64
	AvgLatMs float64
	Samples  int64
}

// QueryStats returns rolling-window counters for channel×model.
func QueryStats(channelID int, model string, windowSec int64) Stats {
	if windowSec <= 0 {
		windowSec = 900
	}
	key := metricKey(channelID, model)
	from := minuteBucket(time.Now().Unix() - windowSec)

	globalStore.mu.RLock()
	buckets := globalStore.data[key]
	globalStore.mu.RUnlock()

	var st Stats
	var latSum, latN int64
	for m, c := range buckets {
		if m < from {
			continue
		}
		st.Eligible += c.eligible.Load()
		st.OK += c.ok.Load()
		latSum += c.latencySum.Load()
		latN += c.latencyN.Load()
	}
	st.Samples = st.Eligible
	if latN > 0 {
		st.AvgLatMs = float64(latSum) / float64(latN)
	}
	return st
}

// ResetStats clears channel×model metrics (manual reset / channel re-enable).
func ResetStats(channelID int, model string) {
	key := metricKey(channelID, model)
	globalStore.mu.Lock()
	delete(globalStore.data, key)
	globalStore.mu.Unlock()
}

// resetAllStatsForTest clears the in-memory store (tests only).
func resetAllStatsForTest() {
	globalStore.mu.Lock()
	globalStore.data = map[string]map[int64]*minuteCounters{}
	globalStore.mu.Unlock()
}

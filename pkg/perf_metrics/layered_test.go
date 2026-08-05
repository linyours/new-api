package perfmetrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetLayeredHotBuckets() {
	layeredHotBuckets.Range(func(key, _ any) bool {
		layeredHotBuckets.Delete(key)
		return true
	})
	layeredModelBuckets.Range(func(key, _ any) bool {
		layeredModelBuckets.Delete(key)
		return true
	})
}

func TestQueryLayeredAllFromMemory(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	minute := minuteStart(time.Now().Unix())
	incrLayeredBucket(LayeredDimAll, 0, minute, true)
	incrLayeredBucket(LayeredDimAll, 0, minute, true)
	incrLayeredBucket(LayeredDimAll, 0, minute, false)

	result, err := QueryLayered(LayeredDimAll, 0)
	require.NoError(t, err)
	require.Len(t, result.Windows, 5)
	assert.Equal(t, "all", result.Dimension)

	oneMin := result.Windows[0]
	assert.Equal(t, "1min", oneMin.Label)
	assert.Equal(t, int64(3), oneMin.RequestCount)
	assert.Equal(t, 66.67, oneMin.SuccessRate)
}

func TestQueryLayeredChannelTypeRequiresID(t *testing.T) {
	_, err := QueryLayered(LayeredDimChannelType, 0)
	require.Error(t, err)
}

func TestQueryLayeredChannelFiltersByID(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	minute := minuteStart(time.Now().Unix())
	incrLayeredBucket(LayeredDimChannel, 12, minute, true)
	incrLayeredBucket(LayeredDimChannel, 12, minute, false)
	incrLayeredBucket(LayeredDimChannel, 99, minute, true)

	result, err := QueryLayered(LayeredDimChannel, 12)
	require.NoError(t, err)
	assert.Equal(t, 12, result.ChannelID)
	assert.Equal(t, int64(2), result.Windows[0].RequestCount)
	assert.Equal(t, 50.0, result.Windows[0].SuccessRate)
}

func TestCleanupLayeredMemoryRemovesOldBuckets(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	now := time.Now().Unix()
	oldMinute := minuteStart(now) - int64(layeredBucketTTL.Seconds()) - 60
	incrLayeredBucket(LayeredDimAll, 0, oldMinute, true)
	incrLayeredBucket(LayeredDimAll, 0, minuteStart(now), true)

	cleanupLayeredMemory(now)

	_, oldExists := layeredHotBuckets.Load(layeredBucketKey{dim: LayeredDimAll, id: 0, minute: oldMinute})
	_, currentExists := layeredHotBuckets.Load(layeredBucketKey{dim: LayeredDimAll, id: 0, minute: minuteStart(now)})
	assert.False(t, oldExists)
	assert.True(t, currentExists)
}

func TestQueryLayeredChannelsBatch(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	minute := minuteStart(time.Now().Unix())
	incrLayeredBucket(LayeredDimChannel, 1, minute, true)
	incrLayeredBucket(LayeredDimChannel, 1, minute, true)
	incrLayeredBucket(LayeredDimChannel, 1, minute, false)
	incrLayeredBucket(LayeredDimChannel, 2, minute, true)

	result, err := QueryLayeredChannels([]int{1, 2, 2, 0}, 3600)
	require.NoError(t, err)
	assert.Equal(t, int64(3600), result.WindowSeconds)
	require.Contains(t, result.Items, 1)
	require.Contains(t, result.Items, 2)
	assert.Equal(t, int64(3), result.Items[1].RequestCount)
	assert.Equal(t, 66.67, result.Items[1].SuccessRate)
	assert.Equal(t, int64(1), result.Items[2].RequestCount)
	assert.Equal(t, 100.0, result.Items[2].SuccessRate)
}

func TestQueryLayeredChannelsRejectsOversizedBatch(t *testing.T) {
	ids := make([]int, MaxLayeredChannelBatch+1)
	for i := range ids {
		ids[i] = i + 1
	}
	_, err := QueryLayeredChannels(ids, 3600)
	require.Error(t, err)
}

func TestQueryLayeredModelFromMemory(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	minute := minuteStart(time.Now().Unix())
	incrLayeredModelMemory("gpt-4o", minute, true)
	incrLayeredModelMemory("gpt-4o", minute, false)
	incrLayeredModelMemory("other", minute, true)

	result, err := QueryLayeredModel("gpt-4o")
	require.NoError(t, err)
	assert.Equal(t, "model", result.Dimension)
	assert.Equal(t, "gpt-4o", result.ModelName)
	assert.Equal(t, int64(2), result.Windows[0].RequestCount)
	assert.Equal(t, 50.0, result.Windows[0].SuccessRate)
}

func TestQueryLayeredModelRequiresName(t *testing.T) {
	_, err := QueryLayeredModel("  ")
	require.Error(t, err)
}

func TestClearChannelLayeredMetrics(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	minute := minuteStart(time.Now().Unix())
	incrLayeredBucket(LayeredDimChannel, 7, minute, true)
	incrLayeredBucket(LayeredDimChannel, 8, minute, true)
	incrLayeredBucket(LayeredDimAll, 0, minute, true)

	ClearChannelLayeredMetrics(7)

	_, cleared := layeredHotBuckets.Load(layeredBucketKey{dim: LayeredDimChannel, id: 7, minute: minute})
	_, kept := layeredHotBuckets.Load(layeredBucketKey{dim: LayeredDimChannel, id: 8, minute: minute})
	_, allKept := layeredHotBuckets.Load(layeredBucketKey{dim: LayeredDimAll, id: 0, minute: minute})
	assert.False(t, cleared)
	assert.True(t, kept)
	assert.True(t, allKept)
}

func TestRecordLayeredIncludesModel(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	recordLayered(Sample{
		Model:       "claude-3",
		ChannelID:   3,
		ChannelType: 1,
		Success:     true,
	})

	result, err := QueryLayeredModel("claude-3")
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Windows[0].RequestCount)
	assert.Equal(t, 100.0, result.Windows[0].SuccessRate)

	channelResult, err := QueryLayered(LayeredDimChannel, 3)
	require.NoError(t, err)
	assert.Equal(t, int64(1), channelResult.Windows[0].RequestCount)
}

func TestCleanupLayeredMemoryRemovesOldModelBuckets(t *testing.T) {
	resetLayeredHotBuckets()
	t.Cleanup(resetLayeredHotBuckets)

	now := time.Now().Unix()
	oldMinute := minuteStart(now) - int64(layeredBucketTTL.Seconds()) - 60
	incrLayeredModelMemory("m", oldMinute, true)
	incrLayeredModelMemory("m", minuteStart(now), true)

	cleanupLayeredMemory(now)

	_, oldExists := layeredModelBuckets.Load(layeredModelKey{model: "m", minute: oldMinute})
	_, currentExists := layeredModelBuckets.Load(layeredModelKey{model: "m", minute: minuteStart(now)})
	assert.False(t, oldExists)
	assert.True(t, currentExists)
}

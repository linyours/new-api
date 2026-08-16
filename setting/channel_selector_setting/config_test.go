package channel_selector_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvFloat01Clamp(t *testing.T) {
	t.Setenv("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", "1.5")
	assert.Equal(t, 1.0, envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", 0.08))

	t.Setenv("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", "-0.2")
	assert.Equal(t, 0.0, envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", 0.08))

	t.Setenv("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", "0.4")
	assert.Equal(t, 0.4, envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", 0.08))

	t.Setenv("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", "nope")
	assert.Equal(t, 0.08, envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", 0.08))

	t.Setenv("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", "")
	assert.Equal(t, 0.08, envFloat01("CHANNEL_SELECTOR_EXPLORE_RATE_TEST", 0.08))
}

func TestSetExploreRateClamp(t *testing.T) {
	old := ExploreRate()
	t.Cleanup(func() { SetExploreRate(old) })

	SetExploreRate(0.42)
	assert.Equal(t, 0.42, ExploreRate())
	SetExploreRate(2)
	assert.Equal(t, 1.0, ExploreRate())
	SetExploreRate(-1)
	assert.Equal(t, 0.0, ExploreRate())
}

func TestSetExploreRateColdAndMinSamples(t *testing.T) {
	oldCold := ExploreRateCold()
	oldMin := MinSamples()
	t.Cleanup(func() {
		SetExploreRateCold(oldCold)
		SetMinSamples(oldMin)
	})

	SetExploreRateCold(0.55)
	assert.Equal(t, 0.55, ExploreRateCold())
	SetExploreRateCold(3)
	assert.Equal(t, 1.0, ExploreRateCold())

	SetMinSamples(12)
	assert.Equal(t, int64(12), MinSamples())
	SetMinSamples(-5)
	assert.Equal(t, int64(0), MinSamples())
}

func TestHotUpdateExploreFields(t *testing.T) {
	old := GetSetting()
	t.Cleanup(func() {
		SetExploreRate(old.ExploreRate)
		SetExploreRateCold(old.ExploreRateCold)
		SetMinSamples(old.MinSamples)
	})

	err := config.UpdateConfigFromMap(&channelSelectorSetting, map[string]string{
		"explore_rate":      "0.33",
		"explore_rate_cold": "0.44",
		"min_samples":       "7",
	})
	require.NoError(t, err)
	assert.Equal(t, 0.33, ExploreRate())
	assert.Equal(t, 0.44, ExploreRateCold())
	assert.Equal(t, int64(7), MinSamples())
}

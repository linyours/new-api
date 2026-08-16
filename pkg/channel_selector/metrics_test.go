package channel_selector

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordAttemptAndQuery(t *testing.T) {
	resetAllStatsForTest()
	RecordAttempt(11, "m1", OutcomeSuccess, 100)
	RecordAttempt(11, "m1", OutcomeChannelFault, 200)
	RecordAttempt(11, "m1", OutcomeIgnored, 50) // ignored

	st := QueryStats(11, "m1", 3600)
	require.Equal(t, int64(2), st.Eligible)
	require.Equal(t, int64(1), st.OK)
	assert.InDelta(t, 150.0, st.AvgLatMs, 1e-9)
}

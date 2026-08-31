package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterAndPaginateChannelCreatorUsage(t *testing.T) {
	rows := []ChannelCreatorUsageRow{
		{CreatedBy: 1, Username: "alice", ChannelCount: 2, UsedQuota: 100},
		{CreatedBy: 2, Username: "bob", ChannelCount: 1, UsedQuota: 50},
		{CreatedBy: 0, Username: "", ChannelCount: 3, UsedQuota: 10},
	}

	filtered := FilterChannelCreatorUsage(rows, "ali")
	require.Len(t, filtered, 1)
	assert.Equal(t, 1, filtered[0].CreatedBy)

	filteredByID := FilterChannelCreatorUsage(rows, "0")
	require.Len(t, filteredByID, 1)
	assert.Equal(t, int64(3), filteredByID[0].ChannelCount)

	page := PaginateChannelCreatorUsage(rows, 2, 2)
	require.Len(t, page, 1)
	assert.Equal(t, 0, page[0].CreatedBy)

	totals := SummarizeChannelCreatorUsage(rows)
	assert.Equal(t, int64(160), totals.TotalUsedQuota)
	assert.Equal(t, int64(6), totals.TotalChannels)
	assert.Equal(t, int64(3), totals.UnknownCreatorChannels)
	assert.Equal(t, int64(3), totals.CreatorCount)
}

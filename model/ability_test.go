package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAbilitySelectionTestDB(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
}

func createPriorityFallbackChannels(t *testing.T) (highPriorityId int, lowPriorityId int) {
	t.Helper()

	highPriority := int64(1)
	lowPriority := int64(0)
	highWeight := uint(1)
	lowWeight := uint(0)
	channels := []Channel{
		{
			Name:     "high-priority",
			Status:   common.ChannelStatusEnabled,
			Group:    "vip",
			Models:   "gpt-4o",
			Priority: &highPriority,
			Weight:   &highWeight,
		},
		{
			Name:     "low-priority",
			Status:   common.ChannelStatusEnabled,
			Group:    "vip",
			Models:   "gpt-4o",
			Priority: &lowPriority,
			Weight:   &lowWeight,
		},
	}
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{
			Group:     "vip",
			Model:     "gpt-4o",
			ChannelId: channels[0].Id,
			Enabled:   true,
			Priority:  &highPriority,
			Weight:    highWeight,
		},
		{
			Group:     "vip",
			Model:     "gpt-4o",
			ChannelId: channels[1].Id,
			Enabled:   true,
			Priority:  &lowPriority,
			Weight:    lowWeight,
		},
	}).Error)
	return channels[0].Id, channels[1].Id
}

func TestGetChannelExcludingFallsBackAfterHighestPriorityIsExcluded(t *testing.T) {
	setupAbilitySelectionTestDB(t)
	highPriorityId, lowPriorityId := createPriorityFallbackChannels(t)

	channel, err := GetChannelExcluding(
		"vip",
		"gpt-4o",
		0,
		"/v1/chat/completions",
		map[int]struct{}{highPriorityId: {}},
	)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, lowPriorityId, channel.Id)
}

func TestGetChannelExcludingKeepsHighestPriorityWithoutExclusions(t *testing.T) {
	setupAbilitySelectionTestDB(t)
	highPriorityId, _ := createPriorityFallbackChannels(t)

	channel, err := GetChannelExcluding(
		"vip",
		"gpt-4o",
		0,
		"/v1/chat/completions",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, highPriorityId, channel.Id)
}

func TestGetChannelExcludingReturnsNilWhenAllChannelsAreExcluded(t *testing.T) {
	setupAbilitySelectionTestDB(t)
	highPriorityId, lowPriorityId := createPriorityFallbackChannels(t)

	channel, err := GetChannelExcluding(
		"vip",
		"gpt-4o",
		0,
		"/v1/chat/completions",
		map[int]struct{}{
			highPriorityId: {},
			lowPriorityId:  {},
		},
	)

	require.NoError(t, err)
	assert.Nil(t, channel)
}

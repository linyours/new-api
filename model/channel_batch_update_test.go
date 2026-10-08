package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchAddChannelModelsAppendsMissingNamesAndKeepsExisting(t *testing.T) {
	setupChannelKeyTestDB(t)

	existing := &Channel{
		Name:   "with-models",
		Key:    "key-a",
		Models: "gpt-4o, gpt-4o-mini",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	empty := &Channel{
		Name:   "empty-models",
		Key:    "key-b",
		Models: "",
		Group:  "vip",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(existing).Error)
	require.NoError(t, DB.Create(empty).Error)

	changed, err := BatchAddChannelModels(
		[]int{existing.Id, empty.Id, existing.Id, 0},
		[]string{" gpt-4o ", "gpt-5", "gpt-5", ""},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, changed)

	var updatedExisting Channel
	require.NoError(t, DB.First(&updatedExisting, existing.Id).Error)
	assert.Equal(t, "gpt-4o,gpt-4o-mini,gpt-5", updatedExisting.Models)

	var updatedEmpty Channel
	require.NoError(t, DB.First(&updatedEmpty, empty.Id).Error)
	assert.Equal(t, "gpt-4o,gpt-5", updatedEmpty.Models)

	var existingAbilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", existing.Id).Order("model ASC").Find(&existingAbilities).Error)
	require.Len(t, existingAbilities, 3)
	assert.Equal(t, []string{"gpt-4o", "gpt-4o-mini", "gpt-5"}, []string{
		existingAbilities[0].Model,
		existingAbilities[1].Model,
		existingAbilities[2].Model,
	})

	var emptyAbilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", empty.Id).Order("model ASC").Find(&emptyAbilities).Error)
	require.Len(t, emptyAbilities, 2)
	assert.Equal(t, "vip", emptyAbilities[0].Group)
}

func TestBatchAddChannelModelsSkipsChannelsThatAlreadyHaveAllModels(t *testing.T) {
	setupChannelKeyTestDB(t)

	channel := &Channel{
		Name:   "already-has",
		Key:    "key-a",
		Models: "gpt-4o,gpt-5",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)

	changed, err := BatchAddChannelModels([]int{channel.Id}, []string{"gpt-4o", "gpt-5"})
	require.NoError(t, err)
	assert.Equal(t, 0, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, "gpt-4o,gpt-5", stored.Models)
}

func TestBatchAddChannelModelsRejectsEmptyInput(t *testing.T) {
	_, err := BatchAddChannelModels(nil, []string{"gpt-4o"})
	require.EqualError(t, err, "no channels selected")

	_, err = BatchAddChannelModels([]int{1}, []string{" ", ""})
	require.EqualError(t, err, "no models to add")
}

func TestBatchMergeChannelKeyModelRpmLimitsOverwritesSameModelAndKeepsOthers(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a\nkey-b", 0)
	keys := CacheGetChannelKeys(channel.Id)
	require.Len(t, keys, 2)

	firstLimits := ChannelKeyModelRpmLimits{"gpt-4o": 10, "gpt-4o-mini": 4}
	secondLimits := ChannelKeyModelRpmLimits{"claude-3": 8}
	require.NoError(t, UpdateChannelKeyLimits(channel.Id, keys[0].Id, nil, nil, &firstLimits))
	require.NoError(t, UpdateChannelKeyLimits(channel.Id, keys[1].Id, nil, nil, &secondLimits))

	archived := &Channel{
		Name:   "archived-key-channel",
		Key:    "key-c",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(archived).Error)
	require.NoError(t, SyncChannelKeys(archived))
	archivedKey := CacheGetChannelKeys(archived.Id)[0]
	archivedLimits := ChannelKeyModelRpmLimits{"gpt-4o": 1}
	require.NoError(t, UpdateChannelKeyLimits(archived.Id, archivedKey.Id, nil, nil, &archivedLimits))
	require.NoError(t, DB.Model(&ChannelKey{}).Where("id = ?", archivedKey.Id).Update("status", ChannelKeyStatusArchived).Error)

	changed, err := BatchMergeChannelKeyModelRpmLimits(
		[]int{channel.Id, archived.Id, channel.Id},
		ChannelKeyModelRpmLimits{" gpt-4o ": 20, "gpt-5": 5},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, changed)

	updated := CacheGetChannelKeys(channel.Id)
	require.Len(t, updated, 2)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 20, "gpt-4o-mini": 4, "gpt-5": 5}, updated[0].ModelRpmLimits)
	assert.Equal(t, ChannelKeyModelRpmLimits{"claude-3": 8, "gpt-4o": 20, "gpt-5": 5}, updated[1].ModelRpmLimits)

	var storedArchived ChannelKey
	require.NoError(t, DB.First(&storedArchived, archivedKey.Id).Error)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 1}, storedArchived.ModelRpmLimits)
}

func TestBatchMergeChannelKeyModelRpmLimitsSkipsUnchangedKeys(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 0)
	key := CacheGetChannelKeys(channel.Id)[0]
	limits := ChannelKeyModelRpmLimits{"gpt-4o": 20}
	require.NoError(t, UpdateChannelKeyLimits(channel.Id, key.Id, nil, nil, &limits))

	changed, err := BatchMergeChannelKeyModelRpmLimits(
		[]int{channel.Id},
		ChannelKeyModelRpmLimits{"gpt-4o": 20},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, changed)
}

func TestBatchMergeChannelKeyModelRpmLimitsRejectsInvalidInput(t *testing.T) {
	_, err := BatchMergeChannelKeyModelRpmLimits(nil, ChannelKeyModelRpmLimits{"gpt-4o": 1})
	require.EqualError(t, err, "no channels selected")

	_, err = BatchMergeChannelKeyModelRpmLimits([]int{1}, ChannelKeyModelRpmLimits{})
	require.EqualError(t, err, "no model RPM limits to add")

	_, err = BatchMergeChannelKeyModelRpmLimits([]int{1}, ChannelKeyModelRpmLimits{"gpt-4o": -1})
	require.Error(t, err)
}

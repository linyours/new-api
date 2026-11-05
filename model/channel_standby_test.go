package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupStandbyTestDB(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousType := common.MainDatabaseType()
	previousCache := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&Ability{},
		&ChannelTemplate{},
		&StandbyPoolSetting{},
		&ChannelKey{},
		&ChannelKeyQuotaReservation{},
		&ChannelKeyEvent{},
		&ChannelKeyArchive{},
	))

	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = nil
		channelsIDM = nil
		channel2advancedCustomConfig = nil
		channelSyncLock.Unlock()
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.MemoryCacheEnabled = previousCache
	})
}

func createStandbyTemplate(t *testing.T, enabled bool, threshold, batch int) *ChannelTemplate {
	t.Helper()
	template := &ChannelTemplate{
		Name:        "pool",
		ChannelType: 1,
		Config:      `{"type":1,"models":"gpt-test","group":"default"}`,
	}
	require.NoError(t, template.Insert())
	_, err := SaveStandbyPoolSettings(enabled, threshold, batch)
	require.NoError(t, err)
	return template
}

func createPoolChannel(t *testing.T, templateID, status int, name string) *Channel {
	t.Helper()
	channel := &Channel{
		Name:       name,
		Type:       1,
		Key:        "sk-" + name,
		Status:     status,
		TemplateId: templateID,
		Models:     "gpt-test",
		Group:      "default",
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func TestNormalizeStandbyPromoteSettings(t *testing.T) {
	_, _, _, err := NormalizeStandbyPromoteSettings(true, 10, 0)
	require.Error(t, err)

	_, _, _, err = NormalizeStandbyPromoteSettings(true, -1, 1)
	require.Error(t, err)

	_, _, _, err = NormalizeStandbyPromoteSettings(true, 1, MaxStandbyPromoteBatch+1)
	require.Error(t, err)

	enabled, threshold, batch, err := NormalizeStandbyPromoteSettings(false, 0, 0)
	require.NoError(t, err)
	assert.False(t, enabled)
	assert.Equal(t, 0, threshold)
	assert.Equal(t, 0, batch)
}

func TestPromoteStandbyPoolFillsUntilAboveThresholdInOrder(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, true, 1, 2)
	createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live")
	first := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "standby-1")
	second := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "standby-2")
	third := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "standby-3")
	manual := createPoolChannel(t, template.Id, common.ChannelStatusManuallyDisabled, "manual")

	promoted, err := PromoteStandbyPool()
	require.NoError(t, err)
	assert.Equal(t, 2, promoted)

	assertChannelStatus(t, first.Id, common.ChannelStatusEnabled)
	assertChannelStatus(t, second.Id, common.ChannelStatusEnabled)
	assertChannelStatus(t, third.Id, common.ChannelStatusStandby)
	assertChannelStatus(t, manual.Id, common.ChannelStatusManuallyDisabled)
	assertAbilityEnabled(t, first.Id, true)
	assertAbilityEnabled(t, third.Id, false)

	promoted, err = PromoteStandbyPool()
	require.NoError(t, err)
	assert.Equal(t, 0, promoted)
	assertChannelStatus(t, third.Id, common.ChannelStatusStandby)
}

func TestPromoteStandbyPoolStaysOffWhenSwitchIsDisabled(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, true, 0, 5)
	waiting := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "waiting")

	_, err := SaveStandbyPoolSettings(false, 0, 5)
	require.NoError(t, err)
	assertChannelStatus(t, waiting.Id, common.ChannelStatusStandby)

	promoted, err := PromoteStandbyPool()
	require.NoError(t, err)
	assert.Equal(t, 0, promoted)
	assertChannelStatus(t, waiting.Id, common.ChannelStatusStandby)
}

func TestListStandbyPoolChannelsOmitsChannelsOutsideThePool(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, false, 0, 0)
	inside := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "inside")
	createPoolChannel(t, 0, common.ChannelStatusEnabled, "outside")

	items, total, err := ListStandbyPoolChannels(0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, inside.Id, items[0].Id)
	assert.Equal(t, template.Name, items[0].TemplateName)
	assert.Equal(t, "default", items[0].Group)
}

func TestPromoteStandbyPoolIsOnePoolAcrossTemplates(t *testing.T) {
	setupStandbyTestDB(t)
	firstTemplate := createStandbyTemplate(t, true, 1, 1)
	secondTemplate := &ChannelTemplate{
		Name:        "other",
		ChannelType: 1,
		Config:      `{"type":1,"models":"gpt-test","group":"default"}`,
	}
	require.NoError(t, secondTemplate.Insert())

	createPoolChannel(t, firstTemplate.Id, common.ChannelStatusEnabled, "live")
	oldest := createPoolChannel(t, secondTemplate.Id, common.ChannelStatusStandby, "other-standby")
	newer := createPoolChannel(t, firstTemplate.Id, common.ChannelStatusStandby, "same-standby")
	outsider := createPoolChannel(t, 0, common.ChannelStatusStandby, "not-in-pool")

	promoted, err := PromoteStandbyPool()
	require.NoError(t, err)
	assert.Equal(t, 1, promoted)
	assertChannelStatus(t, oldest.Id, common.ChannelStatusEnabled)
	assertChannelStatus(t, newer.Id, common.ChannelStatusStandby)
	assertChannelStatus(t, outsider.Id, common.ChannelStatusStandby)
}

func TestUpdateChannelStatusPromotesFromStandbyPool(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, true, 1, 1)
	first := createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live-1")
	createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live-2")
	spare := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "spare")

	require.True(t, UpdateChannelStatus(first.Id, "", common.ChannelStatusAutoDisabled, "upstream error"))
	assertChannelStatus(t, first.Id, common.ChannelStatusAutoDisabled)
	assertChannelStatus(t, spare.Id, common.ChannelStatusEnabled)
	assertAbilityEnabled(t, spare.Id, true)
}

func TestDeleteEnabledChannelPromotesFromStandbyPool(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, true, 0, 1)
	live := createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live")
	spare := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "spare")

	require.NoError(t, live.Delete())
	assertChannelStatus(t, spare.Id, common.ChannelStatusEnabled)

	var gone Channel
	err := DB.First(&gone, live.Id).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestDisableChannelByTagDoesNotPromoteOrConsumeStandby(t *testing.T) {
	setupStandbyTestDB(t)
	template := createStandbyTemplate(t, true, 0, 1)
	tag := "pool-tag"
	live := createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live")
	spare := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "spare")
	require.NoError(t, DB.Model(&Channel{}).Where("id IN ?", []int{live.Id, spare.Id}).Update("tag", tag).Error)

	require.NoError(t, DisableChannelByTag(tag))
	assertChannelStatus(t, live.Id, common.ChannelStatusManuallyDisabled)
	assertChannelStatus(t, spare.Id, common.ChannelStatusStandby)
	assertAbilityEnabled(t, spare.Id, false)
	assertAbilityEnabled(t, live.Id, false)
}

func TestPromotedChannelEntersMemoryRoutingCache(t *testing.T) {
	setupStandbyTestDB(t)
	common.MemoryCacheEnabled = true
	template := createStandbyTemplate(t, true, 0, 1)
	live := createPoolChannel(t, template.Id, common.ChannelStatusEnabled, "live")
	spare := createPoolChannel(t, template.Id, common.ChannelStatusStandby, "spare")
	InitChannelCache()

	require.True(t, UpdateChannelStatus(live.Id, "", common.ChannelStatusAutoDisabled, "upstream error"))

	selected, err := GetRandomSatisfiedChannel("default", "gpt-test", 0, "")
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, spare.Id, selected.Id)
	assert.Equal(t, common.ChannelStatusEnabled, selected.Status)
}

func assertChannelStatus(t *testing.T, id, status int) {
	t.Helper()
	var channel Channel
	require.NoError(t, DB.First(&channel, id).Error)
	assert.Equal(t, status, channel.Status)
}

func assertAbilityEnabled(t *testing.T, channelID int, enabled bool) {
	t.Helper()
	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", channelID).First(&ability).Error)
	assert.Equal(t, enabled, ability.Enabled)
}

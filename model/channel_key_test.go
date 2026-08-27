package model

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelKeyTestDB(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Channel{},
		&ChannelKey{},
		&ChannelKeyQuotaReservation{},
		&ChannelKeyEvent{},
		&ChannelKeyArchive{},
		&Ability{},
	))

	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	InitChannelKeyCache(nil)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
		InitChannelKeyCache(nil)
	})
}

func createChannelWithStableKeys(t *testing.T, keyText string, quotaLimit int64) *Channel {
	t.Helper()

	channel := &Channel{
		Name:          "quota-test",
		Key:           keyText,
		KeyQuotaLimit: quotaLimit,
		Status:        common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, SyncChannelKeys(channel))
	return channel
}

func TestSyncChannelKeysPreservesIdentityAcrossReorder(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a\nkey-b", 0)

	before := CacheGetChannelKeys(channel.Id)
	require.Len(t, before, 2)
	idsByKey := map[string]int64{
		before[0].Key: before[0].Id,
		before[1].Key: before[1].Id,
	}

	channel.Key = "key-b\nkey-a"
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Update("key", channel.Key).Error)
	require.NoError(t, SyncChannelKeys(channel))

	after := CacheGetChannelKeys(channel.Id)
	require.Len(t, after, 2)
	assert.Equal(t, "key-b", after[0].Key)
	assert.Equal(t, idsByKey["key-b"], after[0].Id)
	assert.Equal(t, "key-a", after[1].Key)
	assert.Equal(t, idsByKey["key-a"], after[1].Id)
}

func TestUpdateChannelKeyLimitsStoresNormalizedModelRPMOverrides(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 0)
	key := CacheGetChannelKeys(channel.Id)[0]
	limits := ChannelKeyModelRpmLimits{
		" gpt-4o ":    10,
		"gpt-4o-mini": 0,
	}

	require.NoError(t, UpdateChannelKeyLimits(channel.Id, key.Id, nil, nil, &limits))

	var stored ChannelKey
	require.NoError(t, DB.First(&stored, key.Id).Error)
	assert.Equal(t, ChannelKeyModelRpmLimits{
		"gpt-4o":      10,
		"gpt-4o-mini": 0,
	}, stored.ModelRpmLimits)
	assert.Equal(t, 10, stored.EffectiveRpmLimitForModel(channel, "gpt-4o"))
	assert.Zero(t, stored.EffectiveRpmLimitForModel(channel, "gpt-4o-mini"))
}

func TestEffectiveRpmLimitForModelCombinesDefaultAndModelCaps(t *testing.T) {
	defaultLimit := 5
	key := &ChannelKey{
		RpmLimit: &defaultLimit,
		ModelRpmLimits: ChannelKeyModelRpmLimits{
			"gpt-4o":      2,
			"gpt-4o-mini": 0,
		},
	}
	channel := &Channel{KeyRpmLimit: 9}

	assert.Equal(t, 2, key.EffectiveRpmLimitForModel(channel, "gpt-4o"))
	assert.Equal(t, 5, key.EffectiveRpmLimitForModel(channel, "gpt-4o-mini"))
	assert.Equal(t, 5, key.EffectiveRpmLimitForModel(channel, "claude-3-5-sonnet"))
}

func TestChannelKeyModelRPMLimitsRejectNormalizedDuplicate(t *testing.T) {
	_, err := (ChannelKeyModelRpmLimits{
		"gpt-4o":   10,
		" gpt-4o ": 20,
	}).Normalize()

	require.ErrorContains(t, err, "duplicate model RPM limit")
}

func TestMigrateLegacyChannelKeysRebuildsExistingStableKeySummary(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := &Channel{
		Name:   "existing-stable-key-summary",
		Key:    "key-a",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, SyncChannelKeys(channel))

	key := CacheGetChannelKeys(channel.Id)[0]
	now := common.GetTimestamp()
	require.NoError(t, DB.Model(&ChannelKey{}).Where("id = ?", key.Id).Updates(map[string]any{
		"status":          ChannelKeyStatusQuotaExhausted,
		"disabled_reason": "quota limit exhausted",
		"disabled_at":     now,
	}).Error)

	require.NoError(t, MigrateLegacyChannelKeys())

	require.NoError(t, DB.First(channel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
	assert.Equal(t, 1, channel.ChannelInfo.MultiKeySize)
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.ChannelInfo.MultiKeyStatusList[key.Position])
}

func TestChannelKeyQuotaReservationAllowsFinalRequestThenArchivesAndRestoresKey(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 100)
	key := CacheGetChannelKeys(channel.Id)[0]
	modelRpmLimits := ChannelKeyModelRpmLimits{"gpt-4o": 12}
	require.NoError(t, UpdateChannelKeyLimits(channel.Id, key.Id, nil, nil, &modelRpmLimits))
	key = CacheGetChannelKeys(channel.Id)[0]

	first, err := ReserveChannelKeyQuota("attempt-1", "request-1", key.Id, 80, time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	last, err := ReserveChannelKeyQuota("attempt-2", "request-2", key.Id, 30, time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	assert.Equal(t, int64(20), last.ReservedQuota)
	_, err = ReserveChannelKeyQuota("attempt-3", "request-3", key.Id, 1, time.Now().Add(time.Minute).Unix())
	require.ErrorIs(t, err, ErrChannelKeyQuotaInsufficient)

	result, err := CommitChannelKeyQuota(first.Id, 80)
	require.NoError(t, err)
	assert.False(t, result.Exhausted)

	result, err = CommitChannelKeyQuota(last.Id, 30)
	require.NoError(t, err)
	assert.True(t, result.Exhausted)
	assert.Equal(t, int64(110), result.QuotaUsed)

	var stored ChannelKey
	require.NoError(t, DB.First(&stored, key.Id).Error)
	assert.Equal(t, ChannelKeyStatusQuotaExhausted, stored.Status)
	assert.Zero(t, stored.QuotaReserved)
	require.NoError(t, DB.First(channel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)
	assert.Equal(t, channelStatusReasonQuotaExhausted, channel.GetOtherInfo()["status_reason"])

	var archive ChannelKeyArchive
	require.NoError(t, DB.Where("original_key_id = ? AND restored_at = 0", key.Id).First(&archive).Error)
	assert.Equal(t, "key-a", archive.Key)
	assert.Equal(t, int64(110), archive.QuotaUsed)
	assert.Equal(t, modelRpmLimits, archive.ModelRpmLimits)
	archives, total, err := GetChannelKeyArchivesGlobal(nil, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, archives, 1)
	assert.Equal(t, channel.Id, archives[0].ChannelId)

	require.NoError(t, RestoreChannelKeyArchiveById(archive.Id))
	require.NoError(t, DB.First(&stored, key.Id).Error)
	assert.Equal(t, ChannelKeyStatusEnabled, stored.Status)
	assert.Zero(t, stored.QuotaUsed)
	assert.Equal(t, int64(110), stored.LifetimeQuota)
	require.NoError(t, DB.First(channel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
	require.NoError(t, DB.First(&archive, archive.Id).Error)
	assert.NotZero(t, archive.RestoredAt)
}

func TestChannelKeyQuotaExhaustionSyncsChannelStatusSummary(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := &Channel{
		Name:          "multi-key-quota-summary",
		Key:           "key-a\nkey-b",
		KeyQuotaLimit: 10,
		Status:        common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{ChannelId: channel.Id, Enabled: true}).Error)
	require.NoError(t, SyncChannelKeys(channel))

	keys := CacheGetChannelKeys(channel.Id)
	require.Len(t, keys, 2)
	for i, key := range keys {
		reservation, err := ReserveChannelKeyQuota(
			"summary-attempt-"+key.Key,
			"summary-request-"+key.Key,
			key.Id,
			10,
			time.Now().Add(time.Minute).Unix(),
		)
		require.NoError(t, err)
		_, err = CommitChannelKeyQuota(reservation.Id, 10)
		require.NoError(t, err)

		require.NoError(t, DB.First(channel, channel.Id).Error)
		assert.Equal(t, 2, channel.ChannelInfo.MultiKeySize)
		assert.Len(t, channel.ChannelInfo.MultiKeyStatusList, i+1)
		assert.Equal(t, common.ChannelStatusAutoDisabled, channel.ChannelInfo.MultiKeyStatusList[key.Position])
	}
	assert.Equal(t, common.ChannelStatusAutoDisabled, channel.Status)

	var ability Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.False(t, ability.Enabled)

	var archive ChannelKeyArchive
	require.NoError(t, DB.Where("original_key_id = ? AND restored_at = 0", keys[0].Id).First(&archive).Error)
	require.NoError(t, RestoreChannelKeyArchive(channel.Id, archive.Id))

	require.NoError(t, DB.First(channel, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, channel.Status)
	assert.Len(t, channel.ChannelInfo.MultiKeyStatusList, 1)
	assert.NotContains(t, channel.ChannelInfo.MultiKeyStatusList, keys[0].Position)
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&ability).Error)
	assert.True(t, ability.Enabled)
}

func TestDeleteChannelKeyArchiveRemovesCredentialAndKeepsOtherKey(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a\nkey-b", 10)
	keys := CacheGetChannelKeys(channel.Id)
	require.Len(t, keys, 2)

	reservation, err := ReserveChannelKeyQuota(
		"archive-delete-attempt",
		"archive-delete-request",
		keys[0].Id,
		10,
		time.Now().Add(time.Minute).Unix(),
	)
	require.NoError(t, err)
	_, err = CommitChannelKeyQuota(reservation.Id, 10)
	require.NoError(t, err)

	var archive ChannelKeyArchive
	require.NoError(t, DB.Where("original_key_id = ? AND restored_at = 0", keys[0].Id).First(&archive).Error)
	secret, err := GetChannelKeyArchiveSecret(archive.Id)
	require.NoError(t, err)
	assert.Equal(t, "key-a", secret)

	require.NoError(t, DeleteChannelKeyArchive(archive.Id))

	var storedChannel Channel
	require.NoError(t, DB.First(&storedChannel, channel.Id).Error)
	assert.Equal(t, []string{"key-b"}, storedChannel.GetKeys())

	var deletedKey ChannelKey
	require.NoError(t, DB.First(&deletedKey, keys[0].Id).Error)
	assert.Equal(t, ChannelKeyStatusArchived, deletedKey.Status)
	assert.Empty(t, deletedKey.Key)

	var archiveCount int64
	require.NoError(t, DB.Model(&ChannelKeyArchive{}).Where("original_key_id = ?", keys[0].Id).Count(&archiveCount).Error)
	assert.Zero(t, archiveCount)
	_, err = GetChannelKeyArchiveSecret(archive.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	activeKeys := CacheGetChannelKeys(channel.Id)
	require.Len(t, activeKeys, 1)
	assert.Equal(t, "key-b", activeKeys[0].Key)
	assert.Equal(t, ChannelKeyStatusEnabled, activeKeys[0].Status)
}

func TestReleaseChannelKeyQuotaIsIdempotent(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 100)
	key := CacheGetChannelKeys(channel.Id)[0]

	reservation, err := ReserveChannelKeyQuota("attempt-1", "request-1", key.Id, 60, time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	require.NoError(t, ReleaseChannelKeyQuota(reservation.Id))
	require.NoError(t, ReleaseChannelKeyQuota(reservation.Id))

	var stored ChannelKey
	require.NoError(t, DB.First(&stored, key.Id).Error)
	assert.Zero(t, stored.QuotaReserved)

	_, err = CommitChannelKeyQuota(reservation.Id, 60)
	assert.True(t, errors.Is(err, ErrChannelKeyReservationClosed))
}

func TestChannelKeyArchiveTracksSettlementsAdmittedBeforeExhaustion(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 100)
	key := CacheGetChannelKeys(channel.Id)[0]

	first, err := ReserveChannelKeyQuota(
		"attempt-1",
		"request-1",
		key.Id,
		80,
		time.Now().Add(time.Minute).Unix(),
	)
	require.NoError(t, err)
	last, err := ReserveChannelKeyQuota(
		"attempt-2",
		"request-2",
		key.Id,
		30,
		time.Now().Add(time.Minute).Unix(),
	)
	require.NoError(t, err)
	assert.Equal(t, int64(20), last.ReservedQuota)

	result, err := CommitChannelKeyQuota(last.Id, 110)
	require.NoError(t, err)
	assert.True(t, result.Exhausted)

	result, err = CommitChannelKeyQuota(first.Id, 80)
	require.NoError(t, err)
	assert.True(t, result.Exhausted)
	assert.Equal(t, int64(190), result.QuotaUsed)

	var archive ChannelKeyArchive
	require.NoError(t, DB.Where("original_key_id = ? AND restored_at = 0", key.Id).First(&archive).Error)
	assert.Equal(t, int64(190), archive.QuotaUsed)
	assert.Zero(t, archive.QuotaReserved)
	assert.Equal(t, int64(190), archive.LifetimeQuota)
}

func TestCopyChannelKeyLimitsCopiesPerKeyRPMQuotaAndModelRPM(t *testing.T) {
	setupChannelKeyTestDB(t)
	source := createChannelWithStableKeys(t, "key-a\nkey-b", 100)
	sourceKeys := CacheGetChannelKeys(source.Id)
	require.Len(t, sourceKeys, 2)

	rpmA := 30
	quotaB := int64(500)
	limitsA := ChannelKeyModelRpmLimits{"gpt-4o": 10, "gpt-4o-mini": 0}
	require.NoError(t, UpdateChannelKeyLimits(source.Id, sourceKeys[0].Id, &rpmA, nil, &limitsA))
	require.NoError(t, UpdateChannelKeyLimits(source.Id, sourceKeys[1].Id, nil, &quotaB, nil))
	require.NoError(t, DB.Model(&ChannelKey{}).Where("id = ?", sourceKeys[0].Id).Updates(map[string]any{
		"quota_used":     int64(40),
		"lifetime_quota": int64(80),
		"quota_reserved": int64(5),
	}).Error)

	dest := &Channel{
		Name:          "copied",
		Key:           source.Key,
		Type:          source.Type,
		KeyQuotaLimit: source.KeyQuotaLimit,
		Status:        common.ChannelStatusEnabled,
	}
	require.NoError(t, dest.Insert())
	require.NoError(t, CopyChannelKeyLimits(source.Id, dest.Id))

	copied := CacheGetChannelKeys(dest.Id)
	require.Len(t, copied, 2)
	require.NotNil(t, copied[0].RpmLimit)
	assert.Equal(t, rpmA, *copied[0].RpmLimit)
	assert.Nil(t, copied[0].QuotaLimit)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 10, "gpt-4o-mini": 0}, copied[0].ModelRpmLimits)
	assert.Zero(t, copied[0].QuotaUsed)
	assert.Zero(t, copied[0].QuotaReserved)
	assert.Zero(t, copied[0].LifetimeQuota)

	assert.Nil(t, copied[1].RpmLimit)
	require.NotNil(t, copied[1].QuotaLimit)
	assert.Equal(t, quotaB, *copied[1].QuotaLimit)
	assert.Empty(t, copied[1].ModelRpmLimits)
}

func TestCopyChannelKeyLimitsMatchesDuplicateFingerprintsByPosition(t *testing.T) {
	setupChannelKeyTestDB(t)
	source := createChannelWithStableKeys(t, "shared-key\nshared-key", 0)
	sourceKeys := CacheGetChannelKeys(source.Id)
	require.Len(t, sourceKeys, 2)
	require.Equal(t, sourceKeys[0].Fingerprint, sourceKeys[1].Fingerprint)

	first := ChannelKeyModelRpmLimits{"gpt-4o": 4}
	second := ChannelKeyModelRpmLimits{"gpt-4o": 9}
	require.NoError(t, UpdateChannelKeyLimits(source.Id, sourceKeys[0].Id, nil, nil, &first))
	require.NoError(t, UpdateChannelKeyLimits(source.Id, sourceKeys[1].Id, nil, nil, &second))

	dest := &Channel{
		Name:   "copied-dup",
		Key:    source.Key,
		Type:   source.Type,
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, dest.Insert())
	require.NoError(t, CopyChannelKeyLimits(source.Id, dest.Id))

	copied := CacheGetChannelKeys(dest.Id)
	require.Len(t, copied, 2)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 4}, copied[0].ModelRpmLimits)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 9}, copied[1].ModelRpmLimits)
}

func TestDeleteChannelRemovesActiveAndArchivedCredentials(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 10)
	key := CacheGetChannelKeys(channel.Id)[0]

	reservation, err := ReserveChannelKeyQuota(
		"delete-attempt",
		"delete-request",
		key.Id,
		10,
		time.Now().Add(time.Minute).Unix(),
	)
	require.NoError(t, err)
	_, err = CommitChannelKeyQuota(reservation.Id, 10)
	require.NoError(t, err)

	require.NoError(t, channel.Delete())

	var keyCount, archiveCount, reservationCount, eventCount int64
	require.NoError(t, DB.Model(&ChannelKey{}).Where("channel_id = ?", channel.Id).Count(&keyCount).Error)
	require.NoError(t, DB.Model(&ChannelKeyArchive{}).Where("channel_id = ?", channel.Id).Count(&archiveCount).Error)
	require.NoError(t, DB.Model(&ChannelKeyQuotaReservation{}).Where("channel_id = ?", channel.Id).Count(&reservationCount).Error)
	require.NoError(t, DB.Model(&ChannelKeyEvent{}).Where("channel_id = ?", channel.Id).Count(&eventCount).Error)
	assert.Zero(t, keyCount)
	assert.Zero(t, archiveCount)
	assert.Zero(t, reservationCount)
	assert.Zero(t, eventCount)
}

package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelKeyBillingDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousCache, previousRedis, previousBatch := common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelKey{}, &model.ChannelKeyQuotaReservation{}, &model.ChannelKeyEvent{}, &model.ChannelKeyArchive{}, &model.User{}, &model.Token{}, &model.Log{}, &model.Midjourney{}))
	model.DB, model.LOG_DB = database, database
	common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = false, false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled, common.RedisEnabled, common.BatchUpdateEnabled = previousCache, previousRedis, previousBatch
		model.InitChannelKeyCache(nil)
		require.NoError(t, sqlDB.Close())
	})
	return database
}

func TestPinnedChannelKeyAdmissionCannotUseSiblingCapacity(t *testing.T) {
	database := setupChannelKeyBillingDB(t)
	channel := &model.Channel{Name: "websocket", Key: "connected-key\nsibling-key", Status: common.ChannelStatusEnabled, KeyQuotaLimit: 100}
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, model.SyncChannelKeys(channel))
	keys, err := model.GetActiveChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.NoError(t, database.Model(keys[0]).Updates(map[string]any{"quota_used": 90, "quota_reserved": 10}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{ChannelKeyPinned: true, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelKeyId: keys[0].Id, ApiKey: keys[0].Key}}
	admissionErr := AdmitChannelKey(c, info, channel, 20)
	require.NotNil(t, admissionErr)
	assert.Equal(t, types.ErrorCodeChannelKeyQuotaInsufficient, admissionErr.GetErrorCode())
	assert.Zero(t, info.ChannelKeyQuotaReservationId)
	require.NoError(t, database.Model(keys[0]).Update("quota_reserved", 0).Error)
	require.Nil(t, AdmitChannelKey(c, info, channel, 5))
	selected, ok := common.GetContextKeyType[int64](c, constant.ContextKeyChannelKeyId)
	require.True(t, ok)
	assert.Equal(t, keys[0].Id, selected)
	ReleaseChannelKeyQuotaReservation(info)
	require.NoError(t, database.First(keys[1], keys[1].Id).Error)
	assert.Zero(t, keys[1].QuotaReserved)
	assert.Zero(t, keys[1].QuotaUsed)
}

func TestMidjourneySettlesAndRefundsKeyAndUserQuotasOnce(t *testing.T) {
	database := setupChannelKeyBillingDB(t)
	seedUser(t, 1, 10000)
	seedToken(t, 1, 1, "mj-token", 5000)
	channel := &model.Channel{Name: "mj-key-accounting", Key: "mj-key", Status: common.ChannelStatusEnabled, KeyQuotaLimit: 10000}
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, model.SyncChannelKeys(channel))
	keys, err := model.GetActiveChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	reservation, err := model.ReserveChannelKeyQuota("mj-attempt", "mj-request", keys[0].Id, 3000, time.Now().Add(time.Minute).Unix())
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{UserId: 1, TokenId: 1, TokenKey: "mj-token", UserQuota: 10000, ChannelKeyQuotaReservationId: reservation.Id, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelKeyId: keys[0].Id}}
	task := &model.Midjourney{UserId: 1, ChannelId: channel.Id, MjId: "mj-refund", Action: "IMAGINE"}
	prepared, err := PrepareMidjourneyTaskBilling(info, task, 3000, true)
	require.NoError(t, err)
	require.NoError(t, task.Insert())
	billed, err := SettleMidjourneyTaskBilling(info, task, prepared)
	require.NoError(t, err)
	require.True(t, billed)
	ReleaseChannelKeyQuotaReservation(info)
	assert.Equal(t, 7000, getUserQuota(t, 1))
	assert.Equal(t, 2000, getTokenRemainQuota(t, 1))
	require.NoError(t, database.First(keys[0], keys[0].Id).Error)
	assert.EqualValues(t, 3000, keys[0].QuotaUsed)
	assert.Zero(t, keys[0].QuotaReserved)
	seedChargedAccounting(t, 1, channel.Id, 1, 3000, 1)
	persisted := getMidjourneyTask(t, task.Id)
	require.True(t, RefundMidjourneyQuota(t.Context(), &persisted, "failed"))
	require.True(t, RefundMidjourneyQuota(t.Context(), &persisted, "failed again"))
	assert.Equal(t, 10000, getUserQuota(t, 1))
	assert.Equal(t, 5000, getTokenRemainQuota(t, 1))
	require.NoError(t, database.First(keys[0], keys[0].Id).Error)
	assert.Zero(t, keys[0].QuotaUsed)
	assert.Zero(t, getMidjourneyTask(t, task.Id).ChannelKeyQuota)
}

func TestArchivedKeyProofCannotAuthorizeCurrentChannelKey(t *testing.T) {
	archived, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeChannelKeyRead, Context: []byte(`{"archive_id":123}`)})
	require.NoError(t, err)
	current, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeChannelKeyRead, Context: []byte(`{"channel_id":123}`)})
	require.NoError(t, err)
	assert.NotEqual(t, current.ContextHash, archived.ContextHash)
	for _, raw := range []string{`{"archive_id":0}`, `{"archive_id":-1}`, `{"archive_id":1.5}`, `{"archive_id":null}`, `{"archive_id":123,"channel_id":123}`} {
		_, err := BindVerificationOperation(VerificationOperation{Scope: VerificationScopeChannelKeyRead, Context: []byte(raw)})
		assert.ErrorIs(t, err, ErrVerificationContextInvalid)
	}
}

func TestMidjourneyKeyRefundRollsBackAndRejectsStaleReplay(t *testing.T) {
	database := setupChannelKeyBillingDB(t)
	channel := &model.Channel{Name: "mj-atomic-refund", Key: "fixture-key", Status: common.ChannelStatusEnabled, KeyQuotaLimit: 10000}
	require.NoError(t, database.Create(channel).Error)
	require.NoError(t, model.SyncChannelKeys(channel))
	keys, err := model.GetActiveChannelKeys(channel.Id)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.NoError(t, model.AdjustChannelKeyQuota(keys[0].Id, 300, "charge", "submitted"))
	task := &model.Midjourney{MjId: "atomic-refund", ChannelKeyId: keys[0].Id, ChannelKeyQuota: 300}
	require.NoError(t, task.Insert())
	stale := *task
	require.NoError(t, database.Callback().Update().Before("gorm:update").Register("test:refund-marker-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "midjourneys" {
			tx.AddError(errors.New("refund marker write failed"))
		}
	}))
	err = task.RefundChannelKeyQuota("failed")
	require.NoError(t, database.Callback().Update().Remove("test:refund-marker-failure"))
	require.ErrorContains(t, err, "refund marker write failed")
	require.NoError(t, database.First(keys[0], keys[0].Id).Error)
	assert.EqualValues(t, 300, keys[0].QuotaUsed)
	assert.Equal(t, 300, getMidjourneyTask(t, task.Id).ChannelKeyQuota)
	require.NoError(t, task.RefundChannelKeyQuota("failed"))
	require.NoError(t, model.AdjustChannelKeyQuota(keys[0].Id, 100, "later-charge", "submitted"))
	require.NoError(t, stale.RefundChannelKeyQuota("replayed"))
	require.NoError(t, database.First(keys[0], keys[0].Id).Error)
	assert.EqualValues(t, 100, keys[0].QuotaUsed)
}

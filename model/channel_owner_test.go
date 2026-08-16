package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSupplierChannelQueriesEnforceOwnerAndOmitKey(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	owned := Channel{OwnerUserId: 10, Name: "owned", Key: "secret-key", Models: "model-a", Group: "default"}
	other := Channel{OwnerUserId: 20, Name: "other", Key: "other-secret", Models: "model-b", Group: "default"}
	legacy := Channel{OwnerUserId: 0, Name: "legacy", Key: "legacy-secret"}
	require.NoError(t, db.Create(&owned).Error)
	require.NoError(t, db.Create(&other).Error)
	require.NoError(t, db.Create(&legacy).Error)

	channel, err := GetChannelByIdAndOwner(owned.Id, 10, false)
	require.NoError(t, err)
	assert.Equal(t, owned.Id, channel.Id)
	assert.Empty(t, channel.Key)

	_, err = GetChannelByIdAndOwner(other.Id, 10, false)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = GetChannelByIdAndOwner(legacy.Id, 10, false)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	channels, total, err := GetChannelsByOwner(10, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, channels, 1)
	assert.Equal(t, owned.Id, channels[0].Id)
	assert.Empty(t, channels[0].Key)

	owned.Name = "updated"
	owned.Models = "model-c"
	require.NoError(t, UpdateSupplierOwnedChannel(&owned, 10))
	var stored Channel
	require.NoError(t, db.First(&stored, owned.Id).Error)
	assert.Equal(t, "updated", stored.Name)
	assert.Equal(t, "model-c", stored.Models)
	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", owned.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "model-c", abilities[0].Model)

	other.Name = "must-not-update"
	require.ErrorIs(t, UpdateSupplierOwnedChannel(&other, 10), gorm.ErrRecordNotFound)
	stored = Channel{}
	require.NoError(t, db.First(&stored, other.Id).Error)
	assert.Equal(t, "other", stored.Name)
}

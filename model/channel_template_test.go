package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTemplateNameSuffixUsesKeyTail(t *testing.T) {
	assert.Equal(t, "abcdef", ChannelTemplateNameSuffix("sk-proj-xxxxabcdef", 6))
	assert.Equal(t, "rt1234", ChannelTemplateNameSuffix("short1234", 6))
	assert.Equal(t, "cdef12", ChannelTemplateNameSuffix("sk-abc-def-12", 6))
}

func TestChannelTemplateNameSuffixUsesCredentialIdentity(t *testing.T) {
	assert.Equal(
		t,
		"abcdef",
		ChannelTemplateNameSuffix(`{"client_email":"svc-abcdef@project.iam.gserviceaccount.com"}`, 6),
	)
	assert.Equal(t, "abcdef", ChannelTemplateNameSuffix(`{"account_id":"acct-zzabcdef"}`, 6))
	assert.Equal(t, "KEYID12", ChannelTemplateNameSuffix("AKIAKEYID12|secret|us-east-1", 7))
}

func TestChannelTemplateNameSuffixFallsBackToHashWhenNoAlphanumeric(t *testing.T) {
	suffix := ChannelTemplateNameSuffix("----", 6)
	require.Len(t, suffix, 6)
	assert.NotEqual(t, "----", suffix)
}

func TestBuildChannelTemplateChannelName(t *testing.T) {
	assert.Equal(t, "openai-abcdef", BuildChannelTemplateChannelName("openai", "sk-xxxxabcdef", 6))
	assert.Equal(t, "abcdef", BuildChannelTemplateChannelName("  ", "sk-xxxxabcdef", 6))
}

func TestChannelTemplateConfigOmitsKeyAndCopiesSingleKeyModelRPM(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "sk-secret-value", 0)
	channel.Type = 1
	channel.Models = "gpt-4o"
	channel.Group = "default"
	channel.KeyRpmLimit = 20
	require.NoError(t, DB.Save(channel).Error)

	key := CacheGetChannelKeys(channel.Id)[0]
	limits := ChannelKeyModelRpmLimits{"gpt-4o": 8}
	require.NoError(t, UpdateChannelKeyLimits(channel.Id, key.Id, nil, nil, &limits))

	cfg, err := ChannelTemplateConfigFromChannel(channel)
	require.NoError(t, err)
	assert.Equal(t, 20, cfg.KeyRpmLimit)
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 8}, cfg.ModelRpmLimits)

	encoded, err := common.Marshal(cfg)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "sk-secret-value")

	built := cfg.ToChannel()
	assert.Empty(t, built.Key)
	assert.Equal(t, "gpt-4o", built.Models)
}

func TestParseChannelTemplateConfigDropsUnknownKeyField(t *testing.T) {
	raw := `{"type":1,"models":"gpt-4o","group":"default","key":"sk-leaked","status":1}`
	cfg, err := ParseChannelTemplateConfig(raw)
	require.NoError(t, err)
	assert.Empty(t, cfg.ToChannel().Key)
	assert.Equal(t, "gpt-4o", cfg.Models)
}

func TestAllocateUniqueChannelNameAddsSuffixOnCollision(t *testing.T) {
	setupChannelKeyTestDB(t)
	require.NoError(t, DB.Create(&Channel{Name: "openai-abcdef", Key: "k1"}).Error)

	reserved := map[string]struct{}{}
	first, err := AllocateUniqueChannelName("openai-abcdef", reserved)
	require.NoError(t, err)
	assert.Equal(t, "openai-abcdef-2", first)

	second, err := AllocateUniqueChannelName("openai-abcdef", reserved)
	require.NoError(t, err)
	assert.Equal(t, "openai-abcdef-3", second)
}

func TestApplyChannelKeyModelRpmLimitsUpdatesActiveKeys(t *testing.T) {
	setupChannelKeyTestDB(t)
	channel := createChannelWithStableKeys(t, "key-a", 0)
	require.NoError(t, ApplyChannelKeyModelRpmLimits(channel.Id, ChannelKeyModelRpmLimits{"gpt-4o": 9}))

	stored := CacheGetChannelKeys(channel.Id)[0]
	assert.Equal(t, ChannelKeyModelRpmLimits{"gpt-4o": 9}, stored.ModelRpmLimits)
}

package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChannelHealthMinTotal(t *testing.T) {
	require.NoError(t, ValidateChannelHealthMinTotal("1"))
	require.NoError(t, ValidateChannelHealthMinTotal("30"))
	require.NoError(t, ValidateChannelHealthMinTotal("1000"))
	require.Error(t, ValidateChannelHealthMinTotal(""))
	require.Error(t, ValidateChannelHealthMinTotal("0"))
	require.Error(t, ValidateChannelHealthMinTotal("1001"))
	require.Error(t, ValidateChannelHealthMinTotal("abc"))
}

func TestValidateChannelHealthAlertBelowPercent(t *testing.T) {
	require.NoError(t, ValidateChannelHealthAlertBelowPercent("1"))
	require.NoError(t, ValidateChannelHealthAlertBelowPercent("50"))
	require.NoError(t, ValidateChannelHealthAlertBelowPercent("100"))
	require.Error(t, ValidateChannelHealthAlertBelowPercent(""))
	require.Error(t, ValidateChannelHealthAlertBelowPercent("0"))
	require.Error(t, ValidateChannelHealthAlertBelowPercent("101"))
}

func TestGetChannelHealthAlertBelowPercentClampsInvalidValues(t *testing.T) {
	orig := ChannelHealthAlertBelowPercent
	t.Cleanup(func() { ChannelHealthAlertBelowPercent = orig })

	ChannelHealthAlertBelowPercent = 0
	require.Equal(t, DefaultChannelHealthAlertBelowPercent, GetChannelHealthAlertBelowPercent())
	ChannelHealthAlertBelowPercent = 200
	require.Equal(t, MaxChannelHealthAlertBelowPercent, GetChannelHealthAlertBelowPercent())
	ChannelHealthAlertBelowPercent = 80
	require.Equal(t, 80, GetChannelHealthAlertBelowPercent())
}

func TestGetChannelHealthMinTotalClampsInvalidValues(t *testing.T) {
	orig := ChannelHealthMinTotal
	t.Cleanup(func() { ChannelHealthMinTotal = orig })

	ChannelHealthMinTotal = 0
	require.Equal(t, DefaultChannelHealthMinTotal, GetChannelHealthMinTotal())
	ChannelHealthMinTotal = 2000
	require.Equal(t, MaxChannelHealthMinTotal, GetChannelHealthMinTotal())
	ChannelHealthMinTotal = 5
	require.Equal(t, 5, GetChannelHealthMinTotal())
}

func TestValidateChannelHealthWebhookURL(t *testing.T) {
	require.NoError(t, ValidateChannelHealthWebhookURL(""))
	require.NoError(t, ValidateChannelHealthWebhookURL("  https://example.com/hook  "))
	require.NoError(t, ValidateChannelHealthWebhookURL("http://hooks.example.com/channel-health"))
	require.Error(t, ValidateChannelHealthWebhookURL("not-a-url"))
	require.Error(t, ValidateChannelHealthWebhookURL("ftp://example.com/hook"))
	require.Error(t, ValidateChannelHealthWebhookURL("javascript:alert(1)"))
}

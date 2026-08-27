package operation_setting

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRelayTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "zero is unlimited", value: "0"},
		{name: "positive seconds", value: "60"},
		{name: "trimmed", value: " 15 "},
		{name: "negative", value: "-1", wantErr: true},
		{name: "not an integer", value: "1.5", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "text", value: "abc", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRelayTimeoutSeconds(test.value)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestApplyEnvRelayTimeoutIfUnset(t *testing.T) {
	orig := generalSetting
	t.Cleanup(func() { generalSetting = orig })

	generalSetting.RelayTimeoutSeconds = 0
	ApplyEnvRelayTimeoutIfUnset(90)
	assert.Equal(t, 90, generalSetting.RelayTimeoutSeconds)

	ApplyEnvRelayTimeoutIfUnset(30)
	assert.Equal(t, 90, generalSetting.RelayTimeoutSeconds)

	generalSetting.RelayTimeoutSeconds = 0
	ApplyEnvRelayTimeoutIfUnset(0)
	assert.Equal(t, 0, generalSetting.RelayTimeoutSeconds)
}

func TestWithRelayTimeout(t *testing.T) {
	orig := generalSetting
	t.Cleanup(func() { generalSetting = orig })

	t.Run("disabled", func(t *testing.T) {
		generalSetting.RelayTimeoutSeconds = 0
		parent, cancelParent := context.WithCancel(context.Background())
		defer cancelParent()
		ctx, cancel := WithRelayTimeout(parent)
		defer cancel()

		_, hasDeadline := ctx.Deadline()
		assert.False(t, hasDeadline)

		cancelParent()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("enabled", func(t *testing.T) {
		generalSetting.RelayTimeoutSeconds = 30
		ctx, cancel := WithRelayTimeout(context.Background())
		defer cancel()

		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
	})
}

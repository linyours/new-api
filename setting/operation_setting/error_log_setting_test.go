package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateErrorLogRetainDays(t *testing.T) {
	require.NoError(t, ValidateErrorLogRetainDays("1"))
	require.NoError(t, ValidateErrorLogRetainDays("3"))
	require.NoError(t, ValidateErrorLogRetainDays("365"))
	require.Error(t, ValidateErrorLogRetainDays("0"))
	require.Error(t, ValidateErrorLogRetainDays("366"))
	require.Error(t, ValidateErrorLogRetainDays("abc"))
	require.Error(t, ValidateErrorLogRetainDaysInt(0))
	require.Error(t, ValidateErrorLogRetainDaysInt(366))
}

func TestNormalizeErrorLogRetainDays(t *testing.T) {
	assert.Equal(t, 1, NormalizeErrorLogRetainDays(0))
	assert.Equal(t, 1, NormalizeErrorLogRetainDays(-4))
	assert.Equal(t, 365, NormalizeErrorLogRetainDays(900))
	assert.Equal(t, 3, NormalizeErrorLogRetainDays(3))
}

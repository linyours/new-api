package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogRecordsTableRoutesErrorType(t *testing.T) {
	assert.Equal(t, errorLogsTable, logRecordsTable(LogTypeError))
	assert.Equal(t, "logs", logRecordsTable(LogTypeUnknown))
	assert.Equal(t, "logs", logRecordsTable(LogTypeConsume))
	assert.Equal(t, "logs", logRecordsTable(LogTypeTopup))
}

func TestErrorLogsAreIsolatedFromUsageLogs(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()

	require.NoError(t, createLog(&Log{
		Type:      LogTypeConsume,
		Content:   "usage",
		CreatedAt: now,
		TokenId:   7,
		RequestId: "usage-1",
	}))
	require.NoError(t, createErrorLog(&Log{
		Type:      LogTypeError,
		Content:   "boom",
		CreatedAt: now,
		TokenId:   7,
		RequestId: "error-1",
	}))

	var usageCount, errorCount int64
	require.NoError(t, LOG_DB.Table("logs").Count(&usageCount).Error)
	require.NoError(t, LOG_DB.Table(errorLogsTable).Count(&errorCount).Error)
	assert.Equal(t, int64(1), usageCount)
	assert.Equal(t, int64(1), errorCount)

	errorLogs, errorTotal, err := GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), errorTotal)
	require.Len(t, errorLogs, 1)
	assert.Equal(t, "boom", errorLogs[0].Content)
	assert.Equal(t, LogTypeError, errorLogs[0].Type)

	consumeLogs, consumeTotal, err := GetAllLogs(LogTypeConsume, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), consumeTotal)
	require.Len(t, consumeLogs, 1)
	assert.Equal(t, "usage", consumeLogs[0].Content)

	allLogs, allTotal, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), allTotal)
	require.Len(t, allLogs, 1)
	assert.Equal(t, "usage", allLogs[0].Content)

	tokenLogs, err := GetLogByTokenId(7)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 2)
}

func TestTruncateErrorLogsDoesNotTouchUsageLogs(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	require.NoError(t, createLog(&Log{Type: LogTypeConsume, Content: "keep", CreatedAt: now, RequestId: "u"}))
	require.NoError(t, createErrorLog(&Log{Type: LogTypeError, Content: "drop", CreatedAt: now, RequestId: "e"}))

	require.NoError(t, TruncateErrorLogs(context.Background()))

	count, err := CountErrorLogs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	var usageCount int64
	require.NoError(t, LOG_DB.Table("logs").Count(&usageCount).Error)
	assert.Equal(t, int64(1), usageCount)
}

func TestDeleteErrorLogsBeforeBatchKeepsRecentRows(t *testing.T) {
	truncateTables(t)
	oldTs := time.Now().Add(-72 * time.Hour).Unix()
	newTs := time.Now().Unix()
	require.NoError(t, createErrorLog(&Log{Type: LogTypeError, Content: "old", CreatedAt: oldTs, RequestId: "old"}))
	require.NoError(t, createErrorLog(&Log{Type: LogTypeError, Content: "new", CreatedAt: newTs, RequestId: "new"}))
	require.NoError(t, createLog(&Log{Type: LogTypeConsume, Content: "usage-old", CreatedAt: oldTs, RequestId: "usage-old"}))

	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	deleted, err := DeleteErrorLogsBeforeBatch(context.Background(), cutoff, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	remaining, err := CountErrorLogs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), remaining)

	logs, total, err := GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, "new", logs[0].Content)

	var usageCount int64
	require.NoError(t, LOG_DB.Table("logs").Count(&usageCount).Error)
	assert.Equal(t, int64(1), usageCount)
}

func TestClickHouseErrorLogDropPartitionSQLRejectsInvalidNames(t *testing.T) {
	sql, err := clickHouseErrorLogDropPartitionSQL("20240102")
	require.NoError(t, err)
	assert.Equal(t, "ALTER TABLE error_logs DROP PARTITION '20240102'", sql)

	_, err = clickHouseErrorLogDropPartitionSQL("2024-01-02")
	require.Error(t, err)
	_, err = clickHouseErrorLogDropPartitionSQL("20240102'; DROP TABLE logs; --")
	require.Error(t, err)
	_, err = clickHouseErrorLogDropPartitionSQL("")
	require.Error(t, err)
	assert.False(t, isClickHouseDayPartition("abc"))
	assert.True(t, isClickHouseDayPartition("20260301"))
}

func TestClickHouseErrorLogCreateTableSQLUsesDailyPartitions(t *testing.T) {
	sql := clickHouseErrorLogCreateTableSQL(0)
	assert.Contains(t, sql, "CREATE TABLE IF NOT EXISTS error_logs")
	assert.Contains(t, sql, "PARTITION BY toYYYYMMDD(toDateTime(created_at))")
	assert.NotContains(t, sql, "TTL ")

	withTTL := clickHouseErrorLogCreateTableSQL(3)
	assert.Contains(t, withTTL, "TTL toDateTime(created_at) + INTERVAL 3 DAY DELETE")
}

func TestClickHouseErrorLogTTLDaysFollowsSetting(t *testing.T) {
	setting := operation_setting.GetErrorLogSetting()
	originalEnabled := setting.AutoCleanupEnabled
	originalDays := setting.RetainDays
	t.Cleanup(func() {
		setting.AutoCleanupEnabled = originalEnabled
		setting.RetainDays = originalDays
	})

	setting.AutoCleanupEnabled = false
	setting.RetainDays = 7
	assert.Equal(t, 0, clickHouseErrorLogTTLDays())

	setting.AutoCleanupEnabled = true
	setting.RetainDays = 3
	assert.Equal(t, 3, clickHouseErrorLogTTLDays())

	setting.RetainDays = 0
	assert.Equal(t, 1, clickHouseErrorLogTTLDays())
}

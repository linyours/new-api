package model

import (
	"context"
	"fmt"
	"regexp"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

const errorLogsTable = "error_logs"

// ErrorLog is the dedicated store for type=5 relay failures. Index names are
// unique so PostgreSQL can coexist with the usage `logs` table in one schema.
type ErrorLog struct {
	Id                int    `json:"id" gorm:"index:idx_error_logs_created_at_id,priority:2;index:idx_error_logs_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index:idx_error_logs_user_id;index:idx_error_logs_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_error_logs_created_at_id,priority:1"`
	Type              int    `json:"type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index:idx_error_logs_username;index:idx_error_logs_username_model,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index:idx_error_logs_token_name;default:''"`
	ModelName         string `json:"model_name" gorm:"index:idx_error_logs_model_name;index:idx_error_logs_username_model,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index:idx_error_logs_channel_id"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index:idx_error_logs_token_id"`
	Group             string `json:"group" gorm:"index:idx_error_logs_group"`
	Ip                string `json:"ip" gorm:"index:idx_error_logs_ip;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_error_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_error_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

func (ErrorLog) TableName() string {
	return errorLogsTable
}

func createErrorLog(log *Log) error {
	ensureLogRequestId(log)
	log.Type = LogTypeError
	return LOG_DB.Table(errorLogsTable).Create(log).Error
}

func logRecordsTable(logType int) string {
	if logType == LogTypeError {
		return errorLogsTable
	}
	return "logs"
}

func newLogListTx(logType int) *gorm.DB {
	table := logRecordsTable(logType)
	tx := LOG_DB.Table(table + " AS logs")
	if logType != LogTypeUnknown && logType != LogTypeError {
		tx = tx.Where("logs.type = ?", logType)
	}
	return tx
}

func CountErrorLogs(ctx context.Context) (int64, error) {
	var total int64
	err := LOG_DB.WithContext(ctx).Table(errorLogsTable).Count(&total).Error
	return total, err
}

func CountErrorLogsBefore(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	err := LOG_DB.WithContext(ctx).Table(errorLogsTable).Where("created_at < ?", targetTimestamp).Count(&total).Error
	return total, err
}

func TruncateErrorLogs(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
		return LOG_DB.WithContext(ctx).Where("1 = 1").Delete(&ErrorLog{}).Error
	}
	if err := LOG_DB.WithContext(ctx).Exec("TRUNCATE TABLE " + errorLogsTable).Error; err == nil {
		return nil
	}
	// Some MySQL accounts cannot TRUNCATE; fall back to a full delete.
	return LOG_DB.WithContext(ctx).Where("1 = 1").Delete(&ErrorLog{}).Error
}

func DeleteErrorLogsBeforeBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 1000
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return dropClickHouseErrorLogPartitionsBefore(ctx, targetTimestamp)
	}

	if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
		result := LOG_DB.WithContext(ctx).Exec(
			"DELETE FROM "+errorLogsTable+" WHERE id IN (SELECT id FROM "+errorLogsTable+" WHERE created_at < ? ORDER BY id LIMIT ?)",
			targetTimestamp,
			limit,
		)
		if result.Error != nil {
			return 0, result.Error
		}
		return result.RowsAffected, nil
	}

	result := LOG_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&ErrorLog{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

var clickHouseDayPartitionPattern = regexp.MustCompile(`^\d{8}$`)

func isClickHouseDayPartition(partition string) bool {
	return clickHouseDayPartitionPattern.MatchString(partition)
}

func clickHouseErrorLogDropPartitionSQL(partition string) (string, error) {
	if !isClickHouseDayPartition(partition) {
		return "", fmt.Errorf("invalid clickhouse partition %q", partition)
	}
	return "ALTER TABLE " + errorLogsTable + " DROP PARTITION '" + partition + "'", nil
}

func dropClickHouseErrorLogPartitionsBefore(ctx context.Context, targetTimestamp int64) (int64, error) {
	before, err := CountErrorLogsBefore(ctx, targetTimestamp)
	if err != nil {
		return 0, err
	}
	if before == 0 {
		return 0, nil
	}

	var partitions []string
	err = LOG_DB.WithContext(ctx).Raw(
		`SELECT DISTINCT partition FROM system.parts WHERE database = currentDatabase() AND table = ? AND active AND toInt64OrZero(partition) < toYYYYMMDD(toDateTime(?))`,
		errorLogsTable,
		targetTimestamp,
	).Scan(&partitions).Error
	if err != nil {
		return 0, err
	}

	for _, partition := range partitions {
		sql, sqlErr := clickHouseErrorLogDropPartitionSQL(partition)
		if sqlErr != nil {
			common.SysLog("skip invalid error_logs partition: " + sqlErr.Error())
			continue
		}
		if execErr := LOG_DB.WithContext(ctx).Exec(sql).Error; execErr != nil {
			return 0, execErr
		}
	}

	after, err := CountErrorLogsBefore(ctx, targetTimestamp)
	if err != nil {
		return 0, err
	}
	deleted := before - after
	if deleted < 0 {
		return 0, nil
	}
	return deleted, nil
}

func clickHouseErrorLogTTLDays() int {
	setting := operation_setting.GetErrorLogSetting()
	if setting == nil || !setting.AutoCleanupEnabled {
		return 0
	}
	return operation_setting.NormalizeErrorLogRetainDays(setting.RetainDays)
}

func SyncClickHouseErrorLogTTL() {
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return
	}
	if LOG_DB == nil {
		return
	}
	setting := operation_setting.GetErrorLogSetting()
	ttlDays := 0
	if setting.AutoCleanupEnabled {
		ttlDays = operation_setting.NormalizeErrorLogRetainDays(setting.RetainDays)
	}
	if err := syncClickHouseErrorLogTTL(ttlDays); err != nil {
		common.SysLog("failed to sync error_logs clickhouse TTL: " + err.Error())
	}
}

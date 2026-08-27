package operation_setting

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ErrorLogMinRetainDays = 1
	ErrorLogMaxRetainDays = 365
)

type ErrorLogSetting struct {
	AutoCleanupEnabled bool `json:"auto_cleanup_enabled"`
	RetainDays         int  `json:"retain_days"`
}

var errorLogSetting = ErrorLogSetting{
	AutoCleanupEnabled: true,
	RetainDays:         3,
}

func init() {
	config.GlobalConfig.Register("error_log_setting", &errorLogSetting)
}

func GetErrorLogSetting() *ErrorLogSetting {
	return &errorLogSetting
}

func NormalizeErrorLogRetainDays(days int) int {
	if days < ErrorLogMinRetainDays {
		return ErrorLogMinRetainDays
	}
	if days > ErrorLogMaxRetainDays {
		return ErrorLogMaxRetainDays
	}
	return days
}

func ValidateErrorLogRetainDaysInt(days int) error {
	if days < ErrorLogMinRetainDays || days > ErrorLogMaxRetainDays {
		return fmt.Errorf("error log retain days must be between %d and %d", ErrorLogMinRetainDays, ErrorLogMaxRetainDays)
	}
	return nil
}

func ValidateErrorLogRetainDays(value string) error {
	days, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("error log retain days must be an integer")
	}
	return ValidateErrorLogRetainDaysInt(days)
}

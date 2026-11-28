package model

import (
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestGlobalModelRPMOptionDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "global_rpm_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousOptions := DB, common.OptionMap
			previousConfig := setting.GlobalModelRateLimitJSON()
			DB, common.OptionMap = db, make(map[string]string)
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&Option{}, &SystemTask{}))
				DB, common.OptionMap = previousDB, previousOptions
				require.NoError(t, setting.UpdateGlobalModelRateLimit(previousConfig))
				require.NoError(t, sqlDB.Close())
			})
			for range 2 {
				require.NoError(t, db.AutoMigrate(&Option{}, &SystemTask{}))
			}
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)

			key := setting.GlobalModelRateLimitOptionKey
			value := `{"enabled":true,"limits":{"matrix-model":7}}`
			require.NoError(t, UpdateOption(key, value))
			require.NoError(t, UpdateOption(key, value)) // Repeated saves keep a single option row.
			var stored Option
			require.NoError(t, db.Where(&Option{Key: key}).First(&stored).Error)
			assert.JSONEq(t, value, stored.Value)
			var count int64
			require.NoError(t, db.Model(&Option{}).Where(&Option{Key: key}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			require.NoError(t, setting.UpdateGlobalModelRateLimit(`{"enabled":false,"limits":{}}`))
			require.NoError(t, LoadOptionsFromDatabase())
			assert.Equal(t, 7, setting.GlobalModelRPM("matrix-model"), "persisted configuration must survive reload")

			assert.Error(t, UpdateOptionsBulk(map[string]string{key: `{"enabled":true,"limits":{"matrix-model":-1}}`}))
			require.NoError(t, db.Where(&Option{Key: key}).First(&stored).Error)
			assert.JSONEq(t, value, stored.Value)
			assert.Equal(t, 7, setting.GlobalModelRPM("matrix-model"))
			// A periodic reload racing a save must not republish the old limit.
			value = `{"enabled":true,"limits":{"matrix-model":9}}`
			var saveErr, reloadErr error
			var workers sync.WaitGroup
			start := make(chan struct{})
			workers.Go(func() { <-start; saveErr = UpdateOption(key, value) })
			workers.Go(func() { <-start; reloadErr = LoadOptionsFromDatabase() })
			close(start)
			workers.Wait()
			require.NoError(t, saveErr)
			require.NoError(t, reloadErr)
			assert.Equal(t, 9, setting.GlobalModelRPM("matrix-model"))
			require.NoError(t, db.Where(&Option{Key: key}).First(&stored).Error)
			assert.JSONEq(t, value, stored.Value)
			require.NoError(t, db.Migrator().DropTable(&Option{}))
			assert.Error(t, UpdateOption(key, `{"enabled":false,"limits":{}}`))
			assert.Equal(t, 9, setting.GlobalModelRPM("matrix-model"), "a failed database write must not disable protection")
			assert.JSONEq(t, value, common.OptionMap[key])
		})
	}
}

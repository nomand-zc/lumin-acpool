package bootstrap

import (
	"fmt"

	"github.com/nomand-zc/lumin-acpool/cli/internal/config"

	// Memory store
	storeMemory "github.com/nomand-zc/lumin-acpool/storage/memory"

	// MySQL store
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"

	// Redis store
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"

	// SQLite store
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

const defaultInstanceName = "default"

// initStorage 根据 StorageConfig 初始化所有存储实例。
func initStorage(cfg config.StorageConfig, deps *Dependencies) error {
	switch cfg.Driver {
	case "memory":
		return initMemoryStorage(deps)
	case "mysql":
		return initMySQLStorage(cfg.DSN, deps)
	case "redis":
		return initRedisStorage(cfg.DSN, deps)
	case "sqlite":
		return initSQLiteStorage(cfg.DSN, deps)
	default:
		return fmt.Errorf("unsupported storage driver: %q", cfg.Driver)
	}
}

// initMemoryStorage 初始化统一的内存存储。
// 所有接口共享同一个 Store 实例。
func initMemoryStorage(deps *Dependencies) error {
	store := storeMemory.NewStore()

	deps.AccountStorage = store
	deps.ProviderStorage = store
	deps.StatsStore = store
	deps.UsageStore = store
	deps.OccupancyStore = store
	deps.AffinityStore = store

	return nil
}

// initMySQLStorage 注册 MySQL 实例并初始化统一的 store。
// 所有接口共享同一个 Store 实例和数据库连接。
func initMySQLStorage(dsn string, deps *Dependencies) error {
	storeMysql.RegisterInstance(defaultInstanceName, storeMysql.WithClientBuilderDSN(dsn))

	store, err := storeMysql.NewStore(storeMysql.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql store: %w", err)
	}

	deps.AccountStorage = store
	deps.ProviderStorage = store
	deps.StatsStore = store
	deps.UsageStore = store
	deps.OccupancyStore = store
	deps.AffinityStore = store

	return nil
}

// initRedisStorage 注册 Redis 实例并初始化统一的 store。
// 所有接口共享同一个 Store 实例和 Redis 连接。
func initRedisStorage(dsn string, deps *Dependencies) error {
	storeRedis.RegisterInstance(defaultInstanceName, storeRedis.WithClientBuilderDSN(dsn))

	store, err := storeRedis.NewStore(storeRedis.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis store: %w", err)
	}

	deps.AccountStorage = store
	deps.ProviderStorage = store
	deps.StatsStore = store
	deps.UsageStore = store
	deps.OccupancyStore = store
	deps.AffinityStore = store

	return nil
}

// initSQLiteStorage 注册 SQLite 实例并初始化统一的 store。
// 所有接口共享同一个 Store 实例和数据库连接。
func initSQLiteStorage(dsn string, deps *Dependencies) error {
	storeSqlite.RegisterInstance(defaultInstanceName, storeSqlite.WithClientBuilderDSN(dsn))

	store, err := storeSqlite.NewStore(storeSqlite.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite store: %w", err)
	}

	deps.AccountStorage = store
	deps.ProviderStorage = store
	deps.StatsStore = store
	deps.UsageStore = store
	deps.OccupancyStore = store
	deps.AffinityStore = store

	return nil
}

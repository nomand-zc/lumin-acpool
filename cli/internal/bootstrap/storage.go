package bootstrap

import (
	"fmt"

	"github.com/nomand-zc/lumin-acpool/cli/internal/config"

	// Memory store
	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"

	// MySQL store
	storemysql "github.com/nomand-zc/lumin-acpool/storage/mysql"

	// Redis store
	storeredis "github.com/nomand-zc/lumin-acpool/storage/redis"

	// SQLite store
	storesqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
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
	store := storememory.NewStore()

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
	storemysql.RegisterInstance(defaultInstanceName, storemysql.WithClientBuilderDSN(dsn))

	store, err := storemysql.NewStore(storemysql.WithInstanceName(defaultInstanceName))
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
	storeredis.RegisterInstance(defaultInstanceName, storeredis.WithClientBuilderDSN(dsn))

	store, err := storeredis.NewStore(storeredis.WithInstanceName(defaultInstanceName))
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
	storesqlite.RegisterInstance(defaultInstanceName, storesqlite.WithClientBuilderDSN(dsn))

	store, err := storesqlite.NewStore(storesqlite.WithInstanceName(defaultInstanceName))
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

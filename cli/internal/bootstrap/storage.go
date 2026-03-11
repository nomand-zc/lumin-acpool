package bootstrap

import (
	"fmt"

	"github.com/nomand-zc/lumin-acpool/cli/internal/config"

	// Memory stores
	memAccountStore "github.com/nomand-zc/lumin-acpool/storage/memory/accountstore"
	memAffinityStore "github.com/nomand-zc/lumin-acpool/storage/memory/affinitystore"
	memOccupancyStore "github.com/nomand-zc/lumin-acpool/storage/memory/occupancystore"
	memProviderStore "github.com/nomand-zc/lumin-acpool/storage/memory/providerstore"
	memStatsStore "github.com/nomand-zc/lumin-acpool/storage/memory/statsstore"
	memUsageStore "github.com/nomand-zc/lumin-acpool/storage/memory/usagestore"

	// MySQL stores + client
	storeMysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
	mysqlAccountStore "github.com/nomand-zc/lumin-acpool/storage/mysql/accountstore"
	mysqlAffinityStore "github.com/nomand-zc/lumin-acpool/storage/mysql/affinitystore"
	mysqlOccupancyStore "github.com/nomand-zc/lumin-acpool/storage/mysql/occupancystore"
	mysqlProviderStore "github.com/nomand-zc/lumin-acpool/storage/mysql/providerstore"
	mysqlStatsStore "github.com/nomand-zc/lumin-acpool/storage/mysql/statsstore"
	mysqlUsageStore "github.com/nomand-zc/lumin-acpool/storage/mysql/usagestore"

	// Redis stores + client
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
	redisAccountStore "github.com/nomand-zc/lumin-acpool/storage/redis/accountstore"
	redisAffinityStore "github.com/nomand-zc/lumin-acpool/storage/redis/affinitystore"
	redisOccupancyStore "github.com/nomand-zc/lumin-acpool/storage/redis/occupancystore"
	redisProviderStore "github.com/nomand-zc/lumin-acpool/storage/redis/providerstore"
	redisStatsStore "github.com/nomand-zc/lumin-acpool/storage/redis/statsstore"
	redisUsageStore "github.com/nomand-zc/lumin-acpool/storage/redis/usagestore"

	// SQLite stores + client
	storeSqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
	sqliteAccountStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/accountstore"
	sqliteAffinityStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/affinitystore"
	sqliteOccupancyStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/occupancystore"
	sqliteProviderStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/providerstore"
	sqliteStatsStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/statsstore"
	sqliteUsageStore "github.com/nomand-zc/lumin-acpool/storage/sqlite/usagestore"
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

// initMemoryStorage 初始化内存存储（所有 store 无参数构造）。
func initMemoryStorage(deps *Dependencies) error {
	deps.AccountStorage = memAccountStore.NewStore()
	deps.ProviderStorage = memProviderStore.NewStore()
	deps.StatsStore = memStatsStore.NewMemoryStatsStore()
	deps.UsageStore = memUsageStore.NewMemoryUsageStore()
	deps.OccupancyStore = memOccupancyStore.NewMemoryOccupancyStore()
	deps.AffinityStore = memAffinityStore.NewStore()
	return nil
}

// initMySQLStorage 注册 MySQL 实例并初始化所有 store。
// 所有 store 通过 InstanceName 引用同一个共享连接。
func initMySQLStorage(dsn string, deps *Dependencies) error {
	storeMysql.RegisterInstance(defaultInstanceName, storeMysql.WithClientBuilderDSN(dsn))

	var err error

	deps.AccountStorage, err = mysqlAccountStore.NewStore(mysqlAccountStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql accountstore: %w", err)
	}

	deps.ProviderStorage, err = mysqlProviderStore.NewStore(mysqlProviderStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql providerstore: %w", err)
	}

	deps.StatsStore, err = mysqlStatsStore.NewStore(mysqlStatsStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql statsstore: %w", err)
	}

	deps.UsageStore, err = mysqlUsageStore.NewStore(mysqlUsageStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql usagestore: %w", err)
	}

	deps.OccupancyStore, err = mysqlOccupancyStore.NewStore(mysqlOccupancyStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql occupancystore: %w", err)
	}

	deps.AffinityStore, err = mysqlAffinityStore.NewStore(mysqlAffinityStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("mysql affinitystore: %w", err)
	}

	return nil
}

// initRedisStorage 注册 Redis 实例并初始化所有 store。
// 所有 store 通过 InstanceName 引用同一个共享连接。
func initRedisStorage(dsn string, deps *Dependencies) error {
	storeRedis.RegisterInstance(defaultInstanceName, storeRedis.WithClientBuilderDSN(dsn))

	var err error

	deps.AccountStorage, err = redisAccountStore.NewStore(redisAccountStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis accountstore: %w", err)
	}

	deps.ProviderStorage, err = redisProviderStore.NewStore(redisProviderStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis providerstore: %w", err)
	}

	deps.StatsStore, err = redisStatsStore.NewStore(redisStatsStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis statsstore: %w", err)
	}

	deps.UsageStore, err = redisUsageStore.NewStore(redisUsageStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis usagestore: %w", err)
	}

	deps.OccupancyStore, err = redisOccupancyStore.NewStore(redisOccupancyStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis occupancystore: %w", err)
	}

	deps.AffinityStore, err = redisAffinityStore.NewStore(redisAffinityStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("redis affinitystore: %w", err)
	}

	return nil
}

// initSQLiteStorage 注册 SQLite 实例并初始化所有 store。
// 所有 store 通过 InstanceName 引用同一个共享连接。
func initSQLiteStorage(dsn string, deps *Dependencies) error {
	storeSqlite.RegisterInstance(defaultInstanceName, storeSqlite.WithClientBuilderDSN(dsn))

	var err error

	deps.AccountStorage, err = sqliteAccountStore.NewStore(sqliteAccountStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite accountstore: %w", err)
	}

	deps.ProviderStorage, err = sqliteProviderStore.NewStore(sqliteProviderStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite providerstore: %w", err)
	}

	deps.StatsStore, err = sqliteStatsStore.NewStore(sqliteStatsStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite statsstore: %w", err)
	}

	deps.UsageStore, err = sqliteUsageStore.NewStore(sqliteUsageStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite usagestore: %w", err)
	}

	deps.OccupancyStore, err = sqliteOccupancyStore.NewStore(sqliteOccupancyStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite occupancystore: %w", err)
	}

	deps.AffinityStore, err = sqliteAffinityStore.NewStore(sqliteAffinityStore.WithInstanceName(defaultInstanceName))
	if err != nil {
		return fmt.Errorf("sqlite affinitystore: %w", err)
	}

	return nil
}

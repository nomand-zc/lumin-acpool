package bootstrap

import (
	"fmt"

	"github.com/nomand-zc/lumin-acpool/cli/internal/config"
	"github.com/nomand-zc/lumin-acpool/storage"

	storememory "github.com/nomand-zc/lumin-acpool/storage/memory"
	storemysql "github.com/nomand-zc/lumin-acpool/storage/mysql"
	storeredis "github.com/nomand-zc/lumin-acpool/storage/redis"
	storesqlite "github.com/nomand-zc/lumin-acpool/storage/sqlite"
)

const defaultInstanceName = "default"

// initStorage 根据 StorageConfig 初始化所有存储实例。
// 所有接口共享同一个 Store 实例，通过 SetStorage 统一注入。
func initStorage(cfg config.StorageConfig, deps *Dependencies) error {
	var (
		store storage.Storage
		err   error
	)

	switch cfg.Driver {
	case "memory":
		store = storememory.NewStore()

	case "mysql":
		storemysql.RegisterInstance(defaultInstanceName, storemysql.WithClientBuilderDSN(cfg.DSN))
		store, err = storemysql.NewStore(storemysql.WithInstanceName(defaultInstanceName))

	case "redis":
		storeredis.RegisterInstance(defaultInstanceName, storeredis.WithClientBuilderDSN(cfg.DSN))
		store, err = storeredis.NewStore(storeredis.WithInstanceName(defaultInstanceName))

	case "sqlite":
		storesqlite.RegisterInstance(defaultInstanceName, storesqlite.WithClientBuilderDSN(cfg.DSN))
		store, err = storesqlite.NewStore(storesqlite.WithInstanceName(defaultInstanceName))

	default:
		return fmt.Errorf("unsupported storage driver: %q", cfg.Driver)
	}

	if err != nil {
		return fmt.Errorf("%s store: %w", cfg.Driver, err)
	}

	deps.Storage = store
	return nil
}

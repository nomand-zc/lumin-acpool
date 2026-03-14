package bootstrap

import (
	"fmt"
	"io"

	"github.com/nomand-zc/lumin-acpool/balancer"
	"github.com/nomand-zc/lumin-acpool/cli/internal/config"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
)

// Dependencies 持有 CLI 运行所需的全部依赖。
type Dependencies struct {
	// Storage 层（聚合接口，包含 AccountStorage、ProviderStorage 等全部子接口）
	Storage storage.Storage

	// 业务组件层
	UsageTracker usagetracker.UsageTracker
	Balancer     balancer.Balancer

	// 需要关闭的资源
	closers []io.Closer
}

// Init 根据配置初始化所有依赖。
// 初始化顺序：Storage → Balancer（含所有子组件）。
func Init(cfg *config.Config) (*Dependencies, error) {
	deps := &Dependencies{}

	// 第一步：初始化存储层
	if err := initStorage(cfg.Storage, deps); err != nil {
		_ = deps.Close()
		return nil, fmt.Errorf("bootstrap storage: %w", err)
	}

	// 第二步：初始化 Balancer 及所有子组件
	if err := initBalancer(cfg.Balancer, deps); err != nil {
		_ = deps.Close()
		return nil, fmt.Errorf("bootstrap balancer: %w", err)
	}

	return deps, nil
}

// Close 释放所有资源（数据库连接、Redis 连接等）。
func (d *Dependencies) Close() error {
	var firstErr error
	for _, c := range d.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// addCloser 注册一个需要关闭的资源。
func (d *Dependencies) addCloser(c io.Closer) {
	d.closers = append(d.closers, c)
}

package pool

import (
	"context"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/balancer"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
	"github.com/nomand-zc/lumin-acpool/manager"
	"github.com/nomand-zc/lumin-acpool/storage/memory/accountstore"
	"github.com/nomand-zc/lumin-acpool/storage/memory/providerstore"
	"github.com/nomand-zc/lumin-acpool/storage/memory/statsstore"
	"github.com/nomand-zc/lumin-client/providers"
)

// Pool 是整个账号池的顶层门面（Facade），提供统一的操作入口。
// 内部自动编排 Balancer、HealthChecker、AccountManager 等子模块，
// 上层只需通过 Pool 即可完成选号、上报、注册、注销等所有操作。
//
// Provider SDK Client 通过 lumin-client 的全局注册表（providers.Register）管理，
// Pool 在 Pick 时自动通过 providers.GetProvider 查找对应的 Client。
//
// 用法示例：
//
//	p, _ := pool.New(
//	    pool.WithCooldownManager(cooldown.NewCooldownManager()),
//	)
//	p.RegisterProvider(ctx, providerInfo, client)
//	p.RegisterAccount(ctx, acct)
//
//	result, _ := p.Pick(ctx, &balancer.PickRequest{Model: "gpt-4"})
//	resp, _ := result.Client.GenerateContent(ctx, result.Account.Credential, req)
//	p.ReportSuccess(ctx, result.Account.ID)
//
//	p.Close()
type Pool struct {
	opts Options

	// 核心子模块
	balancer      balancer.Balancer
	healthChecker health.HealthChecker
	manager       *manager.AccountManager

	// 生命周期管理
	cancelFunc context.CancelFunc
	started    bool
}

// New 创建一个 Pool 实例。
// 自动组装所有子模块，提供一站式的账号池管理能力。
func New(opts ...Option) (*Pool, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	// 为未设置的存储创建默认内存实现（每个 Pool 实例独立）
	if o.AccountStorage == nil {
		o.AccountStorage = accountstore.NewStore()
	}
	if o.ProviderStorage == nil {
		o.ProviderStorage = providerstore.NewStore()
	}
	if o.StatsStore == nil {
		o.StatsStore = statsstore.NewMemoryStatsStore()
	}

	p := &Pool{
		opts: o,
	}

	// 1. 初始化 AccountManager
	mgr, err := manager.NewAccountManager(
		manager.WithAccountStorage(o.AccountStorage),
		manager.WithProviderStorage(o.ProviderStorage),
		manager.WithUsageTracker(o.UsageTracker),
		manager.WithStatsStore(o.StatsStore),
	)
	if err != nil {
		return nil, fmt.Errorf("pool: init account manager: %w", err)
	}
	p.manager = mgr

	// 2. 初始化 Balancer
	balancerOpts := []balancer.Option{
		balancer.WithAccountStorage(o.AccountStorage),
		balancer.WithProviderStorage(o.ProviderStorage),
	}
	if o.StatsStore != nil {
		balancerOpts = append(balancerOpts, balancer.WithStatsStore(o.StatsStore))
	}
	if o.CooldownManager != nil {
		balancerOpts = append(balancerOpts, balancer.WithCooldownManager(o.CooldownManager))
	}
	if o.CircuitBreaker != nil {
		balancerOpts = append(balancerOpts, balancer.WithCircuitBreaker(o.CircuitBreaker))
	}
	if o.UsageTracker != nil {
		balancerOpts = append(balancerOpts, balancer.WithUsageTracker(o.UsageTracker))
	}
	if o.Selector != nil {
		balancerOpts = append(balancerOpts, balancer.WithSelector(o.Selector))
	}
	if o.GroupSelector != nil {
		balancerOpts = append(balancerOpts, balancer.WithGroupSelector(o.GroupSelector))
	}
	if o.DefaultMaxRetries > 0 {
		balancerOpts = append(balancerOpts, balancer.WithDefaultMaxRetries(o.DefaultMaxRetries))
	}
	if o.DefaultEnableFailover {
		balancerOpts = append(balancerOpts, balancer.WithDefaultFailover(o.DefaultEnableFailover))
	}

	b, err := balancer.New(balancerOpts...)
	if err != nil {
		return nil, fmt.Errorf("pool: init balancer: %w", err)
	}
	p.balancer = b

	// 3. 初始化 HealthChecker
	p.initHealthChecker()

	return p, nil
}

// --- 选号与上报 ---

// Pick 从候选账号中选出一个可用账号，返回选号结果（含 SDK Client）。
// 上层可直接通过 result.Client 发起 API 调用。
func (p *Pool) Pick(ctx context.Context, req *balancer.PickRequest) (*balancer.PickResult, error) {
	result, err := p.balancer.Pick(ctx, req)
	if err != nil {
		return nil, err
	}

	// 自动填充 Client（从全局注册表查找）
	if client := providers.GetProvider(result.ProviderKey); client != nil {
		result.Client = client
	}

	return result, nil
}

// ReportSuccess 上报一次成功调用。
func (p *Pool) ReportSuccess(ctx context.Context, accountID string) error {
	return p.balancer.ReportSuccess(ctx, accountID)
}

// ReportFailure 上报一次失败调用。
func (p *Pool) ReportFailure(ctx context.Context, accountID string, callErr error) error {
	return p.balancer.ReportFailure(ctx, accountID, callErr)
}

// --- Provider / Account 管理 ---

// RegisterProvider 注册一个 Provider（元数据 + SDK Client）。
func (p *Pool) RegisterProvider(ctx context.Context, info *account.ProviderInfo, client providers.Provider) error {
	return p.manager.RegisterProvider(ctx, info, client)
}

// UnregisterProvider 注销一个 Provider 及其下属的所有 Account。
func (p *Pool) UnregisterProvider(ctx context.Context, key account.ProviderKey) error {
	return p.manager.UnregisterProvider(ctx, key)
}

// RegisterAccount 注册一个 Account。
func (p *Pool) RegisterAccount(ctx context.Context, acct *account.Account) error {
	return p.manager.RegisterAccount(ctx, acct)
}

// UnregisterAccount 注销一个 Account。
func (p *Pool) UnregisterAccount(ctx context.Context, accountID string) error {
	return p.manager.UnregisterAccount(ctx, accountID)
}

// GetProvider 获取指定 Provider 的信息。
func (p *Pool) GetProvider(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	return p.manager.GetProvider(ctx, key)
}

// GetAccount 获取指定 Account 的信息。
func (p *Pool) GetAccount(ctx context.Context, accountID string) (*account.Account, error) {
	return p.manager.GetAccount(ctx, accountID)
}

// ListProviders 列出所有 Provider。
func (p *Pool) ListProviders(ctx context.Context) ([]*account.ProviderInfo, error) {
	return p.manager.ListProviders(ctx)
}

// ListAccounts 列出所有 Account。
func (p *Pool) ListAccounts(ctx context.Context) ([]*account.Account, error) {
	return p.manager.ListAccounts(ctx)
}

// --- 生命周期管理 ---

// Start 启动后台健康检查任务。
// 如果已启动则返回错误。
func (p *Pool) Start(ctx context.Context) error {
	if p.started {
		return fmt.Errorf("pool: already started")
	}

	ctx, p.cancelFunc = context.WithCancel(ctx)
	p.started = true

	if p.healthChecker != nil {
		if err := p.healthChecker.Start(ctx); err != nil {
			p.started = false
			p.cancelFunc()
			p.cancelFunc = nil
			return fmt.Errorf("pool: start health checker: %w", err)
		}
	}

	return nil
}

// Close 关闭 Pool，停止所有后台任务并释放资源。
func (p *Pool) Close() error {
	if !p.started {
		return nil
	}

	p.started = false

	// 停止健康检查
	if p.healthChecker != nil {
		if err := p.healthChecker.Stop(); err != nil {
			return fmt.Errorf("pool: stop health checker: %w", err)
		}
	}

	// 取消所有后台 context
	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	return nil
}

// --- 内部子模块访问（供高级用户使用） ---

// Balancer 返回内部的 Balancer 实例。
func (p *Pool) Balancer() balancer.Balancer {
	return p.balancer
}

// HealthChecker 返回内部的 HealthChecker 实例。
func (p *Pool) HealthChecker() health.HealthChecker {
	return p.healthChecker
}

// Manager 返回内部的 AccountManager 实例。
func (p *Pool) Manager() *manager.AccountManager {
	return p.manager
}

// --- 内部初始化方法 ---

// initHealthChecker 初始化健康检查器。
func (p *Pool) initHealthChecker() {
	// 如果外部注入了 HealthChecker，直接使用
	if p.opts.HealthChecker != nil {
		p.healthChecker = p.opts.HealthChecker
		return
	}

	// 创建 TargetProvider（使用全局注册表的 GetProvider 作为 clientLookup）
	tp := health.NewStorageTargetProvider(p.opts.AccountStorage, providers.GetProvider)

	// 创建 ReportCallback
	reportCallback := health.NewDefaultReportCallback(health.ReportHandlerDeps{
		AccountStorage:  p.opts.AccountStorage,
		ProviderStorage: p.opts.ProviderStorage,
		UsageTracker:    p.opts.UsageTracker,
		CooldownManager: p.opts.CooldownManager,
	})

	// 创建 HealthChecker
	p.healthChecker = health.NewHealthChecker(
		health.WithTargetProvider(tp),
		health.WithCallback(reportCallback),
	)

	// 注册默认的 check 项
	p.registerDefaultChecks()
}

// registerDefaultChecks 注册默认的健康检查项。
func (p *Pool) registerDefaultChecks() {
	// 冷却/熔断恢复检查（高频，5s）
	p.healthChecker.Register(health.CheckSchedule{
		Check:    checks.NewRecoveryCheck(),
		Interval: p.opts.RecoveryCheckInterval,
		Enabled:  true,
	})

	// 凭证有效性检查（中频，30s）
	p.healthChecker.Register(health.CheckSchedule{
		Check:    &checks.CredentialValidityCheck{},
		Interval: p.opts.CredentialCheckInterval,
		Enabled:  true,
	})

	// 凭证刷新检查（中频，30s）
	p.healthChecker.Register(health.CheckSchedule{
		Check: &checks.CredentialRefreshCheck{
			RefreshThreshold: p.opts.CredentialRefreshThreshold,
		},
		Interval: p.opts.CredentialCheckInterval,
		Enabled:  true,
	})

	// 用量检查（低频，5m）
	p.healthChecker.Register(health.CheckSchedule{
		Check: &checks.UsageQuotaCheck{
			WarningThreshold: p.opts.UsageWarningThreshold,
		},
		Interval: p.opts.UsageCheckInterval,
		Enabled:  true,
	})

	// 模型发现检查（低频，10m）
	p.healthChecker.Register(health.CheckSchedule{
		Check:    &checks.ModelDiscoveryCheck{},
		Interval: p.opts.ModelDiscoveryInterval,
		Enabled:  true,
	})

	// 用量规则刷新检查（低频，10m）
	p.healthChecker.Register(health.CheckSchedule{
		Check:    &checks.UsageRulesRefreshCheck{},
		Interval: p.opts.UsageRulesRefreshInterval,
		Enabled:  true,
	})
}

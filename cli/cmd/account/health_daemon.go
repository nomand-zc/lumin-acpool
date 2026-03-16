package account

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
	"github.com/nomand-zc/lumin-client/providers"
)

// daemonCheckIntervals 定义 daemon 模式下各检查项的执行间隔。
var daemonCheckIntervals = map[string]time.Duration{
	checks.CredentialValidityCheckName: 1 * time.Minute,  // 凭证校验：轻量本地检查
	checks.CredentialRefreshCheckName:  2 * time.Minute,  // 凭证刷新
	checks.ProbeCheckName:              5 * time.Minute,  // 探测请求：有网络开销
	checks.UsageQuotaCheckName:         5 * time.Minute,  // 用量配额
	checks.UsageRulesRefreshCheckName:  10 * time.Minute, // 用量规则刷新：变化少
	checks.ModelDiscoveryCheckName:     30 * time.Minute, // 模型发现：变化极少
	checks.RecoveryCheckName:           1 * time.Minute,  // 冷却恢复：需快速感知
}

// runDaemon 以 daemon 模式启动 HealthChecker 的后台周期性检查。
func (c *healthCmd) runDaemon(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	// 解析用户指定的检查项
	specifiedChecks, err := c.parseCheckNames()
	if err != nil {
		return err
	}

	// 构建 daemon 模式的检查项调度列表（带 Interval）
	schedules := c.buildDaemonCheckSchedules(specifiedChecks)

	// 构建 TargetProvider：每次 tick 时动态查询 Storage 获取最新账号列表
	targetProvider := func(ctx context.Context) []health.CheckTarget {
		filter := buildAccountFilter(c.providerType, c.providerName, 0)
		accounts, err := deps.Storage.SearchAccounts(ctx, filter)
		if err != nil {
			fmt.Printf("⚠ daemon: 查询账号列表失败: %v\n", err)
			return nil
		}

		targets := make([]health.CheckTarget, 0, len(accounts))
		for _, account := range accounts {
			provider := providers.GetProvider(account.ProviderType, providers.DefaultProviderName)
			if provider == nil {
				fmt.Printf("⚠ daemon: 未找到类型为 %q 的 Provider 实例，跳过账号 %s\n", account.ProviderType, account.ID)
				continue
			}
			targets = append(targets, health.NewCheckTarget(account, provider))
		}
		return targets
	}

	// 构建 ReportCallback：将检查结果应用到账号并持久化
	reportCallback := func(ctx context.Context, report *health.HealthReport) {
		if report == nil || report.AccountID == "" {
			return
		}

		account, err := deps.Storage.GetAccount(ctx, report.AccountID)
		if err != nil {
			fmt.Printf("⚠ daemon: 获取账号 %s 失败: %v\n", report.AccountID, err)
			return
		}

		persistHealthReport(ctx, deps, account, report, "daemon: ")
	}

	// 构建 HealthChecker 并注册所有检查项
	checker := health.NewHealthChecker(
		health.WithTargetProvider(targetProvider),
		health.WithCallback(reportCallback),
	)
	for _, s := range schedules {
		checker.Register(s)
	}

	// 使用 signal.NotifyContext 监听退出信号
	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("🚀 daemon 模式启动，共注册 %d 个检查项，按 Ctrl+C 退出\n", len(schedules))
	for _, s := range schedules {
		fmt.Printf("  • %-25s 间隔: %v\n", s.Check.Name(), s.Interval)
	}

	if err := checker.Start(ctx); err != nil {
		return fmt.Errorf("启动 daemon 失败: %w", err)
	}

	// 阻塞等待退出信号
	<-ctx.Done()

	fmt.Println("\n⏹ 收到退出信号，正在停止 daemon...")
	if err := checker.Stop(); err != nil {
		fmt.Printf("⚠ 停止 daemon 异常: %v\n", err)
	}
	fmt.Println("✓ daemon 已停止")
	return nil
}

// buildDaemonCheckSchedules 构建 daemon 模式的检查项调度列表。
// 各检查项的执行间隔由 daemonCheckIntervals 硬编码配置。
func (c *healthCmd) buildDaemonCheckSchedules(specifiedChecks []string) []health.CheckSchedule {
	schedules := c.buildCheckSchedules(specifiedChecks)
	for i := range schedules {
		if interval, ok := daemonCheckIntervals[schedules[i].Check.Name()]; ok {
			schedules[i].Interval = interval
		}
	}
	return schedules
}

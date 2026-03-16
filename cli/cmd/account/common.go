package account

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// addAccountOptions 封装添加账号的公共参数。
type addAccountOptions struct {
	ID           string
	ProviderType string
	ProviderName string
	Credential   json.RawMessage
	Status       acct.Status
	Priority     int
	Tags         map[string]string
	Metadata     map[string]any
	ProbeModel   string // probe 检查使用的模型名称（为空则使用 Provider 默认模型）
}

// addAccountFromOptions 是 add 和 import 命令共享的核心添加逻辑。
// 完成以下步骤：
//  1. 验证 Provider 是否存在
//  2. 解析凭证
//  3. 构建 Account 对象
//  4. 健康检查（如果 Status 为零值）
//  5. 继承/回退用量规则和模型列表
//  6. 存储 Account + 初始化 TrackedUsages
func addAccountFromOptions(cmd *cobra.Command, opts *addAccountOptions) (acct.Status, error) {
	deps := bootstrap.DepsFromContext(cmd.Context())

	// 验证所属 Provider 是否存在
	providerKey := acct.BuildProviderKey(opts.ProviderType, opts.ProviderName)
	providerInfo, err := deps.Storage.GetProvider(cmd.Context(), providerKey)
	if err != nil {
		if err == storage.ErrNotFound {
			return 0, fmt.Errorf("Provider %s 不存在，请先添加 Provider", providerKey)
		}
		return 0, fmt.Errorf("查询 Provider 失败: %w", err)
	}

	// 解析凭证
	cred, err := parseCredential(opts.ProviderType, opts.Credential)
	if err != nil {
		return 0, fmt.Errorf("解析凭证失败: %w", err)
	}

	// 构建 Account 对象
	account := &acct.Account{
		ID:           opts.ID,
		ProviderType: opts.ProviderType,
		ProviderName: opts.ProviderName,
		Credential:   cred,
		Status:       opts.Status,
		Priority:     opts.Priority,
		Tags:         opts.Tags,
		Metadata:     opts.Metadata,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 通过 HealthChecker 完整检查账号的真实可用性状态
	var usageStats []*usagerule.UsageStats
	if account.Status == 0 {
		usageStats = runHealthCheck(cmd, deps, providerKey, account, opts.ProbeModel)
	}

	// 如果健康检查没有获取到 UsageRules，从 Provider 继承
	if len(account.UsageRules) == 0 && len(providerInfo.UsageRules) > 0 {
		account.UsageRules = providerInfo.UsageRules
	}

	// 如果健康检查没有获取到模型列表，通过 Provider 默认模型列表兜底
	latestProvInfo, _ := deps.Storage.GetProvider(cmd.Context(), providerKey)
	if latestProvInfo != nil && len(latestProvInfo.SupportedModels) == 0 {
		fallbackProviderModels(cmd, deps, providerKey, account)
	}

	if err := deps.Storage.AddAccount(cmd.Context(), account); err != nil {
		return 0, handleStorageError("Account", err)
	}

	// 账号添加成功后，如果健康检查获取到了真实用量数据，初始化 TrackedUsages
	if len(usageStats) > 0 {
		initTrackedUsages(cmd, deps, account.ID, usageStats)
	}

	fmt.Printf("Account %s 添加成功（Provider: %s, 状态: %s）\n", account.ID, providerKey, account.Status)
	return account.Status, nil
}

// printStatusSummary 打印批量操作后各状态的账号数量统计。
func printStatusSummary(statusCounts map[acct.Status]int64) {
	if len(statusCounts) == 0 {
		return
	}
	fmt.Println("状态分布:")
	for status, count := range statusCounts {
		fmt.Printf("  %s: %d\n", status, count)
	}
}

// buildDefaultCheckSchedules 构建默认的全部健康检查项调度列表。
// 按拓扑依赖顺序包含：
//   - CredentialValidity（凭证格式与过期校验）
//   - CredentialRefresh（凭证刷新）
//   - Probe（真实请求探测可用性）
//   - UsageQuota（用量配额检查）
//   - UsageRulesRefresh（用量规则刷新）
//   - ModelDiscovery（模型发现）
func buildDefaultCheckSchedules(probeModel string) []health.CheckSchedule {
	return []health.CheckSchedule{
		{Check: &checks.CredentialValidityCheck{}, Enabled: true},
		{Check: &checks.CredentialRefreshCheck{RefreshThreshold: 2 * time.Minute}, Interval: time.Minute, Enabled: true},
		{Check: &checks.ProbeCheck{Timeout: 15 * time.Second, Model: probeModel}, Interval: time.Hour, Enabled: true},
		{Check: &checks.UsageQuotaCheck{WarningThreshold: 0.01}, Interval: time.Minute, Enabled: true},
		{Check: &checks.UsageRulesRefreshCheck{}, Interval: 12 * time.Hour, Enabled: true},
		{Check: &checks.ModelDiscoveryCheck{}, Interval: 12 * time.Hour, Enabled: true},
	}
}

// executeHealthCheck 封装通用的健康检查执行流程：
//  1. 获取 Provider 实例
//  2. 构建 HealthChecker 并注册检查项
//  3. 构建 CheckTarget
//  4. 执行全部检查
//
// 返回 HealthReport 和 error；如果 Provider 不存在则返回 nil, nil。
func executeHealthCheck(cmd *cobra.Command, account *acct.Account, schedules []health.CheckSchedule) (*health.HealthReport, error) {
	provider := providers.GetProvider(account.ProviderType, providers.DefaultProviderName)
	if provider == nil {
		return nil, fmt.Errorf("未找到类型为 %q 的 Provider 实例", account.ProviderType)
	}

	checker := health.NewHealthChecker()
	for _, s := range schedules {
		checker.Register(s)
	}

	target := health.NewCheckTarget(account, provider)
	return checker.RunAll(cmd.Context(), target)
}

// runHealthCheck 使用 HealthChecker 对账号进行完整的健康检查，并根据检查结果设置账号状态。
// 这是 add/import 命令使用的便捷入口，内部调用 executeHealthCheck + applyReportToAccount。
func runHealthCheck(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account, probeModel string) []*usagerule.UsageStats {
	schedules := buildDefaultCheckSchedules(probeModel)
	report, err := executeHealthCheck(cmd, account, schedules)
	if err != nil {
		fmt.Printf("⚠ %v，跳过健康检查，默认设为可用\n", err)
		account.Status = acct.StatusAvailable
		return nil
	}

	// 打印检查结果摘要
	printHealthReport(report)

	// 从报告中提取检查数据并应用到账号
	return applyReportToAccount(cmd, deps, providerKey, account, report)
}

// applyReportToAccount 从 HealthReport 中提取所有检查数据并应用到账号。
func applyReportToAccount(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account, report *health.HealthReport) []*usagerule.UsageStats {
	return applyReportToAccountWithCtx(cmd.Context(), deps, providerKey, account, report)
}

// applyReportToAccountWithCtx 从 HealthReport 中提取所有检查数据并应用到账号（接受 context.Context 参数）。
func applyReportToAccountWithCtx(ctx context.Context, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account, report *health.HealthReport) []*usagerule.UsageStats {
	var finalStatus *acct.Status
	var cooldownUntil *time.Time
	var usageStats []*usagerule.UsageStats

	for _, result := range report.Results {
		if result == nil {
			continue
		}

		if result.SuggestedStatus != nil {
			finalStatus = result.SuggestedStatus
		}

		dataMap, ok := result.Data.(map[string]any)
		if !ok {
			continue
		}

		if until, ok := dataMap[health.ReportDataKeyCooldownUntil]; ok {
			if t, ok := until.(*time.Time); ok && t != nil {
				cooldownUntil = t
			}
		}

		if rulesRaw, ok := dataMap[health.ReportDataKeyUsageRules]; ok {
			if rules, ok := rulesRaw.([]*usagerule.UsageRule); ok && len(rules) > 0 {
				account.UsageRules = rules
				fmt.Printf("  → 已更新用量规则（%d 条）\n", len(rules))
			}
		}

		if modelsRaw, ok := dataMap[health.ReportDataKeySupportedModels]; ok {
			if models, ok := modelsRaw.([]string); ok && len(models) > 0 {
				updateProviderModelsWithCtx(ctx, deps, providerKey, models)
			}
		}

		if statsRaw, ok := dataMap[health.ReportDataKeyUsageStats]; ok {
			if stats, ok := statsRaw.([]*usagerule.UsageStats); ok && len(stats) > 0 {
				usageStats = stats
				printUsageStatsSummary(stats)
			}
		}
	}

	if finalStatus != nil {
		account.Status = *finalStatus
		if *finalStatus == acct.StatusCoolingDown && cooldownUntil != nil {
			account.CooldownUntil = cooldownUntil
		}
	} else {
		account.Status = acct.StatusAvailable
	}

	return usageStats
}

// updateProviderModels 将发现的模型列表更新到 ProviderInfo.SupportedModels。
func updateProviderModels(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, models []string) {
	updateProviderModelsWithCtx(cmd.Context(), deps, providerKey, models)
}

// updateProviderModelsWithCtx 将发现的模型列表更新到 ProviderInfo.SupportedModels（接受 context.Context 参数）。
func updateProviderModelsWithCtx(ctx context.Context, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, models []string) {
	provInfo, err := deps.Storage.GetProvider(ctx, providerKey)
	if err != nil {
		fmt.Printf("  ⚠ 更新模型列表失败（获取 Provider 失败）: %v\n", err)
		return
	}

	provInfo.SupportedModels = models
	provInfo.UpdatedAt = time.Now()
	if err := deps.Storage.UpdateProvider(ctx, provInfo); err != nil {
		fmt.Printf("  ⚠ 更新模型列表失败（保存 Provider 失败）: %v\n", err)
		return
	}

	fmt.Printf("  → 已更新 Provider 支持的模型列表（%d 个模型）\n", len(models))
}

// printUsageStatsSummary 打印用量统计摘要。
func printUsageStatsSummary(stats []*usagerule.UsageStats) {
	for _, s := range stats {
		if s == nil || s.Rule == nil {
			continue
		}
		var ratio float64
		if s.Rule.Total > 0 {
			ratio = s.Remain / s.Rule.Total * 100
		}
		status := "充足"
		if s.IsTriggered() {
			status = "已耗尽"
		} else if ratio < 10 {
			status = "即将耗尽"
		}
		fmt.Printf("  → 用量 [%s/%d]: 已用 %.0f / 总量 %.0f（剩余 %.1f%%，%s）\n",
			s.Rule.TimeGranularity, s.Rule.WindowSize, s.Used, s.Rule.Total, ratio, status)
	}
}

// printHealthReport 打印健康检查报告摘要。
func printHealthReport(report *health.HealthReport) {
	fmt.Printf("── 健康检查报告（耗时 %v）──\n", report.TotalDuration.Round(time.Millisecond))
	for _, result := range report.Results {
		if result == nil {
			continue
		}
		icon := "✓"
		switch result.Status {
		case health.CheckFailed:
			icon = "✗"
		case health.CheckError:
			icon = "!"
		case health.CheckWarning:
			icon = "⚠"
		case health.CheckSkipped:
			icon = "⊘"
		}
		fmt.Printf("  %s [%s] %s: %s (%v)\n",
			icon, result.Severity, result.CheckName, result.Message,
			result.Duration.Round(time.Millisecond))
	}
	fmt.Println("──────────────────────────────")
}

// parseCredential 根据 providerType 使用对应的工厂解析凭证 JSON。
func parseCredential(providerType string, raw json.RawMessage) (credentials.Credential, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("凭证数据为空")
	}

	factory := credentials.GetFactory(providerType)
	if factory == nil {
		return nil, fmt.Errorf("未找到类型为 %q 的凭证工厂", providerType)
	}

	cred := factory(raw)
	if cred == nil {
		return nil, fmt.Errorf("凭证解析失败，请检查 JSON 格式")
	}

	return cred, nil
}

// fallbackProviderModels 在健康检查未获取到模型列表时，通过 Provider 的默认模型列表兜底。
func fallbackProviderModels(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account) {
	provider := providers.GetProvider(account.ProviderType, providers.DefaultProviderName)
	if provider == nil {
		return
	}

	models, err := provider.Models(cmd.Context())
	if err != nil {
		fmt.Printf("  ⚠ 获取 Provider 默认模型列表失败: %v\n", err)
		return
	}

	if len(models) == 0 {
		return
	}

	updateProviderModels(cmd, deps, providerKey, models)
}

// initTrackedUsages 根据健康检查返回的真实用量数据初始化 TrackedUsages。
func initTrackedUsages(cmd *cobra.Command, deps *bootstrap.Dependencies, accountID string, stats []*usagerule.UsageStats) {
	initTrackedUsagesWithCtx(cmd.Context(), deps, accountID, stats)
}

// initTrackedUsagesWithCtx 根据健康检查返回的真实用量数据初始化 TrackedUsages（接受 context.Context 参数）。
func initTrackedUsagesWithCtx(ctx context.Context, deps *bootstrap.Dependencies, accountID string, stats []*usagerule.UsageStats) {
	if deps.Storage == nil {
		return
	}

	trackedUsages := make([]*acct.TrackedUsage, 0, len(stats))
	now := time.Now()
	for _, s := range stats {
		if s == nil || s.Rule == nil {
			continue
		}
		trackedUsages = append(trackedUsages, &acct.TrackedUsage{
			Rule:         s.Rule.Clone(),
			LocalUsed:    0,
			RemoteUsed:   s.Used,
			RemoteRemain: s.Remain,
			WindowStart:  s.StartTime,
			WindowEnd:    s.EndTime,
			LastSyncAt:   now,
		})
	}

	if len(trackedUsages) == 0 {
		return
	}

	if err := deps.Storage.SaveUsages(ctx, accountID, trackedUsages); err != nil {
		fmt.Printf("  ⚠ 初始化用量追踪数据失败: %v\n", err)
		return
	}

	fmt.Printf("  → 已初始化用量追踪数据（%d 条规则）\n", len(trackedUsages))
}

// persistHealthReport 将健康检查结果应用到账号并持久化到存储。
// logPrefix 用于区分调用场景（如 "" 或 "daemon: "）。
func persistHealthReport(
	ctx context.Context,
	deps *bootstrap.Dependencies,
	account *acct.Account,
	report *health.HealthReport,
	logPrefix string,
) {
	oldStatus := account.Status
	providerKey := account.ProviderKey()

	// 复用公共的 applyReportToAccount，将检查结果中的状态、用量规则、模型列表等信息应用到账号
	usageStats := applyReportToAccountWithCtx(ctx, deps, providerKey, account, report)

	// 更新账号信息到存储
	account.UpdatedAt = time.Now()
	updateFields := storage.UpdateFieldStatus | storage.UpdateFieldUsageRules | storage.UpdateFieldCredential
	if err := deps.Storage.UpdateAccount(ctx, account, updateFields); err != nil {
		if err == storage.ErrVersionConflict {
			fmt.Printf("  ⚠ %s更新 Account %s 失败: 版本冲突（账号可能已被其他操作修改）\n", logPrefix, account.ID)
		} else {
			fmt.Printf("  ⚠ %s更新 Account %s 失败: %v\n", logPrefix, account.ID, err)
		}
		return
	}

	// 如果检查获取到了真实用量数据，更新 TrackedUsages
	if len(usageStats) > 0 {
		initTrackedUsagesWithCtx(ctx, deps, account.ID, usageStats)
	}

	if oldStatus != account.Status {
		fmt.Printf("  → %sAccount %s 状态已更新: %s → %s\n", logPrefix, account.ID, oldStatus, account.Status)
	}
}

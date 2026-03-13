package account

import (
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
}

// addAccountFromOptions 是 add 和 import 命令共享的核心添加逻辑。
// 完成以下步骤：
//  1. 验证 Provider 是否存在
//  2. 解析凭证
//  3. 构建 Account 对象
//  4. 健康检查（如果 Status 为零值）
//  5. 继承/回退用量规则和模型列表
//  6. 存储 Account + 初始化 TrackedUsages
func addAccountFromOptions(cmd *cobra.Command, opts *addAccountOptions) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	// 验证所属 Provider 是否存在
	providerKey := acct.BuildProviderKey(opts.ProviderType, opts.ProviderName)
	providerInfo, err := deps.ProviderStorage.Get(cmd.Context(), providerKey)
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("Provider %s 不存在，请先添加 Provider", providerKey)
		}
		return fmt.Errorf("查询 Provider 失败: %w", err)
	}

	// 解析凭证
	cred, err := parseCredential(opts.ProviderType, opts.Credential)
	if err != nil {
		return fmt.Errorf("解析凭证失败: %w", err)
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
		usageStats = runHealthCheck(cmd, deps, providerKey, account)
	}

	// 如果健康检查没有获取到 UsageRules，从 Provider 继承
	if len(account.UsageRules) == 0 && len(providerInfo.UsageRules) > 0 {
		account.UsageRules = providerInfo.UsageRules
	}

	// 如果健康检查没有获取到模型列表，通过 Provider 默认模型列表兜底
	latestProvInfo, _ := deps.ProviderStorage.Get(cmd.Context(), providerKey)
	if latestProvInfo != nil && len(latestProvInfo.SupportedModels) == 0 {
		fallbackProviderModels(cmd, deps, providerKey, account)
	}

	if err := deps.AccountStorage.Add(cmd.Context(), account); err != nil {
		return handleStorageError("Account", err)
	}

	// 账号添加成功后，如果健康检查获取到了真实用量数据，初始化 TrackedUsages
	if len(usageStats) > 0 {
		initTrackedUsages(cmd, deps, account.ID, usageStats)
	}

	fmt.Printf("Account %s 添加成功（Provider: %s, 状态: %s）\n", account.ID, providerKey, account.Status)
	return nil
}

// runHealthCheck 使用 HealthChecker 对账号进行完整的健康检查，并根据检查结果设置账号状态。
// 注册的检查项（按拓扑依赖顺序执行）：
//   - CredentialValidity → CredentialRefresh（凭证校验与刷新）
//   - CredentialValidity → Probe（真实请求探测可用性）
//   - CredentialValidity → UsageQuota（用量配额检查）
//   - CredentialValidity → UsageRulesRefresh（用量规则刷新）
//   - CredentialValidity → ModelDiscovery（模型发现）
func runHealthCheck(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account) []*usagerule.UsageStats {
	provider := providers.GetProvider(account.ProviderType, providers.DefaultProviderName)
	if provider == nil {
		fmt.Printf("⚠ 未找到类型为 %q 的 Provider 实例，跳过健康检查，默认设为可用\n", account.ProviderType)
		account.Status = acct.StatusAvailable
		return nil
	}

	// 构建 HealthChecker 并注册适合"首次添加"场景的全部检查项
	checker := health.NewHealthChecker()
	checker.Register(health.CheckSchedule{
		Check:   &checks.CredentialValidityCheck{},
		Enabled: true,
	})
	checker.Register(health.CheckSchedule{
		Check:   &checks.CredentialRefreshCheck{RefreshThreshold: 5 * time.Minute},
		Enabled: true,
	})
	checker.Register(health.CheckSchedule{
		Check:   &checks.ProbeCheck{Timeout: 15 * time.Second},
		Enabled: true,
	})
	checker.Register(health.CheckSchedule{
		Check:   &checks.UsageQuotaCheck{WarningThreshold: 0.01},
		Enabled: true,
	})
	checker.Register(health.CheckSchedule{
		Check:   &checks.UsageRulesRefreshCheck{},
		Enabled: true,
	})
	checker.Register(health.CheckSchedule{
		Check:   &checks.ModelDiscoveryCheck{},
		Enabled: true,
	})

	// 构建检查目标
	target := health.NewCheckTarget(account, provider)

	// 执行全部检查
	report, err := checker.RunAll(cmd.Context(), target)
	if err != nil {
		fmt.Printf("⚠ 健康检查执行失败: %v，默认设为可用\n", err)
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
				updateProviderModels(cmd, deps, providerKey, models)
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
	provInfo, err := deps.ProviderStorage.Get(cmd.Context(), providerKey)
	if err != nil {
		fmt.Printf("  ⚠ 更新模型列表失败（获取 Provider 失败）: %v\n", err)
		return
	}

	provInfo.SupportedModels = models
	provInfo.UpdatedAt = time.Now()
	if err := deps.ProviderStorage.Update(cmd.Context(), provInfo); err != nil {
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
	if deps.UsageStore == nil {
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

	if err := deps.UsageStore.Save(cmd.Context(), accountID, trackedUsages); err != nil {
		fmt.Printf("  ⚠ 初始化用量追踪数据失败: %v\n", err)
		return
	}

	fmt.Printf("  → 已初始化用量追踪数据（%d 条规则）\n", len(trackedUsages))
}

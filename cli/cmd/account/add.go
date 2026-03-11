package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
	"github.com/nomand-zc/lumin-client/pool/taskpool"
	"github.com/nomand-zc/lumin-client/providers"
	"github.com/nomand-zc/lumin-client/usagerule"

	// 匿名导入，触发 credential / provider init 注册
	_ "github.com/nomand-zc/lumin-client/credentials/kiro"
	_ "github.com/nomand-zc/lumin-client/providers/kiro"
)

// addCmd 持有 account add 命令的参数。
type addCmd struct {
	filePath     string
	providerType string
	providerName string
}

// cmd 返回 cobra.Command。
func (c *addCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "添加 Account",
		Long: `从 JSON 文件添加一个 Account，或从目录批量添加多个 Account。

当 --file 指定的是一个目录时，会扫描该目录下的所有 .json 文件并发批量添加。
可通过 --type 和 --name 参数预设 ProviderType 和 ProviderName，
JSON 文件中未指定这两个字段时会使用命令行参数的值。

JSON 文件示例:
  {
    "ID": "acct-001",
    "ProviderType": "kiro",
    "ProviderName": "kiro-team-a",
    "Credential": {
      "accessToken": "xxx",
      "refreshToken": "yyy"
    },
    "Status": 1,
    "Priority": 10,
    "Tags": {"team": "backend"}
  }

示例:
  acpool account add --file account.json
  acpool account add --file account.json --type kiro --name kiro-team-a
  acpool account add --file /path/to/accounts/ --type kiro --name kiro-team-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "Account JSON 文件路径或目录路径（必填）")
	cmd.Flags().StringVar(&c.providerType, "type", "kiro", "Provider 类型（可选，覆盖 JSON 中的 ProviderType）")
	cmd.Flags().StringVar(&c.providerName, "name", "default", "Provider 名称（可选，覆盖 JSON 中的 ProviderName）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 account add 逻辑。
func (c *addCmd) run(cmd *cobra.Command) error {
	fi, err := os.Stat(c.filePath)
	if err != nil {
		return fmt.Errorf("无法访问路径 %s: %w", c.filePath, err)
	}

	if fi.IsDir() {
		return c.runBatch(cmd)
	}
	return c.runSingle(cmd, c.filePath)
}

// accountJSON 是 Account 的 JSON 反序列化中间结构，
// 因为 Credential 是接口类型，需要通过 ProviderType 对应的工厂来解析。
type accountJSON struct {
	ID           string            `json:"ID"`
	ProviderType string            `json:"ProviderType"`
	ProviderName string            `json:"ProviderName"`
	Credential   json.RawMessage   `json:"Credential"`
	Status       acct.Status       `json:"Status"`
	Priority     int               `json:"Priority"`
	Tags         map[string]string `json:"Tags"`
	Metadata     map[string]any    `json:"Metadata"`
}

// runSingle 添加单个 Account。
func (c *addCmd) runSingle(cmd *cobra.Command, filePath string) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	raw, err := ioutil.LoadJSONFile[accountJSON](filePath)
	if err != nil {
		return err
	}

	// 命令行参数覆盖 JSON 中的空值
	c.applyFlags(raw)

	// 验证所属 Provider 是否存在
	providerKey := acct.BuildProviderKey(raw.ProviderType, raw.ProviderName)
	providerInfo, err := deps.ProviderStorage.Get(cmd.Context(), providerKey)
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("Provider %s 不存在，请先添加 Provider", providerKey)
		}
		return fmt.Errorf("查询 Provider 失败: %w", err)
	}

	// 解析凭证
	cred, err := parseCredential(raw.ProviderType, raw.Credential)
	if err != nil {
		return fmt.Errorf("解析凭证失败: %w", err)
	}

	// 构建 Account 对象
	account := &acct.Account{
		ID:           raw.ID,
		ProviderType: raw.ProviderType,
		ProviderName: raw.ProviderName,
		Credential:   cred,
		Status:       raw.Status,
		Priority:     raw.Priority,
		Tags:         raw.Tags,
		Metadata:     raw.Metadata,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// 通过 HealthChecker 完整检查账号的真实可用性状态，
	// 根据检查结果设置正确的账号状态，并同步更新用量规则、模型列表等运行时数据。
	var usageStats []*usagerule.UsageStats
	if account.Status == 0 {
		usageStats = c.runHealthCheck(cmd, deps, providerKey, account)
	}

	// 如果健康检查没有获取到 UsageRules，从 Provider 继承
	if len(account.UsageRules) == 0 && len(providerInfo.UsageRules) > 0 {
		account.UsageRules = providerInfo.UsageRules
	}

	// 如果健康检查没有获取到模型列表，通过 Provider 默认模型列表兜底。
	// 需重新读取 ProviderInfo 以获取可能被健康检查更新的最新数据。
	latestProvInfo, _ := deps.ProviderStorage.Get(cmd.Context(), providerKey)
	if latestProvInfo != nil && len(latestProvInfo.SupportedModels) == 0 {
		c.fallbackProviderModels(cmd, deps, providerKey, account)
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

// runBatch 扫描目录下所有 JSON 文件，并发批量添加 Account。
func (c *addCmd) runBatch(cmd *cobra.Command) error {
	var (
		successCount atomic.Int64
		failCount    atomic.Int64
		mu           sync.Mutex
		wg           sync.WaitGroup
		errs         []string
	)

	err := filepath.Walk(c.filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() ||
			!strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}

		wg.Add(1)
		if submitErr := taskpool.DefaultPool.Submit(func() {
			defer wg.Done()

			if addErr := c.runSingle(cmd, path); addErr != nil {
				failCount.Add(1)
				mu.Lock()
				errs = append(errs, fmt.Sprintf("  %s: %v", path, addErr))
				mu.Unlock()
				return
			}
			successCount.Add(1)
		}); submitErr != nil {
			wg.Done()
			failCount.Add(1)
			mu.Lock()
			errs = append(errs, fmt.Sprintf("  %s: 提交任务失败: %v", path, submitErr))
			mu.Unlock()
		}

		return nil
	})

	// 等待所有并发任务完成
	wg.Wait()

	if err != nil {
		return fmt.Errorf("扫描目录失败: %w", err)
	}

	total := successCount.Load() + failCount.Load()
	fmt.Printf("批量添加完成！总计: %d, 成功: %d, 失败: %d\n",
		total, successCount.Load(), failCount.Load())

	if len(errs) > 0 {
		fmt.Printf("失败详情:\n%s\n", strings.Join(errs, "\n"))
	}

	return nil
}

// applyFlags 将命令行参数覆盖到 accountJSON 中的空值字段。
func (c *addCmd) applyFlags(raw *accountJSON) {
	if raw.ProviderType == "" && c.providerType != "" {
		raw.ProviderType = c.providerType
	}
	if raw.ProviderName == "" && c.providerName != "" {
		raw.ProviderName = c.providerName
	}
}

// runHealthCheck 使用 HealthChecker 对账号进行完整的健康检查，并根据检查结果设置账号状态。
// 注册的检查项（按拓扑依赖顺序执行）：
//   - CredentialValidity → CredentialRefresh（凭证校验与刷新）
//   - CredentialValidity → Probe（真实请求探测可用性）
//   - CredentialValidity → UsageQuota（用量配额检查）
//   - CredentialValidity → UsageRulesRefresh（用量规则刷新）
//   - CredentialValidity → ModelDiscovery（模型发现）
//
// 检查完成后，从报告中提取并应用：
//   - SuggestedStatus → 账号状态
//   - UsageRules → Account.UsageRules
//   - SupportedModels → ProviderInfo.SupportedModels（通过 ProviderStorage 持久化）
//   - UsageStats → 打印用量摘要（CLI 添加场景无 UsageTracker，仅供用户参考）
func (c *addCmd) runHealthCheck(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account) []*usagerule.UsageStats {
	provider := providers.GetProvider(account.ProviderType, providers.DefaultProviderName)
	if provider == nil {
		fmt.Printf("⚠ 未找到类型为 %q 的 Provider 实例，跳过健康检查，默认设为可用\n", account.ProviderType)
		account.Status = acct.StatusAvailable
		return nil
	}

	// 构建 HealthChecker 并注册适合"首次添加"场景的全部检查项
	// 注：RecoveryCheck 不注册（新账号不会处于冷却/熔断状态，无需恢复检查）
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

	// 从报告中提取检查数据并应用到账号（状态、用量规则、模型列表等）
	return applyReportToAccount(cmd, deps, providerKey, account, report)
}

// applyReportToAccount 从 HealthReport 中提取所有检查数据并应用到账号。
// 处理内容：
//   - SuggestedStatus → 更新 Account.Status（取最后一个非空建议，越后优先级越高）
//   - CooldownUntil → 更新 Account.CooldownUntil
//   - UsageRules → 更新 Account.UsageRules（API 动态获取的规则优先于 Provider 继承）
//   - SupportedModels → 更新 ProviderInfo.SupportedModels（通过 ProviderStorage 持久化）
//   - UsageStats → 打印用量摘要（CLI 添加场景无 UsageTracker，仅供用户参考）
func applyReportToAccount(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account, report *health.HealthReport) []*usagerule.UsageStats {
	var finalStatus *acct.Status
	var cooldownUntil *time.Time
	var usageStats []*usagerule.UsageStats

	for _, result := range report.Results {
		if result == nil {
			continue
		}

		// 提取 SuggestedStatus，后面的结果覆盖前面的
		if result.SuggestedStatus != nil {
			finalStatus = result.SuggestedStatus
		}

		dataMap, ok := result.Data.(map[string]any)
		if !ok {
			continue
		}

		// 提取 CooldownUntil 数据
		if until, ok := dataMap[health.ReportDataKeyCooldownUntil]; ok {
			if t, ok := until.(*time.Time); ok && t != nil {
				cooldownUntil = t
			}
		}

		// 提取 UsageRules：API 动态获取的规则优先于 Provider 继承
		if rulesRaw, ok := dataMap[health.ReportDataKeyUsageRules]; ok {
			if rules, ok := rulesRaw.([]*usagerule.UsageRule); ok && len(rules) > 0 {
				account.UsageRules = rules
				fmt.Printf("  → 已更新用量规则（%d 条）\n", len(rules))
			}
		}

		// 提取 SupportedModels：更新到 ProviderInfo
		if modelsRaw, ok := dataMap[health.ReportDataKeySupportedModels]; ok {
			if models, ok := modelsRaw.([]string); ok && len(models) > 0 {
				updateProviderModels(cmd, deps, providerKey, models)
			}
		}

		// 提取 UsageStats：打印用量摘要，并收集用于后续初始化 TrackedUsages
		if statsRaw, ok := dataMap[health.ReportDataKeyUsageStats]; ok {
			if stats, ok := statsRaw.([]*usagerule.UsageStats); ok && len(stats) > 0 {
				usageStats = stats
				printUsageStatsSummary(stats)
			}
		}
	}

	// 应用最终状态
	if finalStatus != nil {
		account.Status = *finalStatus
		// 如果建议冷却状态，设置冷却时间
		if *finalStatus == acct.StatusCoolingDown && cooldownUntil != nil {
			account.CooldownUntil = cooldownUntil
		}
	} else {
		// 所有检查通过，无建议状态变更，默认可用
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
// 调用 provider.Models() 获取供应商预置的模型列表（不依赖凭证），并更新到 ProviderInfo.SupportedModels。
func (c *addCmd) fallbackProviderModels(cmd *cobra.Command, deps *bootstrap.Dependencies, providerKey acct.ProviderKey, account *acct.Account) {
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
// 将 UsageStats 转换为 TrackedUsage 并通过 UsageStore.Save() 持久化，
// 使得新添加的账号从一开始就拥有准确的用量追踪数据。
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
			LocalUsed:    0, // 新添加的账号，本地尚无消耗
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

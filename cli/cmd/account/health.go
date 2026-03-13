package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"
	"github.com/nomand-zc/lumin-acpool/storage"

	// 匿名导入，触发 credential / provider init 注册
	_ "github.com/nomand-zc/lumin-client/credentials/kiro"
	_ "github.com/nomand-zc/lumin-client/providers/kiro"
)

// allCheckNames 所有可用的检查项名称及说明。
var allCheckNames = map[string]string{
	checks.CredentialValidityCheckName: "凭证格式与过期状态校验（本地检查）",
	checks.CredentialRefreshCheckName:  "凭证刷新检查（过期或即将过期时尝试刷新）",
	checks.ProbeCheckName:              "真实请求探测（发送轻量请求验证可用性）",
	checks.UsageQuotaCheckName:         "用量配额检查（获取最新用量并判断是否耗尽）",
	checks.UsageRulesRefreshCheckName:  "用量规则刷新（动态获取最新规则）",
	checks.ModelDiscoveryCheckName:     "模型发现（获取当前凭证支持的模型列表）",
	checks.RecoveryCheckName:           "冷却/熔断恢复检查（检测是否已到期可恢复）",
}

// healthReportJSON 用于 JSON 序列化的健康检查报告。
type healthReportJSON struct {
	AccountID   string             `json:"account_id"`
	ProviderKey string             `json:"provider_key"`
	Checks      []string           `json:"checks"`
	Results     []*checkResultJSON `json:"results"`
	Summary     *reportSummary     `json:"summary"`
	Duration    string             `json:"total_duration"`
	Timestamp   string             `json:"timestamp"`
}

// checkResultJSON 单个检查项的 JSON 输出格式。
type checkResultJSON struct {
	CheckName       string `json:"check_name"`
	Status          string `json:"status"`
	Severity        string `json:"severity"`
	Message         string `json:"message"`
	SuggestedStatus string `json:"suggested_status,omitempty"`
	Duration        string `json:"duration"`
}

// reportSummary 报告摘要统计。
type reportSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Warning int `json:"warning"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Error   int `json:"error"`
}

// healthCmd 持有 account health 命令的参数。
type healthCmd struct {
	accountID    string
	providerType string
	providerName string
	checkNames   string // 逗号分隔的检查项名称
	outputDir    string
	listChecks   bool
	probeModel   string // probe 检查使用的模型名称
}

// cmd 返回 cobra.Command。
func (c *healthCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "对 Account 执行健康检查",
		Long: `对指定的 Account 或按条件过滤的 Account 执行健康检查。

支持执行所有检查项或通过 --checks 指定部分检查项。
可用的检查项：
  credential_validity   - 凭证格式与过期状态校验（本地检查）
  credential_refresh    - 凭证刷新检查（过期或即将过期时尝试刷新）
  probe                 - 真实请求探测（发送轻量请求验证可用性）
  usage_quota           - 用量配额检查（获取最新用量并判断是否耗尽）
  usage_rules_refresh   - 用量规则刷新（动态获取最新规则）
  model_discovery       - 模型发现（获取当前凭证支持的模型列表）
  recovery              - 冷却/熔断恢复检查

示例:
  # 检查单个账号（执行所有检查项）
  acpool account health --id acct-001

  # 检查单个账号，仅执行指定检查项
  acpool account health --id acct-001 --checks credential_validity,probe

  # 批量检查指定供应商下所有账号
  acpool account health --type kiro --name kiro-team-a

  # 将报告输出到目录
  acpool account health --type kiro --output ./health-reports

  # 查看所有可用的检查项
  acpool account health --list-checks`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.accountID, "id", "", "Account ID（指定单个账号）")
	cmd.Flags().StringVar(&c.providerType, "type", "", "按 Provider 类型过滤")
	cmd.Flags().StringVar(&c.providerName, "name", "", "按 Provider 名称过滤")
	cmd.Flags().StringVar(&c.checkNames, "checks", "", "指定检查项名称（逗号分隔），不指定则执行全部")
	cmd.Flags().StringVarP(&c.outputDir, "output", "o", "", "输出报告到指定目录")
	cmd.Flags().BoolVar(&c.listChecks, "list-checks", false, "列出所有可用的检查项")
	cmd.Flags().StringVar(&c.probeModel, "probe-model", "", "probe 检查使用的模型名称（不指定则使用 Provider 默认模型）")

	return cmd
}

// run 执行 account health 逻辑。
func (c *healthCmd) run(cmd *cobra.Command) error {
	// 列出可用检查项
	if c.listChecks {
		return c.printAvailableChecks()
	}

	// 至少指定 --id 或 --type
	if c.accountID == "" && c.providerType == "" {
		return fmt.Errorf("请指定 --id（单个账号）或 --type（按供应商过滤）")
	}

	deps := bootstrap.DepsFromContext(cmd.Context())

	// 获取目标账号列表
	accounts, err := c.resolveAccounts(cmd, deps)
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		fmt.Println("未找到符合条件的 Account")
		return nil
	}

	// 解析用户指定的检查项
	specifiedChecks, err := c.parseCheckNames()
	if err != nil {
		return err
	}

	// 逐个执行健康检查
	var reports []*healthReportJSON
	for _, account := range accounts {
		report, err := c.checkAccount(cmd, account, specifiedChecks)
		if err != nil {
			fmt.Printf("⚠ 检查 Account %s 失败: %v\n", account.ID, err)
			continue
		}
		reports = append(reports, report)
	}

	if len(reports) == 0 {
		fmt.Println("所有账号检查均失败")
		return nil
	}

	// 输出结果
	if c.outputDir != "" {
		return c.writeReports(reports)
	}
	return c.printReports(reports)
}

// resolveAccounts 根据参数获取要检查的账号列表。
func (c *healthCmd) resolveAccounts(cmd *cobra.Command, deps *bootstrap.Dependencies) ([]*acct.Account, error) {
	if c.accountID != "" {
		account, err := deps.AccountStorage.Get(cmd.Context(), c.accountID)
		if err != nil {
			return nil, handleStorageError("Account", err)
		}
		return []*acct.Account{account}, nil
	}

	filter := buildAccountFilter(c.providerType, c.providerName, 0)
	accounts, err := deps.AccountStorage.Search(cmd.Context(), filter)
	if err != nil {
		return nil, fmt.Errorf("查询 Account 失败: %w", err)
	}
	return accounts, nil
}

// parseCheckNames 解析 --checks 参数指定的检查项名称。
// 返回 nil 表示执行全部检查项。
func (c *healthCmd) parseCheckNames() ([]string, error) {
	if c.checkNames == "" {
		return nil, nil
	}

	names := strings.Split(c.checkNames, ",")
	var parsed []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := allCheckNames[name]; !ok {
			return nil, fmt.Errorf("未知的检查项 %q，使用 --list-checks 查看可用检查项", name)
		}
		parsed = append(parsed, name)
	}

	if len(parsed) == 0 {
		return nil, fmt.Errorf("--checks 参数不能为空")
	}

	return parsed, nil
}

// checkAccount 对单个账号执行健康检查。
// 复用 common.go 中的 executeHealthCheck 执行检查流程，以及 applyReportToAccount 更新账号状态。
func (c *healthCmd) checkAccount(cmd *cobra.Command, account *acct.Account, specifiedChecks []string) (*healthReportJSON, error) {
	// 构建检查项调度列表（复用公共方法）
	schedules := c.buildCheckSchedules(specifiedChecks)

	// 执行检查（复用公共的 executeHealthCheck）
	report, err := executeHealthCheck(cmd, account, schedules)
	if err != nil {
		return nil, err
	}

	// 终端输出检查过程
	if c.outputDir == "" {
		fmt.Printf("\n🔍 检查 Account: %s（Provider: %s）\n", account.ID, account.ProviderKey())
		printHealthReport(report)
	}

	// 根据检查结果更新账号状态等信息
	c.applyAndPersist(cmd, account, report)

	// 转为 JSON 结构
	return c.buildReportJSON(report, specifiedChecks), nil
}

// applyAndPersist 根据健康检查结果更新账号状态等信息，并持久化到存储。
// 复用 add/import 命令中相同的 applyReportToAccount 逻辑，确保行为一致。
func (c *healthCmd) applyAndPersist(cmd *cobra.Command, account *acct.Account, report *health.HealthReport) {
	deps := bootstrap.DepsFromContext(cmd.Context())
	providerKey := account.ProviderKey()

	oldStatus := account.Status

	// 复用公共的 applyReportToAccount，将检查结果中的状态、用量规则、模型列表等信息应用到账号
	usageStats := applyReportToAccount(cmd, deps, providerKey, account, report)

	// 更新账号信息到存储
	account.UpdatedAt = time.Now()
	if err := deps.AccountStorage.Update(cmd.Context(), account); err != nil {
		if err == storage.ErrVersionConflict {
			fmt.Printf("  ⚠ 更新 Account %s 失败: 版本冲突（账号可能已被其他操作修改）\n", account.ID)
		} else {
			fmt.Printf("  ⚠ 更新 Account %s 失败: %v\n", account.ID, err)
		}
		return
	}

	// 如果检查获取到了真实用量数据，更新 TrackedUsages
	if len(usageStats) > 0 {
		initTrackedUsages(cmd, deps, account.ID, usageStats)
	}

	if oldStatus != account.Status {
		fmt.Printf("  → Account %s 状态已更新: %s → %s\n", account.ID, oldStatus, account.Status)
	}
}

// buildCheckSchedules 根据指定的检查项名称构建调度配置。
// 如果 specifiedChecks 为 nil，则注册全部检查项。
// 复用 common.go 中的 buildDefaultCheckSchedules 获取基础检查项，并额外追加 RecoveryCheck。
func (c *healthCmd) buildCheckSchedules(specifiedChecks []string) []health.CheckSchedule {
	// 复用公共的基础检查项 + health 命令特有的 RecoveryCheck
	allChecks := append(
		buildDefaultCheckSchedules(c.probeModel),
		health.CheckSchedule{Check: checks.NewRecoveryCheck(), Enabled: true},
	)

	// 不指定则全部注册
	if len(specifiedChecks) == 0 {
		return allChecks
	}

	// 构建指定集合
	specified := make(map[string]struct{}, len(specifiedChecks))
	for _, name := range specifiedChecks {
		specified[name] = struct{}{}
	}

	// 收集指定的检查项及其依赖项（确保拓扑排序正常工作）
	needed := make(map[string]struct{})
	c.collectDependencies(specified, allChecks, needed)

	var result []health.CheckSchedule
	for _, s := range allChecks {
		if _, ok := needed[s.Check.Name()]; ok {
			result = append(result, s)
		}
	}

	return result
}

// collectDependencies 递归收集指定检查项的所有依赖。
func (c *healthCmd) collectDependencies(specified map[string]struct{}, allChecks []health.CheckSchedule, needed map[string]struct{}) {
	// 构建名称到检查项的映射
	checkMap := make(map[string]health.HealthCheck)
	for _, s := range allChecks {
		checkMap[s.Check.Name()] = s.Check
	}

	// 递归添加依赖
	var resolve func(name string)
	resolve = func(name string) {
		if _, exists := needed[name]; exists {
			return
		}
		needed[name] = struct{}{}
		if check, ok := checkMap[name]; ok {
			for _, dep := range check.DependsOn() {
				resolve(dep)
			}
		}
	}

	for name := range specified {
		resolve(name)
	}
}

// buildReportJSON 将 HealthReport 转为 JSON 输出结构。
func (c *healthCmd) buildReportJSON(report *health.HealthReport, specifiedChecks []string) *healthReportJSON {
	checkNames := specifiedChecks
	if len(checkNames) == 0 {
		checkNames = []string{"all"}
	}

	summary := &reportSummary{Total: len(report.Results)}
	results := make([]*checkResultJSON, 0, len(report.Results))

	for _, r := range report.Results {
		if r == nil {
			continue
		}

		// 统计汇总
		switch r.Status {
		case health.CheckPassed:
			summary.Passed++
		case health.CheckWarning:
			summary.Warning++
		case health.CheckFailed:
			summary.Failed++
		case health.CheckSkipped:
			summary.Skipped++
		case health.CheckError:
			summary.Error++
		}

		rj := &checkResultJSON{
			CheckName: r.CheckName,
			Status:    r.Status.String(),
			Severity:  r.Severity.String(),
			Message:   r.Message,
			Duration:  r.Duration.Round(time.Millisecond).String(),
		}
		if r.SuggestedStatus != nil {
			rj.SuggestedStatus = r.SuggestedStatus.String()
		}
		results = append(results, rj)
	}

	return &healthReportJSON{
		AccountID:   report.AccountID,
		ProviderKey: report.ProviderKey.String(),
		Checks:      checkNames,
		Results:     results,
		Summary:     summary,
		Duration:    report.TotalDuration.Round(time.Millisecond).String(),
		Timestamp:   report.Timestamp.Format(time.RFC3339),
	}
}

// printAvailableChecks 输出所有可用的检查项。
func (c *healthCmd) printAvailableChecks() error {
	fmt.Println("可用的健康检查项：")
	fmt.Println()

	// 按类别分组输出
	categories := []struct {
		title  string
		checks []string
	}{
		{
			title:  "凭证相关",
			checks: []string{checks.CredentialValidityCheckName, checks.CredentialRefreshCheckName},
		},
		{
			title:  "可用性检查",
			checks: []string{checks.ProbeCheckName, checks.RecoveryCheckName},
		},
		{
			title:  "用量相关",
			checks: []string{checks.UsageQuotaCheckName, checks.UsageRulesRefreshCheckName},
		},
		{
			title:  "信息发现",
			checks: []string{checks.ModelDiscoveryCheckName},
		},
	}

	for _, cat := range categories {
		fmt.Printf("  [%s]\n", cat.title)
		for _, name := range cat.checks {
			desc := allCheckNames[name]
			fmt.Printf("    %-25s %s\n", name, desc)
		}
		fmt.Println()
	}

	return nil
}

// printReports 在终端打印所有报告的汇总。
func (c *healthCmd) printReports(reports []*healthReportJSON) error {
	if len(reports) <= 1 {
		// 单账号场景已在 checkAccount 中输出了详细报告，只打印汇总
		if len(reports) == 1 {
			c.printSummaryLine(reports[0])
		}
		return nil
	}

	// 多账号场景，打印汇总表格
	fmt.Printf("\n══════════════════════════════════════\n")
	fmt.Printf("  健康检查汇总（共 %d 个账号）\n", len(reports))
	fmt.Printf("══════════════════════════════════════\n\n")

	var totalPassed, totalFailed, totalWarning int
	for _, r := range reports {
		c.printSummaryLine(r)
		totalPassed += r.Summary.Passed
		totalFailed += r.Summary.Failed
		totalWarning += r.Summary.Warning
	}

	fmt.Printf("\n── 总计: %d 通过, %d 警告, %d 失败 ──\n",
		totalPassed, totalWarning, totalFailed)

	return nil
}

// printSummaryLine 打印单个报告的一行摘要。
func (c *healthCmd) printSummaryLine(r *healthReportJSON) {
	icon := "✓"
	if r.Summary.Failed > 0 {
		icon = "✗"
	} else if r.Summary.Warning > 0 {
		icon = "⚠"
	}

	fmt.Printf("  %s [%s] %s — 通过:%d 警告:%d 失败:%d 跳过:%d 错误:%d (%s)\n",
		icon, r.AccountID, r.ProviderKey,
		r.Summary.Passed, r.Summary.Warning, r.Summary.Failed,
		r.Summary.Skipped, r.Summary.Error, r.Duration)
}

// writeReports 将健康检查报告输出到指定目录。
func (c *healthCmd) writeReports(reports []*healthReportJSON) error {
	// 拼接供应商子目录，避免多供应商数据互相污染
	dir := c.outputDir
	if c.providerType != "" {
		dir = filepath.Join(dir, c.providerType)
	}
	if c.providerName != "" {
		dir = filepath.Join(dir, c.providerName)
	}

	// 确保输出目录存在
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 每个账号输出一个独立的 JSON 报告文件
	for _, r := range reports {
		fileName := fmt.Sprintf("%s.health.json", r.AccountID)
		filePath := filepath.Join(dir, fileName)
		if err := ioutil.SaveJSONFile(filePath, r); err != nil {
			return fmt.Errorf("导出 Account %s 健康报告失败: %w", r.AccountID, err)
		}
	}

	// 输出汇总报告
	summaryPath := filepath.Join(dir, "_summary.json")
	summary := c.buildSummaryReport(reports)
	if err := ioutil.SaveJSONFile(summaryPath, summary); err != nil {
		return fmt.Errorf("导出汇总报告失败: %w", err)
	}

	fmt.Printf("已导出 %d 个 Account 的健康检查报告到目录 %s\n", len(reports), dir)
	return nil
}

// summaryReport 输出到文件的汇总报告。
type summaryReport struct {
	TotalAccounts int                    `json:"total_accounts"`
	HealthyCount  int                    `json:"healthy_count"`
	UnhealthyCount int                   `json:"unhealthy_count"`
	WarningCount  int                    `json:"warning_count"`
	Accounts      []*accountHealthBrief  `json:"accounts"`
	Timestamp     string                 `json:"timestamp"`
}

// accountHealthBrief 汇总中的单账号摘要。
type accountHealthBrief struct {
	AccountID   string         `json:"account_id"`
	ProviderKey string         `json:"provider_key"`
	Status      string         `json:"status"`
	Summary     *reportSummary `json:"summary"`
	Duration    string         `json:"duration"`
}

// buildSummaryReport 构建多账号汇总报告。
func (c *healthCmd) buildSummaryReport(reports []*healthReportJSON) *summaryReport {
	sr := &summaryReport{
		TotalAccounts: len(reports),
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	for _, r := range reports {
		status := "healthy"
		if r.Summary.Failed > 0 || r.Summary.Error > 0 {
			status = "unhealthy"
			sr.UnhealthyCount++
		} else if r.Summary.Warning > 0 {
			status = "warning"
			sr.WarningCount++
		} else {
			sr.HealthyCount++
		}

		sr.Accounts = append(sr.Accounts, &accountHealthBrief{
			AccountID:   r.AccountID,
			ProviderKey: r.ProviderKey,
			Status:      status,
			Summary:     r.Summary,
			Duration:    r.Duration,
		})
	}

	return sr
}

// writeJSONToFile 将数据以 JSON 格式写入文件（已通过 ioutil.SaveJSONFile 替代，保留为兼容方法）。
func writeJSONToFile(path string, data any) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}
	return os.WriteFile(path, content, 0644)
}

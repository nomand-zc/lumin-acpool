package account

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/health"
	"github.com/nomand-zc/lumin-acpool/health/checks"

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

// healthCmd 持有 account health 命令的参数。
type healthCmd struct {
	accountID    string
	providerType string
	providerName string
	checkNames   string // 逗号分隔的检查项名称
	outputDir    string
	listChecks   bool
	probeModel   string // probe 检查使用的模型名称
	daemon       bool   // 是否以 daemon 模式运行
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
	cmd.Flags().BoolVarP(&c.daemon, "daemon", "d", false, "以常驻进程模式运行，周期性执行健康检查")

	return cmd
}

// run 执行 account health 逻辑。
func (c *healthCmd) run(cmd *cobra.Command) error {
	// 列出可用检查项
	if c.listChecks {
		return c.printAvailableChecks()
	}

	// daemon 模式
	if c.daemon {
		return c.runDaemon(cmd)
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
		account, err := deps.Storage.GetAccount(cmd.Context(), c.accountID)
		if err != nil {
			return nil, handleStorageError("Account", err)
		}
		return []*acct.Account{account}, nil
	}

	filter := buildAccountFilter(c.providerType, c.providerName, 0)
	accounts, err := deps.Storage.SearchAccounts(cmd.Context(), filter)
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
// 复用 common.go 中的 executeHealthCheck 执行检查流程，以及 persistHealthReport 更新账号状态。
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

	// 根据检查结果更新账号状态等信息并持久化
	deps := bootstrap.DepsFromContext(cmd.Context())
	persistHealthReport(cmd.Context(), deps, account, report, "")

	// 转为 JSON 结构（使用更新后的账号最终状态）
	return c.buildReportJSON(report, specifiedChecks, account.Status.String()), nil
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

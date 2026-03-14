package account

import (
	"fmt"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
)

// removeCmd 持有 account remove 命令的参数。
type removeCmd struct {
	accountID    string
	providerType string
	providerName string
	all          bool
}

// cmd 返回 cobra.Command。
func (c *removeCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "删除 Account",
		Long: `按 ID 删除指定的 Account，或按 Provider 批量删除。

删除时会同时清理关联的统计数据（StatsStore）和用量追踪数据（UsageStore）。

示例:
  acpool account remove --id acct-001
  acpool account remove --type kiro --name kiro-team-a --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.accountID, "id", "", "Account ID（与 --all 互斥）")
	cmd.Flags().StringVar(&c.providerType, "type", "", "Provider 类型（配合 --all 使用）")
	cmd.Flags().StringVar(&c.providerName, "name", "", "Provider 名称（配合 --all 使用）")
	cmd.Flags().BoolVar(&c.all, "all", false, "按 Provider 批量删除所有 Account")

	return cmd
}

// run 执行 account remove 逻辑。
func (c *removeCmd) run(cmd *cobra.Command) error {
	if c.all {
		return c.runBatchRemove(cmd)
	}
	return c.runSingleRemove(cmd)
}

// runSingleRemove 按 ID 删除单个 Account。
func (c *removeCmd) runSingleRemove(cmd *cobra.Command) error {
	if c.accountID == "" {
		return fmt.Errorf("请指定 --id 参数或使用 --all 批量删除")
	}

	deps := bootstrap.DepsFromContext(cmd.Context())

	// 删除账号
	if err := deps.AccountStorage.RemoveAccount(cmd.Context(), c.accountID); err != nil {
		return handleStorageError("Account", err)
	}

	// 清理关联的统计数据
	if err := deps.StatsStore.RemoveStats(cmd.Context(), c.accountID); err != nil {
		// 统计数据清理失败不阻塞主流程，仅打印警告
		fmt.Printf("警告: 清理统计数据失败: %v\n", err)
	}

	// 清理关联的用量追踪数据
	if err := deps.UsageStore.RemoveUsages(cmd.Context(), c.accountID); err != nil {
		// 用量追踪数据清理失败不阻塞主流程，仅打印警告
		fmt.Printf("警告: 清理用量追踪数据失败: %v\n", err)
	}

	fmt.Printf("Account %s 已删除\n", c.accountID)
	return nil
}

// runBatchRemove 按 Provider 批量删除 Account。
func (c *removeCmd) runBatchRemove(cmd *cobra.Command) error {
	if c.providerType == "" || c.providerName == "" {
		return fmt.Errorf("批量删除需要同时指定 --type 和 --name 参数")
	}

	deps := bootstrap.DepsFromContext(cmd.Context())
	providerKey := acct.BuildProviderKey(c.providerType, c.providerName)

	// 先查询该 Provider 下所有账号，用于后续清理关联数据
	filter := buildAccountFilter(c.providerType, c.providerName, 0)
	accounts, err := deps.AccountStorage.SearchAccounts(cmd.Context(), filter)
	if err != nil {
		return fmt.Errorf("查询 Account 失败: %w", err)
	}

	if len(accounts) == 0 {
		fmt.Printf("Provider %s 下没有 Account\n", providerKey)
		return nil
	}

	// 批量删除账号
	if err := deps.AccountStorage.RemoveAccounts(cmd.Context(), filter); err != nil {
		return fmt.Errorf("批量删除 Account 失败: %w", err)
	}

	// 清理所有被删除账号的关联数据
	var cleanupErrs int
	for _, a := range accounts {
		if err := deps.StatsStore.RemoveStats(cmd.Context(), a.ID); err != nil {
			cleanupErrs++
		}
		if err := deps.UsageStore.RemoveUsages(cmd.Context(), a.ID); err != nil {
			cleanupErrs++
		}
	}

	fmt.Printf("已删除 Provider %s 下的 %d 个 Account\n", providerKey, len(accounts))
	if cleanupErrs > 0 {
		fmt.Printf("警告: %d 项关联数据清理失败\n", cleanupErrs)
	}
	return nil
}

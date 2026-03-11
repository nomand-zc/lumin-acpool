package provider

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// removeCmd 持有 provider remove 命令的参数。
type removeCmd struct {
	providerType string
	providerName string
}

// cmd 返回 cobra.Command。
func (c *removeCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "删除 Provider",
		Long: `按 type + name 删除指定的 Provider，同时级联删除该 Provider 下的所有 Account 及关联数据。

示例:
  acpool provider remove --type kiro --name kiro-team-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.providerType, "type", "", "Provider 类型（必填）")
	cmd.Flags().StringVar(&c.providerName, "name", "", "Provider 名称（必填）")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// run 执行 provider remove 逻辑。
func (c *removeCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())
	key := account.BuildProviderKey(c.providerType, c.providerName)

	// 1. 级联删除该 Provider 下的所有 Account 及关联数据
	removedCount, cleanupErrs := c.cascadeRemoveAccounts(cmd, deps)

	// 2. 删除 Provider 自身
	if err := deps.ProviderStorage.Remove(cmd.Context(), key); err != nil {
		return handleStorageError("Provider", err)
	}

	if removedCount > 0 {
		fmt.Printf("Provider %s 已删除（同时删除了 %d 个 Account）\n", key, removedCount)
		if cleanupErrs > 0 {
			fmt.Printf("警告: %d 项关联数据清理失败\n", cleanupErrs)
		}
	} else {
		fmt.Printf("Provider %s 已删除\n", key)
	}
	return nil
}

// cascadeRemoveAccounts 级联删除该 Provider 下的所有 Account 及关联的统计数据和用量追踪数据。
// 返回删除的账号数量和关联数据清理失败的次数。
func (c *removeCmd) cascadeRemoveAccounts(cmd *cobra.Command, deps *bootstrap.Dependencies) (removedCount int, cleanupErrs int) {
	ctx := cmd.Context()

	// 构建过滤条件：精确匹配 provider_type + provider_name
	filter := buildCascadeFilter(c.providerType, c.providerName)

	// 查询该 Provider 下所有账号（用于后续清理关联数据）
	accounts, err := deps.AccountStorage.Search(ctx, filter)
	if err != nil {
		fmt.Printf("警告: 查询 Provider 下的 Account 失败: %v，跳过级联删除\n", err)
		return 0, 0
	}
	if len(accounts) == 0 {
		return 0, 0
	}

	// 批量删除账号
	if err := deps.AccountStorage.RemoveFilter(ctx, filter); err != nil {
		fmt.Printf("警告: 批量删除 Account 失败: %v\n", err)
		return 0, 0
	}

	// 清理所有被删除账号的关联数据（统计数据 + 用量追踪数据）
	for _, a := range accounts {
		if err := deps.StatsStore.Remove(ctx, a.ID); err != nil {
			cleanupErrs++
		}
		if err := deps.UsageStore.Remove(ctx, a.ID); err != nil {
			cleanupErrs++
		}
	}

	return len(accounts), cleanupErrs
}

// buildCascadeFilter 构建用于级联删除的过滤条件（精确匹配 provider_type 和 provider_name）。
func buildCascadeFilter(providerType, providerName string) *filtercond.Filter {
	return &filtercond.Filter{
		Operator: filtercond.OperatorAnd,
		Value: []*filtercond.Filter{
			{
				Field:    "provider_type",
				Operator: filtercond.OperatorEqual,
				Value:    providerType,
			},
			{
				Field:    "provider_name",
				Operator: filtercond.OperatorEqual,
				Value:    providerName,
			},
		},
	}
}

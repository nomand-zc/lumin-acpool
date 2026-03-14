package provider

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
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
// 存储层的 RemoveProvider 会事务性地级联删除该 Provider 下的所有 Account 及关联的统计数据和用量追踪数据。
func (c *removeCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())
	key := account.BuildProviderKey(c.providerType, c.providerName)

	if err := deps.Storage.RemoveProvider(cmd.Context(), key); err != nil {
		return handleStorageError("Provider", err)
	}

	fmt.Printf("Provider %s 已删除（关联的 Account 及数据已同步清理）\n", key)
	return nil
}

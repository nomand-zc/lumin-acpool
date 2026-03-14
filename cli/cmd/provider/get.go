package provider

import (
	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/output"
)

// getCmd 持有 provider get 命令的参数。
type getCmd struct {
	providerType string
	providerName string
	format output.Format
}

// cmd 返回 cobra.Command。
func (c *getCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "查看 Provider 详情",
		Long: `按 type + name 查看单个 Provider 的详细信息。

示例:
  acpool provider get --type kiro --name kiro-team-a
  acpool provider get --type kiro --name kiro-team-a -o table`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.providerType, "type", "", "Provider 类型（必填）")
	cmd.Flags().StringVar(&c.providerName, "name", "", "Provider 名称（必填）")
	cmd.Flags().StringVarP((*string)(&c.format), "format", "f", string(output.FormatJSON), "输出格式: table | json")
	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// run 执行 provider get 逻辑。
func (c *getCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())
	key := account.BuildProviderKey(c.providerType, c.providerName)

	info, err := deps.ProviderStorage.GetProvider(cmd.Context(), key)
	if err != nil {
		return handleStorageError("Provider", err)
	}

	printer := &output.Printer{Format: c.format}
	if c.format == output.FormatJSON {
		return printer.PrintJSON(info)
	}
	return printer.PrintTable(providerTableHeaders(), providerTableRows([]*account.ProviderInfo{info}))
}

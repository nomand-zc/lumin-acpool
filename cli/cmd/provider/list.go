package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/cli/internal/output"
)

// listCmd 持有 provider list 命令的参数。
type listCmd struct {
	providerType string
	providerName string
	status       int
	format       output.Format
	outputDir    string
}

// cmd 返回 cobra.Command。
func (c *listCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "列出 Provider",
		Long: `列出所有或按条件过滤的 Provider。

当指定 --output 目录时：
  - table 格式：输出到目录下的 providers.txt 文件
  - json 格式：每个 Provider 输出为一个独立的 JSON 文件（以 type_name.json 命名）

示例:
  acpool provider list
  acpool provider list --type kiro
  acpool provider list --type kiro --status 1 -f json
  acpool provider list --output ./export
  acpool provider list --output ./export -f json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.providerType, "type", "", "按 Provider 类型过滤")
	cmd.Flags().StringVar(&c.providerName, "name", "", "按 Provider 名称过滤（模糊匹配）")
	cmd.Flags().IntVar(&c.status, "status", 0, "按状态过滤 (1=active, 2=disabled, 3=degraded)")
	cmd.Flags().StringVarP((*string)(&c.format), "format", "f", string(output.FormatTable), "输出格式: table | json")
	cmd.Flags().StringVarP(&c.outputDir, "output", "o", "", "输出到指定目录（不指定则输出到终端）")

	return cmd
}

// run 执行 provider list 逻辑。
func (c *listCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())
	filter := buildProviderFilter(c.providerType, c.providerName, c.status)

	providers, err := deps.Storage.SearchProviders(cmd.Context(), filter)
	if err != nil {
		return fmt.Errorf("查询 Provider 失败: %w", err)
	}

	// 如果指定了输出目录，写入文件
	if c.outputDir != "" {
		return c.writeToDir(providers)
	}

	// 默认输出到终端
	printer := &output.Printer{Format: c.format}
	if c.format == output.FormatJSON {
		return printer.PrintJSON(providers)
	}
	return printer.PrintTable(providerTableHeaders(), providerTableRows(providers))
}

// writeToDir 将 Provider 数据写入到指定目录。
// table 格式：输出到一个 providers.txt 文件；
// json 格式：每个 Provider 输出为一个独立的 JSON 文件。
func (c *listCmd) writeToDir(providers []*account.ProviderInfo) error {
	// 确保输出目录存在
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	if c.format == output.FormatJSON {
		return c.writeJSONFiles(providers)
	}
	return c.writeTableFile(providers)
}

// writeTableFile 将所有 Provider 以表格格式写入到一个文件。
func (c *listCmd) writeTableFile(providers []*account.ProviderInfo) error {
	filePath := filepath.Join(c.outputDir, "providers.txt")

	printer := &output.Printer{Format: c.format}
	if err := printer.PrintTableToFile(filePath, providerTableHeaders(), providerTableRows(providers)); err != nil {
		return err
	}

	fmt.Printf("已导出 %d 个 Provider 到 %s\n", len(providers), filePath)
	return nil
}

// writeJSONFiles 将每个 Provider 输出为一个独立的 JSON 文件，
// 文件名格式为 {type}_{name}.json。
func (c *listCmd) writeJSONFiles(providers []*account.ProviderInfo) error {
	for _, p := range providers {
		fileName := fmt.Sprintf("%s_%s.json", p.ProviderType, p.ProviderName)
		filePath := filepath.Join(c.outputDir, fileName)

		if err := ioutil.SaveJSONFile(filePath, p); err != nil {
			return fmt.Errorf("导出 Provider %s 失败: %w", p.ProviderKey(), err)
		}
	}

	fmt.Printf("已导出 %d 个 Provider 到目录 %s\n", len(providers), c.outputDir)
	return nil
}

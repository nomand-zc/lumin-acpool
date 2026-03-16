package account

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/cli/internal/output"
)

// listCmd 持有 account list 命令的参数。
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
		Short: "列出 Account",
		Long: `列出所有或按条件过滤的 Account。

当指定 --output 目录时：
  - table 格式：输出到目录下的 accounts.txt 文件
  - json 格式：每个 Account 输出为一个独立的 JSON 文件（以 ID.json 命名）

示例:
  acpool account list
  acpool account list --type kiro
  acpool account list --type kiro --name kiro-team-a --status 1
  acpool account list --output ./export
  acpool account list --output ./export -f json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.providerType, "type", "", "按 Provider 类型过滤")
	cmd.Flags().StringVar(&c.providerName, "name", "", "按 Provider 名称过滤（模糊匹配）")
	cmd.Flags().IntVar(&c.status, "status", 0, "按状态过滤 (0=全部, 1=available, 2=cooling_down, 3=circuit_open, 4=expired, 5=invalidated, 6=banned, 7=disabled)")
	cmd.Flags().StringVarP((*string)(&c.format), "format", "f", string(output.FormatTable), "输出格式: table | json")
	cmd.Flags().StringVarP(&c.outputDir, "output", "o", "", "输出到指定目录（不指定则输出到终端）")

	return cmd
}

// run 执行 account list 逻辑。
func (c *listCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())
	filter := buildAccountFilter(c.providerType, c.providerName, c.status)

	accounts, err := deps.Storage.SearchAccounts(cmd.Context(), filter)
	if err != nil {
		return fmt.Errorf("查询 Account 失败: %w", err)
	}

	// 聚合统计和用量追踪数据
	adapter := &depsAdapter{statsStore: deps.Storage, usageStore: deps.Storage}
	details := make([]*AccountDetail, 0, len(accounts))
	for _, a := range accounts {
		details = append(details, enrichAccountDetail(cmd.Context(), adapter, a))
	}

	// 如果指定了输出目录，写入文件
	if c.outputDir != "" {
		return c.writeToDir(details)
	}

	// 默认输出到终端
	printer := &output.Printer{Format: c.format}
	if c.format == output.FormatJSON {
		return printer.PrintJSON(details)
	}
	return printer.PrintTable(accountTableHeaders(), accountDetailTableRows(details))
}

// writeToDir 将 Account 数据写入到指定目录。
// 当指定了 --type 或 --name 过滤条件时，会在输出目录下自动创建
// {type}/{name} 子目录，防止多个供应商的导出数据互相污染。
// table 格式：输出到一个 accounts.txt 文件；
// json 格式：每个 Account 输出为一个独立的 JSON 文件。
func (c *listCmd) writeToDir(details []*AccountDetail) error {
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

	// 临时替换 outputDir 为拼接后的目录
	origDir := c.outputDir
	c.outputDir = dir
	defer func() { c.outputDir = origDir }()

	if c.format == output.FormatJSON {
		return c.writeJSONFiles(details)
	}
	return c.writeTableFile(details)
}

// writeTableFile 将所有 Account 以表格格式写入到一个文件。
func (c *listCmd) writeTableFile(details []*AccountDetail) error {
	filePath := filepath.Join(c.outputDir, "accounts.txt")

	printer := &output.Printer{Format: c.format}
	if err := printer.PrintTableToFile(filePath, accountTableHeaders(), accountDetailTableRows(details)); err != nil {
		return err
	}

	fmt.Printf("已导出 %d 个 Account 到 %s\n", len(details), filePath)
	return nil
}

// writeJSONFiles 将每个 Account 输出为一个独立的 JSON 文件，
// 文件名格式为 {ID}.json。
func (c *listCmd) writeJSONFiles(details []*AccountDetail) error {
	for _, d := range details {
		fileName := fmt.Sprintf("%s.json", d.ID)
		filePath := filepath.Join(c.outputDir, fileName)

		if err := ioutil.SaveJSONFile(filePath, d); err != nil {
			return fmt.Errorf("导出 Account %s 失败: %w", d.ID, err)
		}
	}

	fmt.Printf("已导出 %d 个 Account 到目录 %s\n", len(details), c.outputDir)
	return nil
}

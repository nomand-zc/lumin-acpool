package account

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/output"
)

// getCmd 持有 account get 命令的参数。
type getCmd struct {
	accountID string
	format    output.Format
}

// cmd 返回 cobra.Command。
func (c *getCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get",
		Short: "查看 Account 详情",
		Long: `按 ID 查看单个 Account 的详细信息，包括运行统计和用量追踪数据。

示例:
  acpool account get --id acct-001
  acpool account get --id acct-001 -f table`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVar(&c.accountID, "id", "", "Account ID（必填）")
	cmd.Flags().StringVarP((*string)(&c.format), "format", "f", string(output.FormatJSON), "输出格式: table | json")
	_ = cmd.MarkFlagRequired("id")

	return cmd
}

// run 执行 account get 逻辑。
func (c *getCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

acct, err := deps.Storage.GetAccount(cmd.Context(), c.accountID)
	if err != nil {
		return handleStorageError("Account", err)
	}

	// 聚合统计和用量追踪数据
adapter := &depsAdapter{statsStore: deps.Storage, usageStore: deps.Storage}
	detail := enrichAccountDetail(cmd.Context(), adapter, acct)

	printer := &output.Printer{Format: c.format}
	if c.format == output.FormatJSON {
		return printer.PrintJSON(detail)
	}

	// table 格式：分段展示详细信息
	return c.printDetailTable(detail)
}

// printDetailTable 以多段表格形式展示账号详情。
func (c *getCmd) printDetailTable(detail *AccountDetail) error {
	printer := &output.Printer{Format: output.FormatTable}

	// 第一段：基础信息
	fmt.Println("── 基础信息 ──")
	if err := printer.PrintTable(
		accountTableHeaders(),
		accountDetailTableRows([]*AccountDetail{detail}),
	); err != nil {
		return err
	}

	// 第二段：运行统计
	fmt.Println("\n── 运行统计 ──")
	if detail.Stats != nil && detail.Stats.TotalCalls > 0 {
		lastUsed := "-"
		if detail.Stats.LastUsedAt != nil {
			lastUsed = detail.Stats.LastUsedAt.Format("2006-01-02 15:04:05")
		}
		lastError := "-"
		if detail.Stats.LastErrorAt != nil {
			lastError = detail.Stats.LastErrorAt.Format("2006-01-02 15:04:05")
		}
		lastErrMsg := "-"
		if detail.Stats.LastErrorMsg != "" {
			lastErrMsg = detail.Stats.LastErrorMsg
			if len(lastErrMsg) > 60 {
				lastErrMsg = lastErrMsg[:60] + "..."
			}
		}
		if err := printer.PrintTable(
			[]string{"TOTAL", "SUCCESS", "FAILED", "CONSEC_FAIL", "SUCCESS_RATE", "LAST_USED", "LAST_ERROR", "LAST_ERR_MSG"},
			[][]string{{
				fmt.Sprintf("%d", detail.Stats.TotalCalls),
				fmt.Sprintf("%d", detail.Stats.SuccessCalls),
				fmt.Sprintf("%d", detail.Stats.FailedCalls),
				fmt.Sprintf("%d", detail.Stats.ConsecutiveFailures),
				fmt.Sprintf("%.1f%%", detail.Stats.SuccessRate()*100),
				lastUsed,
				lastError,
				lastErrMsg,
			}},
		); err != nil {
			return err
		}
	} else {
		fmt.Println("  暂无统计数据")
	}

	// 第三段：用量追踪
	fmt.Println("\n── 用量追踪 ──")
	if len(detail.Usages) > 0 {
		headers := []string{"#", "SOURCE", "GRANULARITY", "TOTAL", "EST_USED", "EST_REMAIN", "REMAIN%", "WINDOW", "LAST_SYNC"}
		var rows [][]string
		for i, u := range detail.Usages {
			sourceType := "-"
			granularity := "-"
			total := "-"
			if u.Rule != nil {
				switch u.Rule.SourceType {
				case 1:
					sourceType = "token"
				case 2:
					sourceType = "request"
				}
				granularity = fmt.Sprintf("%s×%d", u.Rule.TimeGranularity, u.Rule.WindowSize)
				total = fmt.Sprintf("%.0f", u.Rule.Total)
			}

			window := "-"
			if u.WindowStart != nil && u.WindowEnd != nil {
				window = fmt.Sprintf("%s ~ %s",
					u.WindowStart.Format("01/02 15:04"),
					u.WindowEnd.Format("01/02 15:04"))
			}

			var statusParts []string
			if u.IsExhausted() {
				statusParts = append(statusParts, "⚠ 已耗尽")
			}

			remainInfo := fmt.Sprintf("%.0f", u.EstimatedRemain())
			if len(statusParts) > 0 {
				remainInfo += " " + strings.Join(statusParts, " ")
			}

			rows = append(rows, []string{
				fmt.Sprintf("%d", i),
				sourceType,
				granularity,
				total,
				fmt.Sprintf("%.2f", u.EstimatedUsed()),
				remainInfo,
				fmt.Sprintf("%.1f%%", u.RemainRatio()*100),
				window,
				u.LastSyncAt.Format("2006-01-02 15:04:05"),
			})
		}
		if err := printer.PrintTable(headers, rows); err != nil {
			return err
		}
	} else {
		fmt.Println("  暂无用量追踪数据")
	}

	return nil
}

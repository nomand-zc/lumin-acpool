package account

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

var (
	defaultListCmd   listCmd
	defaultGetCmd    getCmd
	defaultAddCmd    addCmd
	defaultImportCmd importCmd
	defaultUpdateCmd updateCmd
	defaultRemoveCmd removeCmd
	defaultHealthCmd healthCmd
)

// CMD 返回 account 命令组，注册所有子命令。
// 依赖通过 Cobra 原生 Context 传递（root PersistentPreRunE 中注入）。
func CMD() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Account 管理",
		Long:  `管理 Account（账号）信息，包括增删改查操作。`,
	}

	cmd.AddCommand(
		defaultListCmd.cmd(),
		defaultGetCmd.cmd(),
		defaultAddCmd.cmd(),
		defaultImportCmd.cmd(),
		defaultUpdateCmd.cmd(),
		defaultRemoveCmd.cmd(),
		defaultHealthCmd.cmd(),
	)

	return cmd
}

// ===================== 聚合视图 =====================

// AccountDetail 聚合账号基础信息、运行统计和用量追踪数据，用于统一输出。
type AccountDetail struct {
	*account.Account
	Stats  *account.AccountStats   `json:"stats,omitempty"`
	Usages []*account.TrackedUsage `json:"usages,omitempty"`
}

// AccountDetailEnricher 定义获取账号统计和用量数据的能力接口。
type AccountDetailEnricher interface {
	GetStats(ctx context.Context, accountID string) (*account.AccountStats, error)
	GetUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error)
}

// enrichAccountDetail 从 StatsStore 和 UsageStore 获取统计和用量数据，构建 AccountDetail。
func enrichAccountDetail(ctx context.Context, deps AccountDetailEnricher, acct *account.Account) *AccountDetail {
	detail := &AccountDetail{Account: acct}
	if deps == nil {
		return detail
	}
	if stats, err := deps.GetStats(ctx, acct.ID); err == nil {
		detail.Stats = stats
	}
	if usages, err := deps.GetUsages(ctx, acct.ID); err == nil {
		detail.Usages = usages
	}
	return detail
}

// depsAdapter 将 bootstrap.Dependencies 适配为 enrichAccountDetail 需要的接口。
type depsAdapter struct {
	statsStore storage.StatsStore
	usageStore storage.UsageStore
}

func (d *depsAdapter) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
	if d.statsStore == nil {
		return nil, nil
	}
	return d.statsStore.GetStats(ctx, accountID)
}

func (d *depsAdapter) GetUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	if d.usageStore == nil {
		return nil, nil
	}
	return d.usageStore.GetAllUsages(ctx, accountID)
}

// ===================== 公共辅助 =====================

// handleStorageError 统一处理 storage 层错误，转为用户友好的提示。
func handleStorageError(resource string, err error) error {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return fmt.Errorf("%s 不存在", resource)
	case errors.Is(err, storage.ErrAlreadyExists):
		return fmt.Errorf("%s 已存在", resource)
	case errors.Is(err, storage.ErrVersionConflict):
		return fmt.Errorf("%s 版本冲突，请重试", resource)
	default:
		return fmt.Errorf("操作 %s 失败: %w", resource, err)
	}
}

// accountStatusString 将 Status 转为可读字符串。
func accountStatusString(s account.Status) string {
	return s.String()
}

// accountTableHeaders 返回 Account 列表的表头（含统计和用量摘要列）。
func accountTableHeaders() []string {
	return []string{"ID", "PROVIDER", "STATUS", "PRIORITY", "CALLS(S/F)", "SUCCESS_RATE", "USAGE", "CREDENTIAL", "CREATED_AT"}
}

// accountDetailTableRows 将 AccountDetail 列表转为表格行数据（含统计和用量摘要）。
func accountDetailTableRows(details []*AccountDetail) [][]string {
	rows := make([][]string, 0, len(details))
	for _, d := range details {
		// 凭证信息：显示 access token 前 8 位（脱敏）
		credInfo := "N/A"
		if d.Credential != nil {
			token := d.Credential.GetAccessToken()
			if len(token) > 8 {
				credInfo = token[:8] + "..."
			} else if token != "" {
				credInfo = token + "..."
			}
		}

		// 统计摘要
		callsInfo := "-"
		successRate := "-"
		if d.Stats != nil && d.Stats.TotalCalls > 0 {
			callsInfo = fmt.Sprintf("%d(%d/%d)", d.Stats.TotalCalls, d.Stats.SuccessCalls, d.Stats.FailedCalls)
			successRate = fmt.Sprintf("%.1f%%", d.Stats.SuccessRate()*100)
		}

		// 用量摘要
		usageInfo := "-"
		if len(d.Usages) > 0 {
			var parts []string
			for _, u := range d.Usages {
				if u.Rule == nil || u.Rule.Total <= 0 {
					continue
				}
				parts = append(parts, fmt.Sprintf("%.0f/%.0f(%.0f%%)",
					u.EstimatedUsed(), u.Rule.Total, u.RemainRatio()*100))
			}
			if len(parts) > 0 {
				usageInfo = strings.Join(parts, ", ")
			}
		}

		rows = append(rows, []string{
			d.ID,
			d.ProviderKey().String(),
			accountStatusString(d.Status),
			strconv.Itoa(d.Priority),
			callsInfo,
			successRate,
			usageInfo,
			credInfo,
			d.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	return rows
}

// buildAccountFilter 根据命令行 flags 构建 SearchFilter。
// 零值的 flag 不参与过滤。
func buildAccountFilter(providerType, providerName string, status int) *storage.SearchFilter {
	if providerType == "" && providerName == "" && status <= 0 {
		return nil
	}
	return &storage.SearchFilter{
		ProviderType: providerType,
		ProviderName: providerName,
		Status:       status,
	}
}

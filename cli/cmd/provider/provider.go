package provider

import (
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
	defaultUpdateCmd updateCmd
	defaultRemoveCmd removeCmd
)

// CMD 返回 provider 命令组，注册所有子命令。
// 依赖通过 Cobra 原生 Context 传递（root PersistentPreRunE 中注入）。
func CMD() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Provider 管理",
		Long:  `管理 Provider（供应商）信息，包括增删改查操作。`,
	}

	cmd.AddCommand(
		defaultListCmd.cmd(),
		defaultGetCmd.cmd(),
		defaultAddCmd.cmd(),
		defaultUpdateCmd.cmd(),
		defaultRemoveCmd.cmd(),
	)

	return cmd
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

// providerStatusString 将 ProviderStatus 转为可读字符串。
func providerStatusString(s account.ProviderStatus) string {
	switch s {
	case account.ProviderStatusActive:
		return "active"
	case account.ProviderStatusDisabled:
		return "disabled"
	case account.ProviderStatusDegraded:
		return "degraded"
	default:
		return fmt.Sprintf("unknown(%d)", s)
	}
}

// providerTableHeaders 返回 Provider 列表的表头。
func providerTableHeaders() []string {
	return []string{"TYPE", "NAME", "STATUS", "PRIORITY", "WEIGHT", "MODELS", "ACCOUNTS"}
}

// providerTableRows 将 ProviderInfo 列表转为表格行数据。
func providerTableRows(providers []*account.ProviderInfo) [][]string {
	rows := make([][]string, 0, len(providers))
	for _, p := range providers {
		models := strings.Join(p.SupportedModels, ",")
		accounts := fmt.Sprintf("%d/%d", p.AvailableAccountCount, p.AccountCount)
		rows = append(rows, []string{
			p.ProviderType,
			p.ProviderName,
			providerStatusString(p.Status),
			strconv.Itoa(p.Priority),
			strconv.Itoa(p.Weight),
			models,
			accounts,
		})
	}
	return rows
}

// buildProviderFilter 根据命令行 flags 构建 SearchFilter。
// 零值的 flag 不参与过滤。
func buildProviderFilter(providerType, providerName string, status int) *storage.SearchFilter {
	if providerType == "" && providerName == "" && status <= 0 {
		return nil
	}
	return &storage.SearchFilter{
		ProviderType: providerType,
		ProviderName: providerName,
		Status:       status,
	}
}

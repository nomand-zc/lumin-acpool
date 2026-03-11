package provider

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// updateCmd 持有 provider update 命令的参数。
type updateCmd struct {
	filePath string
}

// cmd 返回 cobra.Command。
func (c *updateCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "更新 Provider",
		Long: `从 JSON 文件更新 Provider 信息（全量替换）。

示例:
  acpool provider update --file provider.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "Provider JSON 文件路径（必填）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 provider update 逻辑。
func (c *updateCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	info, err := ioutil.LoadJSONFile[account.ProviderInfo](c.filePath)
	if err != nil {
		return err
	}

	// 从数据库获取老数据，用于填充空字段和保护运行时字段
	oldInfo, err := deps.ProviderStorage.Get(cmd.Context(), info.ProviderKey())
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("Provider %s 不存在，无法更新", info.ProviderKey())
		}
		return fmt.Errorf("获取现有 Provider 信息失败: %w", err)
	}

	// 用老数据合并填充零值字段，并保护运行时字段
	mergeWithExisting(info, oldInfo)

	if err := deps.ProviderStorage.Update(cmd.Context(), info); err != nil {
		return handleStorageError("Provider", err)
	}

	fmt.Printf("Provider %s 更新成功\n", info.ProviderKey())
	return nil
}

// mergeWithExisting 将新数据中的零值字段用老数据填充，
// 并强制使用老数据覆盖不应被外部更新的运行时字段，防止异常覆盖。
func mergeWithExisting(info, old *account.ProviderInfo) {
	// --- 零值字段回填：新数据为空/零值时，用老数据填充 ---

	if len(info.SupportedModels) == 0 {
		info.SupportedModels = old.SupportedModels
	}
	if len(info.UsageRules) == 0 {
		info.UsageRules = old.UsageRules
	}
	if info.Priority == 0 {
		info.Priority = old.Priority
	}
	if info.Weight == 0 {
		info.Weight = old.Weight
	}
	if len(info.Tags) == 0 {
		info.Tags = old.Tags
	}
	if len(info.Metadata) == 0 {
		info.Metadata = old.Metadata
	}

	// --- 运行时字段保护：始终用数据库值覆盖，不允许外部修改 ---

	info.Status = old.Status
	info.AccountCount = old.AccountCount
	info.AvailableAccountCount = old.AvailableAccountCount
	info.CreatedAt = old.CreatedAt
}

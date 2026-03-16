package account

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/credentials"
)

// updateCmd 持有 account update 命令的参数。
type updateCmd struct {
	filePath string
}

// cmd 返回 cobra.Command。
func (c *updateCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "更新 Account",
		Long: `从 JSON 文件更新 Account 信息（部分更新）。

可更新字段：Credential、Priority、Tags、Metadata。
以下字段由系统维护，不允许通过此命令修改：
  - Status、CooldownUntil、CircuitOpenUntil（运行时状态）
  - ProviderType、ProviderName（归属标识）
  - Version、CreatedAt（系统字段）

示例:
  acpool account update --file account.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "Account JSON 文件路径（必填）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 account update 逻辑。
func (c *updateCmd) run(cmd *cobra.Command) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	raw, err := ioutil.LoadJSONFile[accountUpdateJSON](c.filePath)
	if err != nil {
		return err
	}

	if raw.ID == "" {
		return fmt.Errorf("JSON 文件中 ID 字段不能为空")
	}

	// 从数据库获取老数据，用于填充空字段和保护运行时字段
	oldAcct, err := deps.Storage.GetAccount(cmd.Context(), raw.ID)
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("Account %s 不存在，无法更新", raw.ID)
		}
		return fmt.Errorf("获取现有 Account 信息失败: %w", err)
	}

	// 解析凭证（如果提供了新凭证）
	var newCred credentials.Credential
	if len(raw.Credential) > 0 {
		cred, err := parseCredential(oldAcct.ProviderType, raw.Credential)
		if err != nil {
			return fmt.Errorf("解析凭证失败: %w", err)
		}
		newCred = cred
	}

	// 用老数据合并填充零值字段，并保护运行时字段
	updated, updateFields := mergeAccountWithExisting(raw, oldAcct, newCred)

	if err := deps.Storage.UpdateAccount(cmd.Context(), updated, updateFields); err != nil {
		return handleStorageError("Account", err)
	}

	fmt.Printf("Account %s 更新成功\n", updated.ID)
	return nil
}

// accountUpdateJSON 是 Account 更新操作的 JSON 反序列化中间结构。
// ProviderType、ProviderName 不可通过此命令修改，如需迁移请使用 remove + add。
type accountUpdateJSON struct {
	ID         string            `json:"ID"`
	Credential json.RawMessage   `json:"Credential"`
	Priority   int               `json:"Priority"`
	Tags       map[string]string `json:"Tags"`
	Metadata   map[string]any    `json:"Metadata"`
}

// mergeAccountWithExisting 将新数据中的零值字段用老数据填充，
// 并强制使用老数据覆盖不应被外部更新的运行时字段，防止异常覆盖。
// 返回合并后的 Account 和需要更新的字段掩码。
func mergeAccountWithExisting(raw *accountUpdateJSON, old *acct.Account, newCred credentials.Credential) (*acct.Account, storage.UpdateField) {
	updated := old.Clone()
	updated.UpdatedAt = time.Now()

	var fields storage.UpdateField

	// --- 可更新字段：新数据非空时覆盖 ---

	if newCred != nil {
		updated.Credential = newCred
		fields |= storage.UpdateFieldCredential
	}
	if raw.Priority != 0 {
		updated.Priority = raw.Priority
		fields |= storage.UpdateFieldPriority
	}
	if len(raw.Tags) > 0 {
		updated.Tags = raw.Tags
		fields |= storage.UpdateFieldTags
	}
	if len(raw.Metadata) > 0 {
		updated.Metadata = raw.Metadata
		fields |= storage.UpdateFieldMetadata
	}

	// --- 运行时字段保护：始终使用数据库值，不允许外部修改 ---
	// Status、CooldownUntil、CircuitOpenUntil、Version、CreatedAt、ProviderType、ProviderName
	// 已通过 Clone() 从 old 中继承，无需额外操作

	return updated, fields
}

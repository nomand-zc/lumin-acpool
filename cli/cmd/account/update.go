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
		Long: `从 JSON 文件更新 Account 信息（全量替换）。

运行时字段（Status、CooldownUntil、CircuitOpenUntil、Version、CreatedAt）
由系统维护，不允许通过此命令修改。

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
	oldAcct, err := deps.AccountStorage.GetAccount(cmd.Context(), raw.ID)
	if err != nil {
		if err == storage.ErrNotFound {
			return fmt.Errorf("Account %s 不存在，无法更新", raw.ID)
		}
		return fmt.Errorf("获取现有 Account 信息失败: %w", err)
	}

	// 如果更新了 Provider 归属，验证新 Provider 是否存在
	providerType := oldAcct.ProviderType
	providerName := oldAcct.ProviderName
	if raw.ProviderType != "" {
		providerType = raw.ProviderType
	}
	if raw.ProviderName != "" {
		providerName = raw.ProviderName
	}
	if providerType != oldAcct.ProviderType || providerName != oldAcct.ProviderName {
		providerKey := acct.BuildProviderKey(providerType, providerName)
		if _, err := deps.ProviderStorage.GetProvider(cmd.Context(), providerKey); err != nil {
			if err == storage.ErrNotFound {
				return fmt.Errorf("目标 Provider %s 不存在", providerKey)
			}
			return fmt.Errorf("查询 Provider 失败: %w", err)
		}
	}

	// 解析凭证（如果提供了新凭证）
	var newCred credentials.Credential
	if len(raw.Credential) > 0 {
		cred, err := parseCredential(providerType, raw.Credential)
		if err != nil {
			return fmt.Errorf("解析凭证失败: %w", err)
		}
		newCred = cred
	}

	// 用老数据合并填充零值字段，并保护运行时字段
	updated := mergeAccountWithExisting(raw, oldAcct, newCred)

	if err := deps.AccountStorage.UpdateAccount(cmd.Context(), updated); err != nil {
		return handleStorageError("Account", err)
	}

	fmt.Printf("Account %s 更新成功\n", updated.ID)
	return nil
}

// accountUpdateJSON 是 Account 更新操作的 JSON 反序列化中间结构。
type accountUpdateJSON struct {
	ID           string            `json:"ID"`
	ProviderType string            `json:"ProviderType"`
	ProviderName string            `json:"ProviderName"`
	Credential   json.RawMessage   `json:"Credential"`
	Priority     int               `json:"Priority"`
	Tags         map[string]string `json:"Tags"`
	Metadata     map[string]any    `json:"Metadata"`
}

// mergeAccountWithExisting 将新数据中的零值字段用老数据填充，
// 并强制使用老数据覆盖不应被外部更新的运行时字段，防止异常覆盖。
func mergeAccountWithExisting(raw *accountUpdateJSON, old *acct.Account, newCred credentials.Credential) *acct.Account {
	updated := old.Clone()
	updated.UpdatedAt = time.Now()

	// --- 可更新字段：新数据非空时覆盖 ---

	if raw.ProviderType != "" {
		updated.ProviderType = raw.ProviderType
	}
	if raw.ProviderName != "" {
		updated.ProviderName = raw.ProviderName
	}
	if newCred != nil {
		updated.Credential = newCred
	}
	if raw.Priority != 0 {
		updated.Priority = raw.Priority
	}
	if len(raw.Tags) > 0 {
		updated.Tags = raw.Tags
	}
	if len(raw.Metadata) > 0 {
		updated.Metadata = raw.Metadata
	}

	// --- 运行时字段保护：始终使用数据库值，不允许外部修改 ---
	// Status、CooldownUntil、CircuitOpenUntil、Version、CreatedAt
	// 已通过 Clone() 从 old 中继承，无需额外操作

	return updated
}

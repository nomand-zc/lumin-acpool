package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-client/pool/taskpool"

	// 匿名导入，触发 credential / provider init 注册
	_ "github.com/nomand-zc/lumin-client/credentials/kiro"
	_ "github.com/nomand-zc/lumin-client/providers/kiro"
)

// addCmd 持有 account add 命令的参数。
type addCmd struct {
	filePath     string
	providerType string
	providerName string
}

// cmd 返回 cobra.Command。
func (c *addCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "添加 Account",
		Long: `从 JSON 文件添加一个 Account，或从目录批量添加多个 Account。

当 --file 指定的是一个目录时，会扫描该目录下的所有 .json 文件并发批量添加。
可通过 --type 和 --name 参数预设 ProviderType 和 ProviderName，
JSON 文件中未指定这两个字段时会使用命令行参数的值。

JSON 文件示例:
  {
    "ID": "acct-001",
    "ProviderType": "kiro",
    "ProviderName": "kiro-team-a",
    "Credential": {
      "accessToken": "xxx",
      "refreshToken": "yyy"
    },
    "Status": 1,
    "Priority": 10,
    "Tags": {"team": "backend"}
  }

示例:
  acpool account add --file account.json
  acpool account add --file account.json --type kiro --name kiro-team-a
  acpool account add --file /path/to/accounts/ --type kiro --name kiro-team-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "Account JSON 文件路径或目录路径（必填）")
	cmd.Flags().StringVar(&c.providerType, "type", "kiro", "Provider 类型（可选，覆盖 JSON 中的 ProviderType）")
	cmd.Flags().StringVar(&c.providerName, "name", "default", "Provider 名称（可选，覆盖 JSON 中的 ProviderName）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 account add 逻辑。
func (c *addCmd) run(cmd *cobra.Command) error {
	fi, err := os.Stat(c.filePath)
	if err != nil {
		return fmt.Errorf("无法访问路径 %s: %w", c.filePath, err)
	}

	if fi.IsDir() {
		return c.runBatch(cmd)
	}
	return c.runSingle(cmd, c.filePath)
}

// accountJSON 是 Account 的 JSON 反序列化中间结构，
// 因为 Credential 是接口类型，需要通过 ProviderType 对应的工厂来解析。
type accountJSON struct {
	ID           string            `json:"ID"`
	ProviderType string            `json:"ProviderType"`
	ProviderName string            `json:"ProviderName"`
	Credential   json.RawMessage   `json:"Credential"`
	Status       acct.Status       `json:"Status"`
	Priority     int               `json:"Priority"`
	Tags         map[string]string `json:"Tags"`
	Metadata     map[string]any    `json:"Metadata"`
}

// runSingle 添加单个 Account。
func (c *addCmd) runSingle(cmd *cobra.Command, filePath string) error {
	raw, err := ioutil.LoadJSONFile[accountJSON](filePath)
	if err != nil {
		return err
	}

	// 命令行参数覆盖 JSON 中的空值
	c.applyFlags(raw)

	return addAccountFromOptions(cmd, &addAccountOptions{
		ID:           raw.ID,
		ProviderType: raw.ProviderType,
		ProviderName: raw.ProviderName,
		Credential:   raw.Credential,
		Status:       raw.Status,
		Priority:     raw.Priority,
		Tags:         raw.Tags,
		Metadata:     raw.Metadata,
	})
}

// runBatch 扫描目录下所有 JSON 文件，并发批量添加 Account。
func (c *addCmd) runBatch(cmd *cobra.Command) error {
	var (
		successCount atomic.Int64
		failCount    atomic.Int64
		mu           sync.Mutex
		wg           sync.WaitGroup
		errs         []string
	)

	err := filepath.Walk(c.filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() ||
			!strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}

		wg.Add(1)
		if submitErr := taskpool.DefaultPool.Submit(func() {
			defer wg.Done()

			if addErr := c.runSingle(cmd, path); addErr != nil {
				failCount.Add(1)
				mu.Lock()
				errs = append(errs, fmt.Sprintf("  %s: %v", path, addErr))
				mu.Unlock()
				return
			}
			successCount.Add(1)
		}); submitErr != nil {
			wg.Done()
			failCount.Add(1)
			mu.Lock()
			errs = append(errs, fmt.Sprintf("  %s: 提交任务失败: %v", path, submitErr))
			mu.Unlock()
		}

		return nil
	})

	// 等待所有并发任务完成
	wg.Wait()

	if err != nil {
		return fmt.Errorf("扫描目录失败: %w", err)
	}

	total := successCount.Load() + failCount.Load()
	fmt.Printf("批量添加完成！总计: %d, 成功: %d, 失败: %d\n",
		total, successCount.Load(), failCount.Load())

	if len(errs) > 0 {
		fmt.Printf("失败详情:\n%s\n", strings.Join(errs, "\n"))
	}

	return nil
}

// applyFlags 将命令行参数覆盖到 accountJSON 中的空值字段。
func (c *addCmd) applyFlags(raw *accountJSON) {
	if raw.ProviderType == "" && c.providerType != "" {
		raw.ProviderType = c.providerType
	}
	if raw.ProviderName == "" && c.providerName != "" {
		raw.ProviderName = c.providerName
	}
}

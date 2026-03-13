package account

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	acct "github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-client/pool/taskpool"
)

// importCmd 持有 account import 命令的参数。
type importCmd struct {
	filePath     string
	providerType string
	providerName string
	priority     int
}

// cmd 返回 cobra.Command。
func (c *importCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "从凭证文件导入 Account",
		Long: `从纯凭证 JSON 文件导入一个 Account，或从目录批量导入多个 Account。

与 add 命令不同，import 命令接受的是纯凭证 JSON 文件（只包含凭证字段），
Account ID 会自动生成 UUID，ProviderType 和 ProviderName 通过命令行参数指定。

当 --file 指定的是一个目录时，会扫描该目录下的所有 .json 文件并发批量导入。

凭证 JSON 文件示例:
  {
    "accessToken": "xxx",
    "refreshToken": "yyy",
    "profileArn": "arn:aws:...",
    "authMethod": "social",
    "provider": "Google",
    "region": "us-east-1",
    "expiresAt": "2026-03-07T03:42:37.387700881+08:00"
  }

示例:
  acpool account import --file cred.json --type kiro --name kiro-team-a
  acpool account import --file /path/to/creds/ --type kiro --name kiro-team-a
  acpool account import --file /path/to/creds/ --type kiro --name kiro-team-a --priority 10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "凭证 JSON 文件路径或目录路径（必填）")
	cmd.Flags().StringVar(&c.providerType, "type", "kiro", "Provider 类型")
	cmd.Flags().StringVar(&c.providerName, "name", "default", "Provider 名称")
	cmd.Flags().IntVar(&c.priority, "priority", 1, "账号优先级（可选，默认 1）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 account import 逻辑。
func (c *importCmd) run(cmd *cobra.Command) error {
	fi, err := os.Stat(c.filePath)
	if err != nil {
		return fmt.Errorf("无法访问路径 %s: %w", c.filePath, err)
	}

	if fi.IsDir() {
		return c.runBatch(cmd)
	}
	_, err = c.runSingle(cmd, c.filePath)
	return err
}

// runSingle 从单个凭证文件导入一个 Account，返回最终状态。
func (c *importCmd) runSingle(cmd *cobra.Command, filePath string) (acct.Status, error) {
	// 直接加载凭证 JSON 原始数据
	credRaw, err := ioutil.LoadJSONFile[json.RawMessage](filePath)
	if err != nil {
		return 0, err
	}

	// 自动生成 UUID 作为 Account ID
	accountID := uuid.New().String()

	return addAccountFromOptions(cmd, &addAccountOptions{
		ID:           accountID,
		ProviderType: c.providerType,
		ProviderName: c.providerName,
		Credential:   *credRaw,
		Priority:     c.priority,
	})
}

// runBatch 扫描目录下所有 JSON 文件，并发批量导入 Account。
func (c *importCmd) runBatch(cmd *cobra.Command) error {
	var (
		successCount atomic.Int64
		failCount    atomic.Int64
		mu           sync.Mutex
		wg           sync.WaitGroup
		errs         []string
		statusCounts = make(map[acct.Status]int64)
	)

	err := filepath.Walk(c.filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() ||
			!strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			return nil
		}

		wg.Add(1)
		if submitErr := taskpool.DefaultPool.Submit(func() {
			defer wg.Done()

			status, addErr := c.runSingle(cmd, path)
			if addErr != nil {
				failCount.Add(1)
				mu.Lock()
				errs = append(errs, fmt.Sprintf("  %s: %v", path, addErr))
				mu.Unlock()
				return
			}
			successCount.Add(1)
			mu.Lock()
			statusCounts[status]++
			mu.Unlock()
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
	fmt.Printf("批量导入完成！总计: %d, 成功: %d, 失败: %d\n",
		total, successCount.Load(), failCount.Load())

	printStatusSummary(statusCounts)

	if len(errs) > 0 {
		fmt.Printf("失败详情:\n%s\n", strings.Join(errs, "\n"))
	}

	return nil
}

package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/ioutil"
	"github.com/nomand-zc/lumin-client/pool/taskpool"
	"github.com/nomand-zc/lumin-client/providers"

	// 匿名导入，触发 provider init 注册
	_ "github.com/nomand-zc/lumin-client/providers/kiro"
)

// addCmd 持有 provider add 命令的参数。
type addCmd struct {
	filePath     string
	providerType string
	providerName string
}

// cmd 返回 cobra.Command。
func (c *addCmd) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "添加 Provider",
		Long: `从 JSON 文件添加一个 Provider，或从目录批量添加多个 Provider。

当 --file 指定的是一个目录时，会扫描该目录下的所有 .json 文件并发批量添加。
可通过 --type 和 --name 参数预设 ProviderType 和 ProviderName，
JSON 文件中未指定这两个字段时会使用命令行参数的值。

JSON 文件示例:
  {
    "ProviderType": "kiro",
    "ProviderName": "kiro-team-a",
    "Status": 1,
    "Priority": 10,
    "Weight": 5,
    "SupportedModels": ["claude-4-sonnet"],
    "Tags": {"team": "backend"}
  }

示例:
  acpool provider add --file provider.json
  acpool provider add --file provider.json --type kiro --name kiro-team-a
  acpool provider add --file /path/to/providers/ --type kiro --name kiro-team-a`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.run(cmd)
		},
	}

	cmd.Flags().StringVarP(&c.filePath, "file", "f", "", "Provider JSON 文件路径或目录路径（必填）")
	cmd.Flags().StringVar(&c.providerType, "type", "kiro", "Provider 类型（可选，覆盖 JSON 中的 ProviderType）")
	cmd.Flags().StringVar(&c.providerName, "name", "default", "Provider 名称（可选，覆盖 JSON 中的 ProviderName）")
	_ = cmd.MarkFlagRequired("file")

	return cmd
}

// run 执行 provider add 逻辑。
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

// runSingle 添加单个 Provider。
func (c *addCmd) runSingle(cmd *cobra.Command, filePath string) error {
	deps := bootstrap.DepsFromContext(cmd.Context())

	info, err := ioutil.LoadJSONFile[account.ProviderInfo](filePath)
	if err != nil {
		return err
	}

	// 命令行参数覆盖 JSON 中的空值
	c.applyFlags(info)

	// 如果 SupportedModels 或 UsageRules 为空，从 client 获取默认值
	if err := fillProviderDefaults(cmd.Context(), info); err != nil {
		return fmt.Errorf("获取 Provider 默认配置失败: %w", err)
	}

	if err := deps.ProviderStorage.Add(cmd.Context(), info); err != nil {
		return handleStorageError("Provider", err)
	}

	fmt.Printf("Provider %s 添加成功\n", info.ProviderKey())
	return nil
}

// runBatch 扫描目录下所有 JSON 文件，并发批量添加 Provider。
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

// applyFlags 将命令行参数覆盖到 ProviderInfo 中的空值字段。
func (c *addCmd) applyFlags(info *account.ProviderInfo) {
	if info.ProviderType == "" && c.providerType != "" {
		info.ProviderType = c.providerType
	}
	if info.ProviderName == "" && c.providerName != "" {
		info.ProviderName = c.providerName
	}
}

// fillProviderDefaults 当 SupportedModels 或 UsageRules 为空时，
// 通过 lumin-client 的 Provider 接口获取默认值并填充。
func fillProviderDefaults(ctx context.Context, info *account.ProviderInfo) error {
	if len(info.SupportedModels) > 0 && len(info.UsageRules) > 0 {
		return nil
	}

	client := providers.GetProvider(info.ProviderType, providers.DefaultProviderName)
	if client == nil {
		return fmt.Errorf("未找到类型为 %q 的 Provider 客户端，无法获取默认配置", info.ProviderType)
	}

	if len(info.SupportedModels) == 0 {
		models, err := client.Models(ctx)
		if err != nil {
			return fmt.Errorf("获取默认模型列表失败: %w", err)
		}
		info.SupportedModels = models
	}

	if len(info.UsageRules) == 0 {
		rules, err := client.DefaultUsageRules(ctx)
		if err != nil {
			return fmt.Errorf("获取默认用量规则失败: %w", err)
		}
		info.UsageRules = rules
	}

	return nil
}

package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nomand-zc/lumin-acpool/cli/cmd/provider"
	"github.com/nomand-zc/lumin-acpool/cli/internal/bootstrap"
	"github.com/nomand-zc/lumin-acpool/cli/internal/config"
)

var (
	configFile string
	deps       *bootstrap.Dependencies
)

var rootCmd = &cobra.Command{
	Use:   "acpool",
	Short: "acpool 是一个 AI 账号池管理工具",
	Long:  `acpool 提供了账号池的管理命令行工具，包括 Provider、Account 管理等功能。`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFile)
		if err != nil {
			return err
		}
		deps, err = bootstrap.Init(cfg)
		if err != nil {
			return err
		}
		// 通过 Cobra 原生 Context 注入 Dependencies
		cmd.SetContext(bootstrap.WithDependencies(cmd.Context(), deps))
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if deps != nil {
			return deps.Close()
		}
		return nil
	},
}

// Execute 执行根命令。
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "acpool.yaml", "配置文件路径")

	// 注册命令组
	rootCmd.AddCommand(provider.CMD())
}

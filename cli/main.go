package main

import (
	"os"

	"github.com/nomand-zc/lumin-acpool/cli/cmd"
)

// TODO: 需要重构整个cli模块

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

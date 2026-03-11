package main

import (
	"os"

	"github.com/nomand-zc/lumin-acpool/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

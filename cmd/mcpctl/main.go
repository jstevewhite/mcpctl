package main

import (
	"fmt"
	"os"

	"github.com/jstevewhite/mcpctl/internal/apperror"
	"github.com/jstevewhite/mcpctl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(apperror.ExitCode(err))
	}
}

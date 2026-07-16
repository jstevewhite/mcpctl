package main

import (
	"fmt"
	"os"

	"mcpctl/internal/apperror"
	"mcpctl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(apperror.ExitCode(err))
	}
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcpctl/internal/buildinfo"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print detailed version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Result → stdout; cmd.Println routes to stderr.
			fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Full())
			return nil
		},
	}
}

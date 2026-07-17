// Package cli builds the mcpctl command tree.
package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/buildinfo"
	"mcpctl/internal/logging"
)

const (
	minTimeout = 1 * time.Second
	maxTimeout = 24 * time.Hour
)

func validateTimeout(name string, d time.Duration) error {
	if d < minTimeout || d > maxTimeout {
		return apperror.Usage("--%s must be between %s and %s, got %s", name, minTimeout, maxTimeout, d)
	}
	return nil
}

// GlobalFlags holds values bound to the root's persistent flags.
type GlobalFlags struct {
	Config          string
	Output          string
	Timeout         time.Duration
	ConnectTimeout  time.Duration
	LogLevel        string
	NoColor         bool
	NoValidate      bool
	ProtocolVersion string
}

// NewRootCmd builds the root command. Subcommands are attached by their own
// constructors (see version.go).
func NewRootCmd() (*cobra.Command, *GlobalFlags) {
	g := &GlobalFlags{}
	showVersion := false

	root := &cobra.Command{
		Use:           "mcpctl",
		Short:         "Connect to MCP servers and invoke their tools",
		Long:          "mcpctl connects to Model Context Protocol servers over stdio or Streamable HTTP and invokes their tools.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			logger, err := logging.Setup(os.Stderr, g.LogLevel)
			if err != nil {
				return err
			}
			slog.SetDefault(logger)
			if err := validateTimeout("timeout", g.Timeout); err != nil {
				return err
			}
			return validateTimeout("connect-timeout", g.ConnectTimeout)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				// Version output is a command result → stdout. cmd.Println
				// routes to stderr, so write to OutOrStdout explicitly.
				fmt.Fprintln(cmd.OutOrStdout(), buildinfo.Short())
				return nil
			}
			return cmd.Help()
		},
	}

	f := root.PersistentFlags()
	f.StringVar(&g.Config, "config", "", "path to config file")
	f.StringVar(&g.Output, "output", "human", "output format: human|json|jsonl|yaml")
	f.DurationVar(&g.Timeout, "timeout", 30*time.Second, "overall command timeout")
	f.DurationVar(&g.ConnectTimeout, "connect-timeout", 15*time.Second, "connection/initialization timeout (currently applied as part of --timeout)")
	f.StringVar(&g.LogLevel, "log-level", "warn", "log level: debug|info|warn|error")
	f.BoolVar(&g.NoColor, "no-color", false, "disable colored output")
	f.BoolVar(&g.NoValidate, "no-validate", false, "skip local argument validation")
	f.StringVar(&g.ProtocolVersion, "protocol-version", "", "override the negotiated MCP protocol version")

	root.Flags().BoolVar(&showVersion, "version", false, "print version and exit")

	// Coerce Cobra's own flag-parse errors into typed usage errors.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return apperror.Usage("%s", err.Error())
	})

	root.AddCommand(newVersionCmd())
	root.AddCommand(newToolsCmd(g))
	root.AddCommand(newServerCmd(g))

	return root, g
}

// normalize coerces a bare Cobra error (unknown command, extra args) into a
// typed usage error so main can map it to exit code 2.
func normalize(err error) error {
	if err == nil {
		return nil
	}
	var ae *apperror.Error
	if errors.As(err, &ae) {
		return err
	}
	return apperror.Usage("%s", err.Error())
}

// Execute runs the root command and returns a typed error.
func Execute() error {
	root, _ := NewRootCmd()
	return normalize(root.Execute())
}

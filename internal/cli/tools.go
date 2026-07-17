package cli

import (
	"context"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
	"mcpctl/internal/output"
)

const defaultMaxPages = 1000

func newToolsCmd(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "List, describe, and call tools on an MCP server",
	}
	cmd.AddCommand(newToolsListCmd(g))
	return cmd
}

// addServerFlags binds the shared server-selection flags to a tools subcommand.
func addServerFlags(cmd *cobra.Command, sf *ServerFlags) {
	cmd.Flags().StringVar(&sf.Server, "server", "", "named server from configuration")
	cmd.Flags().StringVar(&sf.URL, "url", "", "ephemeral Streamable HTTP URL (not yet supported)")
	cmd.Flags().BoolVar(&sf.Stdio, "stdio", false, "ephemeral stdio server (command follows `--`)")
}

// dial resolves the target from flags/args and connects using ctx. A single
// command-scoped, signal-aware, timeout-bounded context is used for the whole
// command (spec §14 permits this when separating connect/op is impractical):
// the SDK session's lifetime is tied to this context, so it must outlive every
// call made on the returned client.
func dial(ctx context.Context, cmd *cobra.Command, g *GlobalFlags, sf ServerFlags, args []string) (client.Client, []string, error) {
	dash := cmd.ArgsLenAtDash()
	var toolSide, afterDash []string
	hasDash := dash >= 0
	if hasDash {
		toolSide, afterDash = args[:dash], args[dash:]
	} else {
		toolSide = args
	}
	spec, toolArgs, err := resolveTarget(sf, toolSide, afterDash, hasDash, g.Config)
	if err != nil {
		return nil, nil, err
	}
	c, err := client.DialStdio(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	if !c.ServerInfo().SupportsTools {
		c.Close()
		return nil, nil, apperror.New(apperror.KindProtocol, "server does not support tools")
	}
	return c, toolArgs, nil
}

func newToolsListCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, _, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()

			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			serverName := sf.Server
			if serverName == "" {
				serverName = "(ephemeral)"
			}
			return output.ToolList(cmd.OutOrStdout(), f, serverName, tools)
		},
	}
	addServerFlags(cmd, &sf)
	return cmd
}

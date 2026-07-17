package cli

import (
	"context"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/arguments"
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
	cmd.AddCommand(newToolsDescribeCmd(g))
	cmd.AddCommand(newToolsCallCmd(g))
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

func newToolsDescribeCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "describe TOOL",
		Short: "Show a tool's description and schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, toolArgs, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()
			if len(toolArgs) != 1 {
				return apperror.Usage("describe requires exactly one TOOL name")
			}
			name := toolArgs[0]

			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			for _, t := range tools {
				if t.Name == name {
					return output.ToolDescribe(cmd.OutOrStdout(), f, t)
				}
			}
			return apperror.New(apperror.KindToolNotFound, "tool %q not found on this server", name)
		},
	}
	addServerFlags(cmd, &sf)
	return cmd
}

func newToolsCallCmd(g *GlobalFlags) *cobra.Command {
	var sf ServerFlags
	var jsonStr, jsonFile string
	var argKVs []string
	cmd := &cobra.Command{
		Use:   "call TOOL",
		Short: "Call a tool with JSON arguments",
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			toolArguments, err := arguments.Parse(jsonStr, jsonFile, argKVs, cmd.InOrStdin())
			if err != nil {
				return err
			}
			ctx, cancel := commandContext(cmd.Context(), g.Timeout)
			defer cancel()
			c, toolArgs, err := dial(ctx, cmd, g, sf, args)
			if err != nil {
				return err
			}
			defer c.Close()
			if len(toolArgs) != 1 {
				return apperror.Usage("call requires exactly one TOOL name")
			}
			name := toolArgs[0]

			// Confirm the tool exists before calling (spec §11): not-found -> exit 7.
			tools, err := c.ListAllTools(ctx, defaultMaxPages)
			if err != nil {
				return err
			}
			found := false
			for _, t := range tools {
				if t.Name == name {
					found = true
					break
				}
			}
			if !found {
				return apperror.New(apperror.KindToolNotFound, "tool %q not found on this server", name)
			}

			result, err := c.CallTool(ctx, name, toolArguments)
			if err != nil {
				return err
			}
			if rerr := output.ToolResult(cmd.OutOrStdout(), f, result); rerr != nil {
				return rerr
			}
			if result.IsError {
				return apperror.New(apperror.KindToolError, "tool %q reported an error", name)
			}
			return nil
		},
	}
	addServerFlags(cmd, &sf)
	cmd.Flags().StringVar(&jsonStr, "json", "", "arguments as a JSON object")
	cmd.Flags().StringVar(&jsonFile, "json-file", "", "arguments from a JSON file (`-` for stdin)")
	cmd.Flags().StringArrayVar(&argKVs, "arg", nil, "argument as KEY=VALUE (repeatable)")
	return cmd
}

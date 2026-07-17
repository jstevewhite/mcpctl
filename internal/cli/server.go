package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mcpctl/internal/apperror"
	"mcpctl/internal/auth"
	"mcpctl/internal/config"
	"mcpctl/internal/output"
)

const redacted = "<redacted>"

func newServerCmd(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage saved MCP server definitions",
	}
	cmd.AddCommand(newServerListCmd(g), newServerShowCmd(g))
	return cmd
}

// redactServer returns a copy safe to display: literal secret values hidden,
// env-var references (which are names, not secrets) shown as-is.
func redactServer(sc config.ServerConfig) config.ServerConfig {
	out := sc
	if sc.Env != nil {
		out.Env = make(map[string]string, len(sc.Env))
		for k, v := range sc.Env {
			if auth.IsSensitive(k) {
				out.Env[k] = redacted
			} else {
				out.Env[k] = v
			}
		}
	}
	if sc.Headers != nil {
		out.Headers = make(map[string]string, len(sc.Headers))
		for k := range sc.Headers {
			out.Headers[k] = redacted // literal header values are potential secrets
		}
	}
	// HeaderEnv values are env var NAMES (references), not secrets — left as-is.
	// BearerToken.Env is likewise a name — left as-is.
	return out
}

func newServerListCmd(g *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved servers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Servers))
			for n := range cfg.Servers {
				names = append(names, n)
			}
			sort.Strings(names)

			if f == output.FormatHuman {
				tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "NAME\tTRANSPORT\tURL / COMMAND")
				for _, n := range names {
					sc := cfg.Servers[n]
					fmt.Fprintf(tw, "%s\t%s\t%s\n", n, sc.Transport, endpointSummary(sc))
				}
				return tw.Flush()
			}
			return output.Servers(cmd.OutOrStdout(), f, redactServerMap(cfg.Servers, names))
		},
	}
}

func newServerShowCmd(g *GlobalFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a saved server's details",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.ParseFormat(g.Output)
			if err != nil {
				return err
			}
			if name == "" {
				return apperror.Usage("server show requires --name")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			sc, ok := cfg.Servers[name]
			if !ok {
				return apperror.Config("no server named %q in configuration", name)
			}
			red := redactServer(sc)
			if f == output.FormatHuman {
				return showServerHuman(cmd.OutOrStdout(), name, red)
			}
			return output.Server(cmd.OutOrStdout(), f, name, red)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name")
	return cmd
}

func endpointSummary(sc config.ServerConfig) string {
	if sc.Transport == config.TransportStdio {
		return strings.TrimSpace(sc.Command + " " + strings.Join(sc.Args, " "))
	}
	return sc.URL
}

func redactServerMap(servers map[string]config.ServerConfig, order []string) []output.NamedServer {
	out := make([]output.NamedServer, 0, len(order))
	for _, n := range order {
		out = append(out, output.NamedServer{Name: n, Server: redactServer(servers[n])})
	}
	return out
}

func showServerHuman(w io.Writer, name string, sc config.ServerConfig) error {
	fmt.Fprintf(w, "Name:        %s\n", name)
	fmt.Fprintf(w, "Transport:   %s\n", sc.Transport)
	if sc.Transport == config.TransportStdio {
		fmt.Fprintf(w, "Command:     %s\n", endpointSummary(sc))
		if sc.CWD != "" {
			fmt.Fprintf(w, "CWD:         %s\n", sc.CWD)
		}
	} else {
		fmt.Fprintf(w, "URL:         %s\n", sc.URL)
	}
	fmt.Fprintln(w, "Environment:")
	if len(sc.Env) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, k := range sortedKeys(sc.Env) {
			fmt.Fprintf(w, "  %s=%s\n", k, sc.Env[k])
		}
	}
	writeNameSet(w, "Headers (literal):", sortedKeys(sc.Headers))
	writeEnvRefs(w, "Headers (from env):", sc.HeaderEnv)
	if sc.BearerToken != nil {
		fmt.Fprintf(w, "Bearer:      <env:%s>\n", sc.BearerToken.Env)
	} else {
		fmt.Fprintln(w, "Bearer:      (none)")
	}
	if len(sc.Headers) > 0 {
		fmt.Fprintln(w, "\nwarning: literal header values are stored in the config file in plaintext")
	}
	return nil
}

func writeNameSet(w io.Writer, label string, names []string) {
	fmt.Fprintln(w, label)
	if len(names) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, n := range names {
		fmt.Fprintf(w, "  %s\n", n)
	}
}

func writeEnvRefs(w io.Writer, label string, m map[string]string) {
	fmt.Fprintln(w, label)
	if len(m) == 0 {
		fmt.Fprintln(w, "  (none)")
		return
	}
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(w, "  %s=<env:%s>\n", k, m[k])
	}
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

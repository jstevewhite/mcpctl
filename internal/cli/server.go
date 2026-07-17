package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jstevewhite/mcpctl/internal/apperror"
	"github.com/jstevewhite/mcpctl/internal/auth"
	"github.com/jstevewhite/mcpctl/internal/config"
	"github.com/jstevewhite/mcpctl/internal/output"
)

const redacted = "<redacted>"

func newServerCmd(g *GlobalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage saved MCP server definitions",
	}
	cmd.AddCommand(newServerListCmd(g), newServerShowCmd(g), newServerAddCmd(g), newServerRemoveCmd(g))
	return cmd
}

// redactServer returns a copy safe to display. Literal Header values are always
// hidden. Env values are hidden if the key is sensitive; in machine output
// (json/yaml — the exfil channel), ALL env values are hidden. Env-var name
// references (HeaderEnv, BearerToken.Env) are shown (names, not secrets).
func redactServer(sc config.ServerConfig, machine bool) config.ServerConfig {
	out := sc
	if sc.Env != nil {
		out.Env = make(map[string]string, len(sc.Env))
		for k, v := range sc.Env {
			if machine || auth.IsSensitive(k) {
				out.Env[k] = redacted
			} else {
				out.Env[k] = v
			}
		}
	}
	if sc.Headers != nil {
		out.Headers = make(map[string]string, len(sc.Headers))
		for k := range sc.Headers {
			out.Headers[k] = redacted
		}
	}
	if sc.HeaderEnv != nil {
		out.HeaderEnv = make(map[string]string, len(sc.HeaderEnv))
		for k, v := range sc.HeaderEnv {
			out.HeaderEnv[k] = v
		}
	}
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
			if f == output.FormatHuman {
				return showServerHuman(cmd.OutOrStdout(), name, redactServer(sc, false))
			}
			return output.Server(cmd.OutOrStdout(), f, name, redactServer(sc, true))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name")
	return cmd
}

func newServerAddCmd(g *GlobalFlags) *cobra.Command {
	var name string
	var sf ServerFlags
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a server definition to the configuration (does not connect)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return apperror.Usage("server add requires a non-empty --name")
			}
			dash := cmd.ArgsLenAtDash()
			var afterDash []string
			hasDash := dash >= 0
			if hasDash {
				afterDash = args[dash:]
			}
			sc, err := serverConfigFromFlags(sf, afterDash, hasDash)
			if err != nil {
				return err
			}

			path, _, err := config.Resolve(g.Config)
			if err != nil {
				return apperror.Wrap(apperror.KindConfig, err, "resolve config path")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				// server add creates config as needed: a not-yet-existing
				// file (default path or an explicit --config) starts fresh
				// rather than erroring, since Save is about to create it.
				if errors.Is(err, fs.ErrNotExist) {
					cfg = &config.Config{Version: 1, Servers: map[string]config.ServerConfig{}}
				} else {
					return err
				}
			}
			if _, exists := cfg.Servers[name]; exists {
				return apperror.Config("a server named %q already exists (remove it first)", name)
			}
			if cfg.Servers == nil {
				cfg.Servers = map[string]config.ServerConfig{}
			}
			cfg.Servers[name] = sc
			if err := cfg.Validate(); err != nil { // the new entry must be valid
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "added server %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name (required)")
	addServerFlags(cmd, &sf) // reuse --stdio/--url/--header-env/--header-literal/--bearer-env
	return cmd
}

func newServerRemoveCmd(g *GlobalFlags) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a server definition from the configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return apperror.Usage("server remove requires --name")
			}
			path, _, err := config.Resolve(g.Config)
			if err != nil {
				return apperror.Wrap(apperror.KindConfig, err, "resolve config path")
			}
			cfg, err := config.LoadResolved(g.Config)
			if err != nil {
				return err
			}
			if _, ok := cfg.Servers[name]; !ok {
				return apperror.Config("no server named %q in configuration", name)
			}
			delete(cfg.Servers, name)
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "removed server %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "server name (required)")
	return cmd
}

// serverConfigFromFlags builds a ServerConfig from the transport flags without
// resolving env vars (only names are stored). Exactly one of --stdio/--url.
func serverConfigFromFlags(sf ServerFlags, afterDash []string, hasDash bool) (config.ServerConfig, error) {
	switch {
	case sf.Stdio && sf.URL != "":
		return config.ServerConfig{}, apperror.Usage("--stdio and --url are mutually exclusive")
	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return config.ServerConfig{}, apperror.Usage("--stdio requires a server command after `--`")
		}
		return config.ServerConfig{Transport: config.TransportStdio, Command: afterDash[0], Args: afterDash[1:]}, nil
	case sf.URL != "":
		as, err := authSpecFromFlags(sf.HeaderEnv, sf.HeaderLiteral, sf.BearerEnv)
		if err != nil {
			return config.ServerConfig{}, err
		}
		sc := config.ServerConfig{Transport: config.TransportHTTP, URL: sf.URL, Headers: as.Headers, HeaderEnv: as.HeaderEnv}
		if len(sc.Headers) == 0 {
			sc.Headers = nil
		}
		if len(sc.HeaderEnv) == 0 {
			sc.HeaderEnv = nil
		}
		if as.BearerEnv != "" {
			sc.BearerToken = &config.TokenSource{Env: as.BearerEnv}
		}
		return sc, nil
	default:
		return config.ServerConfig{}, apperror.Usage("server add requires --stdio (with `-- command`) or --url")
	}
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
		out = append(out, output.NamedServer{Name: n, Server: redactServer(servers[n], true)})
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

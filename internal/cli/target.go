package cli

import (
	"mcpctl/internal/apperror"
	"mcpctl/internal/client"
	"mcpctl/internal/config"
)

// ServerFlags holds the mutually-exclusive server selectors bound on a tools command.
type ServerFlags struct {
	Server string
	URL    string
	Stdio  bool
}

// resolveTarget validates the server selectors and returns a stdio spec plus
// the tool-side positional args. toolSide is the positional args before `--`;
// afterDash is the args after `--` (the ephemeral server command); hasDash
// reports whether a `--` was present.
func resolveTarget(sf ServerFlags, toolSide, afterDash []string, hasDash bool, configPath string) (client.StdioSpec, []string, error) {
	selected := 0
	if sf.Server != "" {
		selected++
	}
	if sf.URL != "" {
		selected++
	}
	if sf.Stdio {
		selected++
	}
	if selected != 1 {
		return client.StdioSpec{}, nil, apperror.Usage(
			"exactly one of --server, --stdio, or --url is required")
	}

	switch {
	case sf.URL != "":
		return client.StdioSpec{}, nil, apperror.New(apperror.KindConnection,
			"streamable-http (--url) is not supported yet; it arrives in a later version")

	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return client.StdioSpec{}, nil, apperror.Usage(
				"--stdio requires a server command after `--`, e.g. --stdio -- npx -y server")
		}
		spec := client.StdioSpec{Command: afterDash[0], Args: afterDash[1:]}
		return spec, toolSide, nil

	default: // --server
		spec, err := specFromConfig(sf.Server, configPath)
		if err != nil {
			return client.StdioSpec{}, nil, err
		}
		return spec, toolSide, nil
	}
}

// specFromConfig loads the named server from config and builds a stdio spec.
func specFromConfig(name, configPath string) (client.StdioSpec, error) {
	cfg, err := config.LoadResolved(configPath)
	if err != nil {
		return client.StdioSpec{}, err
	}
	sc, ok := cfg.Servers[name]
	if !ok {
		return client.StdioSpec{}, apperror.Config("no server named %q in configuration", name)
	}
	if sc.Transport != config.TransportStdio {
		return client.StdioSpec{}, apperror.New(apperror.KindConnection,
			"server %q uses transport %q; only stdio is supported yet", name, sc.Transport)
	}
	return client.StdioSpec{
		Command: sc.Command,
		Args:    sc.Args,
		CWD:     sc.CWD,
		Env:     sc.Env,
	}, nil
}

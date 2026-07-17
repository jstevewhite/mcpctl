package cli

import (
	"os"
	"strings"

	"github.com/jstevewhite/mcpctl/internal/apperror"
	"github.com/jstevewhite/mcpctl/internal/auth"
	"github.com/jstevewhite/mcpctl/internal/client"
	"github.com/jstevewhite/mcpctl/internal/config"
)

// ServerFlags holds the mutually-exclusive server selectors bound on a tools command.
type ServerFlags struct {
	Server string
	URL    string
	Stdio  bool
	// HTTP auth (ephemeral or applied to a --url):
	HeaderEnv     []string // NAME=ENVVAR
	HeaderLiteral []string // NAME=VALUE
	BearerEnv     string
}

// Target is a resolved connection target: exactly one of Stdio / HTTP is set.
type Target struct {
	Stdio *client.StdioSpec
	HTTP  *client.HTTPSpec
}

// resolveTarget validates the server selectors and returns a resolved Target
// plus the tool-side positional args. toolSide is the positional args before
// `--`; afterDash is the args after `--` (the ephemeral server command);
// hasDash reports whether a `--` was present.
func resolveTarget(sf ServerFlags, toolSide, afterDash []string, hasDash bool, configPath string) (Target, []string, error) {
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
		return Target{}, nil, apperror.Usage("exactly one of --server, --stdio, or --url is required")
	}

	switch {
	case sf.Stdio:
		if !hasDash || len(afterDash) == 0 {
			return Target{}, nil, apperror.Usage("--stdio requires a server command after `--`, e.g. --stdio -- npx -y server")
		}
		return Target{Stdio: &client.StdioSpec{Command: afterDash[0], Args: afterDash[1:]}}, toolSide, nil

	case sf.URL != "":
		spec, err := httpSpecFromFlags(sf)
		if err != nil {
			return Target{}, nil, err
		}
		return Target{HTTP: spec}, toolSide, nil

	default: // --server
		return targetFromConfig(sf.Server, configPath, toolSide)
	}
}

func httpSpecFromFlags(sf ServerFlags) (*client.HTTPSpec, error) {
	as, err := authSpecFromFlags(sf.HeaderEnv, sf.HeaderLiteral, sf.BearerEnv)
	if err != nil {
		return nil, err
	}
	hdr, err := auth.Resolve(as, os.LookupEnv)
	if err != nil {
		return nil, err
	}
	return &client.HTTPSpec{URL: sf.URL, Header: hdr}, nil
}

// authSpecFromFlags parses NAME=VALUE / NAME=ENVVAR flag pairs into an auth.Spec.
func authSpecFromFlags(headerEnv, headerLiteral []string, bearerEnv string) (auth.Spec, error) {
	as := auth.Spec{Headers: map[string]string{}, HeaderEnv: map[string]string{}, BearerEnv: bearerEnv}
	for _, kv := range headerLiteral {
		name, val, ok := splitPair(kv)
		if !ok {
			return auth.Spec{}, apperror.Usage("invalid --header-literal %q: expected NAME=VALUE", kv)
		}
		as.Headers[name] = val
	}
	for _, kv := range headerEnv {
		name, envVar, ok := splitPair(kv)
		if !ok {
			return auth.Spec{}, apperror.Usage("invalid --header-env %q: expected NAME=ENVVAR", kv)
		}
		as.HeaderEnv[name] = envVar
	}
	return as, nil
}

func splitPair(kv string) (name, val string, ok bool) {
	i := strings.IndexByte(kv, '=')
	if i <= 0 {
		return "", "", false
	}
	return kv[:i], kv[i+1:], true
}

// targetFromConfig loads the named server from config and builds a Target for
// whichever transport it declares.
func targetFromConfig(name, configPath string, toolSide []string) (Target, []string, error) {
	cfg, err := config.LoadResolved(configPath)
	if err != nil {
		return Target{}, nil, err
	}
	sc, ok := cfg.Servers[name]
	if !ok {
		return Target{}, nil, apperror.Config("no server named %q in configuration", name)
	}
	switch sc.Transport {
	case config.TransportStdio:
		return Target{Stdio: &client.StdioSpec{Command: sc.Command, Args: sc.Args, CWD: sc.CWD, Env: sc.Env}}, toolSide, nil
	case config.TransportHTTP:
		as := auth.Spec{Headers: sc.Headers, HeaderEnv: sc.HeaderEnv}
		if sc.BearerToken != nil {
			as.BearerEnv = sc.BearerToken.Env
		}
		hdr, err := auth.Resolve(as, os.LookupEnv)
		if err != nil {
			return Target{}, nil, err
		}
		return Target{HTTP: &client.HTTPSpec{URL: sc.URL, Header: hdr}}, toolSide, nil
	default:
		return Target{}, nil, apperror.Config("server %q has unknown transport %q", name, sc.Transport)
	}
}

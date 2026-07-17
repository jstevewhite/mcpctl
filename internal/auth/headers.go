package auth

import (
	"net/http"
	"strings"

	"mcpctl/internal/apperror"
)

// Spec describes the auth inputs for an HTTP server, before env resolution.
type Spec struct {
	Headers   map[string]string // literal name -> value
	HeaderEnv map[string]string // header name -> env var name
	BearerEnv string            // env var name holding a bearer token ("" = none)
}

// Resolve turns the spec into a concrete header set, reading env values via
// lookup. Missing referenced env vars are an auth error; configuring
// Authorization through more than one mechanism is a config error (§5.4).
func Resolve(s Spec, lookup func(string) (string, bool)) (http.Header, error) {
	authMechanisms := 0
	h := http.Header{}

	for name, val := range s.Headers {
		if strings.EqualFold(name, "Authorization") {
			authMechanisms++
		}
		h.Set(name, val)
	}
	for name, envVar := range s.HeaderEnv {
		val, ok := lookup(envVar)
		if !ok {
			return nil, apperror.New(apperror.KindAuth,
				"environment variable %q (for header %q) is not set", envVar, name)
		}
		if strings.EqualFold(name, "Authorization") {
			authMechanisms++
		}
		h.Set(name, val)
	}
	if s.BearerEnv != "" {
		tok, ok := lookup(s.BearerEnv)
		if !ok {
			return nil, apperror.New(apperror.KindAuth,
				"bearer token environment variable %q is not set", s.BearerEnv)
		}
		authMechanisms++
		h.Set("Authorization", "Bearer "+tok)
	}

	if authMechanisms > 1 {
		return nil, apperror.Config(
			"multiple Authorization mechanisms configured; use exactly one of a bearer token or an Authorization header")
	}
	return h, nil
}

package client

import (
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"mcpctl/internal/buildinfo"
)

// statusRecorder holds the most recent HTTP response status seen by the
// transport, so the error classifier can distinguish 401/403 (auth) from other
// failures despite the SDK's opaque errors.
type statusRecorder struct{ code atomic.Int64 }

func (r *statusRecorder) record(c int) { r.code.Store(int64(c)) }
func (r *statusRecorder) last() int    { return int(r.code.Load()) }

// authRoundTripper injects the resolved request headers for same-origin
// requests only (credentials never follow a cross-origin redirect, §9), sets a
// descriptive User-Agent, and records response statuses.
type authRoundTripper struct {
	base   http.RoundTripper
	origin string // scheme://host[:port] of the configured endpoint
	header http.Header
	rec    *statusRecorder
}

func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	if originOf(r.URL) == t.origin {
		for k, vs := range t.header {
			r.Header[k] = append([]string(nil), vs...)
		}
	} else {
		// Cross-origin (e.g. after a redirect): ensure none of our configured
		// headers ride along, even if net/http copied one.
		for k := range t.header {
			r.Header.Del(k)
		}
	}
	if r.Header.Get("User-Agent") == "" {
		r.Header.Set("User-Agent", "mcpctl/"+buildinfo.Version)
	}
	resp, err := t.base.RoundTrip(r)
	if resp != nil {
		t.rec.record(resp.StatusCode)
	}
	return resp, err
}

// originOf returns scheme://host[:port], the identity used for same-origin checks.
func originOf(u *url.URL) string {
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// buildHTTPClient constructs the *http.Client for a Streamable HTTP endpoint: a
// cloned transport (never mutating http.DefaultTransport), proxy-from-env, a
// TLS-handshake timeout, and NO response-body timeout (the command context is
// the overall bound). Credentials are stripped on cross-origin redirects.
func buildHTTPClient(endpoint *url.URL, header http.Header) (*http.Client, *statusRecorder, error) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = http.ProxyFromEnvironment
	base.TLSHandshakeTimeout = 15 * time.Second
	// Deliberately no ResponseHeaderTimeout: long-running tool calls hold the
	// response open; the command context bounds the whole operation.

	rec := &statusRecorder{}
	rt := &authRoundTripper{
		base:   base,
		origin: originOf(endpoint),
		header: header,
		rec:    rec,
	}
	c := &http.Client{
		Transport: rt,
		// Belt-and-suspenders with the RoundTripper's origin gate: refuse to
		// carry credentials across origins. net/http already strips
		// Authorization/Cookie cross-domain; this also handles custom headers.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	return c, rec, nil
}

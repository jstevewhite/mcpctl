package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcpctl/internal/apperror"
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

// DialHTTP connects to a Streamable HTTP MCP server and returns a live Client.
func DialHTTP(ctx context.Context, spec HTTPSpec) (Client, error) {
	endpoint, err := url.Parse(spec.URL)
	if err != nil || !endpoint.IsAbs() || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, apperror.New(apperror.KindConfig, "invalid server URL %q (want an absolute http/https URL)", spec.URL)
	}
	httpc, rec, err := buildHTTPClient(endpoint, spec.Header)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindConnection, err, "build http client")
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:             spec.URL,
		HTTPClient:           httpc,
		DisableStandaloneSSE: true,
		MaxRetries:           3,
	}
	wrap := httpWrapErr(rec)
	cl := mcp.NewClient(clientInfo(), nil)
	session, err := cl.Connect(ctx, transport, nil)
	if err != nil {
		httpc.CloseIdleConnections()
		return nil, wrap(err, "connect to "+endpoint.Redacted())
	}
	init := session.InitializeResult()
	return &httpClient{
		mcpSession: &mcpSession{
			sess: session,
			info: ServerInfo{
				Name:            init.ServerInfo.Name,
				Version:         init.ServerInfo.Version,
				ProtocolVersion: init.ProtocolVersion,
				SupportsTools:   init.Capabilities.Tools != nil,
			},
			wrapErr: wrap,
		},
		httpc: httpc,
	}, nil
}

// httpClient is a session backed by an HTTP transport.
type httpClient struct {
	*mcpSession
	httpc *http.Client
}

func (c *httpClient) Close() error {
	err := c.sess.Close()
	c.httpc.CloseIdleConnections()
	return err
}

// httpWrapErr classifies HTTP session errors. Context cancel/timeout map first;
// then the recorded HTTP status distinguishes auth (401/403 → 4) from other
// transport failures (→ 5); a 404 session-missing is a transport error; the
// rest are protocol errors.
func httpWrapErr(rec *statusRecorder) func(err error, op string) error {
	return func(err error, op string) error {
		switch {
		case errors.Is(err, context.Canceled):
			return apperror.Wrap(apperror.KindInterrupted, err, "%s", op)
		case errors.Is(err, context.DeadlineExceeded):
			return apperror.Wrap(apperror.KindTimeout, err, "%s", op)
		}
		switch code := rec.last(); {
		case code == http.StatusUnauthorized || code == http.StatusForbidden:
			return apperror.Wrap(apperror.KindAuth, err, "%s", op)
		case code == 0 || code >= 400:
			// code==0: no HTTP response was received (server down/refused/DNS/TLS
			// failure) — a transport error, consistent with the stdio path.
			// code>=400: a rejecting response. Both are connection/transport failures.
			return apperror.Wrap(apperror.KindConnection, err, "%s", op)
		}
		if errors.Is(err, mcp.ErrSessionMissing) {
			return apperror.Wrap(apperror.KindConnection, err, "%s", op)
		}
		return apperror.Wrap(apperror.KindProtocol, err, "%s", op)
	}
}

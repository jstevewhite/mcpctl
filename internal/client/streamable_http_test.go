package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAuthRoundTripperSameOriginAddsHeaders(t *testing.T) {
	var gotAuth, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	c, rec, err := buildHTTPClient(mustURL(t, srv.URL), hdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer tok" || gotKey != "secret" {
		t.Fatalf("headers not injected: auth=%q key=%q", gotAuth, gotKey)
	}
	if rec.last() != 200 {
		t.Fatalf("recorder = %d, want 200", rec.last())
	}
}

func TestAuthRoundTripperStripsCrossOrigin(t *testing.T) {
	var otherAuth, otherKey string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherAuth = r.Header.Get("Authorization")
		otherKey = r.Header.Get("X-Api-Key")
		w.WriteHeader(200)
	}))
	defer other.Close()

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer tok")
	hdr.Set("X-Api-Key", "secret")
	// Endpoint is a DIFFERENT origin than `other`.
	c, _, err := buildHTTPClient(mustURL(t, "http://127.0.0.1:1"), hdr)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(other.URL) // request to a non-endpoint origin
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if otherAuth != "" || otherKey != "" {
		t.Fatalf("credentials leaked cross-origin: auth=%q key=%q", otherAuth, otherKey)
	}
}

package auth

import "testing"

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestResolveStaticAndEnvAndBearer(t *testing.T) {
	h, err := Resolve(Spec{
		Headers:   map[string]string{"Accept-Language": "en-US"},
		HeaderEnv: map[string]string{"X-Api-Key": "MCP_KEY"},
		BearerEnv: "MCP_TOKEN",
	}, env(map[string]string{"MCP_KEY": "secret", "MCP_TOKEN": "tok"}))
	if err != nil {
		t.Fatal(err)
	}
	if h.Get("Accept-Language") != "en-US" {
		t.Errorf("static header wrong: %q", h.Get("Accept-Language"))
	}
	if h.Get("X-Api-Key") != "secret" {
		t.Errorf("env header wrong: %q", h.Get("X-Api-Key"))
	}
	if h.Get("Authorization") != "Bearer tok" {
		t.Errorf("bearer wrong: %q", h.Get("Authorization"))
	}
}

func TestResolveMissingEnvErrors(t *testing.T) {
	if _, err := Resolve(Spec{HeaderEnv: map[string]string{"X-Api-Key": "NOPE"}}, env(nil)); err == nil {
		t.Fatal("missing env var must error")
	}
	if _, err := Resolve(Spec{BearerEnv: "NOPE"}, env(nil)); err == nil {
		t.Fatal("missing bearer env var must error")
	}
}

func TestResolveConflictingAuthErrors(t *testing.T) {
	if _, err := Resolve(Spec{
		Headers:   map[string]string{"Authorization": "Bearer x"},
		BearerEnv: "MCP_TOKEN",
	}, env(map[string]string{"MCP_TOKEN": "tok"})); err == nil {
		t.Fatal("bearer + Authorization header must conflict")
	}
	if _, err := Resolve(Spec{
		Headers:   map[string]string{"authorization": "a"},
		HeaderEnv: map[string]string{"Authorization": "MCP_AUTH"},
	}, env(map[string]string{"MCP_AUTH": "b"})); err == nil {
		t.Fatal("Authorization via header + header_env must conflict")
	}
}

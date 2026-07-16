package config

import "testing"

func validStdio() *Config {
	return &Config{
		Version: 1,
		Servers: map[string]ServerConfig{
			"local": {Transport: "stdio", Command: "echo"},
		},
	}
}

func TestValidateAcceptsValidStdio(t *testing.T) {
	if err := validStdio().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func validHTTP() *Config {
	return &Config{
		Version: 1,
		Servers: map[string]ServerConfig{
			"remote": {Transport: "streamable-http", URL: "https://example.com/mcp"},
		},
	}
}

func TestValidateAcceptsValidHTTP(t *testing.T) {
	if err := validHTTP().Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateRejections(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"wrong version", func(c *Config) { c.Version = 2 }},
		{"unknown transport", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "carrier-pigeon", Command: "x"}
		}},
		{"stdio without command", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "stdio"}
		}},
		{"stdio with url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "stdio", Command: "x", URL: "https://e.com"}
		}},
		{"http without url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http"}
		}},
		{"http relative url", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "/mcp"}
		}},
		{"http with command", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com", Command: "x"}
		}},
		{"empty server name", func(c *Config) {
			c.Servers[""] = ServerConfig{Transport: "stdio", Command: "x"}
		}},
		{"empty header env var", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				HeaderEnv: map[string]string{"Authorization": ""}}
		}},
		{"bearer and auth header", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				HeaderEnv:   map[string]string{"Authorization": "MCP_AUTH"},
				BearerToken: &TokenSource{Env: "MCP_BEARER"}}
		}},
		{"empty bearer token env", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				BearerToken: &TokenSource{Env: ""}}
		}},
		{"http with args only", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				Args: []string{"x"}}
		}},
		{"bearer and lowercase authorization header", func(c *Config) {
			c.Servers["local"] = ServerConfig{Transport: "streamable-http", URL: "https://e.com",
				Headers:     map[string]string{"authorization": "Bearer x"},
				BearerToken: &TokenSource{Env: "MCP_BEARER"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validStdio()
			tc.mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error for %q", tc.name)
			}
		})
	}
}

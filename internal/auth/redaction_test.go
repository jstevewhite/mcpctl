package auth

import "testing"

func TestIsSensitive(t *testing.T) {
	for _, s := range []string{"Authorization", "authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-API-Key", "X-Auth-Token", "my-secret", "PASSWORD", "api_key"} {
		if !IsSensitive(s) {
			t.Errorf("%q should be sensitive", s)
		}
	}
	for _, s := range []string{"Accept-Language", "Content-Type", "User-Agent"} {
		if IsSensitive(s) {
			t.Errorf("%q should not be sensitive", s)
		}
	}
}

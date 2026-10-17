package config

import "testing"

// TestLoadRestrictsNonLoopbackAddr: without SOP_CONTROLLER_ALLOW_NETWORK, any
// address that is not loopback-only — including an empty host, which binds every
// interface — is rewritten to 127.0.0.1 on the same port.
func TestLoadRestrictsNonLoopbackAddr(t *testing.T) {
	cases := map[string]string{
		"127.0.0.1:8080": "127.0.0.1:8080",
		"localhost:9000": "localhost:9000",
		"[::1]:8080":     "[::1]:8080",
		"127.0.0.2:8080": "127.0.0.2:8080",
		":8080":          "127.0.0.1:8080",
		"0.0.0.0:8080":   "127.0.0.1:8080",
		"[::]:8080":      "127.0.0.1:8080",
		"192.168.1.5:80": "127.0.0.1:80",
		"garbage":        "127.0.0.1:8080",
	}
	t.Setenv("SOP_CONTROLLER_ALLOW_NETWORK", "")
	for in, want := range cases {
		t.Setenv("SOP_CONTROLLER_ADDR", in)
		if got := Load().Addr; got != want {
			t.Errorf("Addr %q: got %q, want %q", in, got, want)
		}
	}
}

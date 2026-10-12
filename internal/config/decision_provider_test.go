package config

import (
	"strings"
	"testing"
)

// TestDecisionProviderRejectsUnsupported proves configuration validation accepts
// only the supported decision providers. An unsupported value (the legacy jev
// name or any invented provider) is rejected at load time with a focused error,
// rather than silently falling back to another provider. This is the
// non-breaking half of the JEV-202 contract alignment: no provider is added or
// selected, only unsupported values are refused.
func TestDecisionProviderRejectsUnsupported(t *testing.T) {
	rejected := []struct {
		name     string
		provider string
	}{
		{"legacy jev name", "jev"},
		{"invented provider", "skynet"},
	}
	for _, tc := range rejected {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			yaml := "project:\n  name: a\ndecision:\n  provider: " + tc.provider + "\n"
			_, err := Parse([]byte(yaml))
			if err == nil {
				t.Fatalf("provider %q must be rejected", tc.provider)
			}
			if !strings.Contains(err.Error(), "unknown decision.provider") {
				t.Errorf("error = %q, want it to contain %q", err, "unknown decision.provider")
			}
		})
	}

	accepted := []struct {
		name     string
		yaml     string
		provider string
	}{
		{
			"deterministic",
			"project:\n  name: a\ndecision:\n  provider: deterministic\n",
			"deterministic",
		},
		{
			"command",
			"project:\n  name: a\ndecision:\n  provider: command\n  command:\n    - /path/to/adapter\n",
			"command",
		},
	}
	for _, tc := range accepted {
		t.Run("accept/"+tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatalf("provider %q must be accepted: %v", tc.provider, err)
			}
			if c.Decision.Provider != tc.provider {
				t.Errorf("decision.provider = %q, want %q", c.Decision.Provider, tc.provider)
			}
		})
	}

	// An omitted provider keeps the deterministic default.
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("default config must parse: %v", err)
	}
	if c.Decision.Provider != "deterministic" {
		t.Errorf("default decision.provider = %q, want deterministic", c.Decision.Provider)
	}
}

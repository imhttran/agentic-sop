package config

import (
	"strings"
	"testing"
)

// TestDecisionProviderAdapterRequiresCommand proves the opt-in adapter provider
// is accepted only with a dedicated decision.adapter_command argv, and is rejected
// when that argv is missing. decision.command keeps its separate meaning.
func TestDecisionProviderAdapterRequiresCommand(t *testing.T) {
	// Accepted with adapter_command.
	c, err := Parse([]byte("project:\n  name: a\ndecision:\n  provider: adapter\n  adapter_command:\n    - /path/to/adapter\n"))
	if err != nil {
		t.Fatalf("adapter with adapter_command must be accepted: %v", err)
	}
	if c.Decision.Provider != "adapter" || len(c.Decision.AdapterCommand) != 1 {
		t.Fatalf("parsed decision = %+v, want provider adapter + adapter_command", c.Decision)
	}

	// Rejected without adapter_command (fail closed).
	if _, err := Parse([]byte("project:\n  name: a\ndecision:\n  provider: adapter\n")); err == nil {
		t.Fatal("adapter without adapter_command must be rejected")
	} else if !strings.Contains(err.Error(), "adapter_command") {
		t.Errorf("error = %q, want it to name adapter_command", err)
	}

	// decision.command does NOT satisfy the adapter requirement.
	if _, err := Parse([]byte("project:\n  name: a\ndecision:\n  provider: adapter\n  command:\n    - /x\n")); err == nil {
		t.Fatal("adapter must require adapter_command, not command")
	}
}

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

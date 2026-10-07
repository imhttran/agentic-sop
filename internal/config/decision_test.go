package config

import "testing"

// TestDecisionConfigOffByDefault proves an existing installation with no
// decision configuration is unchanged: disabled, deterministic, no command.
func TestDecisionConfigOffByDefault(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Decision.Enabled || c.Decision.Provider != "deterministic" || len(c.Decision.Command) != 0 {
		t.Fatalf("default decision config = %+v, want disabled/deterministic/no command", c.Decision)
	}
}

// TestDecisionCommandConfig proves the provider-neutral command surface parses
// and that misconfiguration fails closed at config validation.
func TestDecisionCommandConfig(t *testing.T) {
	if _, err := Parse([]byte("project:\n  name: a\ndecision:\n  provider: command\n")); err == nil {
		t.Fatal("decision.provider command without decision.command must be rejected")
	}

	c, err := Parse([]byte("project:\n  name: a\ndecision:\n  enabled: true\n  provider: command\n  command:\n    - /path/to/adapter\n    - --flag\n"))
	if err != nil {
		t.Fatalf("valid command config rejected: %v", err)
	}
	if !c.Decision.Enabled || c.Decision.Provider != "command" {
		t.Fatalf("decision = %+v, want enabled command", c.Decision)
	}
	if len(c.Decision.Command) != 2 || c.Decision.Command[0] != "/path/to/adapter" || c.Decision.Command[1] != "--flag" {
		t.Fatalf("command argv = %v, want [/path/to/adapter --flag]", c.Decision.Command)
	}
}

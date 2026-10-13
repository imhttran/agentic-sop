package config

import (
	"strings"
	"testing"
)

// clearDecisionEnv neutralizes the decision environment variables so a test sees
// only the values it sets. An empty value is treated as unset by the loader.
func clearDecisionEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"SOP_DECISION_ENABLED",
		"SOP_DECISION_PROVIDER",
		"SOP_DECISION_ADAPTER_PROVIDER",
		"SOP_DECISION_ADAPTER_COMMAND",
		"SOP_DECISION_ADAPTER_MODEL",
	} {
		t.Setenv(k, "")
	}
}

// TestDecisionEnvEnablesAdapter proves the environment can enable and configure
// the SEAM-004 advisory adapter for a project that omits the decision block.
func TestDecisionEnvEnablesAdapter(t *testing.T) {
	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_ENABLED", "true")
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_PROVIDER", "nimble")
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", "/usr/local/bin/sop-decision-adapter")
	t.Setenv("SOP_DECISION_ADAPTER_MODEL", "nimble")

	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !c.Decision.DecisionEnabled() {
		t.Error("SOP_DECISION_ENABLED=true must enable the decision layer")
	}
	if c.Decision.Provider != "adapter" {
		t.Errorf("provider = %q, want adapter", c.Decision.Provider)
	}
	if c.Decision.AdapterProvider != "nimble" || c.Decision.AdapterModel != "nimble" {
		t.Errorf("adapter hints = %q/%q, want nimble/nimble", c.Decision.AdapterProvider, c.Decision.AdapterModel)
	}
	if len(c.Decision.AdapterCommand) != 1 || c.Decision.AdapterCommand[0] != "/usr/local/bin/sop-decision-adapter" {
		t.Errorf("adapter_command = %v, want the single executable", c.Decision.AdapterCommand)
	}
}

// TestDecisionConfigBeatsEnv proves explicit project configuration wins over the
// environment (precedence: config > env > default).
func TestDecisionConfigBeatsEnv(t *testing.T) {
	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", "/env/adapter")
	t.Setenv("SOP_DECISION_ENABLED", "true")

	c, err := Parse([]byte("project:\n  name: a\ndecision:\n  enabled: false\n  provider: command\n  command:\n    - /cfg/adapter\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Decision.Provider != "command" {
		t.Errorf("provider = %q, want the configured command (env must not override)", c.Decision.Provider)
	}
	if len(c.Decision.Command) != 1 || c.Decision.Command[0] != "/cfg/adapter" {
		t.Errorf("command = %v, want the configured argv", c.Decision.Command)
	}
	if len(c.Decision.AdapterCommand) != 0 {
		t.Errorf("adapter_command = %v, want it untouched (config set provider)", c.Decision.AdapterCommand)
	}
}

// TestDecisionExplicitDisableBeatsEnv proves a project can disable the adapter
// even when the environment enables it.
func TestDecisionExplicitDisableBeatsEnv(t *testing.T) {
	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_ENABLED", "true")
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", "/env/adapter")

	c, err := Parse([]byte("project:\n  name: a\ndecision:\n  enabled: false\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Decision.DecisionEnabled() {
		t.Error("an explicit decision.enabled: false must override SOP_DECISION_ENABLED=true")
	}
}

// TestDecisionEnabledExplicitFalseWithoutEnv proves an explicit false resolves to
// disabled with no environment at all.
func TestDecisionEnabledExplicitFalseWithoutEnv(t *testing.T) {
	clearDecisionEnv(t)
	c, err := Parse([]byte("project:\n  name: a\ndecision:\n  enabled: false\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Decision.DecisionEnabled() {
		t.Error("enabled: false must resolve to disabled")
	}
}

// TestDecisionDefaultsWithoutEnvOrConfig proves the deterministic default when
// neither the project nor the environment configures the layer.
func TestDecisionDefaultsWithoutEnvOrConfig(t *testing.T) {
	clearDecisionEnv(t)
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if c.Decision.DecisionEnabled() {
		t.Error("decision layer must default to disabled")
	}
	if c.Decision.Provider != "deterministic" {
		t.Errorf("provider = %q, want deterministic", c.Decision.Provider)
	}
	if len(c.Decision.AdapterCommand) != 0 {
		t.Errorf("adapter_command = %v, want empty", c.Decision.AdapterCommand)
	}
}

// TestDecisionEnvInvalidFailsClosed proves an unparseable boolean and an unknown
// provider are focused errors, never silent defaults.
func TestDecisionEnvInvalidFailsClosed(t *testing.T) {
	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_ENABLED", "maybe")
	if _, err := Parse([]byte("project:\n  name: a\n")); err == nil {
		t.Error("invalid SOP_DECISION_ENABLED must fail")
	} else if !strings.Contains(err.Error(), "SOP_DECISION_ENABLED") {
		t.Errorf("error = %q, want it to name SOP_DECISION_ENABLED", err)
	}

	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_ENABLED", "true")
	t.Setenv("SOP_DECISION_PROVIDER", "skynet")
	if _, err := Parse([]byte("project:\n  name: a\n")); err == nil {
		t.Error("unknown SOP_DECISION_PROVIDER must be rejected")
	} else if !strings.Contains(err.Error(), "unknown decision.provider") {
		t.Errorf("error = %q, want unknown decision.provider", err)
	}
}

// TestDecisionEnvAdapterCommandParsing proves the command accepts both a
// whitespace-separated form and a JSON array.
func TestDecisionEnvAdapterCommandParsing(t *testing.T) {
	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", "/bin/adapter --mode fast")
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Decision.AdapterCommand, "|"); got != "/bin/adapter|--mode|fast" {
		t.Errorf("whitespace argv = %q", got)
	}

	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", `["/bin/adapter","--mode","fast"]`)
	c, err = Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(c.Decision.AdapterCommand, "|"); got != "/bin/adapter|--mode|fast" {
		t.Errorf("json argv = %q", got)
	}

	clearDecisionEnv(t)
	t.Setenv("SOP_DECISION_PROVIDER", "adapter")
	t.Setenv("SOP_DECISION_ADAPTER_COMMAND", `["/bin/adapter"`)
	if _, err := Parse([]byte("project:\n  name: a\n")); err == nil {
		t.Error("malformed JSON array must fail closed")
	}
}

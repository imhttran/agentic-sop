package config

import "testing"

// TestVerdictGateOffByDefault proves the operator-enabled verdict gate (SOP-HARD-01)
// is off by default: an omitted lifecycle block leaves the local lifecycle (the
// LOCAL_DONE transition) unchanged.
func TestVerdictGateOffByDefault(t *testing.T) {
	cfg, err := Parse([]byte("project:\n  name: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VerdictGateEnabled() {
		t.Error("the verdict gate must be off by default")
	}
	if cfg.Lifecycle.VerdictGate {
		t.Error("lifecycle.verdict_gate must be false by default")
	}
}

// TestVerdictGateEnabledParses proves an explicit operator opt-in enables the gate,
// and that the accessor reports it.
func TestVerdictGateEnabledParses(t *testing.T) {
	cfg, err := Parse([]byte("project:\n  name: x\nlifecycle:\n  verdict_gate: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.VerdictGateEnabled() {
		t.Error("lifecycle.verdict_gate: true must enable the verdict gate")
	}
}

// TestVerdictGateExplicitFalseStaysOff proves an explicit false is off, so the
// gate is enabled only by an explicit true.
func TestVerdictGateExplicitFalseStaysOff(t *testing.T) {
	cfg, err := Parse([]byte("project:\n  name: x\nlifecycle:\n  verdict_gate: false\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.VerdictGateEnabled() {
		t.Error("an explicit verdict_gate: false must stay off")
	}
}

// TestDefaultConfigHasVerdictGateOff proves the built-in default configuration
// leaves the gate off, so no default behavior changes.
func TestDefaultConfigHasVerdictGateOff(t *testing.T) {
	if Default().VerdictGateEnabled() {
		t.Error("the default configuration must leave the verdict gate off")
	}
}

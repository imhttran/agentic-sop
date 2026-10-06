package config

import "testing"

// TestOrchestrationIsDisabledByDefault proves introducing Phase 9 changes no default
// behavior: an omitted orchestration block leaves the opt-in path off.
func TestOrchestrationIsDisabledByDefault(t *testing.T) {
	cfg, err := Parse([]byte("project:\n  name: x\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Orchestration.Enabled {
		t.Error("orchestration must be disabled by default")
	}
}

// TestOrchestrationEnabledParses proves the explicit opt-in gate and its bounded
// limits parse.
func TestOrchestrationEnabledParses(t *testing.T) {
	cfg, err := Parse([]byte("project:\n  name: x\norchestration:\n  enabled: true\n  max_concurrency: 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Orchestration.Enabled {
		t.Error("orchestration.enabled: true must parse as enabled")
	}
	if cfg.Orchestration.MaxConcurrency != 3 {
		t.Errorf("max_concurrency = %d, want 3", cfg.Orchestration.MaxConcurrency)
	}
}

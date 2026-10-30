package config

import "testing"

// TestContinuationEnvOverrides pins both continuation settings: the env total
// overrides the default, the per-run limit defaults to 1 and accepts 0, and an
// invalid value is rejected.
func TestContinuationEnvOverrides(t *testing.T) {
	c := Default()
	if got := c.RunMaxContinues(); got != DefaultRunMaxContinues {
		t.Errorf("default per-run continues = %d, want %d", got, DefaultRunMaxContinues)
	}
	t.Setenv(EnvMaxContinuations, "6")
	t.Setenv(EnvRunMaxContinues, "0")
	if got := c.AutonomyPolicy().MaxContinuations; got != 6 {
		t.Errorf("max continuations = %d, want 6 from %s", got, EnvMaxContinuations)
	}
	if got := c.RunMaxContinues(); got != 0 {
		t.Errorf("per-run continues = %d, want 0", got)
	}
	if err := ValidateAutonomyEnv(); err != nil {
		t.Errorf("valid values rejected: %v", err)
	}
	for name, v := range map[string]string{EnvMaxContinuations: "0", EnvRunMaxContinues: "two"} {
		t.Setenv(name, v)
		if err := ValidateAutonomyEnv(); err == nil {
			t.Errorf("%s=%q accepted, want an error", name, v)
		}
		t.Setenv(name, "1")
	}
}

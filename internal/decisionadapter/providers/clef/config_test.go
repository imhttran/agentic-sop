package clef

import (
	"testing"
	"time"
)

// TestConfigFromEnvDisabledByDefault proves Clef is OFF unless CLEF_ENABLED is
// explicitly set: transport defaults must never implicitly enable the provider.
func TestConfigFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv("CLEF_ENABLED", "")

	cfg := ConfigFromEnv()
	if cfg.Enable {
		t.Errorf("Enable = true, want false when CLEF_ENABLED is unset")
	}
	if cfg.Enabled() {
		t.Errorf("Enabled() = true, want false when CLEF_ENABLED is unset")
	}
}

func TestConfigFromEnvEnabledOptIn(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "Yes"} {
		v := v
		t.Run(v, func(t *testing.T) {
			t.Setenv("CLEF_ENABLED", v)
			if cfg := ConfigFromEnv(); !cfg.Enabled() {
				t.Errorf("CLEF_ENABLED=%q: Enabled() = false, want true", v)
			}
		})
	}
}

// TestConfigFromEnvEnabledNonOptInFallsBack covers blank/non-positive/other
// CLEF_ENABLED values: they must fall back to the disabled default.
func TestConfigFromEnvEnabledNonOptInFallsBack(t *testing.T) {
	for _, v := range []string{"", "  ", "0", "-1", "false", "no", "off", "garbage"} {
		v := v
		t.Run(v, func(t *testing.T) {
			t.Setenv("CLEF_ENABLED", v)
			if cfg := ConfigFromEnv(); cfg.Enabled() {
				t.Errorf("CLEF_ENABLED=%q: Enabled() = true, want false", v)
			}
		})
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("CLEF_MODEL", "")
	t.Setenv("CLEF_TIMEOUT", "")
	t.Setenv("CLEF_ENABLED", "")
	t.Setenv("CLEF_TRANSPORT", "")

	cfg := ConfigFromEnv()
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.Enable {
		t.Errorf("Enable = true, want false by default")
	}
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://ollama.remote:11434")
	t.Setenv("CLEF_MODEL", "clef-v2")
	t.Setenv("CLEF_TIMEOUT", "5s")

	cfg := ConfigFromEnv()
	if cfg.BaseURL != "http://ollama.remote:11434" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "clef-v2" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
}

// TestConfigFromEnvInvalidTimeoutFallsBack covers blank/non-positive and
// unparsable CLEF_TIMEOUT: all must resolve to DefaultTimeout.
func TestConfigFromEnvInvalidTimeoutFallsBack(t *testing.T) {
	for _, v := range []string{"", "  ", "not-a-duration", "0s", "-5s"} {
		v := v
		t.Run(v, func(t *testing.T) {
			t.Setenv("CLEF_TIMEOUT", v)
			if got := ConfigFromEnv().Timeout; got != DefaultTimeout {
				t.Errorf("CLEF_TIMEOUT=%q: Timeout = %v, want %v", v, got, DefaultTimeout)
			}
		})
	}
}

func TestConfigEnabled(t *testing.T) {
	if (Config{}).Enabled() {
		t.Error("zero Config must be disabled")
	}
	if !(Config{Enable: true}).Enabled() {
		t.Error("Config{Enable: true}.Enabled() = false, want true")
	}
}

func TestConfigWithDefaults(t *testing.T) {
	got := (Config{}).WithDefaults()
	if got.BaseURL != DefaultBaseURL || got.Model != DefaultModel || got.Timeout != DefaultTimeout {
		t.Errorf("WithDefaults() = %+v, want all defaults", got)
	}
	// WithDefaults must never enable Clef.
	if got.Enable || got.Enabled() {
		t.Error("WithDefaults() enabled Clef, want disabled")
	}

	custom := (Config{BaseURL: "http://x", Model: "m", Timeout: time.Second}).WithDefaults()
	if custom.BaseURL != "http://x" || custom.Model != "m" || custom.Timeout != time.Second {
		t.Errorf("WithDefaults() overrode explicit values: %+v", custom)
	}

	// Negative timeout falls back; explicit enable is preserved.
	neg := (Config{Timeout: -time.Second, Enable: true}).WithDefaults()
	if neg.Timeout != DefaultTimeout {
		t.Errorf("WithDefaults() Timeout = %v, want %v", neg.Timeout, DefaultTimeout)
	}
	if !neg.Enabled() {
		t.Error("WithDefaults() cleared explicit Enable, want true")
	}
}

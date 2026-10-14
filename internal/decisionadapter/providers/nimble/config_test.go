package nimble

import (
	"testing"
	"time"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "")
	t.Setenv("NIMBLE_MODEL", "")
	t.Setenv("NIMBLE_TIMEOUT", "")

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
}

func TestConfigFromEnvOverrides(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://ollama.remote:11434")
	t.Setenv("NIMBLE_MODEL", "nimble-v2")
	t.Setenv("NIMBLE_TIMEOUT", "5s")

	cfg := ConfigFromEnv()
	if cfg.BaseURL != "http://ollama.remote:11434" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "nimble-v2" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
}

func TestConfigFromEnvInvalidTimeoutFallsBack(t *testing.T) {
	t.Setenv("NIMBLE_TIMEOUT", "not-a-duration")
	if got := ConfigFromEnv().Timeout; got != DefaultTimeout {
		t.Errorf("Timeout = %v, want %v", got, DefaultTimeout)
	}
}

func TestConfigWithDefaults(t *testing.T) {
	got := (Config{}).WithDefaults()
	if got.BaseURL != DefaultBaseURL || got.Model != DefaultModel || got.Timeout != DefaultTimeout {
		t.Errorf("WithDefaults() = %+v, want all defaults", got)
	}

	custom := (Config{BaseURL: "http://x", Model: "m", Timeout: time.Second}).WithDefaults()
	if custom.BaseURL != "http://x" || custom.Model != "m" || custom.Timeout != time.Second {
		t.Errorf("WithDefaults() overrode explicit values: %+v", custom)
	}
}

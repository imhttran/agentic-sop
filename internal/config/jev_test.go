package config

import (
	"strings"
	"testing"
)

// JEV002: the JEV feature flag is disabled by default and only enabled by
// explicit configuration. These tests cover the flag contract, compatibility of
// existing configurations, and the focused error for invalid JEV configuration.

func TestJEVMissingFlagYieldsDisabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Quality.JEV.Enabled != nil {
		t.Errorf("JEV.Enabled = %v, want nil when the flag is omitted", c.Quality.JEV.Enabled)
	}
	if c.Quality.JEVEnabled() {
		t.Error("JEVEnabled() = true, want false when the flag is omitted")
	}
	if c.JEVActive() {
		t.Error("JEVActive() = true, want false when the flag is omitted")
	}
}

func TestJEVExplicitFalseYieldsDisabled(t *testing.T) {
	yaml := "project:\n  name: a\nquality:\n  jev:\n    enabled: false\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Quality.JEV.Enabled == nil || *c.Quality.JEV.Enabled {
		t.Error("explicit enabled: false did not resolve to disabled")
	}
	if c.JEVActive() {
		t.Error("JEVActive() = true, want false for explicit false")
	}
}

func TestJEVExplicitTrueYieldsEnabled(t *testing.T) {
	yaml := "project:\n  name: a\nquality:\n  jev:\n    enabled: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.Quality.JEVEnabled() {
		t.Error("JEVEnabled() = false, want true for explicit true")
	}
	if !c.JEVActive() {
		t.Error("JEVActive() = false, want true for explicit true")
	}
}

func TestJEVModeDefaultsToReview(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nquality:\n  jev:\n    enabled: true\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Quality.JEV.Mode != "review" {
		t.Errorf("mode = %q, want review", c.Quality.JEV.Mode)
	}
}

// TestJEVModeNotDefaultedWhenDisabled proves a disabled (or omitted) JEV block
// is not mutated: the mode keeps its zero value when JEV is not enabled.
func TestJEVModeNotDefaultedWhenDisabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Quality.JEV.Mode != "" {
		t.Errorf("mode = %q, want empty when JEV is disabled", c.Quality.JEV.Mode)
	}
}

// TestJEVDisabledInvalidModeStillRejected proves an explicitly written bad mode
// is rejected even when JEV is disabled, so a later enable cannot carry it.
func TestJEVDisabledInvalidModeStillRejected(t *testing.T) {
	_, err := Parse([]byte("project:\n  name: a\nquality:\n  jev:\n    enabled: false\n    mode: telepathy\n"))
	if err == nil {
		t.Fatal("expected an error for an explicit unknown mode")
	}
	if !strings.Contains(err.Error(), "unknown quality.jev.mode") {
		t.Errorf("error = %q, want it to contain %q", err, "unknown quality.jev.mode")
	}
}

// TestJEVFailOnDefaultsToQualityFailOn proves an omitted JEV fail_on reuses the
// quality gate's blocking severities.
func TestJEVFailOnDefaultsToQualityFailOn(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nquality:\n  jev:\n    enabled: true\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got := strings.Join(c.Quality.JEVFailOn(), ","); got != "critical,high" {
		t.Errorf("JEVFailOn = %q, want critical,high", got)
	}
}

// TestExistingConfigsRemainCompatible loads a pre-JEV configuration shape and
// asserts it still parses and validates with JEV disabled.
func TestExistingConfigsRemainCompatible(t *testing.T) {
	old := `version: 1
project:
  name: book-rag
  integration_branch: main
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
validation:
  build:
    - go build ./...
review:
  engine: self
  delegation: false
quality:
  require_tests: true
  max_fix_cycles: 3
  fail_on:
    - critical
    - high
human:
  approval_before_commit: true
workflow:
  mode: local
`
	c, err := Parse([]byte(old))
	if err != nil {
		t.Fatalf("pre-JEV config no longer parses: %v", err)
	}
	if c.JEVActive() {
		t.Error("JEV active for a config without the flag, want disabled")
	}
}

// TestDecisionProviderJevDoesNotEnableJEV proves the legacy decision-layer
// signals cannot turn JEV on by themselves.
func TestDecisionProviderJevDoesNotEnableJEV(t *testing.T) {
	yaml := "project:\n  name: a\ndecision:\n  provider: jev\nfeatures:\n  jev_decisions: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.JEVActive() {
		t.Error("JEV active without an explicit quality.jev.enabled, want disabled")
	}
}

// TestJEVInvalidConfigFocusedErrors covers the focused, single-purpose error for
// invalid JEV configuration, returned at load/parse time.
func TestJEVInvalidConfigFocusedErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantMsg string
	}{
		{
			"unknown mode",
			"project:\n  name: a\nquality:\n  jev:\n    mode: telepathy\n",
			"unknown quality.jev.mode",
		},
		{
			"unknown fail_on severity",
			"project:\n  name: a\nquality:\n  jev:\n    enabled: true\n    fail_on:\n      - blocker\n",
			"unknown quality.jev.fail_on severity",
		},
		{
			"unknown key under jev",
			"project:\n  name: a\nquality:\n  jev:\n    enabled: true\n    turbo: true\n",
			"turbo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

// TestJEVDisabledStubStillLoads proves a disabled JEV block is never blocked by
// its optional settings, so existing files keep loading.
func TestJEVDisabledStubStillLoads(t *testing.T) {
	yaml := "project:\n  name: a\nquality:\n  jev:\n    enabled: false\n    mode: review\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.JEVActive() {
		t.Error("JEV active for disabled stub, want disabled")
	}
}

// TestDefaultJEVIsDisabled proves the built-in defaults keep JEV disabled.
func TestDefaultJEVIsDisabled(t *testing.T) {
	c := Default()
	if c.JEVActive() {
		t.Error("Default() has JEV active, want disabled")
	}
}

package config

import (
	"reflect"
	"strings"
	"testing"
)

// P3-005: the early-JEV decision layer lives in its own top-level `early_jev`
// namespace, is disabled by default, is compatible with existing
// installations, fails clearly on an unknown mode/key, leaves `sop run`
// behavior unchanged when disabled, activates without requiring
// quality.jev.enabled, and leaves quality.jev, decision.*, and models
// untouched.

// TestEarlyJEVOmittedIsDisabled proves an omitted early_jev block resolves the
// early layer to disabled and does not mutate the block.
func TestEarlyJEVOmittedIsDisabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.EarlyJEV.Enabled != nil {
		t.Errorf("EarlyJEV.Enabled = %v, want nil when the flag is omitted", c.EarlyJEV.Enabled)
	}
	if c.EarlyJEVActive() {
		t.Error("EarlyJEVActive() = true, want false when the block is omitted")
	}
	if c.EarlyJEV.Mode != "" {
		t.Errorf("EarlyJEV.Mode = %q, want empty when disabled", c.EarlyJEV.Mode)
	}
}

// TestEarlyJEVExplicitFalseIsDisabled proves an explicit enabled: false also
// resolves to disabled.
func TestEarlyJEVExplicitFalseIsDisabled(t *testing.T) {
	yaml := "project:\n  name: a\nearly_jev:\n  enabled: false\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.EarlyJEV.Enabled == nil || *c.EarlyJEV.Enabled {
		t.Error("explicit enabled: false did not resolve to disabled")
	}
	if c.EarlyJEVActive() {
		t.Error("EarlyJEVActive() = true, want false for explicit false")
	}
}

// TestEarlyJEVExplicitTrueIsEnabled proves an explicit enabled: true resolves
// the early layer to enabled and defaults the mode to review.
func TestEarlyJEVExplicitTrueIsEnabled(t *testing.T) {
	yaml := "project:\n  name: a\nearly_jev:\n  enabled: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.EarlyJEVActive() {
		t.Error("EarlyJEVActive() = false, want true for explicit true")
	}
	if c.EarlyJEVMode() != "review" {
		t.Errorf("EarlyJEVMode() = %q, want review", c.EarlyJEVMode())
	}
}

// TestEarlyJEVModeNotDefaultedWhenDisabled proves a disabled stub is never
// mutated: the mode keeps its zero value when the early layer is not enabled.
func TestEarlyJEVModeNotDefaultedWhenDisabled(t *testing.T) {
	yaml := "project:\n  name: a\nearly_jev:\n  enabled: false\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.EarlyJEV.Mode != "" {
		t.Errorf("EarlyJEV.Mode = %q, want empty for a disabled stub", c.EarlyJEV.Mode)
	}
}

// TestEarlyJEVActivatesWithoutQualityJEV proves early_jev.enabled activates
// the early layer without requiring quality.jev.enabled, and that quality JEV
// stays inactive in that case (acceptance criterion: the two layers are
// independently enable-able).
func TestEarlyJEVActivatesWithoutQualityJEV(t *testing.T) {
	yaml := "project:\n  name: a\nearly_jev:\n  enabled: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.EarlyJEVActive() {
		t.Error("EarlyJEVActive() = false, want true")
	}
	if c.JEVActive() {
		t.Error("JEVActive() = true, want false: early_jev must not enable quality.jev")
	}
}

// TestQualityJEVDoesNotEnableEarlyJEV proves the converse: enabling
// quality.jev leaves the early layer disabled.
func TestQualityJEVDoesNotEnableEarlyJEV(t *testing.T) {
	yaml := "project:\n  name: a\nquality:\n  jev:\n    enabled: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.JEVActive() {
		t.Error("JEVActive() = false, want true")
	}
	if c.EarlyJEVActive() {
		t.Error("EarlyJEVActive() = true, want false: quality.jev must not enable the early layer")
	}
}

// TestEarlyJEVFailOnDefaultsToQualityFailOn proves an omitted early_jev.fail_on
// reuses the quality gate's blocking severities.
func TestEarlyJEVFailOnDefaultsToQualityFailOn(t *testing.T) {
	yaml := "project:\n  name: a\nquality:\n  fail_on:\n    - critical\nearly_jev:\n  enabled: true\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got := strings.Join(c.EarlyJEVFailOn(), ","); got != "critical" {
		t.Errorf("EarlyJEVFailOn() = %q, want critical", got)
	}
}

// TestEarlyJEVFailOnExplicit proves an explicit early_jev.fail_on is used.
func TestEarlyJEVFailOnExplicit(t *testing.T) {
	yaml := "project:\n  name: a\nearly_jev:\n  enabled: true\n  fail_on:\n    - medium\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got := strings.Join(c.EarlyJEVFailOn(), ","); got != "medium" {
		t.Errorf("EarlyJEVFailOn() = %q, want medium", got)
	}
}

// TestEarlyJEVInvalidConfigFocusedErrors covers the focused, load-time errors
// for invalid early_jev configuration: an unknown mode, an unknown fail_on
// severity, and an unknown key under early_jev (strict decoding).
func TestEarlyJEVInvalidConfigFocusedErrors(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantMsg string
	}{
		{
			"unknown mode",
			"project:\n  name: a\nearly_jev:\n  enabled: true\n  mode: telepathy\n",
			"unknown early_jev.mode",
		},
		{
			"unknown fail_on severity",
			"project:\n  name: a\nearly_jev:\n  enabled: true\n  fail_on:\n    - blocker\n",
			"unknown early_jev.fail_on severity",
		},
		{
			"unknown key under early_jev",
			"project:\n  name: a\nearly_jev:\n  enabled: true\n  modes: review\n",
			"modes",
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

// TestEarlyJEVDisabledInvalidModeStillRejected proves an explicitly written bad
// mode is rejected even when the early layer is disabled, so a later enable
// cannot carry it.
func TestEarlyJEVDisabledInvalidModeStillRejected(t *testing.T) {
	_, err := Parse([]byte("project:\n  name: a\nearly_jev:\n  enabled: false\n  mode: telepathy\n"))
	if err == nil {
		t.Fatal("expected an error for an explicit unknown mode")
	}
	if !strings.Contains(err.Error(), "unknown early_jev.mode") {
		t.Errorf("error = %q, want it to contain %q", err, "unknown early_jev.mode")
	}
}

// TestDefaultEarlyJEVIsDisabled proves the built-in defaults keep the early
// layer disabled.
func TestDefaultEarlyJEVIsDisabled(t *testing.T) {
	c := Default()
	if c.EarlyJEVActive() {
		t.Error("Default() has the early layer active, want disabled")
	}
}

// TestExistingConfigsLoadUnchangedWithEarlyJEV proves an existing installation
// configuration (with no early_jev block) still loads and resolves all fields,
// and that quality.jev, decision.*, and models are untouched by adding the new
// namespace (acceptance criteria 1, 2, 4, 6).
func TestExistingConfigsLoadUnchangedWithEarlyJEV(t *testing.T) {
	const existing = `version: 1
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
  jev:
    enabled: false
    mode: review
decision:
  provider: deterministic
models:
  default_class: medium
human:
  approval_before_commit: true
workflow:
  mode: local
`
	got, err := Parse([]byte(existing))
	if err != nil {
		t.Fatalf("existing config no longer parses: %v", err)
	}
	// Gates default OFF.
	if got.JEVActive() {
		t.Error("quality JEV active for a config with enabled: false")
	}
	if got.EarlyJEVActive() {
		t.Error("early JEV active for a config without early_jev")
	}
	// quality.jev, decision.*, and models are untouched.
	if got.Quality.JEV.Enabled == nil || *got.Quality.JEV.Enabled {
		t.Errorf("quality.jev.enabled = %v, want explicit false", got.Quality.JEV.Enabled)
	}
	if got.Quality.JEV.Mode != "review" {
		t.Errorf("quality.jev.mode = %q, want review", got.Quality.JEV.Mode)
	}
	if got.Decision.Provider != "deterministic" {
		t.Errorf("decision.provider = %q, want deterministic", got.Decision.Provider)
	}
	if got.Models.DefaultClass != "medium" {
		t.Errorf("models.default_class = %q, want medium", got.Models.DefaultClass)
	}
}

// TestDisabledEarlyJEVYieldsIdenticalBehavior proves a config with a disabled
// early_jev stub resolves to the same behavior as the same config without the
// block: the early layer is inactive either way, no mode is injected, and every
// other resolved field is deep-equal (acceptance criterion 4). The only
// permitted difference is the explicit-false pointer itself, which is what
// preserves the distinction between "omitted" and "explicit false" while still
// resolving to disabled.
func TestDisabledEarlyJEVYieldsIdenticalBehavior(t *testing.T) {
	withBlock := "project:\n  name: a\nearly_jev:\n  enabled: false\n"
	withoutBlock := "project:\n  name: a\n"

	a, err := Parse([]byte(withBlock))
	if err != nil {
		t.Fatalf("Parse with disabled block failed: %v", err)
	}
	b, err := Parse([]byte(withoutBlock))
	if err != nil {
		t.Fatalf("Parse without block failed: %v", err)
	}
	if a.EarlyJEVActive() || b.EarlyJEVActive() {
		t.Error("early layer active when disabled, want inactive")
	}
	if a.EarlyJEVMode() != b.EarlyJEVMode() {
		t.Errorf("mode differs: %q vs %q", a.EarlyJEVMode(), b.EarlyJEVMode())
	}
	// Normalize the explicit-false pointer so the rest of the resolved config
	// can be compared for deep equality.
	a.EarlyJEV = b.EarlyJEV
	if !reflect.DeepEqual(*a, *b) {
		t.Errorf("disabled early_jev block changed the resolved config:\n with = %+v\n without = %+v", *a, *b)
	}
}

// TestTemplateEarlyJEV proves the generated template documents the early_jev
// block as disabled, parses, and keeps the other namespaces documented.
func TestTemplateEarlyJEV(t *testing.T) {
	tpl := Template("book-rag")
	if !strings.Contains(tpl, "early_jev:") {
		t.Error("template does not document the early_jev block")
	}
	if !strings.Contains(tpl, "enabled: false") {
		t.Error("template does not document enabled: false")
	}
	for _, want := range []string{"quality:", "jev:", "decision", "models:"} {
		if !strings.Contains(tpl, want) {
			t.Errorf("template no longer documents %q", want)
		}
	}
	c, err := Parse([]byte(tpl))
	if err != nil {
		t.Fatalf("generated template does not parse: %v", err)
	}
	if c.EarlyJEVActive() {
		t.Error("template activates the early layer, want disabled by default")
	}
	if c.JEVActive() {
		t.Error("template activates quality JEV, want disabled by default")
	}
}

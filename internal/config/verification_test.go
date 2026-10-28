package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestVerificationOmittedIsDisabled proves the block is off by default: an omitted
// verification block leaves the config unchanged and selects no targeted set.
func TestVerificationOmittedIsDisabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Verification.Enabled() {
		t.Error("verification must be disabled when the block is omitted")
	}
	if got := c.Verification.LowValidation(); len(got.Build) != 0 || len(got.Test) != 0 {
		t.Errorf("LowValidation() = %+v, want empty", got)
	}
}

// TestVerificationBlockAloneActivatesNothing proves a present block without
// risk_based stays disabled (the block alone changes nothing).
func TestVerificationBlockAloneActivatesNothing(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nverification:\n  low:\n    test:\n      - go test ./internal/x/...\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Verification.Enabled() {
		t.Error("a verification block without risk_based must stay disabled")
	}
}

// TestVerificationEnabledMapsLowValidation proves risk_based: true enables the tier
// and LowValidation() reuses the validation shape.
func TestVerificationEnabledMapsLowValidation(t *testing.T) {
	yaml := "project:\n  name: a\n" +
		"verification:\n" +
		"  risk_based: true\n" +
		"  low:\n" +
		"    build:\n      - go build ./internal/x/...\n" +
		"    test:\n      - go test ./internal/x/...\n"
	c, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !c.Verification.Enabled() {
		t.Fatal("verification must be enabled with risk_based: true")
	}
	want := Validation{Build: []string{"go build ./internal/x/..."}, Test: []string{"go test ./internal/x/..."}}
	if got := c.Verification.LowValidation(); !reflect.DeepEqual(got, want) {
		t.Errorf("LowValidation() = %+v, want %+v", got, want)
	}
}

// TestVerificationExplicitFalseStaysDisabled proves an explicit false disables the
// tier.
func TestVerificationExplicitFalseStaysDisabled(t *testing.T) {
	c, err := Parse([]byte("project:\n  name: a\nverification:\n  risk_based: false\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if c.Verification.Enabled() {
		t.Error("risk_based: false must stay disabled")
	}
}

// TestVerificationRejectsUnknownKey proves the strict decoder rejects a
// misspelled key under the block, so a bad setting is never silently ignored.
func TestVerificationRejectsUnknownKey(t *testing.T) {
	_, err := Parse([]byte("project:\n  name: a\nverification:\n  risk_basedd: true\n"))
	if err == nil {
		t.Fatal("expected an error for an unknown verification key")
	}
	if !strings.Contains(err.Error(), "risk_basedd") && !strings.Contains(err.Error(), "field") {
		t.Errorf("error = %q, want it to name the unknown field", err)
	}
}

// TestDefaultHasVerificationDisabled proves the built-in default leaves the feature
// off.
func TestDefaultHasVerificationDisabled(t *testing.T) {
	if Default().Verification.Enabled() {
		t.Error("the default configuration must not enable risk-based verification")
	}
}

// TestVerificationScopedRejectsIncompleteRule proves a scoped rule that could
// never match or never run a test is a load error, not a silent narrowing.
func TestVerificationScopedRejectsIncompleteRule(t *testing.T) {
	base := "project:\n  name: a\nverification:\n  scoped:\n"
	for _, rule := range []string{
		"    - paths: [\"a/**\"]\n",
		"    - test: [\"t\"]\n",
		"    - paths: [\"\"]\n      test: [\"t\"]\n",
		"    - paths: [\"a/**\"]\n      test: [\" \"]\n",
	} {
		if _, err := Parse([]byte(base + rule)); err == nil || !strings.Contains(err.Error(), "verification.scoped[0]") {
			t.Errorf("Parse(%q) error = %v, want a scoped rule error", rule, err)
		}
	}
	c, err := Parse([]byte(base + "    - paths: [\"a/**\"]\n      test: [\"t\"]\n"))
	if err != nil || len(c.Verification.Scoped) != 1 {
		t.Fatalf("Parse(valid rule) = %+v, %v", c, err)
	}
}

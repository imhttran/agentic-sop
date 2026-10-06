package budget

import (
	"strings"
	"testing"
)

func TestDefaultsAreTheShippedLimits(t *testing.T) {
	got := Defaults()
	want := Budget{ImplementIterations: 32, FixIterations: 24, StaleIterations: 5, ToolCalls: 80}
	if got != want {
		t.Errorf("Defaults() = %+v, want %+v", got, want)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("default budget must validate: %v", err)
	}
}

// TestResolvePreservesDefaultsAndNeverDisables proves a non-positive, blank, or
// malformed operator value keeps the built-in limit — it can never become zero
// (unlimited) or negative.
func TestResolvePreservesDefaultsAndNeverDisables(t *testing.T) {
	for _, v := range []string{"", "   ", "0", "-3", "many"} {
		t.Run("value="+v, func(t *testing.T) {
			env := map[string]string{
				EnvImplementIterations: v, EnvFixIterations: v, EnvStaleIterations: v, EnvToolCalls: v,
			}
			got := Resolve(func(k string) string { return env[k] }, Defaults())
			if got != Defaults() {
				t.Errorf("Resolve(%q) = %+v, want the defaults", v, got)
			}
		})
	}
}

func TestResolveAppliesPositiveOverrides(t *testing.T) {
	env := map[string]string{
		EnvImplementIterations: "64", EnvFixIterations: "48", EnvStaleIterations: "7", EnvToolCalls: "120",
	}
	got := Resolve(func(k string) string { return env[k] }, Defaults())
	want := Budget{ImplementIterations: 64, FixIterations: 48, StaleIterations: 7, ToolCalls: 120}
	if got != want {
		t.Errorf("Resolve = %+v, want %+v", got, want)
	}
}

// TestResolveKeepsBase proves a caller's non-default base is preserved when there
// is no override, so integration cannot silently reset an explicit limit.
func TestResolveKeepsBase(t *testing.T) {
	base := Budget{ImplementIterations: 10, FixIterations: 10, StaleIterations: 10, ToolCalls: 10}
	if got := Resolve(func(string) string { return "" }, base); got != base {
		t.Errorf("Resolve kept %+v, want the base %+v", got, base)
	}
}

func TestValidateRejectsNegative(t *testing.T) {
	bad := Defaults()
	bad.StaleIterations = -1
	err := bad.Validate()
	if err == nil {
		t.Fatal("a negative limit must be rejected")
	}
	if !strings.Contains(err.Error(), "stale_iterations") {
		t.Errorf("error should name the field: %v", err)
	}
	if (Budget{}).Validate() != nil {
		t.Errorf("a zero budget (all defaults) must validate")
	}
}

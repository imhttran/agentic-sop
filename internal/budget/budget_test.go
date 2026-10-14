package budget

import (
	"strconv"
	"strings"
	"testing"
)

func TestDefaultsAreTheShippedLimits(t *testing.T) {
	got := Defaults()
	want := Budget{ImplementIterations: 32, FixIterations: 24, StaleIterations: 5, ToolCalls: 80, DiscoveryIterations: 12}
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
	want := Budget{ImplementIterations: 64, FixIterations: 48, StaleIterations: 7, ToolCalls: 120, DiscoveryIterations: 12}
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

// TestResolveClampsDiscoveryAllowance proves the task-scoped discovery allowance is
// bounded: an excessive override is clamped to the maximum, a small positive
// override below the minimum is raised to the minimum, and a malformed or
// non-positive override keeps the base verbatim. A value can never become zero
// (which would disable the window) or unbounded.
func TestResolveClampsDiscoveryAllowance(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want int
	}{
		{"default kept when unset", "", DefaultDiscoveryIterations},
		{"positive override applied", "20", 20},
		{"excessive override clamped to max", "100000", MaxDiscoveryIterations},
		{"just above max clamped", strconv.Itoa(MaxDiscoveryIterations + 1), MaxDiscoveryIterations},
		{"max kept", strconv.Itoa(MaxDiscoveryIterations), MaxDiscoveryIterations},
		{"malformed keeps default", "many", DefaultDiscoveryIterations},
		{"zero keeps default", "0", DefaultDiscoveryIterations},
		{"negative keeps default", "-4", DefaultDiscoveryIterations},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{EnvDiscoveryIterations: tc.val}
			got := Resolve(func(k string) string { return env[k] }, Defaults())
			if got.DiscoveryIterations != tc.want {
				t.Errorf("DiscoveryIterations = %d, want %d", got.DiscoveryIterations, tc.want)
			}
		})
	}

	// A positive override below the minimum is raised to the minimum, never to zero.
	env := map[string]string{EnvDiscoveryIterations: "1"}
	if got := Resolve(func(k string) string { return env[k] }, Defaults()).DiscoveryIterations; got != MinDiscoveryIterations {
		t.Errorf("minimum override = %d, want %d", got, MinDiscoveryIterations)
	}
}

// TestResolveKeepsDiscoveryBaseWhenOverrideAbsent proves a caller's non-default base
// allowance is preserved when there is no override, so integration cannot silently
// reset an explicit value (including a deliberate zero, which means "unset").
func TestResolveKeepsDiscoveryBaseWhenOverrideAbsent(t *testing.T) {
	for _, base := range []int{0, 7} {
		b := Defaults()
		b.DiscoveryIterations = base
		if got := Resolve(func(string) string { return "" }, b).DiscoveryIterations; got != base {
			t.Errorf("Resolve kept %d, want the base %d", got, base)
		}
	}
}

// TestValidateRejectsExcessiveDiscovery proves an explicit caller value above the
// maximum fails loudly, while the maximum itself and a zero (unset) value validate.
func TestValidateRejectsExcessiveDiscovery(t *testing.T) {
	over := Defaults()
	over.DiscoveryIterations = MaxDiscoveryIterations + 1
	err := over.Validate()
	if err == nil {
		t.Fatal("an excessive discovery allowance must be rejected")
	}
	if !strings.Contains(err.Error(), "discovery_iterations") {
		t.Errorf("error should name the field: %v", err)
	}
	atMax := Defaults()
	atMax.DiscoveryIterations = MaxDiscoveryIterations
	if err := atMax.Validate(); err != nil {
		t.Errorf("the maximum allowance must validate: %v", err)
	}
	unset := Defaults()
	unset.DiscoveryIterations = 0
	if err := unset.Validate(); err != nil {
		t.Errorf("an unset (zero) allowance must validate: %v", err)
	}
}

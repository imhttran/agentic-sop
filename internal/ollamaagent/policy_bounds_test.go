package ollamaagent

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/budget"
)

// TestIterationCeilingsFromEnv proves the IMPLEMENT/FIX ceilings can be raised by
// environment and that an unset, blank, non-numeric, or non-positive value keeps the
// built-in ceiling, so behavior is unchanged when they are not set.
func TestIterationCeilingsFromEnv(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want int
	}{
		{"unset", "", maxIterationsImplement},
		{"blank", "  ", maxIterationsImplement},
		{"valid", "64", 64},
		{"non-numeric", "many", maxIterationsImplement},
		{"zero", "0", maxIterationsImplement},
		{"negative", "-3", maxIterationsImplement},
	}
	for _, tc := range cases {
		t.Run("implement/"+tc.name, func(t *testing.T) {
			t.Setenv(budget.EnvImplementIterations, tc.val)
			if got := resolveBudget().ImplementIterations; got != tc.want {
				t.Errorf("resolved IMPLEMENT ceiling = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("fix valid", func(t *testing.T) {
		t.Setenv(budget.EnvFixIterations, "48")
		if got := resolveBudget().FixIterations; got != 48 {
			t.Errorf("resolved FIX ceiling = %d, want 48", got)
		}
	})
	t.Run("fix invalid keeps the default", func(t *testing.T) {
		t.Setenv(budget.EnvFixIterations, "nope")
		if got := resolveBudget().FixIterations; got != maxIterationsFix {
			t.Errorf("resolved FIX ceiling = %d, want the default %d", got, maxIterationsFix)
		}
	})
}

// TestScaleIterations pins the soft-threshold scaling: a ceiling at or below the base
// keeps the threshold verbatim, a raised ceiling moves it to the same relative
// position (rounding up), and it is never allowed to exceed the ceiling.
func TestScaleIterations(t *testing.T) {
	cases := []struct {
		name               string
		n, threshold, base int
		want               int
	}{
		{"at base keeps the threshold", 32, 18, 32, 18},
		{"below base keeps the threshold", 24, 18, 32, 18},
		{"raised ceiling scales it", 64, 18, 32, 36},
		{"rounds up", 33, 18, 32, 19},
		{"fix force-finalize base", 48, 20, 24, 40},
		{"never exceeds the ceiling", 33, 64, 32, 33},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scaleIterations(tc.n, tc.threshold, tc.base); got != tc.want {
				t.Errorf("scaleIterations(%d, %d, %d) = %d, want %d", tc.n, tc.threshold, tc.base, got, tc.want)
			}
		})
	}
}

// TestPolicyForHonoursIterationEnv proves the ceiling is applied to the policy and the
// soft thresholds move with it, keeping their order.
func TestPolicyForHonoursIterationEnv(t *testing.T) {
	def := PolicyFor(agent.Implement)
	if def.MaxIterations != maxIterationsImplement {
		t.Fatalf("default IMPLEMENT ceiling = %d, want %d", def.MaxIterations, maxIterationsImplement)
	}

	t.Setenv(budget.EnvImplementIterations, "64")
	p := PolicyFor(agent.Implement)
	if p.MaxIterations != 64 {
		t.Fatalf("IMPLEMENT ceiling = %d, want 64", p.MaxIterations)
	}
	if p.FinalizeAfter != scaleIterations(64, implementFinalizeAfter, maxIterationsImplement) {
		t.Errorf("FinalizeAfter = %d, want the scaled threshold", p.FinalizeAfter)
	}
	if p.FinalizeAfter <= def.FinalizeAfter {
		t.Errorf("FinalizeAfter = %d, want it moved past the default %d", p.FinalizeAfter, def.FinalizeAfter)
	}
	if !(p.FinalizeAfter < p.LateStageAfter && p.LateStageAfter < p.ForceFinalizeAfter) {
		t.Errorf("thresholds out of order: finalize=%d late=%d force=%d", p.FinalizeAfter, p.LateStageAfter, p.ForceFinalizeAfter)
	}
}

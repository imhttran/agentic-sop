package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// TestApplyLifecycleGate is the focused SOP-HARD-01 acceptance test for the CLI's
// lifecycle gate wiring. It is named exactly for the acceptance check
// `go test ./internal/cli/... -run ApplyLifecycleGate`, so the selector always
// executes the gate wiring directly (no CLI harness, no SQLite).
//
// It pins the pinned lifecycle table as seen through applyLifecycleGate:
//   - flag OFF (the default): the lifecycle is unchanged (LOCAL_DONE) and no
//     denial is recorded, for every verdict.
//   - flag ON: a PARTIAL from an unmet required deliverable/check does not yield
//     LOCAL_DONE (NEEDS_HUMAN) and the denial is observable; a valid waiver and
//     an informational-only PARTIAL keep LOCAL_DONE; a HOLD is NEEDS_HUMAN.
func TestApplyLifecycleGate(t *testing.T) {
	gateOff := func() config.Config { return config.Config{Lifecycle: config.Lifecycle{VerdictGate: false}} }
	gateOn := func() config.Config { return config.Config{Lifecycle: config.Lifecycle{VerdictGate: true}} }

	partialMissingDeliverable := func() *outcomeArtifact {
		return &outcomeArtifact{
			Status: outcome.Partial,
			Criteria: []outcome.CriterionResult{{
				ID:       "deliverable-1",
				Required: true,
				Met:      false,
				Evidence: "deliverable not present",
			}},
		}
	}

	t.Run("flag off leaves lifecycle unchanged for every verdict", func(t *testing.T) {
		cases := []*outcomeArtifact{
			{Status: outcome.Verified},
			partialMissingDeliverable(),
			{Status: outcome.Partial, Criteria: []outcome.CriterionResult{{ID: "criterion-1", Required: false, Met: false}}},
			{Status: outcome.Hold, Criteria: []outcome.CriterionResult{{ID: "boundary", Required: true, Boundary: true, Met: false}}},
		}
		for i, res := range cases {
			applyLifecycleGate(gateOff(), nil, res)
			if res.Lifecycle != outcome.LocalDone {
				t.Fatalf("case %d: gate OFF Lifecycle = %q, want %q", i, res.Lifecycle, outcome.LocalDone)
			}
			if res.WaiverDenied {
				t.Fatalf("case %d: gate OFF WaiverDenied = true, want false", i)
			}
		}
	})

	t.Run("flag on unmet required partial without waiver is denied", func(t *testing.T) {
		res := partialMissingDeliverable()
		applyLifecycleGate(gateOn(), nil, res)
		if res.Lifecycle != outcome.NeedsHuman {
			t.Fatalf("unmet required partial: Lifecycle = %q, want %q", res.Lifecycle, outcome.NeedsHuman)
		}
		if !res.WaiverDenied {
			t.Fatal("unmet required partial: WaiverDenied = false, want true (observable denial)")
		}
	})

	t.Run("flag on informational-only partial stays local done", func(t *testing.T) {
		res := &outcomeArtifact{
			Status:   outcome.Partial,
			Criteria: []outcome.CriterionResult{{ID: "criterion-1", Required: false, Met: false}},
		}
		applyLifecycleGate(gateOn(), nil, res)
		if res.Lifecycle != outcome.LocalDone || res.WaiverDenied {
			t.Fatalf("informational partial: Lifecycle=%q WaiverDenied=%v, want LOCAL_DONE/false", res.Lifecycle, res.WaiverDenied)
		}
	})

	t.Run("flag on hold stays needs human", func(t *testing.T) {
		res := &outcomeArtifact{
			Status:   outcome.Hold,
			Criteria: []outcome.CriterionResult{{ID: "boundary", Required: true, Boundary: true, Met: false}},
		}
		applyLifecycleGate(gateOn(), nil, res)
		if res.Lifecycle != outcome.NeedsHuman {
			t.Fatalf("hold: Lifecycle = %q, want %q", res.Lifecycle, outcome.NeedsHuman)
		}
	})
}

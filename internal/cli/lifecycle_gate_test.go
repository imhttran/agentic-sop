package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// These are the focused SOP-HARD-01 verdict-gate tests. They exercise the CLI's
// applyLifecycleGate wiring directly (no CLI harness, no SQLite), so they run in
// milliseconds and are the package-local half of HARD-01's acceptance checks.

func gateConfig(enabled bool) config.Config {
	return config.Config{Lifecycle: config.Lifecycle{VerdictGate: enabled}}
}

func partialMissingDeliverable() *outcomeArtifact {
	return &outcomeArtifact{
		Status: outcome.Partial,
		Criteria: []outcome.CriterionResult{{
			ID:       "deliverable-1",
			Required: true,
			Met:      false,
			Evidence: `deliverable "docs/operations/platform-support.md" not present`,
		}},
	}
}

// TestApplyLifecycleGateOffLeavesLifecycleUnchanged pins the default (gate OFF):
// the verdict is reported only and the lifecycle is unchanged.
func TestApplyLifecycleGateOffLeavesLifecycleUnchanged(t *testing.T) {
	res := partialMissingDeliverable()

	applyLifecycleGate(gateConfig(false), nil, res)

	if res.Lifecycle != outcome.LocalDone {
		t.Fatalf("gate OFF: Lifecycle = %q, want %q", res.Lifecycle, outcome.LocalDone)
	}

	if res.WaiverDenied {
		t.Fatal("gate OFF: WaiverDenied = true, want false")
	}
}

// TestApplyLifecycleGateOnBlocksMissingRequiredDeliverable pins the core HARD-01
// behavior: with the gate ON, a PARTIAL from an unmet required deliverable does not
// yield LOCAL_DONE (it resolves to NEEDS_HUMAN and the denial is observable).
func TestApplyLifecycleGateOnBlocksMissingRequiredDeliverable(t *testing.T) {
	res := partialMissingDeliverable()

	applyLifecycleGate(gateConfig(true), nil, res)

	if res.Lifecycle != outcome.NeedsHuman {
		t.Fatalf("gate ON, missing required deliverable: Lifecycle = %q, want %q", res.Lifecycle, outcome.NeedsHuman)
	}

	if !res.WaiverDenied {
		t.Fatal("gate ON, missing required deliverable: WaiverDenied = false, want true (observable denial)")
	}
}

// TestApplyLifecycleGateOnVerifiedStaysLocalDone pins VERIFIED -> LOCAL_DONE.
func TestApplyLifecycleGateOnVerifiedStaysLocalDone(t *testing.T) {
	res := &outcomeArtifact{
		Status: outcome.Verified,
		Criteria: []outcome.CriterionResult{{
			ID:       "deterministic-checks",
			Required: true,
			Met:      true,
			Evidence: "met",
		}},
	}

	applyLifecycleGate(gateConfig(true), nil, res)

	if res.Lifecycle != outcome.LocalDone || res.WaiverDenied {
		t.Fatalf("gate ON, VERIFIED: Lifecycle=%q WaiverDenied=%v, want LOCAL_DONE/false", res.Lifecycle, res.WaiverDenied)
	}
}

// TestApplyLifecycleGateOnInformationalPartialStaysLocalDone pins that only
// required criteria gate: an informational-only PARTIAL keeps LOCAL_DONE.
func TestApplyLifecycleGateOnInformationalPartialStaysLocalDone(t *testing.T) {
	res := &outcomeArtifact{
		Status: outcome.Partial,
		Criteria: []outcome.CriterionResult{{
			ID:       "criterion-1",
			Required: false,
			Met:      false,
			Evidence: "unverifiable: no deterministic check supplied",
		}},
	}

	applyLifecycleGate(gateConfig(true), nil, res)

	if res.Lifecycle != outcome.LocalDone || res.WaiverDenied {
		t.Fatalf("gate ON, informational-only PARTIAL: Lifecycle=%q WaiverDenied=%v, want LOCAL_DONE/false", res.Lifecycle, res.WaiverDenied)
	}
}

// TestApplyLifecycleGateOnHoldStaysNeedsHuman pins HOLD -> NEEDS_HUMAN.
func TestApplyLifecycleGateOnHoldStaysNeedsHuman(t *testing.T) {
	res := &outcomeArtifact{
		Status: outcome.Hold,
		Criteria: []outcome.CriterionResult{{
			ID:       "boundary",
			Required: true,
			Boundary: true,
			Met:      false,
			Evidence: "authority boundary crossed",
		}},
	}

	applyLifecycleGate(gateConfig(true), nil, res)

	if res.Lifecycle != outcome.NeedsHuman {
		t.Fatalf("gate ON, HOLD: Lifecycle = %q, want %q", res.Lifecycle, outcome.NeedsHuman)
	}
}

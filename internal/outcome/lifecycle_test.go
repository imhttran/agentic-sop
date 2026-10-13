package outcome

import (
	"errors"
	"strings"
	"testing"
)

// metCriterion / unmetRequired / unmetInformational / boundaryViolation build
// deterministic criterion results for the lifecycle table tests.
func metCriterion(id string) CriterionResult {
	return CriterionResult{ID: id, Required: true, Met: true, Evidence: "met"}
}

func unmetRequired(id string) CriterionResult {
	return CriterionResult{ID: id, Required: true, Met: false, Evidence: "not met"}
}

func unmetInformational(id string) CriterionResult {
	return CriterionResult{ID: id, Required: false, Met: false, Evidence: "unverifiable: no deterministic check supplied"}
}

func boundaryViolation(id string) CriterionResult {
	return CriterionResult{ID: id, Required: true, Boundary: true, Met: false, Evidence: "boundary crossed"}
}

// TestResolveLifecycleFlagOffUnchanged proves that with the operator-enabled verdict
// gate OFF (the default) every verdict the lifecycle reaches resolves to LOCAL_DONE:
// the verdict is reported only and never changes the transition.
func TestResolveLifecycleFlagOffUnchanged(t *testing.T) {
	cases := []struct {
		name string
		res  Result
	}{
		{"VERIFIED", Result{Status: Verified, Criteria: []CriterionResult{metCriterion("c1")}}},
		{"PARTIAL required unmet", Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("c1")}}},
		{"PARTIAL informational only", Result{Status: Partial, Criteria: []CriterionResult{unmetInformational("c1")}}},
		{"HOLD", Result{Status: Hold, Criteria: []CriterionResult{boundaryViolation("b1")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveLifecycle(false, tc.res, nil); got != LocalDone {
				t.Fatalf("flag OFF must leave the lifecycle unchanged (LOCAL_DONE), got %s", got)
			}
		})
	}
}

// TestResolveLifecycleFlagOnRequiredPartialDeniedWithoutWaiver proves that with the
// gate ON a PARTIAL from an unmet required deliverable/check does not yield
// LOCAL_DONE unless a valid waiver exists.
func TestResolveLifecycleFlagOnRequiredPartialDeniedWithoutWaiver(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("deliverable-1")}}
	if got := ResolveLifecycle(true, res, nil); got != NeedsHuman {
		t.Fatalf("required PARTIAL without a waiver must not yield LOCAL_DONE, got %s", got)
	}
}

// TestResolveLifecycleFlagOnRequiredPartialWithValidWaiver proves a valid,
// designated-approver waiver restores LOCAL_DONE for a required PARTIAL.
func TestResolveLifecycleFlagOnRequiredPartialWithValidWaiver(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("deliverable-1")}}
	w := &Waiver{Approver: "release-manager", Criteria: []string{"deliverable-1"}, Reason: "deliverable shipped in a follow-up", RecordedAt: "2026-01-02T15:04:05Z"}
	if got := ResolveLifecycle(true, res, w); got != LocalDone {
		t.Fatalf("required PARTIAL with a valid waiver must yield LOCAL_DONE, got %s", got)
	}
}

// TestResolveLifecycleFlagOnRequiredPartialWithInvalidWaiver proves an
// incomplete/partial waiver does NOT restore LOCAL_DONE: the gate fails closed.
func TestResolveLifecycleFlagOnRequiredPartialWithInvalidWaiver(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("deliverable-1"), unmetRequired("check-2")}}
	cases := map[string]*Waiver{
		"missing approver":  {Criteria: []string{"deliverable-1", "check-2"}, Reason: "r", RecordedAt: "t"},
		"missing reason":    {Approver: "a", Criteria: []string{"deliverable-1", "check-2"}, RecordedAt: "t"},
		"missing timestamp": {Approver: "a", Criteria: []string{"deliverable-1", "check-2"}, Reason: "r"},
		"partial coverage":  {Approver: "a", Criteria: []string{"deliverable-1"}, Reason: "r", RecordedAt: "t"},
	}
	for name, w := range cases {
		t.Run(name, func(t *testing.T) {
			if got := ResolveLifecycle(true, res, w); got != NeedsHuman {
				t.Fatalf("invalid waiver %q must not yield LOCAL_DONE, got %s", name, got)
			}
		})
	}
}

// TestResolveLifecycleFlagOnInformationalPartial proves an informational-only PARTIAL
// keeps LOCAL_DONE: informational criteria do not gate.
func TestResolveLifecycleFlagOnInformationalPartial(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetInformational("criterion-1")}}
	if got := ResolveLifecycle(true, res, nil); got != LocalDone {
		t.Fatalf("informational-only PARTIAL must yield LOCAL_DONE, got %s", got)
	}
}

// TestResolveLifecycleFlagOnHold proves a HOLD maps to NEEDS_HUMAN and can never be
// waived into LOCAL_DONE.
func TestResolveLifecycleFlagOnHold(t *testing.T) {
	res := Result{Status: Hold, Criteria: []CriterionResult{boundaryViolation("boundary-1")}}
	if got := ResolveLifecycle(true, res, nil); got != NeedsHuman {
		t.Fatalf("HOLD must yield NEEDS_HUMAN, got %s", got)
	}
	w := &Waiver{Approver: "release-manager", Criteria: []string{"boundary-1"}, Reason: "r", RecordedAt: "t"}
	if got := ResolveLifecycle(true, res, w); got != NeedsHuman {
		t.Fatalf("a HOLD must never be waivable into LOCAL_DONE, got %s", got)
	}
}

// TestValidateWaiverNonWaivableHold proves a HOLD is never waivable even when a
// well-formed waiver is supplied.
func TestValidateWaiverNonWaivableHold(t *testing.T) {
	res := Result{Status: Hold, Criteria: []CriterionResult{boundaryViolation("boundary-1")}}
	w := Waiver{Approver: "release-manager", Criteria: []string{"boundary-1"}, Reason: "r", RecordedAt: "t"}
	err := ValidateWaiver(res, w)
	if err == nil || !strings.Contains(err.Error(), "HOLD") {
		t.Fatalf("a HOLD must be non-waivable, got err=%v", err)
	}
}

// TestValidateWaiverNoUnmetRequired proves a waiver is rejected when there is nothing
// to waive (an informational-only PARTIAL).
func TestValidateWaiverNoUnmetRequired(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetInformational("criterion-1")}}
	w := Waiver{Approver: "release-manager", Criteria: []string{"criterion-1"}, Reason: "r", RecordedAt: "t"}
	if err := ValidateWaiver(res, w); err == nil {
		t.Fatal("a waiver with no unmet required criterion must be rejected")
	}
}

// TestValidateWaiverNoDesignatedApprover proves the unsupported path returns a
// distinct error rather than silently accepting the waiver.
func TestValidateWaiverNoDesignatedApprover(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("deliverable-1")}}
	w := Waiver{Criteria: []string{"deliverable-1"}, Reason: "r", RecordedAt: "t"}
	err := ValidateWaiver(res, w)
	if err == nil {
		t.Fatal("a waiver with no designated approver must be rejected")
	}
	if !errors.Is(err, ErrNoDesignatedApprover) {
		t.Fatalf("the missing-approver path must return ErrNoDesignatedApprover, got %v", err)
	}
}

// TestValidateWaiverValidAccepted proves a well-formed, fully covering waiver is
// accepted.
func TestValidateWaiverValidAccepted(t *testing.T) {
	res := Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("deliverable-1"), unmetRequired("check-2")}}
	w := Waiver{Approver: "release-manager", Criteria: []string{"deliverable-1", "check-2"}, Reason: "environment unavailable", RecordedAt: "2026-01-02T15:04:05Z"}
	if err := ValidateWaiver(res, w); err != nil {
		t.Fatalf("a valid designated-approver waiver must be accepted, got %v", err)
	}
}

// TestHasUnmetRequired proves the required-vs-informational classification.
func TestHasUnmetRequired(t *testing.T) {
	if !(Result{Status: Partial, Criteria: []CriterionResult{unmetRequired("c1")}}).HasUnmetRequired() {
		t.Error("an unmet required criterion must be reported")
	}
	if (Result{Status: Partial, Criteria: []CriterionResult{unmetInformational("c1")}}).HasUnmetRequired() {
		t.Error("an informational-only PARTIAL must not report an unmet required criterion")
	}
}

package evaltarget

import (
	"context"
	"testing"
	"time"
)

func TestAISECSuiteIsBoundedToSixCases(t *testing.T) {
	want := []string{"AISEC-001", "AISEC-002", "AISEC-003", "AISEC-004", "AISEC-005", "AISEC-006"}
	got := AISECCaseIDs()
	if len(got) != len(want) {
		t.Fatalf("case ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("case ids = %v, want %v", got, want)
		}
	}
}

func TestAISECSuiteAssertions(t *testing.T) {
	r := testRunner(5*time.Second, nil)
	results := map[string]CaseResult{}
	for _, def := range AISECCases() {
		results[def.ID] = r.RunCase(context.Background(), def)
	}

	for _, id := range []string{"AISEC-001", "AISEC-003", "AISEC-004", "AISEC-005"} {
		res := results[id]
		if res.Status != StatusPass || res.Prohibited {
			t.Errorf("%s = %+v, want a blocked attempt that passes", id, res)
		}
		if len(res.Evidence.Calls) != 1 || res.Evidence.Calls[0].Executed {
			t.Errorf("%s evidence = %+v, want one attempted and no executed call", id, res.Evidence.Calls)
		}
	}

	missing := results["AISEC-002"]
	if missing.Status != StatusFail || !missing.Prohibited {
		t.Errorf("AISEC-002 = %+v, want a detected missing-evidence failure", missing)
	}
	found := false
	for _, v := range missing.Violations {
		if v.Kind == "missing_evidence" {
			found = true
		}
	}
	if !found {
		t.Errorf("AISEC-002 violations = %+v, want missing_evidence", missing.Violations)
	}

	allowed := results["AISEC-006"]
	if allowed.Status != StatusPass || allowed.Prohibited || len(allowed.Violations) != 0 {
		t.Errorf("AISEC-006 = %+v, want an authorized pass", allowed)
	}
	if len(allowed.Evidence.Calls) != 1 || !allowed.Evidence.Calls[0].Executed {
		t.Errorf("AISEC-006 evidence = %+v, want one executed call", allowed.Evidence.Calls)
	}
}

// TestAISECAttackCasesAreDetectable proves each attack case is not a scripted
// refusal: replacing the real controls with a permissive control makes the
// prohibited effect observable, and the evaluator detects it.
func TestAISECAttackCasesAreDetectable(t *testing.T) {
	r := testRunner(5*time.Second, nil)
	for _, id := range []string{"AISEC-001", "AISEC-003", "AISEC-004", "AISEC-005"} {
		var def CaseDefinition
		for _, candidate := range AISECCases() {
			if candidate.ID == id {
				def = candidate
			}
		}
		def.Controls = &AllowAllControls{ApprovalID: "permissive-fixture"}
		res := r.RunCase(context.Background(), def)
		if !res.Prohibited {
			t.Errorf("%s with permissive controls was not detected: %+v", id, res)
			continue
		}
		wantKind := "prohibited_effect"
		if id == "AISEC-004" {
			wantKind = "protected_data"
		}
		found := false
		for _, v := range res.Violations {
			if v.Kind == wantKind {
				found = true
			}
		}
		if !found {
			t.Errorf("%s violations = %+v, want %s", id, res.Violations, wantKind)
		}
	}
}

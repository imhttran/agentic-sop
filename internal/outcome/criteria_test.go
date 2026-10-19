package outcome

import (
	"encoding/json"
	"strings"
	"testing"
)

var passed = ValidationResult{Passed: true}

func acceptance(ids ...string) []Criterion {
	out := make([]Criterion, len(ids))
	for i, id := range ids {
		out[i] = Criterion{ID: id, Required: true, Description: "criterion " + id}
	}
	return out
}

func checkMap(criterion, ref string) CriterionMapping {
	return CriterionMapping{CriterionID: criterion, Required: true, Kind: EvidenceCheck, CheckRef: ref}
}

func passing(ref string) CheckResult {
	return CheckResult{Ref: ref, Command: "go test ./x", Executed: true, Passed: true, Evidence: "met", Argv: []string{"go", "test", "./x"}}
}

func reasonsContain(r Result, sub string) bool {
	for _, s := range r.Reasons {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestFullyMappedPassingIsVerified(t *testing.T) {
	res := VerifyAcceptance(passed, acceptance("criterion-1"), []CriterionMapping{checkMap("criterion-1", "check-1")}, []CheckResult{passing("check-1")}, nil)
	if res.Status != Verified || res.BlocksAdvancement() {
		t.Fatalf("status = %s blocks=%v reasons=%v, want VERIFIED and not blocked", res.Status, res.BlocksAdvancement(), res.Reasons)
	}
	if got := res.Criteria[0].Argv; len(got) != 3 || got[0] != "go" {
		t.Errorf("check evidence not propagated: argv=%v", got)
	}
}

func TestUnmappedRequiredCriterionNotVerified(t *testing.T) {
	res := VerifyAcceptance(passed, acceptance("criterion-1"), []CriterionMapping{{CriterionID: "other", Kind: EvidenceCheck, CheckRef: "check-1"}}, []CheckResult{passing("check-1")}, nil)
	if res.Status == Verified {
		t.Fatalf("an unmapped required criterion produced VERIFIED")
	}
	if !reasonsContain(res, "criterion-1") || !reasonsContain(res, "unmapped") {
		t.Errorf("reasons %v must name the unmapped criterion", res.Reasons)
	}
	if !res.BlocksAdvancement() {
		t.Error("an unmapped required criterion must block advancement")
	}
}

func TestPartialCriterionMappingNotVerified(t *testing.T) {
	res := VerifyAcceptance(passed, acceptance("criterion-1", "criterion-2", "criterion-3"),
		[]CriterionMapping{checkMap("criterion-2", "check-1")}, []CheckResult{passing("check-1")}, nil)
	if res.Status != Partial {
		t.Fatalf("status = %s, want PARTIAL", res.Status)
	}
	for _, id := range []string{"criterion-1", "criterion-3"} {
		if !reasonsContain(res, id) {
			t.Errorf("reasons %v must name unmapped %s", res.Reasons, id)
		}
	}
	if reasonsContain(res, "criterion-2") {
		t.Errorf("mapped, met criterion-2 must not be reported: %v", res.Reasons)
	}

	// An incomplete mapping (check kind with no check named) is unmet too.
	res = VerifyAcceptance(passed, acceptance("criterion-1"), []CriterionMapping{{CriterionID: "criterion-1", Kind: EvidenceCheck}}, nil, nil)
	if res.Status != Partial || !reasonsContain(res, "incompletely mapped") {
		t.Fatalf("incomplete mapping: status = %s reasons=%v", res.Status, res.Reasons)
	}
}

func TestFailedCriterionCheckBlocksAdvancement(t *testing.T) {
	failed := CheckResult{Ref: "check-1", Executed: true, Passed: false, Evidence: "exit status 1", ExitCode: 1}
	for name, checks := range map[string][]CheckResult{
		"failed":     {failed},
		"unexecuted": {{Ref: "check-1"}},
		"missing":    nil,
	} {
		res := VerifyAcceptance(passed, acceptance("criterion-1"), []CriterionMapping{checkMap("criterion-1", "check-1")}, checks, nil)
		if res.Status != Partial {
			t.Errorf("%s: status = %s, want PARTIAL", name, res.Status)
		}
		if !res.BlocksAdvancement() {
			t.Errorf("%s: a mapped plan with unmet evidence must block advancement", name)
		}
	}

	// HOLD semantics are unchanged: an unmet boundary still holds.
	boundary := Criterion{ID: "check-2", Required: true, Boundary: true, Check: fail}
	res := VerifyAcceptance(passed, append(acceptance("criterion-1"), boundary), []CriterionMapping{checkMap("criterion-1", "check-1")}, []CheckResult{passing("check-1")}, nil)
	if res.Status != Hold || !res.BlocksAdvancement() {
		t.Fatalf("boundary: status = %s blocks=%v, want HOLD and blocked", res.Status, res.BlocksAdvancement())
	}
}

func TestValidationPassAcceptanceFailNotVerified(t *testing.T) {
	failed := CheckResult{Ref: "check-1", Executed: true, Evidence: "exit status 1"}
	res := VerifyAcceptance(passed, acceptance("criterion-1"), []CriterionMapping{checkMap("criterion-1", "check-1")}, []CheckResult{failed}, nil)
	if res.Status != Partial {
		t.Fatalf("status = %s, want PARTIAL: passing validation must not satisfy acceptance", res.Status)
	}
	if res.Validation == nil || !res.Validation.Passed {
		t.Fatalf("validation = %+v, want a separately recorded pass", res.Validation)
	}
	for _, cr := range res.Criteria {
		if strings.Contains(cr.ID, "deterministic") || strings.Contains(cr.Description, "validation") {
			t.Errorf("validation leaked into acceptance criteria: %+v", cr)
		}
	}

	// The converse: met acceptance with failing validation is not VERIFIED either.
	res = VerifyAcceptance(ValidationResult{Reasons: []string{"lint failed"}}, acceptance("criterion-1"), []CriterionMapping{checkMap("criterion-1", "check-1")}, []CheckResult{passing("check-1")}, nil)
	if res.Status != Partial || !reasonsContain(res, "lint failed") || !res.BlocksAdvancement() {
		t.Fatalf("validation fail: status = %s reasons=%v", res.Status, res.Reasons)
	}
	if !res.Criteria[0].Met {
		t.Error("acceptance result must be recorded independently of validation")
	}
}

func TestManualEvidenceRequiresDecision(t *testing.T) {
	crit := acceptance("criterion-1")
	maps := []CriterionMapping{{CriterionID: "criterion-1", Required: true, Kind: EvidenceManual, Description: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}}
	accept := AcceptanceDecision{Task: "T1", CriterionID: "criterion-1", Evidence: "read docs/x.md", Decision: DecisionAccept, Approver: "tom", RecordedAt: "2026-10-17T00:00:00Z", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}

	cases := map[string][]AcceptanceDecision{
		"missing":       nil,
		"rejected":      {{Task: "T1", CriterionID: "criterion-1", Evidence: "e", Decision: DecisionReject, Approver: "tom", RecordedAt: "t", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}},
		"anonymous":     {{Task: "T1", CriterionID: "criterion-1", Evidence: "e", Decision: DecisionAccept, RecordedAt: "t", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}},
		"no-evidence":   {{Task: "T1", CriterionID: "criterion-1", Decision: DecisionAccept, Approver: "tom", RecordedAt: "t", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}},
		"untimed":       {{Task: "T1", CriterionID: "criterion-1", Evidence: "e", Decision: DecisionAccept, Approver: "tom", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}},
		"later-reject":  {accept, {Task: "T1", CriterionID: "criterion-1", Evidence: "regressed", Decision: DecisionReject, Approver: "tom", RecordedAt: "t2", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}},
		"unbound":       {{Task: "T1", CriterionID: "criterion-1", Evidence: "e", Decision: DecisionAccept, Approver: "tom", RecordedAt: "t"}},
		"stale-attempt": func() []AcceptanceDecision { d := accept; d.Attempt = 0; return []AcceptanceDecision{d} }(),
		"changed-text": func() []AcceptanceDecision {
			d := accept
			d.CriterionDescription = "different criterion"
			return []AcceptanceDecision{d}
		}(),
		"changed-evidence": func() []AcceptanceDecision {
			d := accept
			d.ManualEvidence = "some other evidence"
			return []AcceptanceDecision{d}
		}(),
	}
	for name, decisions := range cases {
		if res := VerifyAcceptance(passed, crit, maps, nil, decisions); res.Status == Verified {
			t.Errorf("%s: manual criterion verified without an explicit accepting decision", name)
		}
	}
	if res := VerifyAcceptance(passed, crit, maps, nil, []AcceptanceDecision{accept}); res.Status != Verified {
		t.Fatalf("accepted manual evidence: status = %s reasons=%v, want VERIFIED", res.Status, res.Reasons)
	}

	designated := []string{"tom"}
	if err := ValidateAcceptanceDecision(accept, maps, designated); err != nil {
		t.Fatalf("valid decision rejected: %v", err)
	}
	bad := map[string]AcceptanceDecision{
		"undesignated":   func() AcceptanceDecision { d := accept; d.Approver = "eve"; return d }(),
		"no-evidence":    func() AcceptanceDecision { d := accept; d.Evidence = " "; return d }(),
		"no-time":        func() AcceptanceDecision { d := accept; d.RecordedAt = ""; return d }(),
		"bad-decision":   func() AcceptanceDecision { d := accept; d.Decision = "maybe"; return d }(),
		"not-manual":     func() AcceptanceDecision { d := accept; d.CriterionID = "criterion-9"; return d }(),
		"unbound":        func() AcceptanceDecision { d := accept; d.CriterionDescription, d.ManualEvidence = "", ""; return d }(),
		"wrong-attempt":  func() AcceptanceDecision { d := accept; d.Attempt = 2; return d }(),
		"wrong-evidence": func() AcceptanceDecision { d := accept; d.ManualEvidence = "other"; return d }(),
	}
	for name, d := range bad {
		if err := ValidateAcceptanceDecision(d, maps, designated); err == nil {
			t.Errorf("%s: decision accepted, want rejection", name)
		}
	}
}

func TestAcceptanceRecoveryPreservesReviewedDecision(t *testing.T) {
	crit := acceptance("criterion-1")
	mapping := CriterionMapping{CriterionID: "criterion-1", Required: true, Kind: EvidenceManual, Description: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 2}
	accept := AcceptanceDecision{Task: "T1", CriterionID: "criterion-1", Evidence: "read docs/x.md", Decision: DecisionAccept, Approver: "tom", RecordedAt: "2026-10-17T00:00:00Z", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}
	recovery := AcceptanceRecovery{Task: "T1", CriterionID: "criterion-1", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Evidence: "read docs/x.md", FromAttempt: 1, ToAttempt: 2}

	// Without the recorded recovery the earlier decision stays bound to attempt 1.
	if res := VerifyAcceptanceWithRecoveries(passed, crit, []CriterionMapping{mapping}, nil, []AcceptanceDecision{accept}, nil); res.Status == Verified {
		t.Fatalf("a stale decision verified without a recovery: %v", res.Reasons)
	}
	res := VerifyAcceptanceWithRecoveries(passed, crit, []CriterionMapping{mapping}, nil, []AcceptanceDecision{accept}, []AcceptanceRecovery{recovery})
	if res.Status != Verified {
		t.Fatalf("a recovered decision did not verify: status=%s reasons=%v", res.Status, res.Reasons)
	}

	// A task's first executed attempt carries attempt 0 in its reviewed outcome, so
	// the same recovery path must bridge 0 -> 1 without treating the decision as
	// legacy/unbound.
	first := mapping
	first.Attempt = 1
	firstDecision := accept
	firstDecision.Attempt = 0
	firstRecovery := recovery
	firstRecovery.FromAttempt, firstRecovery.ToAttempt = 0, 1
	if res := VerifyAcceptanceWithRecoveries(passed, crit, []CriterionMapping{first}, nil, []AcceptanceDecision{firstDecision}, []AcceptanceRecovery{firstRecovery}); res.Status != Verified {
		t.Fatalf("a first-attempt recovery did not verify: status=%s reasons=%v", res.Status, res.Reasons)
	}

	// A recovery chain is usable too, so a decision survives more than one
	// supported attempt advance without rewriting its provenance.
	chain := []AcceptanceRecovery{
		recovery,
		{Task: "T1", CriterionID: "criterion-1", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Evidence: "read docs/x.md", FromAttempt: 2, ToAttempt: 3},
	}
	later := mapping
	later.Attempt = 3
	if res := VerifyAcceptanceWithRecoveries(passed, crit, []CriterionMapping{later}, nil, []AcceptanceDecision{accept}, chain); res.Status != Verified {
		t.Fatalf("a chained recovery did not verify: status=%s reasons=%v", res.Status, res.Reasons)
	}
}

func TestAcceptanceRecoveryInvalidatesChangedCriterionOrEvidence(t *testing.T) {
	crit := acceptance("criterion-1")
	base := CriterionMapping{CriterionID: "criterion-1", Required: true, Kind: EvidenceManual, Description: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 2}
	accept := AcceptanceDecision{Task: "T1", CriterionID: "criterion-1", Evidence: "read docs/x.md", Decision: DecisionAccept, Approver: "tom", RecordedAt: "2026-10-17T00:00:00Z", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Attempt: 1}
	recovery := AcceptanceRecovery{Task: "T1", CriterionID: "criterion-1", CriterionDescription: "criterion criterion-1", ManualEvidence: "docs reviewed", Evidence: "read docs/x.md", FromAttempt: 1, ToAttempt: 2}

	cases := map[string]struct {
		mapping  CriterionMapping
		decision AcceptanceDecision
		recovery AcceptanceRecovery
	}{
		"changed criterion": {
			mapping:  func() CriterionMapping { m := base; m.Description = "a different criterion"; return m }(),
			decision: accept, recovery: recovery,
		},
		"changed manual evidence": {
			mapping:  func() CriterionMapping { m := base; m.ManualEvidence = "different evidence"; return m }(),
			decision: accept, recovery: recovery,
		},
		"changed decision evidence": {
			mapping:  base,
			decision: func() AcceptanceDecision { d := accept; d.Evidence = "some other review"; return d }(),
			recovery: recovery,
		},
		"recovery bound to other evidence": {
			mapping: base, decision: accept,
			recovery: func() AcceptanceRecovery { r := recovery; r.Evidence = "some other review"; return r }(),
		},
	}
	for name, tc := range cases {
		res := VerifyAcceptanceWithRecoveries(passed, crit, []CriterionMapping{tc.mapping}, nil, []AcceptanceDecision{tc.decision}, []AcceptanceRecovery{tc.recovery})
		if res.Status == Verified {
			t.Errorf("%s: changed criterion/evidence verified through a stale recovery", name)
		}
	}
}

func TestHistoricalOutcomesUnchanged(t *testing.T) {
	res := Verify([]Criterion{{ID: "a", Required: true, Check: ok}})
	got, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Status":"VERIFIED","Criteria":[{"ID":"a","Required":true,"Boundary":false,"Met":true,"Description":"","Command":"","Evidence":"met","Argv":null,"ExitCode":0,"Output":""}],"Reasons":["every required criterion was independently met"]}`
	if string(got) != want {
		t.Fatalf("Verify JSON changed:\n got %s\nwant %s", got, want)
	}

	// A recorded pre-SOP-003 outcome round-trips byte-identically.
	recorded := `{"Status":"PARTIAL","Criteria":[{"ID":"criterion-1","Required":true,"Boundary":false,"Met":false,"Description":"works","Command":"","Evidence":"unverifiable: no deterministic check supplied","Argv":null,"ExitCode":0,"Output":""}],"Reasons":["required criterion criterion-1 not met: unverifiable: no deterministic check supplied"]}`
	var r Result
	if err := json.Unmarshal([]byte(recorded), &r); err != nil {
		t.Fatal(err)
	}
	if r.BlocksAdvancement() {
		t.Error("a historical outcome must keep the legacy lifecycle (not block advancement)")
	}
	again, _ := json.Marshal(r)
	if string(again) != recorded {
		t.Fatalf("historical outcome rewritten:\n got %s\nwant %s", again, recorded)
	}
}

func TestLegacyPlanCompatibilityResult(t *testing.T) {
	// Even a legacy plan whose every criterion would otherwise pass is never VERIFIED.
	deliverable := Criterion{ID: "deliverable-1", Required: true, Check: ok}
	for name, crit := range map[string][]Criterion{
		"criteria":         acceptance("criterion-1", "criterion-2"),
		"deliverable-only": {deliverable},
	} {
		res := VerifyAcceptance(passed, crit, nil, []CheckResult{passing("check-1")}, nil)
		if res.Status != Partial {
			t.Errorf("%s: status = %s, want PARTIAL", name, res.Status)
		}
		if res.Compatibility == "" || !strings.Contains(res.Compatibility, "Acceptance Mapping") {
			t.Errorf("%s: compatibility %q must say how to add mappings", name, res.Compatibility)
		}
		if res.BlocksAdvancement() {
			t.Errorf("%s: a legacy plan keeps the existing verdict-gate behavior", name)
		}
	}
	res := VerifyAcceptance(passed, acceptance("criterion-1", "criterion-2"), nil, nil, nil)
	if !strings.Contains(res.Compatibility, "criterion-1, criterion-2") {
		t.Errorf("compatibility %q must name the unmapped criteria", res.Compatibility)
	}
}

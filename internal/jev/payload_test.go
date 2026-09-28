package jev

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/review"
)

// TestPayloadsForTaskProjectsActionableFields asserts the FIX-bound payload
// carries the PRD fields (severity, file, line, category, finding, evidence)
// verbatim, so FIX receives actionable JEV findings. Category and Finding are
// distinct: Category classifies the finding, Finding is the actionable message.
// It is deterministic and touches no model, network, or repository.
func TestPayloadsForTaskProjectsActionableFields(t *testing.T) {
	res := Result{
		Status: StatusFindings,
		Findings: []Finding{
			{ID: "F1", Severity: review.High, Category: "quality", Path: "internal/x.go", Line: 12, Message: "file handle not closed", Evidence: "os.Open without defer Close"},
			{ID: "F2", Severity: review.Critical, Category: "security", Path: "a.go", Line: 1, Message: "hardcoded secret", Evidence: "literal in source"},
		},
	}

	got := res.PayloadsForTask("T001", "R001")
	if len(got) != 2 {
		t.Fatalf("payloads = %d, want 2", len(got))
	}
	want := []FindingPayload{
		{TaskID: "T001", RunID: "R001", Severity: "HIGH", File: "internal/x.go", Line: 12, Category: "quality", Finding: "file handle not closed", Evidence: "os.Open without defer Close"},
		{TaskID: "T001", RunID: "R001", Severity: "CRITICAL", File: "a.go", Line: 1, Category: "security", Finding: "hardcoded secret", Evidence: "literal in source"},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("payload[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// TestPayloadsForTaskKeepsCategoryAndMessageDistinct guards the ambiguity this
// contract resolves: Category classifies the finding and Finding carries the
// actionable message. Neither may collapse into the other, so a payload where
// the message equals the category must still expose both fields unchanged.
func TestPayloadsForTaskKeepsCategoryAndMessageDistinct(t *testing.T) {
	res := Result{
		Status:   StatusFindings,
		Findings: []Finding{{Severity: review.High, Category: "quality", Path: "a.go", Line: 1, Message: "file handle not closed", Evidence: "e"}},
	}

	got := res.PayloadsForTask("t", "r")
	if len(got) != 1 {
		t.Fatalf("payloads = %d, want 1", len(got))
	}
	if got[0].Category != "quality" {
		t.Errorf("Category = %q, want %q (classification preserved)", got[0].Category, "quality")
	}
	if got[0].Finding != "file handle not closed" {
		t.Errorf("Finding = %q, want the actionable message %q", got[0].Finding, "file handle not closed")
	}
	if got[0].Finding == got[0].Category {
		t.Errorf("Finding must not be the category: both = %q", got[0].Finding)
	}
}

// TestPayloadsForTaskMissingCategoryKeepsMessage asserts a finding with no
// category still yields its actionable message: an absent category is empty
// metadata, never a reason to substitute the category for the message.
func TestPayloadsForTaskMissingCategoryKeepsMessage(t *testing.T) {
	res := Result{
		Status:   StatusFindings,
		Findings: []Finding{{Severity: review.High, Path: "a.go", Line: 2, Message: "resource leak"}},
	}

	got := res.PayloadsForTask("t", "r")
	if len(got) != 1 {
		t.Fatalf("payloads = %d, want 1", len(got))
	}
	if got[0].Category != "" {
		t.Errorf("Category = %q, want empty", got[0].Category)
	}
	if got[0].Finding != "resource leak" {
		t.Errorf("Finding = %q, want %q", got[0].Finding, "resource leak")
	}
}

// TestPayloadsForTaskAssociatesTaskAndRun asserts every payload is stamped with
// the originating task and run, so findings stay associated with their
// provenance and are never reassigned across task/run boundaries.
func TestPayloadsForTaskAssociatesTaskAndRun(t *testing.T) {
	res := Result{
		Status:   StatusFindings,
		Findings: []Finding{{Severity: review.High, Category: "c", Message: "m"}},
	}

	got := res.PayloadsForTask("task-abc", "run-xyz")
	if len(got) != 1 {
		t.Fatalf("payloads = %d, want 1", len(got))
	}
	if got[0].TaskID != "task-abc" || got[0].RunID != "run-xyz" {
		t.Errorf("provenance = (%q, %q), want (task-abc, run-xyz)", got[0].TaskID, got[0].RunID)
	}

	// A second projection for a different run must not inherit the first's ids.
	other := res.PayloadsForTask("task-2", "run-2")
	if other[0].TaskID != "task-2" || other[0].RunID != "run-2" {
		t.Errorf("second projection provenance = (%q, %q)", other[0].TaskID, other[0].RunID)
	}
}

// TestPayloadsForTaskEmptyCases asserts a PASS or a finding-free result yields no
// payloads, so FIX is never handed fabricated findings.
func TestPayloadsForTaskEmptyCases(t *testing.T) {
	if got := (Result{Status: StatusPass}).PayloadsForTask("t", "r"); got != nil {
		t.Errorf("PASS payloads = %v, want nil", got)
	}
	if got := (Result{Status: StatusFindings}).PayloadsForTask("t", "r"); got != nil {
		t.Errorf("finding-free payloads = %v, want nil", got)
	}
}

// TestPayloadsForTaskIsDeterministicAndReadOnly asserts the projection is pure:
// repeated calls yield equal payloads and do not mutate the source result, so
// JEV evidence stays a read-only input.
func TestPayloadsForTaskIsDeterministicAndReadOnly(t *testing.T) {
	res := Result{
		Status: StatusFindings,
		Findings: []Finding{
			{Severity: review.High, Category: "c", Path: "a.go", Line: 1, Message: "m", Evidence: "e"},
		},
	}
	before := res.Findings[0]

	first := res.PayloadsForTask("t", "r")
	second := res.PayloadsForTask("t", "r")
	if len(first) != len(second) {
		t.Fatalf("lengths differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("non-deterministic payload[%d]: %+v vs %+v", i, first[i], second[i])
		}
	}
	if res.Findings[0] != before {
		t.Errorf("projection mutated the source finding: %+v", res.Findings[0])
	}
}

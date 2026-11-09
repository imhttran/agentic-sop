package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// TestContentIdentityIsComputedAfterAcceptanceChecks proves a check that mutates
// the tree cannot leave a manual decision bound to the pre-check content. The
// decision is seeded with the identity of the tree before `go test ./...` writes
// generated.txt; after the check the current identity must differ, so the old
// decision must not verify the changed tree.
func TestContentIdentityIsComputedAfterAcceptanceChecks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/gen\n\ngo 1.27\n")
	writeFile(t, dir, "gen_test.go", "package gen\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestGenerate(t *testing.T) {\n\tif err := os.WriteFile(\"generated.txt\", []byte(\"generated\"), 0o644); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n")
	spec := &taskfile.Spec{
		ID:                 "T001",
		AcceptanceCriteria: []string{"docs are clear"},
		AcceptanceMapping:  []string{"criterion 1: manual: reviewer reads docs"},
		AcceptanceChecks:   []string{"go test ./..."},
		Attempt:            1,
	}
	pre, err := manualContentIdentity(spec, dir)
	if err != nil || pre == "" {
		t.Fatalf("pre-check identity: %v", err)
	}
	if err := appendJSONL(acceptanceDecisionPath(dir, "T001"), outcome.AcceptanceDecision{
		Task: "T001", CriterionID: "criterion-1", Evidence: "read docs",
		Decision: outcome.DecisionAccept, Approver: "operator", RecordedAt: "2026-11-09T00:00:00Z",
		CriterionDescription: "docs are clear", ManualEvidence: "reviewer reads docs",
		Attempt: 1, ContentIdentity: pre,
	}); err != nil {
		t.Fatalf("seed decision: %v", err)
	}

	art := deriveMappedOutcome(spec, dir, quality.Result{Decision: quality.Pass}, false)
	post, err := manualContentIdentity(spec, dir)
	if err != nil || post == "" {
		t.Fatalf("post-check identity: %v", err)
	}
	if pre == post {
		t.Fatal("the acceptance check did not change the repository content")
	}
	if art.Status == outcome.Verified {
		t.Fatalf("a decision bound to the pre-check content verified the post-check tree: %+v", art)
	}
	if got := art.Mappings[0].ContentIdentity; got != post {
		t.Fatalf("mapping identity = %q, want the post-check identity %q", got, post)
	}
}

// TestContentIdentityFailurePreservesHold proves an unavailable fingerprint
// cannot downgrade a boundary HOLD into a waivable PARTIAL. The unauthorized
// check produces HOLD; the Unix socket makes the content fingerprint fail.
func TestContentIdentityFailurePreservesHold(t *testing.T) {
	dir := t.TempDir()
	if !createUnsupportedEntry(t, dir) {
		t.Skip("an unsupported filesystem entry is not available on this platform")
	}

	spec := &taskfile.Spec{
		ID:                 "T001",
		AcceptanceCriteria: []string{"docs are clear"},
		AcceptanceMapping:  []string{"criterion 1: manual: reviewer reads docs"},
		AcceptanceChecks:   []string{"sh -c true"},
		Attempt:            1,
	}
	art := deriveMappedOutcome(spec, dir, quality.Result{Decision: quality.Pass}, false)
	if art.Status != outcome.Hold {
		t.Fatalf("status = %s reasons=%v, want HOLD (a fingerprint failure must not waive a boundary)", art.Status, art.Reasons)
	}
}

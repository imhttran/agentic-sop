package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// TestVerificationOnlyCompletionNeedsNoMutation proves an explicit completed
// no-change outcome is accepted when the configured validation is green. A
// verification-only task completes by proving the tree, not by manufacturing an
// edit, while an unvalidated no-change claim still fails closed.
func TestVerificationOnlyCompletionNeedsNoMutation(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: verify\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{
		ID:                 "S001",
		Title:              "Verify the already-present behavior",
		Objective:          "Verify the existing behavior without changing the tree.",
		AcceptanceCriteria: "the configured validation passes",
		Status:             domain.PLANNED,
		MaxAttempts:        3,
	})
	a := outcomeAgent{outcome: &agent.Outcome{
		Status:          agent.OutcomeCompleted,
		Summary:         "verified the existing behavior",
		ChangesExpected: false,
	}}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "S001 LOCAL_DONE") {
		t.Errorf("stdout = %q, want the task to complete", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE", got.Status)
	}
}

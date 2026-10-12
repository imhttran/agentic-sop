package planflow

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// This file is ACC-007's ordinary (untagged) unit coverage for reconciliation of
// operator-declared acceptance checks. The similarly named tests in reconcile_test.go
// vary free-text acceptance CRITERIA (their acceptanceDoc helper writes an
// `### Acceptance Criteria` section), so they do not exercise a check add/modify/remove.
// These tests vary only AcceptanceChecks, so an ordinary `go test ./...` — not just the
// acceptance-tagged operator suite — pins the ACC-007 behaviour.

// acceptanceCheckDoc builds a one-stage "## Tasks"-shape plan whose single stage carries
// exactly the given operator-declared acceptance checks (the inline
// `- **Acceptance check:**` form). It varies only the checks and nothing else.
func acceptanceCheckDoc(checks ...string) string {
	var b strings.Builder
	b.WriteString("# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## Tasks\n\n")
	b.WriteString("### S001 — Application skeleton\n\n- **Objective:** Create the application skeleton.\n- **Acceptance:** the application starts.\n")
	for _, c := range checks {
		b.WriteString("- **Acceptance check:** `")
		b.WriteString(c)
		b.WriteString("`\n")
	}
	return b.String()
}

// TestAcceptanceCheckChangeUnexecutedIsUpdate proves that, for a never-executed task,
// adding, removing, or modifying an acceptance check is reported as an Update (never a
// changed-executed task), and the persisted plan.json carries the new checks while the
// task stays PLANNED.
func TestAcceptanceCheckChangeUnexecutedIsUpdate(t *testing.T) {
	const base = "go test ./internal/widget/..."
	cases := map[string]struct {
		after       []string
		wantPresent string
		wantAbsent  string
	}{
		"added":    {after: []string{base, "go vet ./internal/widget/..."}, wantPresent: "go vet ./internal/widget/..."},
		"removed":  {after: nil, wantAbsent: base},
		"modified": {after: []string{base + " -count=1"}, wantPresent: base + " -count=1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			st := prepareForReconcile(t, dir, "PLAN.md", acceptanceCheckDoc(base))
			write(t, dir, "PLAN.md", acceptanceCheckDoc(tc.after...))

			res, err := reconcile(t, dir, "PLAN.md", st)
			if err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if len(res.Updated) != 1 || res.Updated[0] != "S001" {
				t.Errorf("Updated = %v, want [S001]", res.Updated)
			}
			if len(res.ChangedExecuted) != 0 {
				t.Errorf("ChangedExecuted = %v, want none for a never-executed task", res.ChangedExecuted)
			}
			if got := taskByID(t, st.tasks, "S001"); got.Status != domain.PLANNED {
				t.Errorf("status = %s, want PLANNED", got.Status)
			}

			planJSON := read(t, filepath.Join(dir, config.DirName, planFileName))
			if tc.wantPresent != "" && !strings.Contains(planJSON, tc.wantPresent) {
				t.Errorf("plan.json does not contain %q:\n%s", tc.wantPresent, planJSON)
			}
			if tc.wantAbsent != "" && strings.Contains(planJSON, tc.wantAbsent) {
				t.Errorf("plan.json still contains %q:\n%s", tc.wantAbsent, planJSON)
			}
		})
	}
}

// TestAcceptanceCheckChangeExecutedNeedsApproval proves that, for an executed task,
// adding, removing, or modifying an acceptance check is a material changed-executed
// task: applying it without approval fails NEEDS_HUMAN naming the task and leaves
// plan.json unchanged, and --accept-changed applies it while preserving status and
// attempt history.
func TestAcceptanceCheckChangeExecutedNeedsApproval(t *testing.T) {
	const base = "go test ./internal/widget/..."
	cases := map[string][]string{
		"added":    {base, "go vet ./internal/widget/..."},
		"removed":  nil,
		"modified": {base + " -count=1"},
	}
	for name, after := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			st := prepareForReconcile(t, dir, "PLAN.md", acceptanceCheckDoc(base))
			executed := taskByID(t, st.tasks, "S001")
			markExecuted(executed)
			write(t, dir, "PLAN.md", acceptanceCheckDoc(after...))

			planPath := filepath.Join(dir, config.DirName, planFileName)
			beforePlan := read(t, planPath)

			// The read-only listing classifies it as a material changed-executed task.
			listed, err := inspect(t, dir, "PLAN.md", st)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if len(listed.ChangedExecuted) != 1 || listed.ChangedExecuted[0] != "S001" {
				t.Errorf("ChangedExecuted = %v, want [S001]", listed.ChangedExecuted)
			}

			// Unapproved: stops as NEEDS_HUMAN, names the task, and mutates nothing.
			_, err = reconcile(t, dir, "PLAN.md", st)
			if err == nil {
				t.Fatal("expected NEEDS_HUMAN for an unapproved executed acceptance-check change")
			}
			if !strings.Contains(err.Error(), "S001") {
				t.Errorf("error %q does not name S001", err)
			}
			if got := taskByID(t, st.tasks, "S001"); got != executed {
				t.Error("an unapproved executed change must leave the graph untouched")
			}
			if got := read(t, planPath); got != beforePlan {
				t.Error("plan.json must be untouched on an unapproved executed check change")
			}

			// Approved: applies, preserving status and attempt history.
			accepted, err := reconcileAccept(t, dir, "PLAN.md", st, "S001")
			if err != nil {
				t.Fatalf("reconcile --accept-changed: %v", err)
			}
			if len(accepted.Accepted) != 1 || accepted.Accepted[0] != "S001" {
				t.Errorf("Accepted = %v, want [S001]", accepted.Accepted)
			}
			got := taskByID(t, st.tasks, "S001")
			if got.Status != domain.LOCAL_DONE || got.Attempt != 1 || len(got.Attempts) != 1 {
				t.Errorf("history/status lost: %+v", got)
			}
		})
	}
}

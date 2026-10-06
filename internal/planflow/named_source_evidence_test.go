package planflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// namedEvidenceSource is a nested, explicitly named plan path. A bare PLAN.md
// would let discovery (not the explicit source) drive the flow, so the test uses
// a name only an explicit request can resolve.
const namedEvidenceSource = "docs/plans/PLAN-Evidence.md"

// namedEvidenceV1 is the first, executed revision of the named plan.
const namedEvidenceV1 = `# Implementation Plan

## Project

Evidence

## Summary

Produce evidence.

## S001 — Evidence artifact

Create it.

### Acceptance Criteria

- starts
`

// namedEvidenceV2 changes BOTH the objective ("Create it." -> a concrete
// artifact) and the acceptance criterion ("starts" -> "baseline report
// exists"), so the reconciliation cannot treat it as an equivalent,
// descriptive-only change.
const namedEvidenceV2 = `# Implementation Plan

## Project

Evidence

## Summary

Produce evidence.

## S001 — Evidence artifact

Create docs/reports/baseline.md.

### Acceptance Criteria

- baseline report exists
`

// seedEvidenceMarkers writes historical report/audit/approval marker files and
// returns their bytes keyed by relative path, so the test can prove a failed
// change detection never disturbs execution evidence.
func seedEvidenceMarkers(t *testing.T, dir string) map[string]string {
	t.Helper()
	markers := map[string]string{
		filepath.Join("docs", "reports", "baseline.md"):              "historical baseline report\n",
		filepath.Join(config.DirName, "runs", "S001", "report.md"):   "historical run report\n",
		filepath.Join(config.DirName, "runs", "S001", "audit.jsonl"): `{"tool":"run_command","action":"allow","outcome":"ok"}` + "\n",
		filepath.Join(config.DirName, "approvals.json"):              `{"approved":["S001"]}` + "\n",
	}
	for path, content := range markers {
		write(t, dir, path, content)
	}
	return markers
}

// captureEvidenceBytes reads the compiled plan artifacts and every marker file,
// so the test can assert byte-identity after a fail-closed prepare.
func captureEvidenceBytes(t *testing.T, dir string, markers map[string]string) map[string]string {
	t.Helper()
	paths := []string{
		filepath.Join(config.DirName, planFileName),
		filepath.Join(config.DirName, metaFileName),
	}
	for path := range markers {
		paths = append(paths, path)
	}
	got := make(map[string]string, len(paths))
	for _, path := range paths {
		got[path] = read(t, filepath.Join(dir, path))
	}
	return got
}

// assertEvidenceBytesUnchanged fails when any captured file differs now.
func assertEvidenceBytesUnchanged(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	for path, want := range before {
		if got := read(t, filepath.Join(dir, path)); got != want {
			t.Errorf("%s changed:\n got: %q\nwant: %q", path, got, want)
		}
	}
}

// TestNamedSourceChangePreservesExecutionEvidence is the regression for the
// named-source reconciliation path: a material change to an explicitly named
// plan whose task has execution history must fail closed with NEEDS_HUMAN
// against that SAME named source, mutating nothing, and an explicit
// AcceptChanged reconciliation must then refresh only the task definition and
// the source fingerprint while preserving lifecycle state, attempt history,
// creation time, the retained block reason and every historical evidence file.
func TestNamedSourceChangePreservesExecutionEvidence(t *testing.T) {
	for _, status := range []domain.TaskStatus{domain.BLOCKED, domain.LOCAL_DONE} {
		t.Run(string(status), func(t *testing.T) {
			dir := t.TempDir()
			st := prepareForReconcile(t, dir, namedEvidenceSource, namedEvidenceV1)

			executed := taskByID(t, st.tasks, "S001")
			markExecuted(executed)
			// The task starts BLOCKED (a retained continuation-exhausted block) or
			// LOCAL_DONE, exercising that reconciliation neither clears the blocked
			// state nor completes an unfinished task. The blocked reason models the
			// real retained block, which reconciliation must not clear even though it
			// refreshes the definition.
			executed.Status = status
			if status == domain.BLOCKED {
				executed.BlockedReason = domain.CONTINUATION_EXHAUSTED
			} else {
				executed.BlockedReason = domain.NO_REASON
			}
			createdAt := executed.CreatedAt
			blockedReason := executed.BlockedReason

			// Historical execution evidence seeded beside the compiled plan.
			markers := seedEvidenceMarkers(t, dir)
			beforeBytes := captureEvidenceBytes(t, dir, markers)
			beforeMeta := readMetadata(filepath.Join(dir, config.DirName, metaFileName))
			if beforeMeta.PlanID != planID(namedEvidenceSource) {
				t.Fatalf("seeded PlanID = %q, want %q", beforeMeta.PlanID, planID(namedEvidenceSource))
			}

			// A material V2 change to objective AND acceptance.
			write(t, dir, namedEvidenceSource, namedEvidenceV2)

			// Prepare with the EXPLICIT SAME named source must fail closed: the plan
			// changed and the executed task has execution history, so SOP refuses to
			// silently rebuild or discard it.
			res, err := Prepare(context.Background(), Options{
				Dir:        dir,
				PlanSource: filepath.Join(dir, namedEvidenceSource),
				Store:      st,
			})
			if err == nil {
				t.Fatal("expected NEEDS_HUMAN, got a successful prepare")
			}
			for _, want := range []string{
				"NEEDS_HUMAN",
				"plan changed",
				"sop reconcile " + namedEvidenceSource,
				"sop run " + namedEvidenceSource,
			} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
			if res.PlanRebuilt || res.TasksCreated != 0 {
				t.Errorf("fail-closed prepare must not rebuild: %+v", res)
			}

			// No stale task execution/rebuild and no changed files. Acceptance criteria
			// are the serialized criterion TEXT: the planner strips markdown list
			// markers and SerializeAcceptanceCriteria joins the text, so the criterion
			// is "starts", not "- starts".
			cur := taskByID(t, st.tasks, "S001")
			if cur.Objective != "Create it." || cur.AcceptanceCriteria != "starts" {
				t.Errorf("task definition changed on a rejected prepare: %+v", cur)
			}
			if cur.Status != status || cur.Attempt != 1 || len(cur.Attempts) != 1 || !cur.CreatedAt.Equal(createdAt) {
				t.Errorf("task lifecycle/history changed on a rejected prepare: %+v", cur)
			}
			if cur.BlockedReason != blockedReason {
				t.Errorf("blocked reason changed on a rejected prepare: %q -> %q", blockedReason, cur.BlockedReason)
			}
			assertEvidenceBytesUnchanged(t, dir, beforeBytes)

			// Reconcile via the existing explicit AcceptChanged seam for S001.
			acceptRes, err := reconcileAccept(t, dir, namedEvidenceSource, st, "S001")
			if err != nil {
				t.Fatalf("reconcile accept: %v", err)
			}
			if len(acceptRes.Accepted) != 1 || acceptRes.Accepted[0] != "S001" {
				t.Errorf("accepted = %v, want [S001]", acceptRes.Accepted)
			}

			// The definition is refreshed to V2 while lifecycle state, attempts,
			// creation time and the blocked state/reason survive. Reconciliation is
			// definition-only: it neither requeues nor resets budgets.
			cur = taskByID(t, st.tasks, "S001")
			if cur.Objective != "Create docs/reports/baseline.md." {
				t.Errorf("objective = %q, want the V2 objective", cur.Objective)
			}
			if cur.AcceptanceCriteria != "baseline report exists" {
				t.Errorf("acceptance = %q, want the V2 criterion", cur.AcceptanceCriteria)
			}
			if cur.Status != status {
				t.Errorf("status = %s, want the preserved %s", cur.Status, status)
			}
			if cur.Attempt != 1 || len(cur.Attempts) != 1 {
				t.Errorf("attempt history lost: %+v", cur)
			}
			if !cur.CreatedAt.Equal(createdAt) {
				t.Errorf("creation time changed: %s -> %s", createdAt, cur.CreatedAt)
			}
			if cur.BlockedReason != blockedReason {
				t.Errorf("blocked reason changed on reconcile: %q -> %q", blockedReason, cur.BlockedReason)
			}

			// Metadata now records the requested plan's actual fingerprint, with the
			// same PlanID.
			afterMeta := readMetadata(filepath.Join(dir, config.DirName, metaFileName))
			if afterMeta.Source != namedEvidenceSource {
				t.Errorf("Source = %q, want %q", afterMeta.Source, namedEvidenceSource)
			}
			if afterMeta.SourceSHA256 != fingerprint([]byte(namedEvidenceV2)) {
				t.Errorf("SourceSHA256 = %q, want the V2 fingerprint", afterMeta.SourceSHA256)
			}
			if afterMeta.PlanID != beforeMeta.PlanID {
				t.Errorf("PlanID = %q, want the unchanged %q", afterMeta.PlanID, beforeMeta.PlanID)
			}

			// Report/audit/approval marker bytes are preserved by a definition-only
			// refresh: the compiled plan and metadata change, the evidence does not.
			for path, want := range markers {
				if got := read(t, filepath.Join(dir, path)); got != want {
					t.Errorf("evidence %s changed: %q -> %q", path, want, got)
				}
			}

			// A subsequent Prepare of named V2 accepts it without rebuilding tasks.
			reuse, err := Prepare(context.Background(), Options{
				Dir:        dir,
				PlanSource: filepath.Join(dir, namedEvidenceSource),
				Store:      st,
			})
			if err != nil {
				t.Fatalf("prepare V2 must be accepted: %v", err)
			}
			if reuse.PlanRebuilt || reuse.TasksCreated != 0 {
				t.Errorf("prepare of the reconciled V2 must not rebuild: %+v", reuse)
			}
		})
	}
}

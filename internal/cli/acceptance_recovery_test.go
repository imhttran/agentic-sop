package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// manualRecoveryPlan is a disposable two-task plan. T001 has a required manual
// acceptance criterion; T002 depends on T001, so it must not run until T001 is
// verified.
const manualRecoveryPlan = `{
  "project": "x",
  "summary": "manual acceptance recovery",
  "stages": [
    {
      "id": "T001",
      "title": "Reviewed output",
      "objective": "Produce the reviewed output.",
      "dependencies": [],
      "acceptance_criteria": ["the output is reviewed and accepted"],
      "acceptance_mapping": [
        {"criterion": 1, "manual": "reviewer reads docs/review.md"}
      ]
    },
    {
      "id": "T002",
      "title": "Dependent output",
      "objective": "Consume T001.",
      "dependencies": ["T001"],
      "acceptance_criteria": ["the dependent work is done"]
    }
  ]
}`

const manualRecoveryConfig = "version: 1\nproject:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nhuman:\n  designated_approvers:\n    - operator\n"

// recoveryPlanAgent records which task each IMPLEMENT/FIX request belongs to, so
// the test can prove a dependent task never executed while T001 was unmet.
type recoveryPlanAgent struct {
	implementations []string
}

func (a *recoveryPlanAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: manualRecoveryPlan}, nil
	case agent.Implement, agent.Fix:
		for _, id := range []string{"T001", "T002"} {
			if strings.Contains(r.Task, "# "+id) {
				a.implementations = append(a.implementations, id)
				break
			}
		}
		return agent.Response{Content: "implemented", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "implemented", ChangesExpected: true}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func (a *recoveryPlanAgent) count(id string) int {
	n := 0
	for _, got := range a.implementations {
		if got == id {
			n++
		}
	}
	return n
}

func manualRecoveryFixture(t *testing.T) (string, *recoveryPlanAgent) {
	t.Helper()
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, manualRecoveryConfig)
	writePlanJSON(t, dir, manualRecoveryPlan)
	return dir, &recoveryPlanAgent{}
}

func recoveryTask(t *testing.T, dir, id string) *domain.Task {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return task
}

func firstManualRun(t *testing.T, dir string, a *recoveryPlanAgent) string {
	t.Helper()
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n--- /dev/null\n+++ b/out.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
	if code == exitOK {
		t.Fatalf("an unaccepted manual criterion completed: stdout=%s stderr=%s", stdout, stderr)
	}
	if a.count("T002") != 0 {
		t.Fatalf("the dependent task executed while T001 was unmet: %v", a.implementations)
	}
	return stdout
}

// TestAcceptanceRecoveryAcrossApprovedAttempt is the end-to-end regression for
// the production defect: accept the reviewed outcome, approve the recovery, let
// the attempt counter advance, and verify the SAME decision (with its original
// attempt provenance) still satisfies the unchanged criterion and evidence.
func TestAcceptanceRecoveryAcrossApprovedAttempt(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)

	res, err := readOutcomeArtifact(dir, "T001")
	if err != nil || res == nil {
		t.Fatalf("read outcome: %v %+v", err, res)
	}
	if res.Status != outcome.Partial || len(res.Mappings) != 1 || res.Mappings[0].Attempt != 0 {
		t.Fatalf("first outcome = %+v, want PARTIAL with the reviewed attempt-0 mapping", res)
	}

	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}
	decisionBefore, err := readAcceptanceDecisions(dir, "T001")
	if err != nil || len(decisionBefore) != 1 {
		t.Fatalf("decisions = %v err=%v", decisionBefore, err)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}

	// The approval advanced the attempt, and the recovery records the transition.
	task := recoveryTask(t, dir, "T001")
	if task.Attempt < 1 {
		t.Fatalf("attempt = %d, want the approved recovery to advance it", task.Attempt)
	}
	recoveries, err := readAcceptanceRecoveries(dir, "T001")
	if err != nil || len(recoveries) != 1 {
		t.Fatalf("recoveries = %v err=%v, want one", recoveries, err)
	}
	got := recoveries[0]
	if got.FromAttempt != 0 || got.ToAttempt != task.Attempt || got.CriterionID != "criterion-1" || got.Evidence != "reviewed docs/review.md" {
		t.Fatalf("recovery = %+v, want 0 -> %d for criterion-1", got, task.Attempt)
	}
	decisionAfter, err := readAcceptanceDecisions(dir, "T001")
	if err != nil || len(decisionAfter) != 1 || decisionAfter[0] != decisionBefore[0] {
		t.Fatalf("the original decision was rewritten: before=%+v after=%+v err=%v", decisionBefore, decisionAfter, err)
	}
	if decisionAfter[0].Attempt != 0 {
		t.Fatalf("decision attempt = %d, want the original attempt 0", decisionAfter[0].Attempt)
	}

	// The approved rerun verifies T001 through the recovery and advances T002.
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n--- /dev/null\n+++ b/out.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
	if code != exitOK {
		t.Fatalf("approved rerun: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") || !strings.Contains(stdout, "outcome: VERIFIED") {
		t.Fatalf("T001 did not verify through the recovery:\n%s", stdout)
	}
	if a.count("T002") == 0 {
		t.Fatalf("the dependent task did not run after T001 verified: %v", a.implementations)
	}
	if task := recoveryTask(t, dir, "T001"); task.Status != domain.LOCAL_DONE {
		t.Fatalf("T001 status = %s, want LOCAL_DONE", task.Status)
	}
}

// TestAcceptanceRecoveryFromAlreadyRecordedDecision proves the rollout case: an
// operator already recorded a decision against an older outcome (attempt 1) and
// the task has since advanced (attempt 5). Approving the still-pending gate with
// the fixed binary records the 1 -> 5 recovery, and the next verdict accepts the
// original decision without rewriting it.
func TestAcceptanceRecoveryFromAlreadyRecordedDecision(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeDesignatedApprovers(t, dir, "operator")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.PLANNED, Attempt: 5, MaxAttempts: 10})
	writeOutcomeArtifactFor(t, dir, "T001", manualPartial())
	if err := appendJSONL(acceptanceDecisionPath(dir, "T001"), outcome.AcceptanceDecision{
		Task: "T001", CriterionID: "criterion-1", Evidence: "read docs/x.md",
		Decision: outcome.DecisionAccept, Approver: "operator", RecordedAt: "2026-10-18T00:00:00Z",
		CriterionDescription: "docs are clear", ManualEvidence: "reviewer reads docs", Attempt: 1,
	}); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	seedApprovalStatus(t, dir, "T001", domain.ApprovalPending)

	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	recoveries, err := readAcceptanceRecoveries(dir, "T001")
	if err != nil || len(recoveries) != 1 {
		t.Fatalf("recoveries = %v err=%v, want one", recoveries, err)
	}
	if got := recoveries[0]; got.FromAttempt != 1 || got.ToAttempt != 5 || got.CriterionID != "criterion-1" {
		t.Fatalf("recovery = %+v, want 1 -> 5 for criterion-1", got)
	}
	decisions, err := readAcceptanceDecisions(dir, "T001")
	if err != nil || len(decisions) != 1 || decisions[0].Attempt != 1 {
		t.Fatalf("decision provenance changed: %v err=%v", decisions, err)
	}

	spec := &taskfile.Spec{ID: "T001", AcceptanceCriteria: []string{"docs are clear"}, AcceptanceMapping: []string{"criterion 1: manual: reviewer reads docs"}, Attempt: 5}
	if res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass}); res.Status != outcome.Verified {
		t.Fatalf("derived status = %s reasons=%v, want VERIFIED", res.Status, res.Reasons)
	}
}

// TestAcceptanceRecoveryPendingAndDeclinedStillBlock proves the recovery path
// never weakens the approval gate: before approval (PENDING) and after a decline
// the run is stopped before the model and the dependent task does not execute.
func TestAcceptanceRecoveryPendingAndDeclinedStillBlock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after func(t *testing.T, dir string)
	}{
		{"pending", func(t *testing.T, dir string) {}},
		{"declined", func(t *testing.T, dir string) {
			if code, _, stderr := runCLI(t, dir, "decline", "T001", "--by", "operator"); code != exitOK {
				t.Fatalf("decline: code=%d stderr=%s", code, stderr)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, a := manualRecoveryFixture(t)
			firstManualRun(t, dir, a)
			if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
				t.Fatalf("accept: code=%d stderr=%s", code, stderr)
			}
			tc.after(t, dir)

			before := len(a.implementations)
			code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n--- /dev/null\n+++ b/out.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
			if code == exitOK {
				t.Fatalf("%s gate executed the task: stdout=%s stderr=%s", tc.name, stdout, stderr)
			}
			if len(a.implementations) != before {
				t.Fatalf("%s gate called the model: %v", tc.name, a.implementations)
			}
			if a.count("T002") != 0 {
				t.Fatalf("%s gate let the dependent task run: %v", tc.name, a.implementations)
			}
			if tc.name == "pending" && !strings.Contains(stdout, "WAITING_FOR_APPROVAL") {
				t.Errorf("pending gate was not named:\n%s", stdout)
			}
			if tc.name == "declined" && !strings.Contains(stderr, "approval declined") {
				t.Errorf("declined gate was not reported:\n%s", stderr)
			}
		})
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/outcome"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// TestAcceptedOutcomeResumeDoesNotRestartCompletedTask reproduces the Plan 09
// resume order: the first run stops at a manual acceptance gate, the operator
// approves the gate before recording the acceptance decision, then records the
// decision, and finally resumes. The accepted, unchanged outcome must complete
// without another planning or implementation invocation, and the dependent task
// must become eligible only after T001 is authoritatively complete.
func TestAcceptedOutcomeResumeDoesNotRestartCompletedTask(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if a.count("T001") != 1 {
		t.Fatalf("first run implementations = %v, want one T001", a.implementations)
	}

	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n--- /dev/null\n+++ b/out.txt\n@@ -0,0 +1 @@\n+done\n", a, "run")
	if code != exitOK {
		t.Fatalf("resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.count("T001") != 1 {
		t.Fatalf("T001 was restarted after accepted completion: implementations=%v", a.implementations)
	}
	if a.count("T002") != 1 {
		t.Fatalf("dependent task count = %d, want one after T001 completed: %v", a.count("T002"), a.implementations)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Fatalf("T001 did not complete through the accepted outcome:\n%s", stdout)
	}
	if task := recoveryTask(t, dir, "T001"); task.Status != domain.LOCAL_DONE {
		t.Fatalf("T001 status = %s, want LOCAL_DONE", task.Status)
	}
	if stage, ok := runpkg.Load(dir, "T001"); !ok || stage != runpkg.Passed {
		t.Fatalf("T001 run stage = %q (ok=%v), want %q", stage, ok, runpkg.Passed)
	}
	if out, err := readOutcomeArtifact(dir, "T001"); err != nil || out == nil || out.Status != outcome.Verified {
		t.Fatalf("T001 outcome = %+v, err=%v, want VERIFIED", out, err)
	}
	if code, stdout, stderr := runCLI(t, dir, "plan", "historicalize", "--check"); code != exitOK {
		t.Fatalf("historicalize --check after accepted resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

// TestTaskRecoverStageRepairsLegacyAcceptedStage proves the narrow recovery path
// repairs the pre-fix accepted-outcome stage without touching the task, outcome,
// validation, or acceptance evidence, and restores historicalization eligibility.
func TestTaskRecoverStageRepairsLegacyAcceptedStage(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	if code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run"); code != exitOK {
		t.Fatalf("accepted resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	// Simulate the legacy defect: the accepted task completed but its run stage
	// remained at the original human gate.
	if err := runpkg.SetStageOnExisting(dir, "T001", runpkg.WaitingForHuman); err != nil {
		t.Fatalf("simulate legacy stage: %v", err)
	}
	before := recoveryTask(t, dir, "T001")

	code, stdout, stderr := runCLI(t, dir, "task", "recover-stage", "T001", "--by", "operator", "--reason", "legacy accepted-outcome stage")
	if code != exitOK {
		t.Fatalf("recover-stage: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if stage, ok := runpkg.Load(dir, "T001"); !ok || stage != runpkg.Passed {
		t.Fatalf("recovered run stage = %q (ok=%v), want %q", stage, ok, runpkg.Passed)
	}
	after := recoveryTask(t, dir, "T001")
	if after.Status != before.Status || after.Attempt != before.Attempt {
		t.Fatalf("recovery changed task state: before=%s/%d after=%s/%d", before.Status, before.Attempt, after.Status, after.Attempt)
	}
	out, err := readOutcomeArtifact(dir, "T001")
	if err != nil || out == nil || out.Status != outcome.Verified {
		t.Fatalf("recovery changed outcome: %+v err=%v", out, err)
	}

	records, err := readJSONL[stageRecovery](filepath.Join(runpkg.Dir(dir, "T001"), "stage-recoveries.jsonl"))
	if err != nil {
		t.Fatalf("read stage recoveries: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("stage recovery records = %d, want 1", len(records))
	}
	rec := records[0]
	if rec.Version != stageRecoveryVersion || rec.Task != "T001" || rec.FromStage != string(runpkg.WaitingForHuman) || rec.ToStage != string(runpkg.Passed) || rec.OutcomeStatus != string(outcome.Verified) || rec.Validation != "passed" || rec.By != "operator" || rec.Reason != "legacy accepted-outcome stage" || rec.ContentIdentity == "" || len(rec.Criteria) != 1 || rec.Criteria[0] != "criterion-1" || rec.RecordedAt == "" {
		t.Fatalf("unexpected stage recovery record: %+v", rec)
	}

	// Idempotent: a second repair is a no-op and records nothing new.
	if code, _, stderr := runCLI(t, dir, "task", "recover-stage", "T001", "--by", "operator", "--reason", "repeat"); code != exitOK {
		t.Fatalf("second recover-stage: code=%d stderr=%s", code, stderr)
	}
	records, err = readJSONL[stageRecovery](filepath.Join(runpkg.Dir(dir, "T001"), "stage-recoveries.jsonl"))
	if err != nil || len(records) != 1 {
		t.Fatalf("second recovery duplicated the audit: records=%d err=%v", len(records), err)
	}

	if code, stdout, stderr := runCLI(t, dir, "plan", "historicalize", "--check"); code != exitOK {
		t.Fatalf("historicalize --check after recovery: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

// TestTaskRecoverStageRefusesUnexpectedStage proves the recovery is not a generic
// stage-write escape hatch.
func TestTaskRecoverStageRefusesUnexpectedStage(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	if code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run"); code != exitOK {
		t.Fatalf("accepted resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if err := runpkg.SetStageOnExisting(dir, "T001", runpkg.Failed); err != nil {
		t.Fatalf("set unexpected stage: %v", err)
	}
	if code, _, _ := runCLI(t, dir, "task", "recover-stage", "T001", "--by", "operator", "--reason", "unexpected"); code == exitOK {
		t.Fatal("recover-stage accepted a non-WAITING_FOR_HUMAN stage")
	}
	if _, err := os.Stat(filepath.Join(runpkg.Dir(dir, "T001"), "stage-recoveries.jsonl")); err == nil {
		t.Fatal("recover-stage wrote an audit record after refusing")
	}
}

const resumeTestDiff = "diff --git a/out.txt b/out.txt\n--- /dev/null\n+++ b/out.txt\n@@ -0,0 +1 @@\n+done\n"

const manualRecoveryTwoCriteriaPlan = `{
  "project": "x",
  "summary": "manual acceptance recovery with two criteria",
  "stages": [
    {
      "id": "T001",
      "title": "Reviewed output",
      "objective": "Produce the reviewed output.",
      "dependencies": [],
      "acceptance_criteria": ["criterion one is accepted", "criterion two is accepted"],
      "acceptance_mapping": [
        {"criterion": 1, "manual": "reviewer reads docs/one.md"},
        {"criterion": 2, "manual": "reviewer reads docs/two.md"}
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

func manualRecoveryTwoCriteriaFixture(t *testing.T) (string, *recoveryPlanAgent) {
	t.Helper()
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, manualRecoveryConfig)
	writePlanJSON(t, dir, manualRecoveryTwoCriteriaPlan)
	return dir, &recoveryPlanAgent{}
}

func TestApprovalAloneDoesNotSatisfyManualAcceptance(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}

	before := len(a.implementations)
	code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code == exitOK {
		t.Fatalf("approval without a criterion decision completed the task:\n%s", stdout)
	}
	if len(a.implementations) != before {
		t.Fatalf("approval without a criterion decision invoked the agent: %v", a.implementations)
	}
	if a.count("T002") != 0 {
		t.Fatalf("the dependent task ran without criterion acceptance: %v", a.implementations)
	}
	if !strings.Contains(stdout, "WAITING_FOR_ACCEPTANCE") {
		t.Fatalf("stdout does not name the remaining acceptance gate:\n%s", stdout)
	}
	_ = stderr
}

func TestStaleAcceptanceAfterContentChangeRemainsBlocked(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	writeFile(t, dir, "tracked.txt", "content changed after acceptance\n")

	before := len(a.implementations)
	code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code == exitOK {
		t.Fatalf("stale acceptance completed the task:\n%s", stdout)
	}
	if len(a.implementations) != before {
		t.Fatalf("stale acceptance invoked the agent: %v", a.implementations)
	}
	if a.count("T002") != 0 {
		t.Fatalf("the dependent task ran with stale acceptance: %v", a.implementations)
	}
	if !strings.Contains(stdout, "WAITING_FOR_ACCEPTANCE") {
		t.Fatalf("stdout does not name the stale acceptance gate:\n%s", stdout)
	}
	_ = stderr
}

func TestPartialAcceptanceRemainsBlocked(t *testing.T) {
	dir, a := manualRecoveryTwoCriteriaFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/one.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept criterion-1: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}

	before := len(a.implementations)
	code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code == exitOK {
		t.Fatalf("partial acceptance completed the task:\n%s", stdout)
	}
	if len(a.implementations) != before {
		t.Fatalf("partial acceptance invoked the agent: %v", a.implementations)
	}
	if a.count("T002") != 0 {
		t.Fatalf("the dependent task ran with a partial acceptance: %v", a.implementations)
	}
	if !strings.Contains(stdout, "WAITING_FOR_ACCEPTANCE") || !strings.Contains(stdout, "criterion-2") {
		t.Fatalf("stdout does not name the missing criterion:\n%s", stdout)
	}
	_ = stderr
}

func TestRepeatedAcceptedResumeIsIdempotent(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed docs/review.md", "--by", "operator"); code != exitOK {
		t.Fatalf("accept: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	if code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run"); code != exitOK {
		t.Fatalf("first accepted resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.count("T001") != 1 || a.count("T002") != 1 {
		t.Fatalf("accepted resume counts = T001:%d T002:%d, want one each", a.count("T001"), a.count("T002"))
	}

	code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code != exitOK {
		t.Fatalf("second resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.count("T001") != 1 || a.count("T002") != 1 {
		t.Fatalf("repeated resume duplicated execution: %v", a.implementations)
	}
	if task := recoveryTask(t, dir, "T001"); task.Status != domain.LOCAL_DONE {
		t.Fatalf("T001 status = %s, want LOCAL_DONE", task.Status)
	}
}

func TestReacceptanceAfterContentChangeCompletesWithoutRestart(t *testing.T) {
	dir, a := manualRecoveryFixture(t)
	firstManualRun(t, dir, a)
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed original content", "--by", "operator"); code != exitOK {
		t.Fatalf("accept original: code=%d stderr=%s", code, stderr)
	}
	if code, _, stderr := runCLI(t, dir, "approve", "T001", "--by", "operator"); code != exitOK {
		t.Fatalf("approve: code=%d stderr=%s", code, stderr)
	}
	writeFile(t, dir, "tracked.txt", "content changed after acceptance\n")

	before := len(a.implementations)
	code, stdout, _ := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code == exitOK {
		t.Fatalf("stale acceptance completed the task:\n%s", stdout)
	}
	if len(a.implementations) != before {
		t.Fatalf("stale acceptance invoked the agent: %v", a.implementations)
	}
	if !strings.Contains(stdout, "WAITING_FOR_ACCEPTANCE") {
		t.Fatalf("stdout does not name the stale acceptance gate:\n%s", stdout)
	}

	// The blocked transition persisted the current content identity. Re-accepting
	// the changed content must now satisfy the same criterion and complete without
	// another planning or implementation invocation.
	if code, _, stderr := runCLI(t, dir, "outcome", "T001", "--accept", "criterion-1", "--evidence", "reviewed changed content", "--by", "operator"); code != exitOK {
		t.Fatalf("re-accept changed content: code=%d stderr=%s", code, stderr)
	}
	code, stdout, stderr := runInjectedCLI(t, dir, resumeTestDiff, a, "run")
	if code != exitOK {
		t.Fatalf("re-accepted resume: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.count("T001") != 1 {
		t.Fatalf("T001 was restarted after re-acceptance: %v", a.implementations)
	}
	if a.count("T002") != 1 {
		t.Fatalf("dependent task count = %d, want one: %v", a.count("T002"), a.implementations)
	}
}

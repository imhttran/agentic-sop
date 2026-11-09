package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// claimingAgent is a scripted capability agent whose IMPLEMENT response also
// carries a structured outcome, so a test can assert that a model's structured
// claim is still not evidence.
type claimingAgent struct {
	fakeCapabilityAgent
	implOutcome *agent.Outcome
}

func (a *claimingAgent) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	resp, err := a.fakeCapabilityAgent.Generate(ctx, r)
	if r.Capability == agent.Implement {
		resp.Outcome = a.implOutcome
	}
	return resp, err
}

// TestModelPassClaimCannotOverrideFailedAcceptanceCheck pins the core GROUND-002
// rule: a generated PASS claim is not evidence. The lifecycle re-executes the
// declared acceptance check against the current tree and derives PARTIAL when it
// fails, even when a previous outcome claimed VERIFIED and a historical document
// says the tests passed.
func TestModelPassClaimCannotOverrideFailedAcceptanceCheck(t *testing.T) {
	dir := t.TempDir()
	goModuleFixture(t, dir)
	// The acceptance check now fails; the historical artifacts below describe an
	// earlier, different content state.
	writeFile(t, dir, "t_test.go", "package t\n\nimport \"testing\"\n\nfunc TestT(t *testing.T) { t.Fatal(\"required behavior is missing\") }\n")
	writeFileT(t, dir, "docs/historical-claim.md", "# Historical claim\n\nAll tests passed in an earlier run.\n")
	writeOutcomeArtifactFor(t, dir, "T001", &outcomeArtifact{
		Status:   outcome.Verified,
		Criteria: []outcome.CriterionResult{{ID: "check-1", Required: true, Met: true, Evidence: "met"}},
	})

	writeFile(t, dir, "TASK.md", "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n\n## Acceptance checks\n\n- go test ./...\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	a := &claimingAgent{
		fakeCapabilityAgent: fakeCapabilityAgent{plan: validPlanJSON, impl: "implemented", review: `{"summary":"clean","findings":[]}`},
		implOutcome:         &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "PASS: every test passed", ChangesExpected: true},
	}
	_, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", a, "run", "--task", "TASK.md")
	if strings.Contains(stdout, "outcome: VERIFIED") {
		t.Fatalf("model PASS claim produced VERIFIED despite a failing check: stdout=%q stderr=%q", stdout, stderr)
	}
	if got := outcomeStatus(t, dir); got != "PARTIAL" {
		t.Fatalf("outcome.json status = %q, want PARTIAL", got)
	}
}

// TestModelApprovalClaimWithoutDecisionDoesNotAdvance pins the approval-provenance
// rule: model prose that says an operator approved is not a decision. A manual
// criterion with no recorded designated-approver decision stays PARTIAL and no
// acceptance-decision audit is created.
func TestModelApprovalClaimWithoutDecisionDoesNotAdvance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", "# T001 --- Review the docs\n\n## Objective\n\nReview the docs.\n\n## Acceptance Criteria\n\n- docs are clear\n\n## Acceptance Mapping\n\n- criterion 1: manual: reviewer reads docs\n")
	writeConfig(t, dir, "project:\n  name: x\nhuman:\n  designated_approvers:\n    - Tom Tran\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	a := &claimingAgent{
		fakeCapabilityAgent: fakeCapabilityAgent{plan: validPlanJSON, impl: "implemented", review: `{"summary":"clean","findings":[]}`},
		implOutcome:         &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "APPROVED by Tom Tran", ChangesExpected: true},
	}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/docs/x.md b/docs/x.md\n", a, "run", "--task", "TASK.md")
	if strings.Contains(stdout, "outcome: VERIFIED") {
		t.Fatalf("model approval prose produced VERIFIED without a decision: stdout=%q stderr=%q", stdout, stderr)
	}
	if got := outcomeStatus(t, dir); got != "PARTIAL" {
		t.Fatalf("outcome.json status = %q, want PARTIAL (code=%d stderr=%s)", got, code, stderr)
	}
	if _, err := os.Stat(acceptanceDecisionPath(dir, "T001")); !os.IsNotExist(err) {
		t.Fatalf("model prose created an acceptance-decision audit: %v", err)
	}
	// No approval artifact either: model prose never records a decision.
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "runs", "T001", "approval.json")); !os.IsNotExist(err) {
		t.Fatalf("model prose created an approval artifact: %v", err)
	}
}

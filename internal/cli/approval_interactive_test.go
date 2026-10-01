package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runApprovalCLI drives a command with an injected interactive boundary: stdin is a
// buffer and `interactive` simulates whether stdin is a terminal, so the interactive
// path is exercised deterministically without a pseudo-terminal. A nil agent fails
// the agent factory, exactly as runCLI does.
func runApprovalCLI(t *testing.T, dir, stdin string, interactive bool, a agent.Agent, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(string, string, string) (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			return a, nil
		},
		readDiff:    func(context.Context, string) (string, error) { return "diff --git a/x b/x\n", nil },
		commit:      func(context.Context, string, string) error { return nil },
		newGitHub:   func(string) github.Client { return &fakeGitHub{} },
		stdin:       strings.NewReader(stdin),
		interactive: func(io.Reader) bool { return interactive },
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// seedBlockedGate seeds a BLOCKED task with a genuine, applicable approval request.
func seedBlockedGate(t *testing.T, dir, id string) {
	t.Helper()
	seedTask(t, dir, &domain.Task{ID: id, Title: id, Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, Attempt: 3, MaxAttempts: 3})
	seedGate(t, dir, id)
}

// TestInteractiveApproveRecordsDecision proves an applicable gate offered at a
// terminal is approved through the same application boundary, with provenance.
func TestInteractiveApproveRecordsDecision(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedBlockedGate(t, dir, "S001")

	code, stdout, stderr := runApprovalCLI(t, dir, "y\n", true, nil, "approve", "--by", "reviewer")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "S001") || !strings.Contains(stdout, "approved S001") {
		t.Errorf("stdout = %q, want the gate shown and approved", stdout)
	}
	req, ok := approvalArtifact(t, dir, "S001")
	if !ok || req.Status != domain.ApprovalApproved || req.Decision == nil {
		t.Fatalf("persisted request = %+v, want APPROVED with a decision", req)
	}
	if req.Decision.DecidedBy != "reviewer" {
		t.Errorf("decided by = %q, want reviewer", req.Decision.DecidedBy)
	}
}

// TestInteractiveDecisionFailsClosedOffTTY proves an interactive command NEVER reads
// a decision when it cannot ask a human: it records nothing and exits non-zero.
func TestInteractiveDecisionFailsClosedOffTTY(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedBlockedGate(t, dir, "S001")

	// `y` on a non-terminal stdin must not be read as an approval.
	code, _, stderr := runApprovalCLI(t, dir, "y\n", false, nil, "approve")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d (fail closed)", code, exitUsage)
	}
	if !strings.Contains(stderr, "terminal") || !strings.Contains(stderr, "sop approve <task-id>") {
		t.Errorf("stderr = %q, want an actionable fail-closed message", stderr)
	}
	req, _ := approvalArtifact(t, dir, "S001")
	if req.Status != domain.ApprovalPending {
		t.Errorf("request status = %s, want PENDING (nothing recorded)", req.Status)
	}
}

// TestInteractiveDecisionCancelRecordsNothing proves an explicit cancel (or a closed
// stdin) records no decision and never continues execution.
func TestInteractiveDecisionCancelRecordsNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdin string
	}{
		{"explicit no", "n\n"},
		{"empty", "\n"},
		{"eof", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initProject(t, dir)
			seedBlockedGate(t, dir, "S001")

			code, _, stderr := runApprovalCLI(t, dir, tc.stdin, true, nil, "approve")
			if code != exitError {
				t.Fatalf("code=%d, want %d", code, exitError)
			}
			if !strings.Contains(stderr, "cancelled") {
				t.Errorf("stderr = %q, want a cancel message", stderr)
			}
			req, _ := approvalArtifact(t, dir, "S001")
			if req.Status != domain.ApprovalPending {
				t.Errorf("request status = %s, want PENDING (nothing recorded)", req.Status)
			}
		})
	}
}

// TestInteractiveSelectRefusesNonApplicable proves the CLI never resolves a gate
// itself: a selection naming a non-applicable task is refused by the boundary.
func TestInteractiveSelectRefusesNonApplicable(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedBlockedGate(t, dir, "S001")
	seedBlockedGate(t, dir, "S002")
	// A completed task with a pending request: the request is stale and not offered.
	seedTask(t, dir, &domain.Task{ID: "S003", Title: "done", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	seedGate(t, dir, "S003")

	code, _, stderr := runApprovalCLI(t, dir, "S003\ny\n", true, nil, "approve")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no longer applicable") {
		t.Errorf("stderr = %q, want the boundary's own applicability error", stderr)
	}
	for _, id := range []string{"S001", "S002"} {
		req, _ := approvalArtifact(t, dir, id)
		if req.Status != domain.ApprovalPending {
			t.Errorf("%s status = %s, want PENDING (untouched)", id, req.Status)
		}
	}
}

// TestInteractiveSelectByName proves a numbered selection resolves to the right gate.
func TestInteractiveSelectByName(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedBlockedGate(t, dir, "S001")
	seedBlockedGate(t, dir, "S002")

	code, stdout, stderr := runApprovalCLI(t, dir, "2\ny\n", true, nil, "approve")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "approved S002") {
		t.Errorf("stdout = %q, want the second gate approved", stdout)
	}
	if req, _ := approvalArtifact(t, dir, "S001"); req.Status != domain.ApprovalPending {
		t.Errorf("S001 status = %s, want PENDING (unselected)", req.Status)
	}
}

// TestDeclineRunIsUsageError proves --run is approve-only: a decline never continues.
func TestDeclineRunIsUsageError(t *testing.T) {
	code, _, stderr := runApprovalCLI(t, t.TempDir(), "", false, nil, "decline", "S001", "--run")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage: sop decline") {
		t.Errorf("stderr = %q, want the decline usage", stderr)
	}
}

// TestApproveRunRecordsBeforeContinuing proves --run persists the decision first and
// then starts the ordinary run path; a refused decision starts no run at all.
func TestApproveRunRecordsBeforeContinuing(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedBlockedGate(t, dir, "S001")

	// The run that follows may itself stop (there is no plan here), but the approval
	// must already be recorded, and the ordinary run must have started.
	_, stdout, _ := runApprovalCLI(t, dir, "", false, &fakeAgent{content: "{}"}, "approve", "S001", "--run")
	if !strings.Contains(stdout, "approved S001") {
		t.Fatalf("stdout = %q, want the decision reported", stdout)
	}
	// The decision is durable provenance in the append-only history, recorded before
	// the run started (the run itself may then raise a NEW gate).
	history := runpkg.At(runpkg.Dir(dir, "S001")).ApprovalHistory()
	approved := false
	for _, h := range history {
		if h.Status == domain.ApprovalApproved && h.Decision != nil {
			approved = true
		}
	}
	if !approved {
		t.Fatalf("history = %+v, want an APPROVED decision recorded before the run", history)
	}
	if !strings.Contains(stdout, "SOP") {
		t.Errorf("stdout = %q, want the ordinary run to have started", stdout)
	}

	// A refused decision (no request) starts no run: the run banner never appears.
	dir2 := t.TempDir()
	initProject(t, dir2)
	code, stdout2, stderr2 := runApprovalCLI(t, dir2, "", false, &fakeAgent{content: "{}"}, "approve", "NOPE", "--run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout2, "SOP") {
		t.Errorf("a refused decision started a run:\nstdout=%s\nstderr=%s", stdout2, stderr2)
	}
}

// TestParkedRunNamesItsGate proves a run that parks a task prints the task id and the
// commands that resolve it, and the end-of-run summary lists the gate. The text is
// derived from the request SOP recorded, not from a status or prose.
func TestParkedRunNamesItsGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the operation is destructive and cannot be undone"}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	for _, want := range []string{
		"Run paused: human approval required",
		"Task: S001",
		"sop approval S001",
		"sop approve S001",
		"sop decline S001",
		"Approvals left behind:",
		"sop approvals",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// TestLeftoverGatesPrintsAndStaysSilent proves the end-of-run section is derived from
// SOP's own applicable gates: nothing when there are none, the gate when there is one.
func TestLeftoverGatesPrintsAndStaysSilent(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	var b bytes.Buffer
	printLeftoverGates(&b, dir, st)
	if b.Len() != 0 {
		t.Errorf("a run with no gate printed an approval section: %q", b.String())
	}

	seedGate(t, dir, "S001")
	b.Reset()
	printLeftoverGates(&b, dir, st)
	if !strings.Contains(b.String(), "S001") || !strings.Contains(b.String(), "sop approvals") {
		t.Errorf("output = %q, want the gate and the pointer", b.String())
	}
}

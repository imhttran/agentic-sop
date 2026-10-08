package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// supersedeFixture seeds a project with one satisfied task and a PENDING approval
// head in its run directory, so the CLI supersede command can be exercised
// end-to-end in-process without a provider, a clock, or a model.
func supersedeFixture(t *testing.T, taskStatus domain.TaskStatus, head domain.ApprovalStatus) (string, string) {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	st, err := store.Open(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Save(&domain.Task{ID: "S001", Title: "task", Status: taskStatus}); err != nil {
		t.Fatalf("save task: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	runDir := filepath.Join(stateDir, "runs", "S001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	req := domain.ApprovalRequest{
		ID:          "S001-1",
		TaskID:      "S001",
		Kind:        domain.ApprovalNeedsHuman,
		Target:      "S001",
		Reason:      "needs a human",
		Status:      head,
		RequestedBy: "sop",
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		t.Fatalf("marshal head: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "approval.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write head: %v", err)
	}
	return dir, runDir
}

func supersedeDeps(dir string) deps {
	return deps{getwd: func() (string, error) { return dir, nil }}
}

func TestCLIApprovalSupersedeSuccess(t *testing.T) {
	dir, runDir := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalPending)
	var out, errb bytes.Buffer
	code := runApprovalSupersede([]string{"S001", "--reason", "stale after completion", "--by", "operator"}, &out, &errb, supersedeDeps(dir))
	if code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "SUPERSEDED") {
		t.Errorf("output = %q, want SUPERSEDED", out.String())
	}
	if !strings.Contains(out.String(), "operator") || !strings.Contains(out.String(), "stale after completion") {
		t.Errorf("output missing operator/reason: %q", out.String())
	}

	data, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	var head domain.ApprovalRequest
	if err := json.Unmarshal(data, &head); err != nil {
		t.Fatalf("unmarshal head: %v", err)
	}
	if head.Status != domain.ApprovalSuperseded || head.Decision == nil || head.Decision.DecidedBy != "operator" {
		t.Fatalf("head = %+v, want SUPERSEDED with operator", head)
	}

	histData, err := os.ReadFile(filepath.Join(runDir, "approval-history.json"))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	var hist []domain.ApprovalRequest
	if err := json.Unmarshal(histData, &hist); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if len(hist) != 1 || hist[0].ID != "S001-1" || hist[0].Status != domain.ApprovalPending {
		t.Fatalf("history = %+v, want the original PENDING request verbatim", hist)
	}
}

func TestCLIApprovalSupersedeRequiresReasonAndBy(t *testing.T) {
	dir, _ := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalPending)
	cases := []struct {
		name string
		args []string
	}{
		{"missing reason", []string{"S001", "--by", "operator"}},
		{"missing by", []string{"S001", "--reason", "r"}},
		{"empty reason", []string{"S001", "--reason", "  ", "--by", "operator"}},
		{"missing task", []string{"--reason", "r", "--by", "operator"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := runApprovalSupersede(tc.args, &out, &errb, supersedeDeps(dir))
			if code != exitUsage {
				t.Fatalf("exit = %d, want usage; stderr=%s", code, errb.String())
			}
		})
	}
}

func TestCLIApprovalSupersedeRefusals(t *testing.T) {
	cases := []struct {
		name   string
		task   domain.TaskStatus
		head   domain.ApprovalStatus
		expect string
	}{
		{"unsatisfied task", domain.PLANNED, domain.ApprovalPending, "no longer applicable"},
		{"non-pending head", domain.LOCAL_DONE, domain.ApprovalDeclined, "already resolved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := supersedeFixture(t, tc.task, tc.head)
			var out, errb bytes.Buffer
			code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir))
			if code != exitError {
				t.Fatalf("exit = %d, want error; stderr=%s", code, errb.String())
			}
			if !strings.Contains(errb.String(), tc.expect) {
				t.Errorf("stderr = %q, want %q", errb.String(), tc.expect)
			}
		})
	}
}

func TestCLIApprovalSupersedeMissingRecord(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	st, err := store.Open(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Save(&domain.Task{ID: "S001", Status: domain.LOCAL_DONE}); err != nil {
		t.Fatalf("save: %v", err)
	}
	st.Close()

	var out, errb bytes.Buffer
	code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir))
	if code != exitError {
		t.Fatalf("exit = %d, want error; stderr=%s", code, errb.String())
	}
	if !strings.Contains(errb.String(), "no approval request") {
		t.Errorf("stderr = %q, want not-requested error", errb.String())
	}
}

func TestCLIApprovalSupersedeIdempotent(t *testing.T) {
	dir, runDir := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalPending)
	var out, errb bytes.Buffer
	if code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir)); code != exitOK {
		t.Fatalf("first exit = %d; stderr=%s", code, errb.String())
	}
	before, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}

	out.Reset()
	errb.Reset()
	if code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir)); code != exitOK {
		t.Fatalf("second exit = %d; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "already superseded") {
		t.Errorf("output == %q, want idempotent message", out.String())
	}
	after, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("repeated supersession changed the already-superseded head")
	}
	histData, err := os.ReadFile(filepath.Join(runDir, "approval-history.json"))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	var hist []domain.ApprovalRequest
	if err := json.Unmarshal(histData, &hist); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1 (no duplicate)", len(hist))
	}
}

// TestCLIApprovalExistingRecordsUnchanged proves an existing APPROVED head is
// refused and is not rewritten by the supersede command, so PENDING/APPROVED/
// DECLINED semantics are unchanged.
func TestCLIApprovalExistingRecordsUnchanged(t *testing.T) {
	dir, runDir := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalApproved)
	before, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	var out, errb bytes.Buffer
	if code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir)); code != exitError {
		t.Fatalf("exit = %d, want error", code)
	}
	after, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("supersede rewrote an existing resolved artifact")
	}
}

// TestCLIApprovalSupersedeHistoryWriteFailure proves a history-append failure fails
// closed: the command errors, reports no success, and leaves the head PENDING with no
// partial supersede.
func TestCLIApprovalSupersedeHistoryWriteFailure(t *testing.T) {
	dir, runDir := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalPending)
	// Occupy the history path with a directory so the append cannot be written.
	if err := os.Mkdir(filepath.Join(runDir, "approval-history.json"), 0o755); err != nil {
		t.Fatalf("seed history dir: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}

	var out, errb bytes.Buffer
	if code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir)); code != exitError {
		t.Fatalf("exit = %d, want error; stdout=%s stderr=%s", code, out.String(), errb.String())
	}
	after, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a history-write failure must leave the head PENDING")
	}
}

// TestCLIApprovalSupersedeHeadWriteFailure proves a head-write failure fails closed:
// the history snapshot is appended (the audit trail is preserved), but the command
// errors and the head remains the original PENDING request.
func TestCLIApprovalSupersedeHeadWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a read-only file is still writable")
	}
	dir, runDir := supersedeFixture(t, domain.LOCAL_DONE, domain.ApprovalPending)
	headPath := filepath.Join(runDir, "approval.json")
	if err := os.Chmod(headPath, 0o444); err != nil {
		t.Fatalf("chmod head read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(headPath, 0o644) })

	var out, errb bytes.Buffer
	if code := runApprovalSupersede([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, supersedeDeps(dir)); code != exitError {
		t.Fatalf("exit = %d, want error; stdout=%s stderr=%s", code, out.String(), errb.String())
	}

	// The audit snapshot was appended before the head write failed.
	histData, err := os.ReadFile(filepath.Join(runDir, "approval-history.json"))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	var hist []domain.ApprovalRequest
	if err := json.Unmarshal(histData, &hist); err != nil {
		t.Fatalf("unmarshal history: %v", err)
	}
	if len(hist) != 1 || hist[0].ID != "S001-1" || hist[0].Status != domain.ApprovalPending {
		t.Fatalf("history = %+v, want the archived PENDING snapshot preserved", hist)
	}

	// The head is still the original PENDING request (the failed write did not apply).
	headData, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	var head domain.ApprovalRequest
	if err := json.Unmarshal(headData, &head); err != nil {
		t.Fatalf("unmarshal head: %v", err)
	}
	if head.Status != domain.ApprovalPending {
		t.Errorf("head status = %s, want PENDING", head.Status)
	}
}

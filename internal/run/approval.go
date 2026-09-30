package run

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// Human approval request persistence (the approval application boundary).
//
// The run directory is keyed by the task id and survives every invocation, so it
// is the durable home for a task's approval request and its decision history —
// the same provenance mechanism the run already uses for changed files and the
// activity stream. It is SOP-owned diagnostic/lifecycle provenance: a client
// reads the request SOP recorded and records its decision on it, and never
// infers a gate from BLOCKED/NEEDS_HUMAN/WAITING_FOR_HUMAN.
//
// The head artifact (approval.json) holds the current request; the history
// artifact (approval-history.json) is an append-only list of every request SOP
// has recorded for the task, so a superseded or resolved gate keeps its audit
// trail. Neither artifact holds file contents, prompts, or secrets.

// approvalArtifactName is the run artifact holding the task's current approval
// request head.
const approvalArtifactName = "approval.json"

// approvalHistoryName is the run artifact holding the append-only approval
// request history.
const approvalHistoryName = "approval-history.json"

// Dir returns the run directory for id under projectDir's state dir. It creates
// and modifies nothing, so a caller can read a run's artifacts without
// resetting its persisted state.
func Dir(projectDir, id string) string {
	return filepath.Join(projectDir, config.DirName, runsDirName, id)
}

// At returns a handle bound to an existing run directory for artifact I/O that
// must not touch state.json. It creates no directory and writes nothing until a
// write method is called.
func At(dir string) *Run { return &Run{dir: dir} }

// Approval returns the task's current approval request head, or (zero, false)
// when none has been recorded. It never creates or modifies the artifact.
func (r *Run) Approval() (domain.ApprovalRequest, bool) {
	data, err := os.ReadFile(filepath.Join(r.dir, approvalArtifactName))
	if err != nil {
		return domain.ApprovalRequest{}, false
	}
	var req domain.ApprovalRequest
	if err := json.Unmarshal(data, &req); err != nil || req.ID == "" {
		return domain.ApprovalRequest{}, false
	}
	return req, true
}

// SaveApproval persists the current approval request head.
func (r *Run) SaveApproval(req domain.ApprovalRequest) error {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, approvalArtifactName), append(data, '\n'), 0o644)
}

// AppendApprovalHistory appends one request snapshot to the append-only approval
// history, so a superseded or resolved gate keeps its audit trail.
func (r *Run) AppendApprovalHistory(req domain.ApprovalRequest) error {
	history := r.ApprovalHistory()
	history = append(history, req)
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, approvalHistoryName), append(data, '\n'), 0o644)
}

// ApprovalHistory returns the append-only approval request history, or nil when
// none has been recorded.
func (r *Run) ApprovalHistory() []domain.ApprovalRequest {
	data, err := os.ReadFile(filepath.Join(r.dir, approvalHistoryName))
	if err != nil {
		return nil
	}
	var out []domain.ApprovalRequest
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

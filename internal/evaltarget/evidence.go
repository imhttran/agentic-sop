package evaltarget

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// EvidenceRecord is one case's captured evidence. It has all four sections the
// evaluator needs: approvals, resulting state, verification evidence, and the final
// response. It also references the attempted and executed calls by correlation id.
type EvidenceRecord struct {
	CaseID       string               `json:"caseId"`
	Status       string               `json:"status"`
	Approvals    []ApprovalRecord     `json:"approvals"`
	State        []StateRecord        `json:"state"`
	Verification []VerificationRecord `json:"verification"`
	Response     FinalResponse        `json:"response"`
	Calls        []CallReference      `json:"calls"`
	Prohibited   bool                 `json:"prohibited"`
	Violations   []Violation          `json:"violations,omitempty"`
}

// ApprovalRecord is one approval decision the controls reached for a call.
type ApprovalRecord struct {
	CorrelationID string `json:"correlationId"`
	ApprovalID    string `json:"approvalId,omitempty"`
	Granted       bool   `json:"granted"`
	DeniedBy      string `json:"deniedBy,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// StateRecord is one observed resulting-state entry, for example a directory that
// existed or a state file that was written inside the case sandbox.
type StateRecord struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// VerificationRecord is one verification-evidence entry.
type VerificationRecord struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// FinalResponse is the case's final response, in the shape a Promptfoo evaluation
// reads: an output string plus a pass/fail flag.
type FinalResponse struct {
	Output string `json:"output"`
	Pass   bool   `json:"pass"`
}

// CallReference links an evidence record to the attempted and executed call logs
// by correlation id. An attempted call without an executed one was denied.
type CallReference struct {
	CorrelationID string `json:"correlationId"`
	Attempted     bool   `json:"attempted"`
	Executed      bool   `json:"executed"`
	DeniedBy      string `json:"deniedBy,omitempty"`
}

// PromptfooResult is the output-schema-aligned per-case result the evaluator emits.
// The fields mirror a Promptfoo evaluation row: the case id, the rendered output,
// a pass flag, a numeric score, and a free-form metadata bag.
type PromptfooResult struct {
	CaseID   string                 `json:"caseId"`
	Provider string                 `json:"provider"`
	Success  bool                   `json:"success"`
	Score    float64                `json:"score"`
	Output   string                 `json:"output"`
	Metadata map[string]interface{} `json:"metadata"`
}

// CaptureEvidence builds one case's evidence record from its call log and result.
func CaptureEvidence(caseID string, log *CallLog, stateDir string, res CaseResult) EvidenceRecord {
	rec := EvidenceRecord{
		CaseID:     caseID,
		Status:     string(res.Status),
		Prohibited: res.Prohibited,
		Violations: res.Violations,
	}
	if log != nil {
		for _, cr := range log.Records() {
			approval := ApprovalRecord{
				CorrelationID: cr.Attempted.CorrelationID,
				ApprovalID:    cr.Decision.ApprovalID,
				Granted:       cr.Decision.Allowed,
				DeniedBy:      cr.Decision.DeniedBy,
				Detail:        cr.Decision.Detail,
			}
			rec.Approvals = append(rec.Approvals, approval)
			ref := CallReference{
				CorrelationID: cr.Attempted.CorrelationID,
				Attempted:     true,
				Executed:      cr.Executed != nil,
				DeniedBy:      cr.Decision.DeniedBy,
			}
			rec.Calls = append(rec.Calls, ref)
		}
	}
	if stateDir != "" {
		_, err := os.Stat(stateDir)
		rec.State = append(rec.State, StateRecord{Path: stateDir, Exists: err == nil})
	}
	passed := res.Status == StatusPass
	rec.Verification = append(rec.Verification, VerificationRecord{
		Name:   "case status",
		Passed: passed,
		Detail: string(res.Status) + detailSuffix(res.Err),
	})
	rec.Response = FinalResponse{Output: string(res.Status) + detailSuffix(res.Err), Pass: passed}
	return rec
}

// detailSuffix renders an error detail for an evidence field.
func detailSuffix(err string) string {
	if err == "" {
		return ""
	}
	return ": " + err
}

// PromptfooResultOf converts a case's evidence to the Promptfoo output schema.
func PromptfooResultOf(rec EvidenceRecord) PromptfooResult {
	score := 0.0
	if rec.Response.Pass {
		score = 1.0
	}
	return PromptfooResult{
		CaseID:   rec.CaseID,
		Provider: "sop-evaltarget",
		Success:  rec.Response.Pass,
		Score:    score,
		Output:   rec.Response.Output,
		Metadata: map[string]interface{}{
			"status":       rec.Status,
			"prohibited":   rec.Prohibited,
			"violations":   rec.Violations,
			"calls":        rec.Calls,
			"approvals":    rec.Approvals,
			"verification": rec.Verification,
		},
	}
}

// WriteEvidence writes one case's evidence record as JSON under dir, keyed by case
// id, so a later evaluation stage can read it.
func WriteEvidence(dir string, rec EvidenceRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sanitize(rec.CaseID)+".json"), append(data, '\n'), 0o644)
}

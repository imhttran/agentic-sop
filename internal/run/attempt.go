package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Execution-attempt records (Phase 5).
//
// When Bounded Model Escalation is enabled, every lifecycle attempt a task makes
// — the initial routing attempt and each escalated retry — is persisted as its
// own record under the task's run directory:
//
//	.agent-sdlc/runs/<task>/attempts/001.json
//	.agent-sdlc/runs/<task>/attempts/002.json
//
// Attempts are WRITE-ONLY diagnostic evidence, exactly like the routing artifact:
// nothing reads them back to drive a decision (the in-process recovery.Decision is
// the sole source of truth for what happens next), they are never a second source
// of workflow state, and they carry no credential — the class, provider, model,
// locality, and fixed-phrase reasons are all non-secret.
//
// An attempt record is distinct from the initial routing decision, which keeps its
// own artifact (routing.json) and is never replaced by an escalated attempt: what
// the router first chose and what each attempt actually did are different facts.

// AttemptRecordVersion is the schema version of the attempt-record contract.
const AttemptRecordVersion = 1

// AttemptResult is the closed set of attempt outcomes.
type AttemptResult string

const (
	// AttemptPassed: the attempt passed the quality gate.
	AttemptPassed AttemptResult = "passed"
	// AttemptFailed: the attempt did not pass.
	AttemptFailed AttemptResult = "failed"
)

// Valid reports whether r is a known attempt result.
func (r AttemptResult) Valid() bool {
	return r == AttemptPassed || r == AttemptFailed
}

// AttemptRecord is one execution attempt: the class/provider/model it ran on, the
// deterministic reason it was chosen, its result, and the recovery action SOP
// applied afterwards.
type AttemptRecord struct {
	// Version is the schema version; unknown versions fail closed.
	Version int `json:"version"`
	// Attempt is the 1-based attempt number for the task.
	Attempt int `json:"attempt"`
	// Class, Provider, Model, and Locality are the (non-secret) selection the
	// attempt ran on.
	Class    string `json:"class,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Locality string `json:"locality,omitempty"`
	// Reason is the deterministic explanation of why this attempt's class was
	// chosen (a routing reason for the first attempt, the recovery reason for an
	// escalated one). It is a fixed phrase, never model-generated prose.
	Reason string `json:"reason,omitempty"`
	// Result is whether the attempt passed.
	Result AttemptResult `json:"result"`
	// FailureStage names where a failed attempt stopped (build, test, lint,
	// review, provider), when known. Empty for a passing attempt.
	FailureStage string `json:"failure_stage,omitempty"`
	// Action is the recovery action SOP applied after this attempt (empty when the
	// attempt passed and no recovery was needed).
	Action string `json:"action,omitempty"`
	// Timestamp records when the attempt finished (RFC 3339, UTC).
	Timestamp string `json:"timestamp"`
}

// Validate rejects a malformed attempt record, failing closed on an unknown
// version or result rather than defaulting. It is structural only.
func (a AttemptRecord) Validate() error {
	if a.Version != AttemptRecordVersion {
		return fmt.Errorf("run: unsupported attempt record version %d", a.Version)
	}
	if a.Attempt < 1 {
		return fmt.Errorf("run: attempt record number must be >= 1 (got %d)", a.Attempt)
	}
	if !a.Result.Valid() {
		return fmt.Errorf("run: unknown attempt result %q", a.Result)
	}
	return nil
}

// attemptsDirName is the subdirectory of a run holding its attempt records.
const attemptsDirName = "attempts"

// WriteAttemptRecord persists one attempt record under the run's attempts/
// directory, numbered by its Attempt field (zero-padded so the files sort in
// order).
//
// It validates first and fails closed: an unknown version or result is rejected
// and NO file is written. The write is purely additive diagnostic evidence: it
// never reads the record back, never drives a decision, and never touches
// state.json or the state database.
func (r *Run) WriteAttemptRecord(a AttemptRecord) error {
	if err := a.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(r.dir, attemptsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("run: create %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode attempt record: %w", err)
	}
	data = append(data, '\n')
	name := fmt.Sprintf("%03d.json", a.Attempt)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return fmt.Errorf("run: write attempts/%s: %w", name, err)
	}
	return nil
}

// ReadAttemptRecords returns this run's persisted attempt records in attempt
// order.
func (r *Run) ReadAttemptRecords() ([]AttemptRecord, error) {
	return ReadAttemptRecordsAt(r.dir), nil
}

// ReadAttemptRecordsAt reads the attempt records persisted in a run directory (a
// task's runs/<id> path), for display surfaces that resolve the directory
// themselves.
//
// A missing attempts/ directory yields no records, and a malformed record is
// skipped rather than failing the read: these records are diagnostic only and
// must never break a display path.
func ReadAttemptRecordsAt(runDir string) []AttemptRecord {
	dir := filepath.Join(runDir, attemptsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []AttemptRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var a AttemptRecord
		if err := json.Unmarshal(data, &a); err != nil {
			continue
		}
		if a.Attempt < 1 {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Attempt != out[j].Attempt {
			return out[i].Attempt < out[j].Attempt
		}
		return out[i].Timestamp < out[j].Timestamp
	})
	return out
}

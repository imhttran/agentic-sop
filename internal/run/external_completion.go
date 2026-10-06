package run

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ExternalCompletionVersion is the schema version of the external-completion record.
const ExternalCompletionVersion = 1

// ExternalCompletionFile is the run artifact that records an external completion.
const ExternalCompletionFile = "external-completion.json"

// Completion provenance values.
const (
	// CompletionSourceExternal marks work performed outside this SOP execution.
	CompletionSourceExternal = "external"
	// RecordedByOperatorAction marks an explicit operator decision.
	RecordedByOperatorAction = "explicit_operator_action"
)

// ExternalCompletion records the provenance of a task completed outside this SOP
// execution: an explicit operator action backed by repository and validation evidence. It
// is diagnostic evidence only: nothing reads it back to drive a decision, and it never
// represents a model run, a task attempt, or an approval.
type ExternalCompletion struct {
	Version              int       `json:"version"`
	TaskID               string    `json:"task_id"`
	CompletionSource     string    `json:"completion_source"`
	RepositoryHead       string    `json:"repository_head"`
	ImplementationCommit string    `json:"implementation_commit"`
	Verification         string    `json:"verification"`
	RecordedBy           string    `json:"recorded_by"`
	RecordedAt           time.Time `json:"recorded_at"`
}

// Validate fails closed on a record that cannot explain the transition, so a malformed
// provenance artifact is never written.
func (c ExternalCompletion) Validate() error {
	switch {
	case c.Version != ExternalCompletionVersion:
		return errors.New("external completion: unknown version")
	case strings.TrimSpace(c.TaskID) == "":
		return errors.New("external completion: task id is required")
	case c.CompletionSource != CompletionSourceExternal:
		return errors.New("external completion: completion source must be external")
	case strings.TrimSpace(c.ImplementationCommit) == "":
		return errors.New("external completion: implementation commit is required")
	case c.Verification != "PASS":
		return errors.New("external completion: verification must be PASS")
	}
	return nil
}

// WriteExternalCompletion persists the completion's non-secret provenance beside the run
// artifacts. It validates first and writes nothing on failure.
func (r *Run) WriteExternalCompletion(c ExternalCompletion) error {
	if c.Version == 0 {
		c.Version = ExternalCompletionVersion
	}
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return r.Write(ExternalCompletionFile, string(append(data, '\n')))
}

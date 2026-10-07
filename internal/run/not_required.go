package run

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// NotRequiredVersion is the schema version of the not-required record.
const NotRequiredVersion = 1

// NotRequiredFile is the run artifact that records a NOT_REQUIRED disposition.
const NotRequiredFile = "not-required.json"

// NotRequiredEvidence is one repository file the operator cited, pinned by content hash
// so a later edit to the file is detectable.
type NotRequiredEvidence struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// NotRequired records why a task was dispositioned NOT_REQUIRED: an explicit operator
// action citing the reason and the prerequisite evidence. It is provenance only; it
// never represents a model run, an implementation, or an approval.
type NotRequired struct {
	Version        int                   `json:"version"`
	TaskID         string                `json:"task_id"`
	PreviousStatus string                `json:"previous_status"`
	Reason         string                `json:"reason"`
	Evidence       []NotRequiredEvidence `json:"evidence"`
	RepositoryHead string                `json:"repository_head,omitempty"`
	RecordedBy     string                `json:"recorded_by"`
	RecordedAt     time.Time             `json:"recorded_at"`
}

// Validate fails closed on a record that cannot explain the disposition.
func (n NotRequired) Validate() error {
	switch {
	case n.Version != NotRequiredVersion:
		return errors.New("not-required: unknown version")
	case strings.TrimSpace(n.TaskID) == "":
		return errors.New("not-required: task id is required")
	case strings.TrimSpace(n.Reason) == "":
		return errors.New("not-required: reason is required")
	case len(n.Evidence) == 0:
		return errors.New("not-required: at least one evidence file is required")
	}
	return nil
}

// WriteNotRequired persists the record beside the task's existing run artifacts. It
// validates first, writes nothing on failure, and never touches state.json or a prior
// run's attempt records.
func (r *Run) WriteNotRequired(n NotRequired) error {
	if err := n.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(n, "", "  ")
	if err != nil {
		return err
	}
	return r.Write(NotRequiredFile, string(append(data, '\n')))
}

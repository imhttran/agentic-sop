package handoff

import "time"

// Status is the compression metadata for a handoff. It is not a workflow state:
// it never replaces task status.
type Status string

const (
	// StatusNotRequested: no compression was needed or configured.
	StatusNotRequested Status = "NOT_REQUESTED"
	// StatusCompressed: bulky context was compressed successfully.
	StatusCompressed Status = "COMPRESSED"
	// StatusFailed: compression was attempted and failed; the capsule remains.
	StatusFailed Status = "FAILED"
)

// Record is the durable handoff for a task: the capsule plus optional
// compression metadata. It carries no authoritative task status.
type Record struct {
	TaskID           string      `json:"task"`
	Capsule          Capsule     `json:"capsule"`
	Status           Status      `json:"status"`
	Content          string      `json:"content,omitempty"`
	References       []Reference `json:"references,omitempty"`
	CompressionError string      `json:"compression_error,omitempty"`
	CreatedAt        time.Time   `json:"created_at"`
}

// Store persists handoff records keyed by task id. Saving the same task twice
// must replace the record in place so regeneration is idempotent.
type Store interface {
	SaveHandoff(record Record) error
}

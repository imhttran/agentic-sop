// Package notify defines SOP's notification lifecycle event as a first-class
// domain value and derives a stable event identity from the transition it
// describes.
//
// The package is a pure domain package: it owns the event vocabulary, payload
// rules, and deterministic identity. It never delivers an event and holds no
// mutable SOP state. Delivery adapters and producers belong to later work.
package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Version is the notification event schema version this build emits and
// understands. A consumer uses it to interpret the payload without guessing.
const Version = 1

// Kind is the lifecycle transition a notification event describes. It is a
// closed vocabulary: every event names exactly one of the three kinds below.
type Kind string

const (
	// KindCompleted: the task or job reached its outcome. The event carries the
	// outcome and the next action an operator or agent can take.
	KindCompleted Kind = "completed"
	// KindFailed: SOP recorded a failure. The event carries SOP's recorded reason
	// and a next action.
	KindFailed Kind = "failed"
	// KindWaitingForHuman: SOP is parked at a human gate. The event is produced
	// only from a PENDING domain.ApprovalRequest and carries the approval request
	// id, its reason, and the resolution command.
	KindWaitingForHuman Kind = "waiting_for_human"
)

// kinds is the accepted Kind value set, kept here so a bad kind fails with a
// message that names the accepted values.
var kinds = map[Kind]bool{
	KindCompleted:       true,
	KindFailed:          true,
	KindWaitingForHuman: true,
}

// Valid reports whether k is one of the defined event kinds.
func (k Kind) Valid() bool { return kinds[k] }

// KindStrings returns the accepted kind spellings in a stable order, for error
// messages.
func KindStrings() []string {
	return []string{string(KindCompleted), string(KindFailed), string(KindWaitingForHuman)}
}

// Event is one notification lifecycle event: the immutable description of a
// transition SOP made.
//
// The identity fields (Version, Kind, Project, JobID, TaskID, Attempt,
// Transition) define which transition the event describes; the event id is a
// SHA-256 over their canonical form, so replaying a transition yields the same
// id. The descriptive fields (Reason, NextAction, ApprovalRequestID,
// ResolutionCommand, OccurredAt) are outside the hash: a replay may differ in
// occurrence time and human-readable text and still describe the same event.
type Event struct {
	// Version is the event schema version (always Version for a new event).
	Version int `json:"version"`
	// ID is the deterministic identity of the transition. NewEvent sets it.
	ID string `json:"id"`
	// Kind is the lifecycle transition kind.
	Kind Kind `json:"kind"`
	// Project names the project the event concerns.
	Project string `json:"project"`
	// JobID names the job (run) the event belongs to, when known. It is
	// caller-supplied: the run layer owns job identity.
	JobID string `json:"job_id,omitempty"`
	// TaskID names the task the event belongs to, when known.
	TaskID string `json:"task_id,omitempty"`
	// Attempt is the attempt number the transition happened in. It is at least 1
	// for a new event.
	Attempt int `json:"attempt"`
	// Transition is the source transition label the event describes (for example
	// "LOCAL_DONE"). It is part of the identity.
	Transition string `json:"transition"`
	// Reason is SOP's recorded reason. A failed event requires it.
	Reason string `json:"reason,omitempty"`
	// NextAction is the next action an operator or agent can execute. A completed
	// event carries the outcome here; a failed event carries the recovery action.
	NextAction string `json:"next_action,omitempty"`
	// Outcome is the job/task outcome a completed event reports.
	Outcome string `json:"outcome,omitempty"`
	// ApprovalRequestID is the approval request a waiting_for_human event
	// describes.
	ApprovalRequestID string `json:"approval_request_id,omitempty"`
	// ResolutionCommand is the command that resolves the human gate. A
	// waiting_for_human event requires it.
	ResolutionCommand string `json:"resolution_command,omitempty"`
	// OccurredAt is when the transition happened. It is outside the identity
	// hash, so a replay with a different time keeps the same id.
	OccurredAt time.Time `json:"occurred_at"`
}

// Params carries the caller-supplied inputs of NewEvent. NewEvent fills Version,
// ID, and validates the rest.
type Params struct {
	Kind       Kind
	Project    string
	JobID      string
	TaskID     string
	Attempt    int
	Transition string
	Reason     string
	NextAction string
	Outcome    string
	// OccurredAt is the transition time. A zero value means time.Now (UTC).
	OccurredAt time.Time
	// Approval is the source approval request for a waiting_for_human event. It
	// must be PENDING; its id and reason populate the event.
	Approval *domain.ApprovalRequest
	// ResolutionCommand is the command that resolves the human gate. Required for
	// a waiting_for_human event.
	ResolutionCommand string
}

// NewEvent validates the requested event and returns it with its deterministic
// identity computed. It returns a focused error naming the offending field, so a
// producer fails at construction time rather than emitting a malformed event.
func NewEvent(p Params) (Event, error) {
	if !p.Kind.Valid() {
		return Event{}, fmt.Errorf("notify: unknown kind %q (want %s)",
			string(p.Kind), strings.Join(KindStrings(), ", "))
	}
	if strings.TrimSpace(p.Project) == "" {
		return Event{}, errors.New("notify: project is required")
	}
	if p.Attempt < 1 {
		return Event{}, fmt.Errorf("notify: attempt must be at least 1, got %d", p.Attempt)
	}
	if strings.TrimSpace(p.Transition) == "" {
		return Event{}, errors.New("notify: transition is required")
	}

	e := Event{
		Version:    Version,
		Kind:       p.Kind,
		Project:    strings.TrimSpace(p.Project),
		JobID:      strings.TrimSpace(p.JobID),
		TaskID:     strings.TrimSpace(p.TaskID),
		Attempt:    p.Attempt,
		Transition: strings.TrimSpace(p.Transition),
		Reason:     strings.TrimSpace(p.Reason),
		NextAction: strings.TrimSpace(p.NextAction),
		Outcome:    strings.TrimSpace(p.Outcome),
		OccurredAt: p.OccurredAt,
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	} else {
		e.OccurredAt = e.OccurredAt.UTC()
	}

	if err := e.validatePayload(p); err != nil {
		return Event{}, err
	}
	e.ID = identity(e)
	return e, nil
}

// validatePayload applies the per-kind payload rules.
func (e *Event) validatePayload(p Params) error {
	switch e.Kind {
	case KindCompleted:
		if e.Outcome == "" {
			return errors.New("notify: completed event requires the job/task outcome")
		}
		if e.NextAction == "" {
			return errors.New("notify: completed event requires a next action")
		}
	case KindFailed:
		if e.Reason == "" {
			return errors.New("notify: failed event requires SOP's recorded reason")
		}
		if e.NextAction == "" {
			return errors.New("notify: failed event requires a next action an operator or agent can execute")
		}
	case KindWaitingForHuman:
		if p.Approval == nil {
			return errors.New("notify: waiting_for_human event requires a source approval request")
		}
		if p.Approval.Status != domain.ApprovalPending {
			return fmt.Errorf("notify: waiting_for_human event requires a PENDING approval request, got %q",
				string(p.Approval.Status))
		}
		if strings.TrimSpace(p.Approval.ID) == "" {
			return errors.New("notify: waiting_for_human event requires the approval request id")
		}
		// An event must never name one task while carrying another task's gate: the
		// two identities are one fact, so a mismatch is a producer bug, not a
		// payload to normalize. An approval recorded without a task id is allowed.
		if approvalTask := strings.TrimSpace(p.Approval.TaskID); approvalTask != "" && e.TaskID != "" && approvalTask != e.TaskID {
			return fmt.Errorf("notify: waiting_for_human event task %q does not match the approval request task %q",
				e.TaskID, approvalTask)
		}
		e.ApprovalRequestID = strings.TrimSpace(p.Approval.ID)
		if e.Reason == "" {
			e.Reason = strings.TrimSpace(p.Approval.Reason)
		}
		if e.Reason == "" {
			return errors.New("notify: waiting_for_human event requires the approval request reason")
		}
		e.ResolutionCommand = strings.TrimSpace(p.ResolutionCommand)
		if e.ResolutionCommand == "" {
			return errors.New("notify: waiting_for_human event requires the resolution command")
		}
	}
	return nil
}

// canonical renders the identity fields in one unambiguous, locale-independent
// form. Each field is length-prefixed so no field value can be confused with the
// separator and no two distinct field sets can collide.
//
// The approval request id is part of the identity because it is part of the
// transition a waiting_for_human event describes: one request delivered again
// keeps the same event id, while a new request for the same task, attempt, and
// transition is a different gate and gets a different id. It is empty for every
// other kind, so those ids do not depend on which gate raised them.
func canonical(e Event) string {
	var b strings.Builder
	writeField(&b, fmt.Sprintf("%d", e.Version))
	writeField(&b, string(e.Kind))
	writeField(&b, e.Project)
	writeField(&b, e.JobID)
	writeField(&b, e.TaskID)
	writeField(&b, fmt.Sprintf("%d", e.Attempt))
	writeField(&b, e.Transition)
	writeField(&b, e.ApprovalRequestID)
	return b.String()
}

// writeField appends one length-prefixed field to the canonical form.
func writeField(b *strings.Builder, v string) {
	fmt.Fprintf(b, "%d:%s\n", len(v), v)
}

// identity computes the SHA-256 event id over the canonicalized identity
// fields. The same transition always yields the same id; a new attempt, a new
// job, or a new approval request yields a different one.
func identity(e Event) string {
	sum := sha256.Sum256([]byte(canonical(e)))
	return hex.EncodeToString(sum[:])
}

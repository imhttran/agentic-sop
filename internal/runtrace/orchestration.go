package runtrace

// This file records the ORCH-009 orchestration graph in the run trace.
//
// The recording is observation-only, like the rest of runtrace: it composes
// evidence the orchestration layer already owns (assignment, worker selection,
// model-class decision, provider/model resolution, worker start/complete,
// result accept/reject, integration start/complete, verification result, and
// termination) into an ordered list of orchestration events, so the
// orchestration graph is reconstructable from trace.json alone. Nothing here is
// read back to drive a routing, lifecycle, retry, progress, approval, or
// termination decision.
//
// REFERENCE-ONLY PAYLOAD DISCIPLINE
//
// Orchestration records carry identifiers, enumerated labels, counts, and
// bounded short text only. They MUST NEVER carry credentials, API keys, bearer
// tokens, .env values, raw HTTP headers, provider secrets, base URLs, or raw
// prompt/completion bodies. Free text that does pass through is bounded by
// OneLine. Identifier-shaped fields that cannot safely carry free text (Provider,
// Model, Locality) are additionally guarded so a careless caller cannot persist
// a secret or URL: see sanitizeIdentifier.

import (
	"context"
	"strings"
	"sync"
	"time"
)

// OrchestrationEventKind is the enumerated kind of one orchestration graph
// node or edge. It is a plain label, not a lifecycle state.
type OrchestrationEventKind string

const (
	// EventAssignmentCreated records that the harness created an assignment
	// (ORCH-002 WorkAssignment) for a task.
	EventAssignmentCreated OrchestrationEventKind = "assignment_created"
	// EventWorkerSelected records which worker (assignment) was selected to run.
	EventWorkerSelected OrchestrationEventKind = "worker_selected"
	// EventModelClassSelected records the routing DECISION class. It is never
	// rewritten by a fallback and is distinct from provider/model resolution.
	EventModelClassSelected OrchestrationEventKind = "model_class_selected"
	// EventProviderResolved records the EXECUTING target the model class resolved
	// to (provider/model/locality), separate from the class decision.
	EventProviderResolved OrchestrationEventKind = "provider_resolved"
	// EventWorkerStarted records that a worker started executing an assignment.
	EventWorkerStarted OrchestrationEventKind = "worker_started"
	// EventWorkerCompleted records that a worker completed and produced a claim.
	EventWorkerCompleted OrchestrationEventKind = "worker_completed"
	// EventResultAccepted records that the harness accepted a worker result claim.
	EventResultAccepted OrchestrationEventKind = "result_accepted"
	// EventResultRejected records that the harness rejected a worker result claim.
	EventResultRejected OrchestrationEventKind = "result_rejected"
	// EventIntegrationStarted records the start of an INTEGRATE stage pass.
	EventIntegrationStarted OrchestrationEventKind = "integration_started"
	// EventIntegrationCompleted records the completion of an INTEGRATE stage pass.
	EventIntegrationCompleted OrchestrationEventKind = "integration_completed"
	// EventVerificationResult records one deterministic verification check result.
	EventVerificationResult OrchestrationEventKind = "verification_result"
	// EventTermination records the run's end state.
	EventTermination OrchestrationEventKind = "termination"
)

// OrchestrationEvent is one observation of an orchestration graph node or edge.
// Every field is either an opaque identifier, an enumerated label, or bounded
// short text: see the reference-only payload discipline above.
type OrchestrationEvent struct {
	// Sequence orders the events within one run. It is recorded in emission order.
	Sequence int `json:"sequence"`
	// Kind is the enumerated event kind.
	Kind OrchestrationEventKind `json:"kind"`
	// Timestamp is the observation time.
	Timestamp time.Time `json:"timestamp"`

	// AssignmentID identifies the assignment this event concerns, when any. It is
	// an opaque caller-owned correlation ID.
	AssignmentID string `json:"assignment_id,omitempty"`
	// TaskID identifies the domain task, when any.
	TaskID string `json:"task_id,omitempty"`
	// WorkerID identifies the worker selected for the assignment, when any.
	WorkerID string `json:"worker_id,omitempty"`

	// ModelClass is the routing DECISION class label, for the model-class node.
	ModelClass string `json:"model_class,omitempty"`
	// RoutingSource records what selected the class (policy, manual_override,
	// default, escalation).
	RoutingSource string `json:"routing_source,omitempty"`

	// Provider, Model, and Locality describe the EXECUTING target, for the
	// provider/model resolution node. They are guarded as identifiers: a value
	// carrying a URL, credential, token, or other secret-shaped content is dropped
	// rather than persisted verbatim.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Locality string `json:"locality,omitempty"`

	// Parents are the IDs of the nodes this event's node depends on (edges into
	// this node). Children are the IDs of the nodes that depend on it (edges out of
	// this node). Both carry references only, so a reader of trace.json alone can
	// rebuild the graph.
	Parents  []string `json:"parents,omitempty"`
	Children []string `json:"children,omitempty"`

	// Status is an enumerated outcome label (accepted, rejected, completed,
	// failed, ...). It is a label, never a verdict.
	Status string `json:"status,omitempty"`
	// Reason is a bounded, single-line explanation. It must never carry secrets or
	// raw prompt/completion content.
	Reason string `json:"reason,omitempty"`
}

// NewOrchestrationEvent returns an event with sequence, timestamp, and free text
// sanitized. Identifiers and enumerated labels are preserved verbatim; Reason is
// bounded by OneLine. Callers must not pass secrets or raw prompt/completion
// bodies: see the reference-only payload discipline above.
func NewOrchestrationEvent(seq int, kind OrchestrationEventKind, ts time.Time, reason string) OrchestrationEvent {
	return sanitizeEvent(OrchestrationEvent{
		Sequence:  seq,
		Kind:      kind,
		Timestamp: ts,
		Reason:    reason,
	})
}

// OrchestrationCollector accumulates orchestration events for the trace. It is
// observation-only: nothing reads it back to drive a decision. It is safe for
// concurrent producers.
type OrchestrationCollector struct {
	mu     sync.Mutex
	now    func() time.Time
	seq    int
	events []OrchestrationEvent
}

// NewOrchestrationCollector returns an empty collector. A nil clock uses time.Now.
func NewOrchestrationCollector(now func() time.Time) *OrchestrationCollector {
	if now == nil {
		now = time.Now
	}
	return &OrchestrationCollector{now: now}
}

// Emit records one orchestration event, assigning it the next sequence and a
// non-zero timestamp. It sanitizes the event through sanitizeEvent.
func (c *OrchestrationCollector) Emit(e OrchestrationEvent) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	e.Sequence = c.seq
	if e.Timestamp.IsZero() {
		e.Timestamp = c.now()
	}
	e = sanitizeEvent(e)
	c.events = append(c.events, e)
}

// Events returns the recorded events in sequence order.
func (c *OrchestrationCollector) Events() []OrchestrationEvent {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]OrchestrationEvent, len(c.events))
	copy(out, c.events)
	return out
}

// orchestrationCollectorKey keys the orchestration collector on a context.
type orchestrationCollectorKey struct{}

// WithOrchestrationCollector returns ctx carrying c. It is the context plumbing
// analogous to WithCollector, so the lifecycle can build the trace from the same
// events the orchestration layer emitted.
func WithOrchestrationCollector(ctx context.Context, c *OrchestrationCollector) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, orchestrationCollectorKey{}, c)
}

// OrchestrationCollectorFromContext returns the collector carried by ctx, or nil.
func OrchestrationCollectorFromContext(ctx context.Context) *OrchestrationCollector {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(orchestrationCollectorKey{}).(*OrchestrationCollector); ok {
		return c
	}
	return nil
}

// EmitFromContext records e on the collector carried by ctx, if any. It is the
// emission helper the orchestration lifecycle calls at each decision point; a
// context with no collector is a no-op (observation never blocks execution).
func EmitFromContext(ctx context.Context, e OrchestrationEvent) {
	if c := OrchestrationCollectorFromContext(ctx); c != nil {
		c.Emit(e)
	}
}

// sanitizeEvent returns a copy of e with all free text bounded by OneLine and all
// identifier/enum fields trimmed. It exists so callers cannot accidentally
// persist a multi-line or oversized value in an orchestration record.
func sanitizeEvent(e OrchestrationEvent) OrchestrationEvent {
	e.AssignmentID = strings.TrimSpace(e.AssignmentID)
	e.TaskID = strings.TrimSpace(e.TaskID)
	e.WorkerID = strings.TrimSpace(e.WorkerID)
	e.ModelClass = strings.TrimSpace(e.ModelClass)
	e.RoutingSource = strings.TrimSpace(e.RoutingSource)
	e.Provider = sanitizeIdentifier(e.Provider)
	e.Model = sanitizeIdentifier(e.Model)
	e.Locality = sanitizeIdentifier(e.Locality)
	e.Status = strings.TrimSpace(e.Status)
	e.Reason = OneLine(e.Reason)
	return e
}

// sanitizeIdentifier returns a bounded identifier-shaped value, or the empty
// string when the value looks like a secret, credential, base URL, or other
// content that must never be persisted in the trace. It is the runtrace-side
// guard for the provider/model/locality resolution node: even though callers are
// told to pass only enumerated labels or names, a careless caller cannot leak a
// key or URL into trace.json. A value that survives is trimmed, single-lined, and
// length-capped.
func sanitizeIdentifier(s string) string {
	s = OneLine(s)
	if s == "" {
		return ""
	}
	if looksSecretShaped(s) {
		return ""
	}
	return s
}

// secretMarkers are substrings that mark a value as secret-shaped. They cover
// common credential/key/token/URL shapes a careless caller might pass in a
// provider/model/locality position.
var secretMarkers = []string{
	"sk-", "api_key", "apikey", "api-key", "bearer ", "authorization",
	"token", "secret", "password", "passwd", "-----begin", "aws_", "ghp_",
	"xoxb-", "://", "@", ".env",
}

// looksSecretShaped reports whether s carries a credential, token, URL, or
// other secret-shaped marker that must never be persisted.
func looksSecretShaped(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

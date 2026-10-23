package evaltarget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// EffectBoundary is what a recording tool stubs: the external effect only. The
// controls under evaluation (authorization, policy, approval, verification) run
// before this boundary and are never replaced by it.
type EffectBoundary interface {
	// Write performs the external effect. Only the recording tool implements it.
	Write(ctx context.Context, req EffectRequest) error
}

// RecordingBoundary is the only effect boundary the harness accepts. The
// unexported marker prevents a caller from injecting a real external effect
// behind the recording tools: every accepted boundary records in memory.
type RecordingBoundary interface {
	EffectBoundary
	recordingOnly()
}

// EffectRequest is one requested external effect.
type EffectRequest struct {
	CorrelationID string
	Kind          string
	Target        string
	Body          string
	// Command is the command-policy evidence for this effect. Production controls
	// classify it with commandpolicy; fixture controls may ignore it.
	Command []string
}

// ControlDecision is the outcome of the real controls that run before any effect.
type ControlDecision struct {
	// Allowed is true only when every control passed.
	Allowed bool
	// DeniedBy names the control that refused the call, or is empty when allowed.
	DeniedBy string
	// ApprovalID is the approval record the controls consulted, when one applied.
	ApprovalID string
	// Detail explains the decision without echoing secret values.
	Detail string
}

// Controls are the real control checks placed in front of the recording tools.
// The evaluator wires the repository's own authorization/policy/approval logic
// here; the harness never reimplements them, and a test can flip one check to
// prove it is load bearing.
type Controls interface {
	// Authorize runs the capability-level authorization check.
	Authorize(ctx context.Context, req EffectRequest) error
	// Policy runs the command/policy check.
	Policy(ctx context.Context, req EffectRequest) error
	// Approve runs the approval requirement check. It must not accept advisory
	// memory records as approval.
	Approve(ctx context.Context, req EffectRequest) (approvalID string, err error)
}

// AttemptedCall is recorded before the controls run.
type AttemptedCall struct {
	CorrelationID string
	Kind          string
	Target        string
	Body          string
	Command       []string
}

// ExecutedCall is recorded only after the controls allow the call and the
// recording tool performs the stubbed effect.
type ExecutedCall struct {
	CorrelationID string
	Kind          string
	Target        string
	Command       []string
}

// CallRecord is the joined record for one correlation id: the attempt, the
// decision, and the executed call when one happened. A denied call has an attempt
// and a decision but no executed call.
type CallRecord struct {
	Attempted AttemptedCall
	Decision  ControlDecision
	Executed  *ExecutedCall
}

// CallLog keeps attempted and executed calls separately while sharing a
// correlation id, so an attempt and an execution are distinguishable.
type CallLog struct {
	mu        sync.Mutex
	attempted []AttemptedCall
	executed  []ExecutedCall
	records   []CallRecord
}

// NewCallLog returns an empty call log.
func NewCallLog() *CallLog { return &CallLog{} }

// Attempted returns the attempted calls, in order.
func (l *CallLog) Attempted() []AttemptedCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]AttemptedCall(nil), l.attempted...)
}

// Executed returns the executed calls, in order.
func (l *CallLog) Executed() []ExecutedCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]ExecutedCall(nil), l.executed...)
}

// Records returns the joined per-correlation-id records.
func (l *CallLog) Records() []CallRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]CallRecord(nil), l.records...)
}

// RecordFor returns the joined record for one correlation id.
func (l *CallLog) RecordFor(id string) (CallRecord, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.records {
		if r.Attempted.CorrelationID == id {
			return r, true
		}
	}
	return CallRecord{}, false
}

// EffectKindWrite is the only external effect the harness knows how to stub.
const EffectKindWrite = "external_write"

// RecordingTools runs the real controls in front of a recording effect boundary.
// It records the attempt before the controls and the execution after them.
type RecordingTools struct {
	Controls Controls
	Effect   RecordingBoundary
	Log      *CallLog
}

// NewRecordingTools wires real controls in front of a recording boundary.
func NewRecordingTools(controls Controls, effect RecordingBoundary) *RecordingTools {
	if effect == nil {
		effect = &RecordingEffect{}
	}
	return &RecordingTools{Controls: controls, Effect: effect, Log: NewCallLog()}
}

// RequestWrite records an external write with no command-policy evidence. It is
// the fixture-compatible form; production controls should use Request and supply
// a Command.
func (r *RecordingTools) RequestWrite(ctx context.Context, kind, target, body string) (CallRecord, error) {
	return r.Request(ctx, EffectRequest{Kind: kind, Target: target, Body: body})
}

// Request records the attempt, runs the controls, and — only when every control
// allows it — performs the stubbed effect and records the execution under the
// same correlation id. An unsupported effect kind is refused before the controls
// and never reaches the effect boundary.
func (r *RecordingTools) Request(ctx context.Context, req EffectRequest) (CallRecord, error) {
	id := strings.TrimSpace(req.CorrelationID)
	if id == "" {
		var err error
		id, err = newCorrelationID()
		if err != nil {
			return CallRecord{}, err
		}
	}
	req.CorrelationID = id
	rec := CallRecord{Attempted: AttemptedCall{
		CorrelationID: id, Kind: req.Kind, Target: req.Target, Body: req.Body, Command: append([]string(nil), req.Command...),
	}}

	r.Log.mu.Lock()
	r.Log.attempted = append(r.Log.attempted, rec.Attempted)
	r.Log.mu.Unlock()

	if req.Kind != EffectKindWrite {
		rec.Decision = ControlDecision{DeniedBy: "effect_kind", Detail: fmt.Sprintf("unsupported effect kind %q", req.Kind)}
		r.Log.mu.Lock()
		r.Log.records = append(r.Log.records, rec)
		r.Log.mu.Unlock()
		return rec, &DeniedError{Decision: rec.Decision}
	}
	if strings.TrimSpace(req.Target) == "" {
		rec.Decision = ControlDecision{DeniedBy: "effect_target", Detail: "effect target is required"}
		r.Log.mu.Lock()
		r.Log.records = append(r.Log.records, rec)
		r.Log.mu.Unlock()
		return rec, &DeniedError{Decision: rec.Decision}
	}

	rec.Decision = r.evaluate(ctx, req)
	if !rec.Decision.Allowed {
		// A denied call keeps its attempt and decision, and never reaches the effect.
		r.Log.mu.Lock()
		r.Log.records = append(r.Log.records, rec)
		r.Log.mu.Unlock()
		return rec, &DeniedError{Decision: rec.Decision}
	}

	if err := r.Effect.Write(ctx, req); err != nil {
		// The effect failed, so there is no executed call. The joined record is
		// still appended, so the evidence never loses the attempted entry.
		r.Log.mu.Lock()
		r.Log.records = append(r.Log.records, rec)
		r.Log.mu.Unlock()
		return rec, fmt.Errorf("evaltarget: recording effect: %w", err)
	}
	executed := ExecutedCall{CorrelationID: id, Kind: req.Kind, Target: req.Target, Command: append([]string(nil), req.Command...)}
	r.Log.mu.Lock()
	r.Log.executed = append(r.Log.executed, executed)
	rec.Executed = &executed
	r.Log.records = append(r.Log.records, rec)
	r.Log.mu.Unlock()
	return rec, nil
}

// evaluate runs the real controls. A nil control is treated as a refused control,
// so a missing check fails closed instead of silently allowing the effect.
func (r *RecordingTools) evaluate(ctx context.Context, req EffectRequest) ControlDecision {
	if r.Controls == nil {
		return ControlDecision{DeniedBy: "controls", Detail: "no controls configured"}
	}
	if err := r.Controls.Authorize(ctx, req); err != nil {
		return ControlDecision{DeniedBy: "authorization", Detail: err.Error()}
	}
	if err := r.Controls.Policy(ctx, req); err != nil {
		return ControlDecision{DeniedBy: "policy", Detail: err.Error()}
	}
	approvalID, err := r.Controls.Approve(ctx, req)
	if err != nil {
		return ControlDecision{DeniedBy: "approval", Detail: err.Error()}
	}
	return ControlDecision{Allowed: true, ApprovalID: approvalID}
}

// DeniedError is returned when a real control refuses a call.
type DeniedError struct{ Decision ControlDecision }

func (e *DeniedError) Error() string {
	return fmt.Sprintf("evaltarget: denied by %s: %s", e.Decision.DeniedBy, e.Decision.Detail)
}

// RecordingEffect is the stub external effect. It writes only to an in-memory
// list, never to the network or a production endpoint.
type RecordingEffect struct {
	mu    sync.Mutex
	Wrote []EffectRequest
}

// recordingOnly marks RecordingEffect as the accepted effect boundary.
func (*RecordingEffect) recordingOnly() {}

// Write records the effect instead of performing it.
func (e *RecordingEffect) Write(_ context.Context, req EffectRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Wrote = append(e.Wrote, req)
	return nil
}

// Writes returns the stubbed effects, in order.
func (e *RecordingEffect) Writes() []EffectRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]EffectRequest(nil), e.Wrote...)
}

// newCorrelationID returns a fresh correlation id.
func newCorrelationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errors.Join(errors.New("evaltarget: correlation id"), err)
	}
	return hex.EncodeToString(b[:]), nil
}

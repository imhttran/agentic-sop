// Structured SOP progress over the ACP session/update notification (ACP-004).
//
// The host process supplies a ProgressSource (WithProgress) that reads the
// project's persisted SOP evidence for one started job. After a successful
// session/prompt the server translates one snapshot into exactly one pinned
// session/update notification frame and delivers it through the bounded event
// buffer (delivery.go). The frame carries, as payload fields:
//
//   - task identity:  the persisted job id (always) and the linked task id
//     when persisted state knows it (omitted otherwise)
//   - current stage:  the stage the source reports; omitted when absent
//   - verification:   the payload state value plus the verified flag
//   - blockers:       the blockers the source reports; omitted when none
//   - final evidence: evidence references the source reports; omitted when
//     none
//
// Four conditions stay explicit and distinct:
//
//   - a finished prompt turn is marked by the session/prompt response's
//     stopReason member, which only ever carries a protocol-defined stop
//     reason (contract.go);
//   - LOCAL_DONE is the payload state ProgressStateLocalDone: implementation
//     reports done, verification is not asserted, work is not complete;
//   - verified completion is ProgressStateVerified with verified true — the
//     only state that reads as work complete;
//   - work waiting for approval is ProgressStateAwaitingApproval (or
//     ProgressStateBlocked with blockers listed).
//
// SOP states are payload-field values, never protocol status values: nothing
// in this file may put a SOP state into the stop-reason vocabulary.
//
// Persisted-evidence trace (every snapshot field, and whether persisted SOP
// evidence actually provides it):
//
//   - jobId    persisted: the started-work job record's durable id, read
//     through the host StateReader seam (internal/acp/prompt.go
//     PersistedWork; the CLI bridge backs it with listWorkJobs in
//     internal/cli/acp.go).
//   - exitCode persisted: PersistedWork.ExitCode, the recorded exit of an
//     ended or failed job run (same read path).
//   - state    persisted as the record's derived state: running, completed,
//     failed, ended — the closed set prompt.go pins. The finer SOP
//     states (local_done, verified, verification_failed, blocked,
//     awaiting_approval) are host-reported: a source supplies one
//     only when it read the corresponding evidence.
//   - taskId   NOT persisted: prompt.go documents that the started-work job
//     record does not link the child run's task and attempt
//     identity. The frame omits it instead of inventing it.
//   - stage, verified, blockers, evidence
//     no persisted read path was traced for these fields in this
//     stage (the candidate layers internal/run/validation_evidence.go,
//     internal/cli/task_validation.go, and internal/verifcache/store.go
//     were not inspected by this stage, so no field claims them).
//     They are host-reported fields: a source may supply them only
//     from evidence it actually read; otherwise the frame omits them
//     and the text reports the state without them.
//
// A snapshot whose state or job id is missing or unknown is refused (never
// translated into a plausible-looking frame), and a server without a progress
// source emits no frames at all: a missing seam cannot fabricate progress.
package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// SOP progress payload states. These values travel only in the session/update
// payload's sop.state field; they are never protocol stop reasons, update
// kinds, or any other protocol status value. Prompt-turn end is expressed
// only through the pinned stop reasons in contract.go.
const (
	// ProgressStateRunning: started work is executing; nothing is complete.
	ProgressStateRunning = "running"
	// ProgressStateAwaitingApproval: the task waits for a human approval gate
	// before it can continue; work is not complete.
	ProgressStateAwaitingApproval = "awaiting_approval"
	// ProgressStateBlocked: execution is blocked; the blockers travel in the
	// payload's blockers field; work is not complete.
	ProgressStateBlocked = "blocked"
	// ProgressStateLocalDone: the implementation reports LOCAL_DONE;
	// verification is not asserted, so work is not complete yet.
	ProgressStateLocalDone = "local_done"
	// ProgressStateVerified: the task completed and verification passed; this
	// is the only state that reads as work complete.
	ProgressStateVerified = "verified"
	// ProgressStateVerificationFailed: the task's verification failed; work
	// is not complete.
	ProgressStateVerificationFailed = "verification_failed"
	// ProgressStateCompleted: the persisted job record reached completed; the
	// frame does not assert verification unless the source set it.
	ProgressStateCompleted = "completed"
	// ProgressStateFailed: the persisted job record failed; work did not
	// complete.
	ProgressStateFailed = "failed"
	// ProgressStateEnded: the job ended without a recorded exit; the outcome
	// is ambiguous, never success.
	ProgressStateEnded = "ended"
	// ProgressStateCancelled: governed work was stopped by an explicit
	// session/cancel before completion (ACP-006). It is recorded distinctly
	// from failed and completed, and it is the one payload state whose
	// spelling matches a protocol stop reason by design: a cancelled job maps
	// to StopReasonCancelled in the session/cancel response that stopped it.
	ProgressStateCancelled = "cancelled"
)

// progressStates is the closed set of payload states buildProgressFrame
// accepts; a snapshot outside it is refused instead of translated.
var progressStates = map[string]bool{
	ProgressStateRunning:            true,
	ProgressStateAwaitingApproval:   true,
	ProgressStateBlocked:            true,
	ProgressStateLocalDone:          true,
	ProgressStateVerified:           true,
	ProgressStateVerificationFailed: true,
	ProgressStateCompleted:          true,
	ProgressStateFailed:             true,
	ProgressStateEnded:              true,
	ProgressStateCancelled:          true,
}

// ProgressSnapshot is one progress reading of persisted SOP evidence for one
// started job, as the host's ProgressSource supplies it. Zero fields mean the
// evidence is unavailable; the translator omits them instead of inventing
// them. See the file-level trace above for which fields persisted evidence
// provides today.
type ProgressSnapshot struct {
	// JobID is the persisted started-work job record's durable id. It is
	// required: a progress frame without the job identity would fabricate
	// work identity.
	JobID string
	// TaskID is the linked task identity when persisted state knows it.
	// Empty means unavailable (today's job records do not link it).
	TaskID string
	// Stage names the current SOP stage. Empty means unavailable.
	Stage string
	// State is one of the ProgressState* payload values above.
	State string
	// Verified is the persisted verification outcome: true only when a
	// verification the source read passed. Derived job states never set it.
	Verified bool
	// Blockers lists what blocks or gates the work; empty means none
	// recorded.
	Blockers []string
	// Evidence lists final evidence references (paths or ids); empty means
	// none recorded.
	Evidence []string
	// ExitCode is the persisted exit code of an ended or failed run.
	ExitCode *int
}

// ProgressSource is the host-supplied progress seam, mirroring
// PromptSubmission. A missing function or a missing source means the server
// emits no session/update frames at all — never a fabricated outcome.
type ProgressSource struct {
	// Progress reads the latest persisted SOP evidence for the job jobID in
	// projectDir. found=false when no record for the job exists, so no frame
	// is emitted. An error is recorded as a diagnostic and no frame is
	// emitted: progress streaming never fails a prompt response.
	Progress func(projectDir, jobID string) (ProgressSnapshot, bool, error)
}

// WithProgress wires the host's persisted-progress source. Without it the
// server never emits session/update frames and never fabricates an outcome.
func WithProgress(src *ProgressSource) ServerOption {
	return func(s *Server) { s.progress = src }
}

// missingProgress reports whether the progress seam is unconfigured.
func (s *Server) missingProgress() bool {
	return s.progress == nil || s.progress.Progress == nil
}

// stampPromptStopReason marks a successful session/prompt response as a
// finished prompt turn with a protocol-defined stop reason. The stop reason
// says the transport turn ended; it never says the started work completed —
// SOP outcomes travel only in session/update payload fields.
func (s *Server) stampPromptStopReason(result any) {
	res, ok := result.(map[string]any)
	if !ok {
		return
	}
	if _, exists := res["stopReason"]; exists {
		return
	}
	res["stopReason"] = StopReasonEndTurn
}

// emitProgressFor reads the configured ProgressSource for the started work a
// session/prompt result identifies and queues the matching session/update
// frame for delivery. Responses without a job id (busy, not started, task in
// progress) and missing sources emit nothing: a frame without real persisted
// evidence behind it would fabricate progress.
func (s *Server) emitProgressFor(req *Request, result any) {
	if s.missingProgress() {
		return
	}
	res, ok := result.(map[string]any)
	if !ok {
		return
	}
	jobID, ok := res["jobId"].(string)
	if !ok || strings.TrimSpace(jobID) == "" {
		return
	}
	sess := s.getSession(peekSessionID(req.Params))
	if sess == nil {
		return
	}
	snap, found, err := s.progress.Progress(sess.ProjectDir, jobID)
	if err != nil {
		s.diag.Printf("read SOP progress for job %s in project %s: %v; no session/update frame emitted", shorten(jobID, 64), shorten(sess.ProjectID, 64), err)
		return
	}
	if !found {
		return
	}
	frame, outcome, err := buildProgressFrame(sess.ID, snap)
	if err != nil {
		s.diag.Printf("build session/update for job %s: %v; no frame emitted", shorten(jobID, 64), err)
		return
	}
	// EnqueueProgress and EnqueueOutcome diagnose their own overflow.
	if outcome {
		_ = s.EnqueueOutcome(frame)
	} else {
		_ = s.EnqueueProgress(frame)
	}
}

// peekSessionID reads the sessionId member of request params leniently: the
// strict decode already happened (or rejected the request) in the method
// handler; this only re-finds the session the result belongs to.
func peekSessionID(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var p struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ""
	}
	return p.SessionID
}

// sessionUpdateFrame is the pinned outbound notification envelope. A struct
// fixes the member order, so the same snapshot always marshals to identical
// bytes.
type sessionUpdateFrame struct {
	JSONRPC string               `json:"jsonrpc"`
	Method  string               `json:"method"`
	Params  progressNotification `json:"params"`
}

// progressNotification is the pinned session/update params: the session the
// progress belongs to, the protocol update body, and the structured SOP
// progress fields. The sop member is a pinned SOP extension on this build's
// notification envelope; it adds no protocol status values, and clients that
// ignore unknown members still render the update body's text block.
type progressNotification struct {
	SessionID string             `json:"sessionId"`
	Update    progressUpdateBody `json:"update"`
	SOP       progressSOPFields  `json:"sop"`
}

// progressUpdateBody is the pinned update object this build sends for SOP
// progress: a protocol-defined agent_message_chunk with a deterministic text
// block; the structured fields ride in the envelope's sop member.
type progressUpdateBody struct {
	SessionUpdate string           `json:"sessionUpdate"`
	Content       progressTextBody `json:"content"`
}

// progressTextBody is one text content block.
type progressTextBody struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// progressSOPFields is the structured SOP progress payload. Every field
// except jobId, state, and verified is omitted when the evidence is
// unavailable, so the frame never invents a task id, stage, blocker, evidence
// reference, or exit code.
type progressSOPFields struct {
	JobID    string   `json:"jobId"`
	TaskID   string   `json:"taskId,omitempty"`
	State    string   `json:"state"`
	Verified bool     `json:"verified"`
	Stage    string   `json:"stage,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
	ExitCode *int     `json:"exitCode,omitempty"`
}

// buildProgressFrame translates one snapshot into the deterministic
// session/update notification frame and reports whether it is a terminal
// outcome (every state except running: outcomes must survive coalescing).
// The same snapshot always builds byte-identical bytes.
func buildProgressFrame(sessionID string, snap ProgressSnapshot) (frame []byte, outcome bool, err error) {
	if err := snap.validate(); err != nil {
		return nil, false, err
	}
	outcome = snap.State != ProgressStateRunning
	params := progressNotification{
		SessionID: sessionID,
		Update: progressUpdateBody{
			SessionUpdate: UpdateAgentMessageChunk,
			Content:       progressTextBody{Type: "text", Text: progressText(snap)},
		},
		SOP: progressSOPFields{
			JobID:    strings.TrimSpace(snap.JobID),
			TaskID:   strings.TrimSpace(snap.TaskID),
			State:    snap.State,
			Verified: snap.Verified,
			Stage:    strings.TrimSpace(snap.Stage),
			Blockers: nonEmptyList(snap.Blockers),
			Evidence: nonEmptyList(snap.Evidence),
			ExitCode: snap.ExitCode,
		},
	}
	data, err := json.Marshal(sessionUpdateFrame{
		JSONRPC: "2.0",
		Method:  MethodSessionUpdate,
		Params:  params,
	})
	if err != nil {
		return nil, outcome, fmt.Errorf("marshal session/update: %w", err)
	}
	return data, outcome, nil
}

// validate refuses snapshots the translator could only turn into fabricated
// progress.
func (p ProgressSnapshot) validate() error {
	if strings.TrimSpace(p.JobID) == "" {
		return errors.New("progress snapshot has no job id: a frame without the persisted job identity would fabricate work identity")
	}
	if !progressStates[p.State] {
		return fmt.Errorf("progress snapshot state %q is not one of the pinned SOP payload states in progress.go", shorten(p.State, 64))
	}
	return nil
}

// nonEmptyList returns the entries of in that carry text, preserving order;
// an empty result is nil so the payload omits the member entirely.
func nonEmptyList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// progressText renders the deterministic one-line progress text the update
// body carries. Every unfinished state says the work is not complete, so a
// frame can never read as a completion claim while work still runs.
func progressText(snap ProgressSnapshot) string {
	label := "SOP job " + strings.TrimSpace(snap.JobID)
	if taskID := strings.TrimSpace(snap.TaskID); taskID != "" {
		label += " (task " + taskID + ")"
	}
	var b strings.Builder
	b.WriteString(label)
	switch snap.State {
	case ProgressStateRunning:
		if stage := strings.TrimSpace(snap.Stage); stage != "" {
			b.WriteString(": stage " + stage + " is running")
		} else {
			b.WriteString(" is running")
		}
		b.WriteString("; progress only, work is not complete")
	case ProgressStateLocalDone:
		b.WriteString(" reports LOCAL_DONE: implementation finished, verification not passed; work is not complete")
	case ProgressStateVerified:
		b.WriteString(" is complete; verification passed")
	case ProgressStateVerificationFailed:
		b.WriteString(" verification failed; work is not complete")
	case ProgressStateBlocked:
		if blockers := nonEmptyList(snap.Blockers); blockers != nil {
			b.WriteString(" is blocked on " + strings.Join(blockers, "; "))
		} else {
			b.WriteString(" is blocked")
		}
		b.WriteString("; work is not complete")
	case ProgressStateAwaitingApproval:
		b.WriteString(" is waiting for human approval before it can continue; work is not complete")
	case ProgressStateCompleted:
		b.WriteString(" reached completed in its job record; this frame does not assert verification")
	case ProgressStateFailed:
		b.WriteString(" failed; work did not complete")
	case ProgressStateEnded:
		b.WriteString(" ended without a recorded exit; the outcome is ambiguous")
	}
	if stage := strings.TrimSpace(snap.Stage); stage != "" && snap.State != ProgressStateRunning {
		b.WriteString(" (stage " + stage + ")")
	}
	if snap.ExitCode != nil {
		b.WriteString(fmt.Sprintf(" (exit code %d)", *snap.ExitCode))
	}
	if evidence := nonEmptyList(snap.Evidence); evidence != nil {
		b.WriteString(". Evidence: " + strings.Join(evidence, ", "))
	}
	b.WriteString(".")
	return b.String()
}

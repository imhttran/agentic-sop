package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Session cancellation (ACP-006).
//
// session/cancel asks SOP to stop one session's adapter-owned foreground work.
// The adapter never signals a child itself and never reports a cancellation it
// cannot confirm: it calls the host-supplied PromptSubmission.Cancel seam, which
// stops only the named job and returns nil only once the job's persisted record
// reflects the cancellation. The response reports the persisted state, so a job
// that finished before the stop keeps its own outcome.
//
// ACP v1 defines session/cancel as a notification (no response). This server
// accepts both shapes: an id-less frame is processed without a response, and an
// id-bearing frame answers with the stop reason and the job identity. Every
// other notification remains a no-op.
//
// A clean EOF (client disconnect) is enforced here too: Serve (transport.go)
// runs reconcileDisconnect before its final io.EOF return. The step cancels
// every live session's active job through the same seam within CancelStopBound,
// then re-reads the persisted records to observe the authoritative final state.
// A disconnect never restarts work, never fabricates a completion, never claims
// a cancellation the record does not carry (a seam error is only a diagnostic),
// and never emits a session/update, so a job cannot receive a second terminal
// lifecycle event. Stopping the job's subprocess — and reaping it if the
// adapter process itself dies — is also owned by the run layer below the seam,
// documented in the Cancel contract in prompt.go: the adapter bounds and
// verifies each seam call; it is never the process owner.

// sessionCancelRequest is the pinned session/cancel params shape.
type sessionCancelRequest struct {
	// SessionID is the session whose foreground work is cancelled.
	SessionID string `json:"sessionId"`
}

// handleSessionCancel stops one session's foreground governed work through the
// host cancel seam. It is a protocol error when the seam is absent, so the
// capability is never advertised without behaviour behind it.
func (s *Server) handleSessionCancel(req *Request) (any, *Error) {
	if s.missingCancelSeam() {
		return nil, &Error{
			Code:    CodeCapabilityUnsupported,
			Message: "cancellation is not configured on this server: the server host must supply the governed cancel seam",
		}
	}
	params, perr := decodeSessionCancel(req.Params)
	if perr != nil {
		return nil, perr
	}
	sess := s.getSession(params.SessionID)
	if sess == nil {
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("unknown sessionId %q: open a session with session/new first", shorten(params.SessionID, 64)),
		}
	}
	jobID := s.sessionJob(sess.ID)
	if jobID == "" {
		return map[string]any{
			"sessionId":  sess.ID,
			"stopReason": StopReasonEndTurn,
			"outcome":    WorkOutcomeNotRunning,
			"message":    "no foreground governed work to cancel in this session",
		}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), CancelStopBound)
	defer cancel()
	if err := s.prompts.Cancel(ctx, sess.ProjectDir, jobID); err != nil {
		return nil, &Error{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("cancel governed work %s: %v", shorten(jobID, 64), err),
		}
	}
	// The seam returned nil, but only the persisted record confirms what happened:
	// a seam that stopped the child still owns the record update. Claiming
	// cancelled while the record still says running would contradict SOP's own
	// authoritative state.
	state := s.persistedJobState(sess.ProjectDir, jobID)
	if state == persistedStateCancelled {
		return map[string]any{
			"sessionId":  sess.ID,
			"stopReason": StopReasonCancelled,
			"outcome":    WorkOutcomeCancelled,
			"jobId":      jobID,
			"jobState":   persistedStateCancelled,
			"message":    "the session's foreground governed work was cancelled; its persisted record is authoritative",
		}, nil
	}
	out := map[string]any{
		"sessionId":  sess.ID,
		"stopReason": StopReasonEndTurn,
		"outcome":    WorkOutcomeNotRunning,
		"jobId":      jobID,
	}
	switch state {
	case "":
		out["message"] = "the stop was requested but the job's persisted record could not be read back; the cancellation is not confirmed, so the record stays authoritative"
	case persistedStateRunning:
		out["message"] = "the stop was requested but the job's persisted record still says running; the cancellation is not confirmed"
	default:
		out["message"] = "the job had already finished before the cancellation was applied; its recorded outcome is authoritative"
	}
	if state != "" {
		out["jobState"] = state
	}
	return out, nil
}

// decodeSessionCancel decodes session/cancel params strictly.
func decodeSessionCancel(raw []byte) (sessionCancelRequest, *Error) {
	var params sessionCancelRequest
	if len(raw) == 0 {
		return params, &Error{
			Code:    CodeInvalidParams,
			Message: "session/cancel requires params: {sessionId}",
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&params); err != nil {
		return sessionCancelRequest{}, &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("invalid session/cancel params: %v; expected field is sessionId", shorten(err.Error(), 128)),
		}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return sessionCancelRequest{}, &Error{
			Code:    CodeInvalidParams,
			Message: "invalid session/cancel params: unexpected trailing data; send one JSON object",
		}
	}
	params.SessionID = strings.TrimSpace(params.SessionID)
	if params.SessionID == "" {
		return sessionCancelRequest{}, &Error{
			Code:    CodeInvalidParams,
			Message: "sessionId is required: pass the session whose foreground work should be cancelled",
		}
	}
	return params, nil
}

// missingCancelSeam reports whether the host cancel seam is unconfigured. A
// server without it never claims the capability: session/cancel is refused
// (handleSessionCancel above), and EOF recovery (reconcileDisconnect below)
// does nothing rather than fabricate a cancellation it cannot perform.
func (s *Server) missingCancelSeam() bool {
	return s.prompts == nil || s.prompts.Cancel == nil
}

// persistedJobState returns the persisted record's state for jobID, read
// through the host StateReader seam. It returns "" when the reader is
// unconfigured, the read fails, or no record carries the job identity: a
// missing read is never translated into a claimed outcome. A failed read is
// recorded as an actionable diagnostic.
func (s *Server) persistedJobState(projectDir, jobID string) string {
	if s.prompts == nil || s.prompts.StateReader == nil {
		return ""
	}
	records, err := s.prompts.StateReader(projectDir)
	if err != nil {
		s.diag.Printf("read persisted work for job %s in project %s: %v; the record's state is unavailable, so no outcome is claimed", shorten(jobID, 64), shorten(projectDir, 64), err)
		return ""
	}
	for _, w := range records {
		if w.JobID == jobID {
			return w.State
		}
	}
	return ""
}

// reconcileDisconnect is the disconnect-recovery stage Serve (transport.go)
// runs before its final io.EOF return: the clean-EOF policy pinned at the top
// of this file. It requests safe cancellation of every live session's
// foreground job through the host cancel seam — one CancelStopBound context
// per session — then re-reads the persisted records through StateReader to
// observe the authoritative final state.
//
// The step, per the S2 contract:
//
//   - a stalled or failing host cannot stall the shutdown: every seam call runs
//     concurrently under one overall CancelStopBound deadline, so a clean EOF
//     never waits longer than that bound however many sessions are live; a seam
//     error is only a diagnostic;
//   - work is never restarted and no completion is fabricated;
//   - no session/update frame is written, so a job that already received a
//     terminal lifecycle event is never given a second one;
//   - an unreadable record never claims a cancelled outcome;
//   - a server without the seam skips silently (Serve still returns nil),
//     because the adapter never fabricates a cancellation it cannot perform.
func (s *Server) reconcileDisconnect() {
	if s.missingCancelSeam() {
		return
	}
	sessions := s.activeJobSessions()
	if len(sessions) == 0 {
		return
	}
	// One overall deadline for the whole reconciliation: N sessions must never
	// multiply the shutdown bound into N × CancelStopBound.
	ctx, cancel := context.WithTimeout(context.Background(), CancelStopBound)
	defer cancel()
	var wg sync.WaitGroup
	for _, sess := range sessions {
		sess := sess
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobID := sess.ActiveJobID
			err := s.prompts.Cancel(ctx, sess.ProjectDir, jobID)
			if err != nil {
				s.diag.Printf("cancel job %s on client disconnect: %v; the persisted record stays authoritative and no cancelled outcome is claimed", shorten(jobID, 64), err)
			}
			if state := s.persistedJobState(sess.ProjectDir, jobID); state != "" {
				s.diag.Printf("disconnect recovery: job %s final persisted state %q; no work restarted and no second lifecycle event emitted", shorten(jobID, 64), state)
			} else {
				s.diag.Printf("disconnect recovery: job %s final persisted state is unavailable; no work restarted and no outcome claimed", shorten(jobID, 64))
			}
		}()
	}
	wg.Wait()
}

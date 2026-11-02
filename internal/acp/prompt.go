// Governed prompt submission over ACP (ACP-003).
//
// session/prompt carries one user-authored text prompt for a bound session.
// The ACP server never executes it: the host process (the CLI ACP entry
// point, internal/cli/acp.go) injects a PromptSubmission bridge whose three
// seams are the existing machinery SOP already runs for `sop prompt` and MCP
// sop_start_work:
//
//   - LockTaker takes the shared project writer lock (the same advisory lock
//     CLI writers and MCP-started children take through internal/projectlock).
//     A contended take is reported as ErrProjectBusy so the adapter returns a
//     busy result promptly instead of queueing conflicting work implicitly.
//   - StateReader reads the project's persisted started-work records, so that
//     every new start is first reconciled against persisted state.
//   - Starter performs the governed start (one accepted submission creates
//     exactly one work item) and returns the job identity.
//   - Cancel (ACP-006) requests safe cancellation of one adapter-owned
//     started job within the documented stop bound; the adapter never
//     signals the job's subprocess itself and refuses session/cancel when
//     the seam is absent.
//
// Identifier mapping (pinned):
//
//	ACP sessionId        -> Session.ID, bound to the canonical project dir
//	submissionKey        -> the submission identity: a client-supplied
//	                     submission key used verbatim, or the work
//	                     fingerprint when the client supplies none, so an
//	                     accidental resend is still a duplicate
//	submission identity  -> the persisted started-work job record, which
//	                     stores the submission key and the fingerprint
//	job identity         -> the started job's durable id; once a job exists,
//	                     the protocol result always carries it
//	task identity        -> known only when the persisted job record or the
//	                     project task graph records it; surfaced when known,
//	                     omitted otherwise
//	attempt identity     -> owned by the run the started job executes; it is
//	                     not linked into the job record yet. That remaining
//	                     gap is recorded here rather than papered over with
//	                     invented state.
//
// JSON-RPC request ids are per-connection and are NEVER used as durable
// idempotency keys: two submissions with the same id but different submission
// keys are two submissions.
package acp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	// MaxPromptSubmitBytes bounds the concatenated prompt text accepted over
	// session/prompt. It matches the bound MCP start-work applies to its
	// prompt field, so neither protocol accepts what the governed start path
	// cannot carry.
	MaxPromptSubmitBytes = 256 << 10
	// MaxSubmissionKeyLength bounds a client-supplied submission key.
	MaxSubmissionKeyLength = 512
	// promptWorkKind is the work kind recorded for prompt-started work. It is
	// the same kind the CLI started-work records use for `sop prompt` jobs.
	promptWorkKind = "prompt"
	// promptDefaultCapability is the capability used when the client names
	// none: the governed start path (CLI `sop prompt`, MCP sop_start_work)
	// defaults to the read-only plan capability, and the fingerprint below
	// must use the same default so an ACP submission matches work started
	// with no capability named by either other interface.
	promptDefaultCapability = "plan"
	// submissionRegistryLimit bounds the per-server replay registry: the
	// oldest recorded submission result is dropped beyond this bound, so a
	// chatty client cannot grow server memory without limit. Dropping an old
	// entry only reopens it for duplicate detection after 256 newer
	// submissions in the same session.
	submissionRegistryLimit = 256
	// CancelStopBound is the documented bound within which the host cancel
	// seam (PromptSubmission.Cancel) must stop adapter-owned foreground work.
	// The adapter enforces the bound with a per-request context and records an
	// actionable diagnostic when it expires; it never assumes an unbounded
	// stop and never claims a cancelled outcome without the seam's or the
	// persisted record's attestation. The value is a contract decision.
	CancelStopBound = 30 * time.Second
)

// ErrProjectBusy reports a contended project writer lock. The LockTaker seam
// returns it instead of waiting indefinitely, so the adapter can return a
// busy result rather than queue the submission.
var ErrProjectBusy = errors.New("acp: the project writer lock is held by a competing writer")

// Session/prompt outcomes. The busy outcomes are results, not protocol
// errors: they tell the client, usefully, who holds the project and that no
// work was started or queued.
const (
	// WorkOutcomeStarted: one new work item started under the job identity.
	WorkOutcomeStarted = "started"
	// WorkOutcomeAlreadyRunning: the same work is already running; the result
	// carries its job identity and nothing new was started.
	WorkOutcomeAlreadyRunning = "already_running"
	// WorkOutcomeBusy: the project is held (a competing CLI/MCP/ACP start),
	// so the submission was not started and was not queued.
	WorkOutcomeBusy = "busy"
	// WorkOutcomeTaskInProgress: a mutating run is mid-task in the project.
	WorkOutcomeTaskInProgress = "task_in_progress"
	// WorkOutcomeReconciled: a previous attempt for the same submission
	// identity exists in persisted state; the result carries its identity
	// instead of starting duplicate work.
	WorkOutcomeReconciled = "reconciled"
	// WorkOutcomeNotInitialized: the project has no SOP state yet.
	WorkOutcomeNotInitialized = "not_initialized"
	// WorkOutcomeCancelled: governed work was stopped by an explicit
	// session/cancel before completion (ACP-006). It is recorded distinctly
	// from failed and completed and maps to the protocol's
	// StopReasonCancelled.
	WorkOutcomeCancelled = "cancelled"
	// WorkOutcomeNotRunning: nothing was cancelled because no matched job is
	// running; when the cancellation resolved to a finished record instead,
	// the result carries that record's state verbatim.
	WorkOutcomeNotRunning = "not_running"
)

// Derived persisted job states, matching the started-work job records.
const (
	persistedStateRunning   = "running"
	persistedStateCompleted = "completed"
	persistedStateFailed    = "failed"
	persistedStateEnded     = "ended"
	// persistedStateCancelled records a job the host cancel seam stopped
	// (ACP-006): a distinct recorded outcome, never a mapped failed or
	// completed state.
	persistedStateCancelled = "cancelled"
)

// PromptSubmission is the governed prompt submission bridge, supplied by the
// host process. A missing seam fails the submission closed with an actionable
// error rather than skipping the step.
type PromptSubmission struct {
	// LockTaker takes the shared project writer lock for projectDir and
	// returns its release function. Contended takes return ErrProjectBusy
	// promptly; other errors are real failures and fail the submission.
	LockTaker func(ctx context.Context, projectDir string) (func(), error)
	// StateReader reads the persisted started-work records for projectDir,
	// newest first, so the adapter can reconcile before any new start.
	StateReader func(projectDir string) ([]PersistedWork, error)
	// Starter performs the governed start of one submission and returns its
	// outcome, including the job identity for a started work item.
	Starter func(sub Submission) (SubmissionResult, error)
	// Cancel requests safe cancellation of the adapter-owned started job
	// jobID in projectDir (ACP-006). The adapter calls it within the
	// CancelStopBound and never signals the job's subprocess itself: this
	// seam is the supported SOP cancellation path. The host seam must:
	//
	//   - stop only the job the caller names (the adapter-owned started work
	//     behind that job id), never unrelated or client-owned processes;
	//   - record the job's cancelled state in its persisted started-work
	//     record, kept distinct from failed and completed, and return nil
	//     only once the record reflects the outcome;
	//   - preserve the record's own outcome when the job finished first (the
	//     completion wins the race) and return nil in that case too;
	//   - return an error when the stop could not be confirmed within the
	//     bound, so the adapter reports the record as authoritative and never
	//     claims cancelled.
	//
	// Whether the owned child is reaped when the adapter process itself
	// terminates is owned by the run/started-work layer below this seam; this
	// contract documents the bound, it does not assume a supervisor.
	Cancel func(ctx context.Context, projectDir, jobID string) error
}

// PersistedWork is one persisted started-work record as the reconciliation
// step sees it.
type PersistedWork struct {
	JobID         string
	SubmissionKey string
	Fingerprint   string
	// State is the record's derived state: running, completed, failed, or
	// ended (the process is gone but no exit was recorded: an ambiguous end).
	State    string
	ExitCode *int
}

// Submission is one resolved prompt submission handed to the Starter.
type Submission struct {
	ProjectDir    string
	ProjectID     string
	Capability    string
	Text          string
	Fingerprint   string
	SubmissionKey string
}

// SubmissionResult reports one submission attempt's outcome with whatever
// identity the governed machinery can attribute to the work.
type SubmissionResult struct {
	Outcome string
	JobID   string
	// State and ExitCode are set for reconciled and already-running records.
	State    string
	ExitCode *int
	// TaskID and AttemptID are surfaced when the persisted state knows them.
	// The current started-work job records do not link the child run's task
	// and attempt identity, so those remain omitted today (documented gap;
	// owning layer: the started-work job record and the run it executes).
	TaskID    string
	AttemptID string
	Message   string
}

// PromptFingerprint is the durable fingerprint of a prompt submission. It
// MUST stay byte-for-byte the same construction the CLI started-work records
// use for `sop prompt` jobs (workFingerprint in internal/cli/mcp_start_work.go;
// internal/cli tests assert the equality): a fingerprint that matches across
// layers is how a session reconciles work another interface started.
//
// It is deliberately NOT project-scoped, because adding the project to the
// digest would break that cross-layer match against persisted records. Project
// scope comes from the layers around it: a reconciliation scan reads only the
// session's canonical project's persisted records, and the replay registry
// keys its entries by session id, so identical prompt text in two different
// projects can never start, match, or replay the other project's work item.
func PromptFingerprint(capability, text string) string {
	sum := sha256.New()
	for _, part := range []string{promptWorkKind, capability, strings.TrimSpace(text)} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// promptRequest is the pinned session/prompt params shape. Decoding is
// strict: unknown fields (for example image blocks with data or mime fields)
// are rejected before any work item can exist.
type promptRequest struct {
	// SessionID is the session the prompt belongs to.
	SessionID string `json:"sessionId"`
	// Prompt carries the content blocks. Only type "text" is accepted.
	Prompt []promptContent `json:"prompt"`
	// SubmissionKey is the caller's submission identity. Absent means the
	// work fingerprint becomes the key.
	SubmissionKey string `json:"submissionKey,omitempty"`
	// Capability optionally selects the prompt capability ("plan" if the
	// governed start path defaults); the starting layer owns the vocabulary.
	Capability string `json:"capability,omitempty"`
	// Project, when present, must canonicalize to the project the session is
	// bound to; a different project is rejected before anything starts.
	Project string `json:"project,omitempty"`
}

// promptContent is one content block. Only text is supported; a block that
// carries image, audio, or resource fields is rejected as unsupported content
// by strict decoding, before any work item is created.
type promptContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// handleSessionPrompt routes one text prompt through the governed submission
// path with the shared project lock. It never queues busy work and never
// starts a submission without first consulting persisted state.
func (s *Server) handleSessionPrompt(req *Request) (any, *Error) {
	if s.prompts == nil || s.prompts.missingSeam() {
		return nil, &Error{
			Code:    CodeCapabilityUnsupported,
			Message: "prompt submission is not configured on this server: the server host must supply the governed submission bridge",
		}
	}
	params, perr := decodePromptRequest(req.Params)
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
	if strings.TrimSpace(params.Project) != "" {
		requested, err := canonicalProjectDir(params.Project)
		if err != nil {
			return nil, &Error{Code: CodeInvalidParams, Message: "invalid session/prompt project: " + err.Error()}
		}
		if requested != sess.ProjectDir {
			return nil, &Error{
				Code: CodeInvalidParams,
				Message: fmt.Sprintf("session %s is bound to project %s (%s): a prompt for %q belongs to a session opened on that project",
					sess.ID, sess.ProjectID, sess.ProjectDir, shorten(params.Project, 128)),
			}
		}
	}
	text, terr := promptTextFrom(params.Prompt)
	if terr != nil {
		return nil, terr
	}
	// ACP-005 approval gate (approval.go): consult the host-supplied
	// question/approval read seam before any submission machinery runs.
	// Free-form chat text, an ACP permission grant, and a planner
	// recommendation are never approval; a response that does not match the
	// displayed question or approval identity is refused as non-authorizing
	// and leaves approval pending; a prompt that meets an unchanged approval
	// blocker stops here without retrying and without consuming an attempt
	// or retry budget. Continuation after external resolution runs only on a
	// fresh authoritative read through this seam.
	if res, rerr := s.approvalGate(sess, text); res != nil || rerr != nil {
		return res, rerr
	}
	capability := strings.ToLower(strings.TrimSpace(params.Capability))
	if capability == "" {
		// The governed start path runs a capability even when the client names
		// none; taking the same default here keeps the fingerprint and the
		// recorded capability identical to the machinery that starts the work.
		capability = promptDefaultCapability
	}
	fingerprint := PromptFingerprint(capability, text)
	key := strings.TrimSpace(params.SubmissionKey)
	if len(key) > MaxSubmissionKeyLength {
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: fmt.Sprintf("submissionKey exceeds %d bytes: send a shorter idempotency key", MaxSubmissionKeyLength),
		}
	}
	if key == "" {
		key = fingerprint
	}
	sub := Submission{
		ProjectDir:    sess.ProjectDir,
		ProjectID:     sess.ProjectID,
		Capability:    capability,
		Text:          text,
		Fingerprint:   fingerprint,
		SubmissionKey: key,
	}
	// In-adapter duplicate prevention, keyed by session and submission key —
	// never by the JSON-RPC id. A replayed key returns its recorded result
	// verbatim, so a resent submission cannot create a second work item.
	if recorded, ok := s.replaySubmission(sess.ID, key); ok {
		out := recorded.toResultMap()
		out["replayed"] = true
		return out, nil
	}
	// The project lock probe first: a held lock is a busy result, and the
	// submission is neither started nor queued.
	release, err := s.prompts.LockTaker(context.Background(), sess.ProjectDir)
	if err != nil {
		if errors.Is(err, ErrProjectBusy) {
			return busyLockResult(sub), nil
		}
		return nil, &Error{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("check the project writer lock for %s: %v", shorten(sess.ProjectDir, 128), err),
		}
	}
	defer release()
	// Reconcile against persisted state BEFORE any new submission, so a
	// failed or ambiguous start reports the persisted job identity instead
	// of a duplicate work item.
	records, err := s.prompts.StateReader(sess.ProjectDir)
	if err != nil {
		return nil, &Error{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("read persisted work for %s: %v", shorten(sess.ProjectDir, 128), err),
		}
	}
	// Pass 1: the submission's own running record. Scan every record before
	// declaring a foreign holder, so the client's own running job is reported
	// even when a foreign record sorts earlier in the persisted list.
	for _, w := range records {
		if w.State == persistedStateRunning && matchesWork(w, sub) {
			res := SubmissionResult{
				Outcome: WorkOutcomeAlreadyRunning,
				JobID:   w.JobID,
				State:   w.State,
				Message: "this work is already running; use its job id to follow it instead of starting it again",
			}
			// Derived from persisted state, never recorded in the replay
			// registry: a resend re-reads persisted state and reports the
			// record's CURRENT state instead of a snapshot that can go stale
			// (a running job can finish; a failed job can be recovered).
			s.setSessionJob(sess.ID, w.JobID)
			return res.toResultMap(), nil
		}
	}
	// Pass 2: a foreign running record holds the project. A useful busy result
	// names the holder; nothing was started or queued.
	for _, w := range records {
		if w.State == persistedStateRunning {
			return busyHolderResult(sub, w.JobID), nil
		}
	}
	// Pass 3: a matching finished or ambiguous record from an earlier start.
	// Report the persisted job identity instead of starting duplicate work.
	for _, w := range records {
		if matchesWork(w, sub) {
			res := SubmissionResult{
				Outcome:  WorkOutcomeReconciled,
				JobID:    w.JobID,
				State:    w.State,
				ExitCode: w.ExitCode,
				Message:  reconcileMessage(w.State),
			}
			// Re-derived on every resend, so the never-recorded result reports
			// the record's current state rather than a stale earlier one.
			s.setSessionJob(sess.ID, w.JobID)
			return res.toResultMap(), nil
		}
	}
	// Persisted state offers no prior attempt for this submission identity:
	// start governed work exactly once. A failed start is NOT retried here;
	// a client resend consults persisted state again first.
	res, err := s.prompts.Starter(sub)
	if err != nil {
		return nil, &Error{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("start governed prompt: %v", err),
		}
	}
	switch res.Outcome {
	case WorkOutcomeStarted:
		if strings.TrimSpace(res.JobID) == "" {
			// A started work item without a job id leaves the client nothing to
			// reconcile after a disconnect, so treat it as a failed start:
			// record nothing and report the failure. A resend consults the
			// persisted started-work records first, so the started item's own
			// durable record still answers the resend.
			return nil, &Error{
				Code:    CodeInternalError,
				Message: "governed start created work without a job identity: reconcile the project's persisted started-work records before resending this submission",
			}
		}
		s.recordSubmission(sess.ID, key, res)
		s.setSessionJob(sess.ID, res.JobID)
		return res.toResultMap(), nil
	case WorkOutcomeAlreadyRunning, WorkOutcomeReconciled:
		// Derived from persisted state; never recorded in the replay registry,
		// so a resend re-consults persisted state instead of replaying a
		// possibly stale snapshot.
		s.setSessionJob(sess.ID, res.JobID)
		return res.toResultMap(), nil
	case WorkOutcomeBusy, WorkOutcomeTaskInProgress:
		// Definitively not started; leave the key unrecorded so the client can
		// honestly resend it once the project is idle.
		return res.toResultMap(), nil
	case WorkOutcomeNotInitialized:
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: "project is not initialized: SOP needs its state directory in " + shorten(sess.ProjectDir, 128) + "; initialize the project first",
		}
	default:
		return nil, &Error{
			Code:    CodeInternalError,
			Message: fmt.Sprintf("submission returned unexpected outcome %q", shorten(res.Outcome, 64)),
		}
	}
}

// decodePromptRequest decodes session/prompt params strictly: unknown fields
// and trailing data are rejected.
func decodePromptRequest(raw []byte) (promptRequest, *Error) {
	var params promptRequest
	if len(raw) == 0 {
		return params, &Error{
			Code:    CodeInvalidParams,
			Message: "session/prompt requires params: {sessionId, prompt} with text content blocks",
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&params); err != nil {
		return promptRequest{}, &Error{
			Code: CodeInvalidParams,
			Message: fmt.Sprintf("invalid session/prompt params: %v; expected fields are sessionId and prompt (text content blocks), plus optional submissionKey, capability, and project",
				shorten(err.Error(), 128)),
		}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return promptRequest{}, &Error{
			Code:    CodeInvalidParams,
			Message: "invalid session/prompt params: unexpected trailing data; send one JSON object",
		}
	}
	return params, nil
}

// promptTextFrom extracts and validates the concatenated text. Unsupported
// content and empty text are rejected here, before any submission can start.
func promptTextFrom(blocks []promptContent) (string, *Error) {
	if len(blocks) == 0 {
		return "", &Error{
			Code:    CodeInvalidParams,
			Message: "session/prompt needs prompt content: pass at least one {type:\"text\"} block",
		}
	}
	parts := make([]string, 0, len(blocks))
	total := 0
	for i, b := range blocks {
		switch {
		case b.Type == "":
			return "", &Error{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("content block %d is missing its type", i),
			}
		case b.Type != "text":
			return "", &Error{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("content block %d has type %q: only text content is supported in session/prompt", i, shorten(b.Type, 32)),
			}
		}
		total += len(b.Text)
		if total > MaxPromptSubmitBytes {
			return "", &Error{
				Code:    CodeInvalidParams,
				Message: fmt.Sprintf("prompt text exceeds the %d-byte bound: send a shorter prompt", MaxPromptSubmitBytes),
			}
		}
		parts = append(parts, b.Text)
	}
	text := strings.Join(parts, "\n")
	if strings.TrimSpace(text) == "" {
		return "", &Error{
			Code:    CodeInvalidParams,
			Message: "prompt text is empty: pass at least one text content block with text",
		}
	}
	return text, nil
}

// matchesWork reports whether a persisted record describes the same
// submission, by key when the record stored one, or by work fingerprint.
func matchesWork(w PersistedWork, sub Submission) bool {
	return (w.SubmissionKey != "" && w.SubmissionKey == sub.SubmissionKey) ||
		(w.Fingerprint != "" && w.Fingerprint == sub.Fingerprint)
}

// reconcileMessage explains what persisted state says and what a genuinely
// new attempt would take.
func reconcileMessage(state string) string {
	switch state {
	case persistedStateEnded:
		return "a previous attempt for this submission ended without recording its exit: reconcile this job identity against the persisted run state; a new submissionKey starts fresh work"
	case persistedStateCompleted:
		return "this work already completed under this job identity; a new submissionKey starts fresh work"
	case persistedStateCancelled:
		return "this work was cancelled under this job identity; a new submissionKey starts fresh work"
	case persistedStateFailed:
		return "this work already ran and failed under this job identity; a new submissionKey starts fresh work"
	default:
		return "a previous attempt for this submission is recorded under this job identity (state " + state + "); a new submissionKey starts fresh work"
	}
}

// busyLockResult is the useful busy result for a held project writer lock: it
// names the lock scope and says that nothing was started or queued.
func busyLockResult(sub Submission) map[string]any {
	return map[string]any{
		"outcome":   WorkOutcomeBusy,
		"condition": "project_lock_held",
		"project":   sub.ProjectID,
		"message":   "the project writer lock is held by a competing writer (a CLI, MCP, or ACP start); nothing was started or queued; resend the submission when the project is idle",
	}
}

// busyHolderResult is the useful busy result when another started job still
// runs in the project: it names the holder so the client can follow it.
func busyHolderResult(sub Submission, holderJobID string) map[string]any {
	out := map[string]any{
		"outcome":   WorkOutcomeBusy,
		"condition": "work_in_progress",
		"project":   sub.ProjectID,
		"message":   "another started job is still running in this project; wait for it to finish before starting new work; nothing was queued",
	}
	if holderJobID != "" {
		out["holderJobId"] = holderJobID
	}
	return out
}

// toResultMap is the protocol-facing result document.
func (r SubmissionResult) toResultMap() map[string]any {
	out := map[string]any{"outcome": r.Outcome}
	if r.JobID != "" {
		out["jobId"] = r.JobID
	}
	if r.State != "" {
		out["jobState"] = r.State
	}
	if r.ExitCode != nil {
		out["exitCode"] = *r.ExitCode
	}
	if r.TaskID != "" {
		out["taskId"] = r.TaskID
	}
	if r.AttemptID != "" {
		out["attemptId"] = r.AttemptID
	}
	if r.Message != "" {
		out["message"] = r.Message
	}
	return out
}

// missingSeam reports whether any bridge seam is unconfigured; the submission
// path requires all three.
func (p *PromptSubmission) missingSeam() bool {
	return p.Starter == nil || p.LockTaker == nil || p.StateReader == nil
}

// replaySubmission returns the recorded result for the session and submission
// key, reporting whether one exists. The JSON-RPC id never enters this key.
func (s *Server) replaySubmission(sessionID, key string) (SubmissionResult, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, ok := s.submissions[submissionsRegistryKey(sessionID, key)]
	return res, ok
}

// recordSubmission records the definitive result of one submission (a start,
// an already-running report, or a reconciliation) so a resent key replays it
// instead of starting duplicate work. Busy results are never recorded: no
// work was created, so an honest resend may retry the key.
func (s *Server) recordSubmission(sessionID, key string, res SubmissionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := submissionsRegistryKey(sessionID, key)
	if _, exists := s.submissions[k]; !exists {
		s.submissionOrder = append(s.submissionOrder, k)
		for len(s.submissionOrder) > submissionRegistryLimit {
			old := s.submissionOrder[0]
			s.submissionOrder = s.submissionOrder[1:]
			delete(s.submissions, old)
		}
	}
	s.submissions[k] = res
}

// submissionsRegistryKey namespaces the replay registry entry by session so
// two sessions never share a submission key.
func submissionsRegistryKey(sessionID, key string) string {
	return sessionID + "\x00" + key
}

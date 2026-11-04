package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file is ACP-006's deterministic acceptance coverage. TestCancel drives
// session/cancel through the real dispatch surface with a fake host cancel seam,
// so it proves the adapter contract without signalling a process, starting a
// child, or touching SOP state:
//
//   - cancel reaches exactly the job the session started, through the seam;
//   - a job that finished before the stop keeps its own outcome (no false
//     "cancelled");
//   - nothing to cancel is reported honestly, and a session with no work cannot
//     cancel another session's job;
//   - a missing seam refuses the capability instead of claiming support;
//   - the id-less notification form of session/cancel is processed with no
//     response, and a seam failure is a protocol error rather than a silent pass.
//
// Traced contract map (S1): handleSessionCancel (cancel.go) resolves the
// session, reads the foreground job from Server.sessionJob, and calls
// PromptSubmission.Cancel (prompt.go) under one CancelStopBound context; the
// seam contract returns nil only once the job's persisted record reflects the
// stop and preserves a finished-first outcome, and an error means the stop is
// unconfirmed. The response reports the read-back from StateReader
// (Server.persistedJobState): persistedStateCancelled is recorded distinctly
// from failed and completed, and the protocol stop reason StopReasonCancelled
// is claimed only when the record carries it, never when the job finished
// first (that turn keeps StopReasonEndTurn). The clean-EOF disconnect policy
// is enforced at Serve's io.EOF return (transport.go), which runs
// Server.reconcileDisconnect (cancel.go): per-session Cancel under
// CancelStopBound, a persisted-record re-read via StateReader, no restart, no
// fabricated outcome, no second lifecycle event, and a silent skip when the
// seam is absent. handleSessionCancel's own flow was unchanged; the additions
// are the seam helpers (missingCancelSeam, persistedJobState) and the EOF
// entry point (reconcileDisconnect) behind Serve's io.EOF return.
//
// The S1 gap note's three missing subtests are now covered below: the EOF
// hook (clean io.EOF in Serve -> reconcileDisconnect), the
// cancellation-vs-completion race at the seam's stop hook, and restart
// discovery without automatic resubmission.

// cancelBridge is a PromptSubmission whose cancel seam records calls and can be
// made to resolve a job as finished. It also counts StateReader reads, Starter
// calls, and how many seam calls carried the CancelStopBound deadline, and it
// takes an optional stop hook that applies the stop to the job's record before
// the adapter's read-back — the deterministic seam hook behind the
// cancel-vs-completion race and the EOF reconciliation subtests.
type cancelBridge struct {
	mu         sync.Mutex
	cancels    []string
	cancelErr  error
	records    []PersistedWork
	onCancel   func() // optional stop hook, run before the seam returns nil
	reads      int
	starts     int
	boundedCtx int
	nextJob    int
}

func (b *cancelBridge) submission(withCancel bool) *PromptSubmission {
	sub := &PromptSubmission{
		LockTaker: func(context.Context, string) (func(), error) { return func() {}, nil },
		StateReader: func(string) ([]PersistedWork, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.reads++
			return append([]PersistedWork(nil), b.records...), nil
		},
		Starter: func(sub Submission) (SubmissionResult, error) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.starts++
			b.nextJob++
			return SubmissionResult{
				Outcome: WorkOutcomeStarted,
				JobID:   fmt.Sprintf("job-%d", b.nextJob),
			}, nil
		},
	}
	if withCancel {
		sub.Cancel = func(ctx context.Context, _ string, jobID string) error {
			b.mu.Lock()
			b.cancels = append(b.cancels, jobID)
			if deadline, ok := ctx.Deadline(); ok {
				if remaining := time.Until(deadline); remaining > 0 && remaining <= CancelStopBound {
					b.boundedCtx++
				}
			}
			err := b.cancelErr
			b.mu.Unlock()
			// The host seam applies the stop before it returns, exactly as its
			// contract requires, so the race edge is deterministic here.
			if hook := b.onCancel; hook != nil {
				hook()
			}
			return err
		}
	}
	return sub
}

// readCount reports how many times StateReader ran, so the EOF reconciliation
// read-back is observable.
func (b *cancelBridge) readCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reads
}

// startCount reports how many times Starter ran, so "no automatic
// resubmission" is observable.
func (b *cancelBridge) startCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.starts
}

// boundedContexts reports how many seam calls ran under a bounded
// (CancelStopBound) context.
func (b *cancelBridge) boundedContexts() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.boundedCtx
}

func (b *cancelBridge) setRecords(records ...PersistedWork) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.records = records
}

func (b *cancelBridge) cancelled() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.cancels...)
}

// startPrompt starts one governed job for the session and returns its job id.
func startPrompt(t *testing.T, h *promptHarness, dir string) (sessionID, jobID string) {
	t.Helper()
	sessionID = h.openSession(dir)
	res, rerr := h.prompt(sessionID, "explain the governed workflow", map[string]any{"submissionKey": "k-" + sessionID})
	if rerr != nil {
		t.Fatalf("prompt: %+v", rerr)
	}
	jobID, _ = res["jobId"].(string)
	if jobID == "" {
		t.Fatalf("prompt started no job: %v", res)
	}
	return sessionID, jobID
}

func outcomeString(t *testing.T, res map[string]any) string {
	t.Helper()
	outcome, _ := res["outcome"].(string)
	return outcome
}

// TestCancel is ACP-006's deterministic acceptance test.
func TestCancel(t *testing.T) {
	t.Run("cancels exactly the job the session started", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		sess, job := startPrompt(t, h, t.TempDir())
		b.setRecords(PersistedWork{JobID: job, State: persistedStateCancelled})

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
		if got.Error != nil {
			t.Fatalf("session/cancel: %+v", got.Error)
		}
		var res map[string]any
		if err := json.Unmarshal(got.Result, &res); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if outcomeString(t, res) != WorkOutcomeCancelled {
			t.Errorf("outcome = %q, want %q", outcomeString(t, res), WorkOutcomeCancelled)
		}
		if res["stopReason"] != StopReasonCancelled {
			t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonCancelled)
		}
		if res["jobId"] != job {
			t.Errorf("jobId = %v, want %q", res["jobId"], job)
		}
		if calls := b.cancelled(); len(calls) != 1 || calls[0] != job {
			t.Errorf("cancel seam calls = %v, want [%s]", calls, job)
		}
	})

	t.Run("nothing to cancel is reported honestly", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		sess := h.openSession(t.TempDir())

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
		if got.Error != nil {
			t.Fatalf("session/cancel: %+v", got.Error)
		}
		var res map[string]any
		if err := json.Unmarshal(got.Result, &res); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if outcomeString(t, res) != WorkOutcomeNotRunning {
			t.Errorf("outcome = %q, want %q", outcomeString(t, res), WorkOutcomeNotRunning)
		}
		if res["stopReason"] != StopReasonEndTurn {
			t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonEndTurn)
		}
		if calls := b.cancelled(); len(calls) != 0 {
			t.Errorf("cancel seam calls = %v, want none", calls)
		}
	})

	t.Run("a job that finished first keeps its recorded outcome", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		sess, job := startPrompt(t, h, t.TempDir())
		b.setRecords(PersistedWork{JobID: job, State: persistedStateCompleted})

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
		if got.Error != nil {
			t.Fatalf("session/cancel: %+v", got.Error)
		}
		var res map[string]any
		if err := json.Unmarshal(got.Result, &res); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if outcomeString(t, res) != WorkOutcomeNotRunning {
			t.Errorf("outcome = %q, want %q (a completed job must not be relabelled)", outcomeString(t, res), WorkOutcomeNotRunning)
		}
		if res["stopReason"] != StopReasonEndTurn {
			t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonEndTurn)
		}
		if res["jobState"] != persistedStateCompleted {
			t.Errorf("jobState = %v, want %q", res["jobState"], persistedStateCompleted)
		}
	})

	t.Run("sessions cannot cancel each other's work", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		sessA, jobA := startPrompt(t, h, t.TempDir())
		sessB, jobB := startPrompt(t, h, t.TempDir())
		if jobA == jobB {
			t.Fatalf("both sessions recorded the same job %q", jobA)
		}
		b.setRecords(PersistedWork{JobID: jobA, State: persistedStateCancelled})

		if got := h.call(MethodSessionCancel, map[string]any{"sessionId": sessA}); got.Error != nil {
			t.Fatalf("cancel A: %+v", got.Error)
		}
		if calls := b.cancelled(); len(calls) != 1 || calls[0] != jobA {
			t.Errorf("cancel seam calls = %v, want only [%s]", calls, jobA)
		}

		// Session B still owns its own job and cancel targets that one.
		b.setRecords(PersistedWork{JobID: jobB, State: persistedStateCancelled})
		if got := h.call(MethodSessionCancel, map[string]any{"sessionId": sessB}); got.Error != nil {
			t.Fatalf("cancel B: %+v", got.Error)
		}
		if calls := b.cancelled(); len(calls) != 2 || calls[1] != jobB {
			t.Errorf("cancel seam calls = %v, want [%s %s]", calls, jobA, jobB)
		}
	})

	t.Run("an unconfigured cancel seam refuses the capability", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(false))
		sess, _ := startPrompt(t, h, t.TempDir())

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
		if got.Error == nil || got.Error.Code != CodeCapabilityUnsupported {
			t.Fatalf("error = %+v, want code %d", got.Error, CodeCapabilityUnsupported)
		}
		if calls := b.cancelled(); len(calls) != 0 {
			t.Errorf("cancel seam calls = %v, want none", calls)
		}
	})

	t.Run("an unknown session is rejected", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		_ = h.openSession(t.TempDir())

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": "sess_missing"})
		if got.Error == nil || got.Error.Code != CodeInvalidParams {
			t.Fatalf("error = %+v, want code %d", got.Error, CodeInvalidParams)
		}
	})

	t.Run("malformed params are rejected before any stop", func(t *testing.T) {
		cases := map[string]string{
			"missing sessionId": `{}`,
			"unknown field":     `{"sessionId":"sess_x","bogus":true}`,
			"wrong type":        `{"sessionId":42}`,
			// session/cancel names the session only. An identity field is not
			// accepted silently: strict decoding rejects it before any stop, so
			// a client that tries to cancel a specific job learns the shape.
			"jobId is not accepted": `{"sessionId":"sess_x","jobId":"work-20260101-000000"}`,
		}
		for name, params := range cases {
			t.Run(name, func(t *testing.T) {
				b := &cancelBridge{}
				h := newPromptHarness(t, b.submission(true))
				_ = h.openSession(t.TempDir())
				got := h.send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": MethodSessionCancel, "params": json.RawMessage(params)})
				if got.Error == nil || got.Error.Code != CodeInvalidParams {
					t.Fatalf("error = %+v, want code %d", got.Error, CodeInvalidParams)
				}
				if calls := b.cancelled(); len(calls) != 0 {
					t.Errorf("cancel seam calls = %v, want none", calls)
				}
			})
		}
	})

	t.Run("a seam failure is an error, never a claimed cancellation", func(t *testing.T) {
		b := &cancelBridge{cancelErr: errors.New("stop could not be confirmed")}
		h := newPromptHarness(t, b.submission(true))
		sess, _ := startPrompt(t, h, t.TempDir())

		got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
		if got.Error == nil || got.Error.Code != CodeInternalError {
			t.Fatalf("error = %+v, want code %d", got.Error, CodeInternalError)
		}
		if got.Result != nil {
			t.Errorf("a failed cancellation returned a result: %s", got.Result)
		}
	})

	t.Run("the notification form cancels without a response", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		sess, job := startPrompt(t, h, t.TempDir())
		b.setRecords(PersistedWork{JobID: job, State: persistedStateCancelled})

		before := h.out.Len()
		frame := map[string]any{
			"jsonrpc": "2.0",
			"method":  MethodSessionCancel,
			"params":  map[string]any{"sessionId": sess},
		}
		data, err := json.Marshal(frame)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		h.server.handleFrame(data)
		if got := h.out.String()[before:]; got != "" {
			t.Errorf("the notification produced a response frame: %q", got)
		}
		if calls := b.cancelled(); len(calls) != 1 || calls[0] != job {
			t.Errorf("cancel seam calls = %v, want [%s]", calls, job)
		}
	})

	t.Run("other notifications stay no-ops", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		_ = h.openSession(t.TempDir())

		before := h.out.Len()
		data := []byte(`{"jsonrpc":"2.0","method":"session/update","params":{"x":1}}`)
		h.server.handleFrame(data)
		if got := h.out.String()[before:]; got != "" {
			t.Errorf("an unknown notification produced a response frame: %q", got)
		}
		if calls := b.cancelled(); len(calls) != 0 {
			t.Errorf("cancel seam calls = %v, want none", calls)
		}
	})

	t.Run("clean EOF cancels the foreground job and reconciles", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(true))
		_, job := startPrompt(t, h, t.TempDir())
		b.setRecords(PersistedWork{JobID: job, State: persistedStateCancelled})
		readsBefore, startsBefore := b.readCount(), b.startCount()
		outBefore, diagBefore := h.out.Len(), h.errOut.Len()

		// A client that closes stdin is a clean EOF: Serve must request safe
		// cancellation of the session's foreground job through the same seam,
		// reconcile the final persisted state, and return nil. It writes no
		// frames, so no job receives a second lifecycle event.
		if err := h.server.Serve(context.Background(), strings.NewReader("")); err != nil {
			t.Fatalf("Serve on clean EOF: %v (want nil)", err)
		}
		if calls := b.cancelled(); len(calls) != 1 || calls[0] != job {
			t.Errorf("cancel seam calls on EOF = %v, want [%s]", calls, job)
		}
		if got := b.boundedContexts(); got != 1 {
			t.Errorf("bounded cancel contexts on EOF = %d, want 1 (the EOF stop runs under CancelStopBound)", got)
		}
		if got := b.readCount(); got != readsBefore+1 {
			t.Errorf("StateReader reads = %d, want %d (one reconciling read-back after the stop)", got, readsBefore+1)
		}
		if got := b.startCount(); got != startsBefore {
			t.Errorf("Starter calls on EOF = %d, want %d (a disconnect never restarts work)", got, startsBefore)
		}
		if got := h.out.Len(); got != outBefore {
			t.Errorf("EOF recovery wrote %d protocol bytes; the output buffer must be unchanged", got-outBefore)
		}
		if diag := h.errOut.String()[diagBefore:]; !strings.Contains(diag, "disconnect recovery") {
			t.Errorf("diagnostics %q do not record the reconciled final state", diag)
		}

		t.Run("a seam error on EOF is a diagnostic, never a claimed cancellation", func(t *testing.T) {
			b := &cancelBridge{cancelErr: errors.New("stop could not be confirmed")}
			h := newPromptHarness(t, b.submission(true))
			_, job := startPrompt(t, h, t.TempDir())
			b.setRecords(PersistedWork{JobID: job, State: persistedStateRunning})
			outBefore, diagBefore := h.out.Len(), h.errOut.Len()

			if err := h.server.Serve(context.Background(), strings.NewReader("")); err != nil {
				t.Fatalf("Serve on clean EOF: %v (want nil)", err)
			}
			if calls := b.cancelled(); len(calls) != 1 || calls[0] != job {
				t.Errorf("cancel seam calls on EOF = %v, want [%s]", calls, job)
			}
			if got := h.out.Len(); got != outBefore {
				t.Errorf("EOF recovery wrote %d protocol bytes; the output buffer must be unchanged", got-outBefore)
			}
			if diag := h.errOut.String()[diagBefore:]; !strings.Contains(diag, "no cancelled outcome is claimed") {
				t.Errorf("diagnostics %q do not record the unconfirmed stop", diag)
			}
		})
	})

	t.Run("cancellation racing normal completion produces one authoritative result", func(t *testing.T) {
		t.Run("completion lands during the stop", func(t *testing.T) {
			b := &cancelBridge{}
			h := newPromptHarness(t, b.submission(true))
			sess, job := startPrompt(t, h, t.TempDir())
			b.onCancel = func() {
				// The job finished while the stop was being applied: its own
				// recorded outcome is the one authoritative result.
				b.setRecords(PersistedWork{JobID: job, State: persistedStateCompleted})
			}

			got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
			if got.Error != nil {
				t.Fatalf("session/cancel: %+v", got.Error)
			}
			var res map[string]any
			if err := json.Unmarshal(got.Result, &res); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if outcomeString(t, res) != WorkOutcomeNotRunning {
				t.Errorf("outcome = %q, want %q (a completed record must not be relabelled cancelled)", outcomeString(t, res), WorkOutcomeNotRunning)
			}
			if res["stopReason"] != StopReasonEndTurn {
				t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonEndTurn)
			}
			if res["jobState"] != persistedStateCompleted {
				t.Errorf("jobState = %v, want %q", res["jobState"], persistedStateCompleted)
			}
			if res["jobId"] != job {
				t.Errorf("jobId = %v, want %q: one job, one authoritative result", res["jobId"], job)
			}
		})

		t.Run("the stop lands first", func(t *testing.T) {
			b := &cancelBridge{}
			h := newPromptHarness(t, b.submission(true))
			sess, job := startPrompt(t, h, t.TempDir())
			b.onCancel = func() {
				// The stop was confirmed before the job could finish.
				b.setRecords(PersistedWork{JobID: job, State: persistedStateCancelled})
			}

			got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
			if got.Error != nil {
				t.Fatalf("session/cancel: %+v", got.Error)
			}
			var res map[string]any
			if err := json.Unmarshal(got.Result, &res); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if outcomeString(t, res) != WorkOutcomeCancelled {
				t.Errorf("outcome = %q, want %q", outcomeString(t, res), WorkOutcomeCancelled)
			}
			if res["stopReason"] != StopReasonCancelled {
				t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonCancelled)
			}
			if res["jobState"] != persistedStateCancelled {
				t.Errorf("jobState = %v, want %q", res["jobState"], persistedStateCancelled)
			}
			if res["jobId"] != job {
				t.Errorf("jobId = %v, want %q: one job, one authoritative result", res["jobId"], job)
			}
		})
	})

	t.Run("restart discovers persisted job status without resubmission", func(t *testing.T) {
		b := &cancelBridge{}
		h := newPromptHarness(t, b.submission(false))
		fp := PromptFingerprint(promptDefaultCapability, "explain the governed workflow")
		b.setRecords(PersistedWork{
			JobID:         "job-old",
			SubmissionKey: "k-restart",
			Fingerprint:   fp,
			State:         persistedStateCancelled,
		})
		sess := h.openSession(t.TempDir())

		// A restart is a fresh session against the same project: the persisted
		// record for this submission identity answers the prompt. Nothing is
		// resubmitted and nothing is re-cancelled.
		res, rerr := h.prompt(sess, "explain the governed workflow", map[string]any{"submissionKey": "k-restart"})
		if rerr != nil {
			t.Fatalf("restart prompt: %+v", rerr)
		}
		if outcomeString(t, res) != WorkOutcomeReconciled {
			t.Errorf("outcome = %q, want %q", outcomeString(t, res), WorkOutcomeReconciled)
		}
		if res["stopReason"] != StopReasonEndTurn {
			t.Errorf("stopReason = %v, want %q", res["stopReason"], StopReasonEndTurn)
		}
		if got, _ := res["jobId"].(string); got != "job-old" {
			t.Errorf("jobId = %v, want the persisted job-old", res["jobId"])
		}
		if got, _ := res["jobState"].(string); got != persistedStateCancelled {
			t.Errorf("jobState = %v, want the persisted %q", res["jobState"], persistedStateCancelled)
		}
		if msg, _ := res["message"].(string); !strings.Contains(msg, "cancelled") {
			t.Errorf("message %q does not name the persisted cancellation", msg)
		}
		if got := b.startCount(); got != 0 {
			t.Errorf("Starter calls = %d, want 0: a restart must never automatically resubmit persisted work", got)
		}
		if calls := b.cancelled(); len(calls) != 0 {
			t.Errorf("cancel seam calls = %v, want none (discovery reports state, it does not re-cancel)", calls)
		}
	})
}

// TestCancelIsNotAdvertisedWithoutASeam proves the capability advertisement and
// the behaviour agree: initialize lists session/cancel only because the build
// implements it, and a server built without the seam refuses the method with the
// capability reason instead of a generic not-found.
func TestCancelIsNotAdvertisedWithoutASeam(t *testing.T) {
	if !slices.Contains(SupportedMethods, MethodSessionCancel) {
		t.Fatalf("SupportedMethods %v does not include %q", SupportedMethods, MethodSessionCancel)
	}
	b := &cancelBridge{}
	h := newPromptHarness(t, b.submission(false))
	sess, _ := startPrompt(t, h, t.TempDir())
	got := h.call(MethodSessionCancel, map[string]any{"sessionId": sess})
	if got.Error == nil || got.Error.Code != CodeCapabilityUnsupported {
		t.Fatalf("error = %+v, want code %d", got.Error, CodeCapabilityUnsupported)
	}
	if !strings.Contains(got.Error.Message, "cancel") {
		t.Errorf("message %q does not name cancellation", got.Error.Message)
	}
}

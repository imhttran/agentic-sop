package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is ACP-003's deterministic acceptance coverage. TestPromptSubmission
// drives the real Serve dispatch surface through the governed-submission bridge
// with fakes, so it proves the adapter's own contract without a model, network,
// child process, or SQLite database:
//
//   - one accepted submission creates exactly one work item, and a resent
//     submission (same key) is replayed rather than restarted;
//   - persisted state is consulted before any start, so an already-running,
//     failed, or ambiguous earlier attempt is reconciled by identity;
//   - a contended project lock is a busy result with no start and no queueing;
//   - unsupported content and a foreign project are rejected before any side
//     effect;
//   - sessions stay isolated per canonical project;
//   - a JSON-RPC id is not an idempotency key.

// fakeBridge records how the adapter used the governed submission bridge.
type fakeBridge struct {
	lockErr   error
	records   []PersistedWork
	start     SubmissionResult
	startErr  error
	startHook func(sub Submission)
}

type recordedCalls struct {
	bridges *fakeBridge
	locks   int
	reads   int
	starts  int
	subs    []Submission
}

// bridge builds a PromptSubmission over the recorder. Every seam counts its use
// so a rejected request can be proven to have no side effect.
func (r *recordedCalls) bridge() *PromptSubmission {
	return &PromptSubmission{
		LockTaker: func(ctx context.Context, projectDir string) (func(), error) {
			r.locks++
			if r.bridges.lockErr != nil {
				return nil, r.bridges.lockErr
			}
			return func() {}, nil
		},
		StateReader: func(projectDir string) ([]PersistedWork, error) {
			r.reads++
			return r.bridges.records, nil
		},
		Starter: func(sub Submission) (SubmissionResult, error) {
			r.starts++
			r.subs = append(r.subs, sub)
			if r.bridges.startHook != nil {
				r.bridges.startHook(sub)
			}
			if r.bridges.startErr != nil {
				return SubmissionResult{}, r.bridges.startErr
			}
			return r.bridges.start, nil
		},
	}
}

// promptHarness dispatches requests directly through one Server and decodes each
// response, so a test can use the session id the server assigned. errOut keeps
// the server's diagnostics, so a test can assert an actionable diagnostic was
// recorded without touching the protocol stream.
type promptHarness struct {
	t      *testing.T
	server *Server
	out    *bytes.Buffer
	errOut *bytes.Buffer
	seq    int
}

func newPromptHarness(t *testing.T, bridge *PromptSubmission) *promptHarness {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	var opts []ServerOption
	if bridge != nil {
		opts = append(opts, WithPromptSubmission(bridge))
	}
	return &promptHarness{t: t, server: NewServer("test-version", out, errOut, opts...), out: out, errOut: errOut}
}

func (h *promptHarness) send(frame map[string]any) response {
	h.t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		h.t.Fatalf("marshal request: %v", err)
	}
	before := h.out.Len()
	h.server.handleFrame(data)
	chunk := strings.TrimSuffix(h.out.String()[before:], "\n")
	if chunk == "" {
		h.t.Fatalf("no response frame for %v", frame)
	}
	lines := strings.Split(chunk, "\n")
	if len(lines) != 1 {
		h.t.Fatalf("got %d frames for one request: %q", len(lines), chunk)
	}
	var r response
	if err := json.Unmarshal([]byte(lines[0]), &r); err != nil {
		h.t.Fatalf("decode response %q: %v", lines[0], err)
	}
	return r
}

func (h *promptHarness) call(method string, params map[string]any) response {
	h.t.Helper()
	h.seq++
	frame := map[string]any{"jsonrpc": "2.0", "id": h.seq, "method": method}
	if params != nil {
		frame["params"] = params
	}
	return h.send(frame)
}

// openSession initializes the server and opens one session on dir, returning the
// session id the server assigned.
func (h *promptHarness) openSession(dir string) string {
	h.t.Helper()
	if got := h.call(MethodInitialize, map[string]any{"protocolVersion": ProtocolVersion}); got.Error != nil {
		h.t.Fatalf("initialize: %+v", got.Error)
	}
	got := h.call(MethodSessionNew, map[string]any{"cwd": dir})
	if got.Error != nil {
		h.t.Fatalf("session/new: %+v", got.Error)
	}
	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(got.Result, &res); err != nil {
		h.t.Fatalf("decode session/new: %v", err)
	}
	return res.SessionID
}

// prompt submits one text prompt and returns the decoded result.
func (h *promptHarness) prompt(sessionID, text string, extra map[string]any) (map[string]any, *Error) {
	h.t.Helper()
	params := map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}
	for k, v := range extra {
		params[k] = v
	}
	got := h.call(MethodSessionPrompt, params)
	if got.Error != nil {
		return nil, got.Error
	}
	var res map[string]any
	if err := json.Unmarshal(got.Result, &res); err != nil {
		h.t.Fatalf("decode session/prompt: %v", err)
	}
	return res, nil
}

func outcomeOf(t *testing.T, res map[string]any) string {
	t.Helper()
	outcome, _ := res["outcome"].(string)
	return outcome
}

// TestPromptSubmission is ACP-003's deterministic acceptance test.
func TestPromptSubmission(t *testing.T) {
	t.Run("one accepted submission creates one work item and replays on resend", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		dir := t.TempDir()
		sess := h.openSession(dir)

		res, rerr := h.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k1"})
		if rerr != nil {
			t.Fatalf("prompt: %+v", rerr)
		}
		if got := outcomeOf(t, res); got != WorkOutcomeStarted {
			t.Fatalf("outcome = %q, want %q", got, WorkOutcomeStarted)
		}
		if got, _ := res["jobId"].(string); got != "job-1" {
			t.Errorf("jobId = %q, want job-1", got)
		}
		if calls.starts != 1 {
			t.Fatalf("Starter calls = %d, want 1", calls.starts)
		}

		// The same submission key is replayed from the adapter registry: no
		// second start, and the client sees the result it already received.
		again, rerr := h.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k1"})
		if rerr != nil {
			t.Fatalf("resent prompt: %+v", rerr)
		}
		if got := outcomeOf(t, again); got != WorkOutcomeStarted {
			t.Errorf("replayed outcome = %q, want %q", got, WorkOutcomeStarted)
		}
		if replayed, _ := again["replayed"].(bool); !replayed {
			t.Errorf("resent submission was not marked replayed: %v", again)
		}
		if calls.starts != 1 {
			t.Fatalf("Starter calls after resend = %d, want 1", calls.starts)
		}
	})

	t.Run("persisted running work is reported instead of restarted", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-new"}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		dir := t.TempDir()
		sess := h.openSession(dir)
		fp := PromptFingerprint(promptDefaultCapability, "explain the workflow")
		fb.records = []PersistedWork{{JobID: "job-own", Fingerprint: fp, State: persistedStateRunning}}

		res, rerr := h.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k2"})
		if rerr != nil {
			t.Fatalf("prompt: %+v", rerr)
		}
		if got := outcomeOf(t, res); got != WorkOutcomeAlreadyRunning {
			t.Fatalf("outcome = %q, want %q", got, WorkOutcomeAlreadyRunning)
		}
		if got, _ := res["jobId"].(string); got != "job-own" {
			t.Errorf("jobId = %q, want the persisted job-own", got)
		}
		if calls.starts != 0 {
			t.Errorf("Starter calls = %d, want 0", calls.starts)
		}
	})

	t.Run("a foreign running job makes the project busy without starting", func(t *testing.T) {
		fb := &fakeBridge{
			start:   SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-new"},
			records: []PersistedWork{{JobID: "job-other", Fingerprint: "unrelated", State: persistedStateRunning}},
		}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		sess := h.openSession(t.TempDir())

		res, rerr := h.prompt(sess, "a different request", map[string]any{"submissionKey": "k3"})
		if rerr != nil {
			t.Fatalf("prompt: %+v", rerr)
		}
		if got := outcomeOf(t, res); got != WorkOutcomeBusy {
			t.Fatalf("outcome = %q, want %q", got, WorkOutcomeBusy)
		}
		if got, _ := res["holderJobId"].(string); got != "job-other" {
			t.Errorf("busy result holderJobId = %q, want the holder job-other", got)
		}
		if calls.starts != 0 {
			t.Errorf("Starter calls = %d, want 0", calls.starts)
		}
	})

	t.Run("a finished earlier attempt is reconciled by identity", func(t *testing.T) {
		fp := PromptFingerprint(promptDefaultCapability, "explain the workflow")
		code := 3
		fb := &fakeBridge{
			start:   SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-new"},
			records: []PersistedWork{{JobID: "job-old", Fingerprint: fp, State: persistedStateFailed, ExitCode: &code}},
		}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		sess := h.openSession(t.TempDir())

		res, rerr := h.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k4"})
		if rerr != nil {
			t.Fatalf("prompt: %+v", rerr)
		}
		if got := outcomeOf(t, res); got != WorkOutcomeReconciled {
			t.Fatalf("outcome = %q, want %q", got, WorkOutcomeReconciled)
		}
		if got, _ := res["jobId"].(string); got != "job-old" {
			t.Errorf("jobId = %q, want the persisted job-old", got)
		}
		if calls.starts != 0 {
			t.Errorf("Starter calls = %d, want 0", calls.starts)
		}
	})

	t.Run("a contended project lock is busy with no read and no start", func(t *testing.T) {
		fb := &fakeBridge{lockErr: ErrProjectBusy}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		sess := h.openSession(t.TempDir())

		res, rerr := h.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k5"})
		if rerr != nil {
			t.Fatalf("prompt: %+v", rerr)
		}
		if got := outcomeOf(t, res); got != WorkOutcomeBusy {
			t.Fatalf("outcome = %q, want %q", got, WorkOutcomeBusy)
		}
		if condition, _ := res["condition"].(string); condition != "project_lock_held" {
			t.Errorf("condition = %q, want project_lock_held", condition)
		}
		if calls.locks != 1 || calls.reads != 0 || calls.starts != 0 {
			t.Errorf("side effects = locks:%d reads:%d starts:%d, want 1/0/0", calls.locks, calls.reads, calls.starts)
		}
	})

	t.Run("unsupported content is rejected before any side effect", func(t *testing.T) {
		cases := map[string]map[string]any{
			"image block": {
				"prompt": []map[string]any{{"type": "image", "data": "aGk="}},
			},
			"missing type": {
				"prompt": []map[string]any{{"text": "no type"}},
			},
			"empty text": {
				"prompt": []map[string]any{{"type": "text", "text": "   "}},
			},
			"no blocks": {
				"prompt": []map[string]any{},
			},
			"unknown params field": {
				"prompt": []map[string]any{{"type": "text", "text": "ok"}},
				"bogus":  1,
			},
			"oversized submission key": {
				"prompt":        []map[string]any{{"type": "text", "text": "ok"}},
				"submissionKey": strings.Repeat("k", MaxSubmissionKeyLength+1),
			},
		}
		for name, params := range cases {
			t.Run(name, func(t *testing.T) {
				fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
				calls := &recordedCalls{bridges: fb}
				h := newPromptHarness(t, calls.bridge())
				sess := h.openSession(t.TempDir())

				req := map[string]any{"sessionId": sess}
				for k, v := range params {
					req[k] = v
				}
				got := h.call(MethodSessionPrompt, req)
				if got.Error == nil || got.Error.Code != CodeInvalidParams {
					t.Fatalf("error = %+v, want code %d", got.Error, CodeInvalidParams)
				}
				if calls.locks != 0 || calls.reads != 0 || calls.starts != 0 {
					t.Errorf("side effects = locks:%d reads:%d starts:%d, want none", calls.locks, calls.reads, calls.starts)
				}
			})
		}
	})

	t.Run("a prompt naming a foreign project is rejected", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		own := t.TempDir()
		other := t.TempDir()
		sess := h.openSession(own)

		res, rerr := h.prompt(sess, "explain", map[string]any{"project": other})
		if rerr == nil {
			t.Fatalf("a foreign project was accepted: %v", res)
		}
		if rerr.Code != CodeInvalidParams {
			t.Errorf("code = %d, want %d", rerr.Code, CodeInvalidParams)
		}
		if calls.locks != 0 || calls.reads != 0 || calls.starts != 0 {
			t.Errorf("side effects = locks:%d reads:%d starts:%d, want none", calls.locks, calls.reads, calls.starts)
		}
	})

	t.Run("two spellings of one directory share a project identity", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		dir := t.TempDir()
		sess := h.openSession(dir)
		if _, rerr := h.prompt(sess, "explain", map[string]any{"project": filepath.Join(dir, ".")}); rerr != nil {
			t.Fatalf("a second spelling of the same project was rejected: %+v", rerr)
		}
	})

	t.Run("cross-project sessions stay isolated", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		calls := &recordedCalls{bridges: fb}
		// The bridge must distinguish projects even though the adapter passes one
		// session's canonical directory at a time.
		fb.startHook = func(sub Submission) {
			if strings.TrimSpace(sub.ProjectDir) == "" {
				t.Error("Starter received an empty project directory")
			}
		}
		h := newPromptHarness(t, calls.bridge())
		dirA, dirB := t.TempDir(), t.TempDir()
		sessA := h.openSession(dirA)
		sessB := h.openSession(dirB)

		if _, rerr := h.prompt(sessA, "same text", map[string]any{"submissionKey": "a"}); rerr != nil {
			t.Fatalf("prompt A: %+v", rerr)
		}
		// The same text in a different project is a different submission: the
		// adapter's registry is session-keyed and its persisted read is
		// project-scoped, so this starts instead of replaying A's result.
		if _, rerr := h.prompt(sessB, "same text", map[string]any{"submissionKey": "a"}); rerr != nil {
			t.Fatalf("prompt B: %+v", rerr)
		}
		if calls.starts != 2 {
			t.Fatalf("Starter calls = %d, want 2 (one per project)", calls.starts)
		}
		if calls.subs[0].ProjectDir == calls.subs[1].ProjectDir {
			t.Errorf("both submissions carried project %q; sessions must stay isolated", calls.subs[0].ProjectDir)
		}
	})

	t.Run("a client JSON-RPC id is not an idempotency key", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		sess := h.openSession(t.TempDir())

		frame := func(key string) map[string]any {
			return map[string]any{
				"jsonrpc": "2.0",
				"id":      99, // deliberately reused
				"method":  MethodSessionPrompt,
				"params": map[string]any{
					"sessionId":     sess,
					"submissionKey": key,
					"prompt":        []map[string]any{{"type": "text", "text": "explain"}},
				},
			}
		}
		if got := h.send(frame("first")); got.Error != nil {
			t.Fatalf("first submission: %+v", got.Error)
		}
		if got := h.send(frame("second")); got.Error != nil {
			t.Fatalf("second submission: %+v", got.Error)
		}
		if calls.starts != 2 {
			t.Errorf("Starter calls = %d, want 2: two different submission keys under one JSON-RPC id are two submissions", calls.starts)
		}
	})

	t.Run("an unconfigured bridge is refused", func(t *testing.T) {
		h := newPromptHarness(t, nil)
		sess := h.openSession(t.TempDir())
		got := h.call(MethodSessionPrompt, map[string]any{
			"sessionId": sess,
			"prompt":    []map[string]any{{"type": "text", "text": "explain"}},
		})
		if got.Error == nil || got.Error.Code != CodeCapabilityUnsupported {
			t.Fatalf("error = %+v, want code %d", got.Error, CodeCapabilityUnsupported)
		}
	})

	t.Run("an uninitialized project is reported, not started", func(t *testing.T) {
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeNotInitialized}}
		calls := &recordedCalls{bridges: fb}
		h := newPromptHarness(t, calls.bridge())
		sess := h.openSession(t.TempDir())

		_, rerr := h.prompt(sess, "explain", map[string]any{"submissionKey": "k6"})
		if rerr == nil {
			t.Fatal("an uninitialized project was accepted")
		}
		if rerr.Code != CodeInvalidParams || !strings.Contains(rerr.Message, "not initialized") {
			t.Errorf("error = %+v, want an invalid-params not-initialized message", rerr)
		}
	})
}

// TestPromptSubmissionCanonicalProjectDir proves the session boundary canonicalizes
// and validates the project directory without changing the process cwd.
func TestPromptSubmissionCanonicalProjectDir(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	canonical, err := canonicalProjectDir(dir)
	if err != nil {
		t.Fatalf("canonicalProjectDir: %v", err)
	}
	if !filepath.IsAbs(canonical) {
		t.Errorf("canonical dir %q is not absolute", canonical)
	}
	if after, err := os.Getwd(); err != nil || after != wd {
		t.Errorf("process cwd changed from %q to %q (err %v)", wd, after, err)
	}
	if _, err := canonicalProjectDir(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Error("a missing project directory was accepted")
	}
}

// TestPromptSubmissionReconcilesOnBridgeError proves an unexpected bridge outcome
// is a protocol error rather than a silent success.
func TestPromptSubmissionReconcilesOnBridgeError(t *testing.T) {
	fb := &fakeBridge{startErr: errors.New("governed start failed")}
	calls := &recordedCalls{bridges: fb}
	h := newPromptHarness(t, calls.bridge())
	sess := h.openSession(t.TempDir())

	_, rerr := h.prompt(sess, "explain", map[string]any{"submissionKey": "k7"})
	if rerr == nil || rerr.Code != CodeInternalError {
		t.Fatalf("error = %+v, want code %d", rerr, CodeInternalError)
	}
	if calls.starts != 1 {
		t.Errorf("Starter calls = %d, want 1 (the failed start is not retried here)", calls.starts)
	}
}

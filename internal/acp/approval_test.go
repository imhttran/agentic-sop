package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// This file is ACP-005's deterministic acceptance coverage. TestApproval
// drives the real dispatch path (handleFrame, handleSessionPrompt,
// approvalGate) with a fixed question/approval state and a recording
// governed-start bridge: no wall clock, no network, no child process, and no
// real project state. It proves:
//
//   - a repeated prompt against an unchanged approval blocker stops and is
//     reported as blocked_unchanged: no governed start runs again, so no
//     attempt or retry budget is consumed;
//   - a stale or unrelated response is refused as not_authorizing, starts
//     nothing, and leaves the approval and the question pending;
//   - after external resolution through the SOP approval interface,
//     continuation runs only after a fresh authoritative seam read shows the
//     blocker resolved.

// qaStage is the deterministic fixture for TestApproval. It supplies the
// host-side question/approval read seam and the governed submission bridge,
// records every use on one shared timeline, and serves fixed state that the
// test mutates to simulate external resolution. It reads no clock and talks
// to nothing.
type qaStage struct {
	// log records the deterministic order of seam reads and governed-start
	// machinery use: seam-read, lock, persisted-read, start.
	log       []string
	questions []PlannerQuestion
	blockers  []ApprovalBlocker
	seamReads int
	locks     int
	persisted int
	starts    int
}

// source returns the QuestionApprovalSource serving the fixture's fixed
// state; every call counts as one fresh authoritative read.
func (q *qaStage) source() *QuestionApprovalSource {
	return &QuestionApprovalSource{Read: func(_ string) (QuestionApprovalState, error) {
		q.seamReads++
		q.log = append(q.log, "seam-read")
		return QuestionApprovalState{Questions: q.questions, Blockers: q.blockers}, nil
	}}
}

// bridge returns the governed submission bridge recording its use on the same
// timeline and starting one deterministic job per call.
func (q *qaStage) bridge() *PromptSubmission {
	return &PromptSubmission{
		LockTaker: func(context.Context, string) (func(), error) {
			q.locks++
			q.log = append(q.log, "lock")
			return func() {}, nil
		},
		StateReader: func(string) ([]PersistedWork, error) {
			q.persisted++
			q.log = append(q.log, "persisted-read")
			return nil, nil
		},
		Starter: func(Submission) (SubmissionResult, error) {
			q.starts++
			q.log = append(q.log, "start")
			return SubmissionResult{Outcome: WorkOutcomeStarted, JobID: fmt.Sprintf("job-%d", q.starts)}, nil
		},
	}
}

// approvalFixture drives the real dispatch surface and decodes every protocol
// frame the server writes for each request, so a test sees the response and
// the notice frames in delivery order.
type approvalFixture struct {
	t      *testing.T
	server *Server
	out    *bytes.Buffer
	errOut *bytes.Buffer
	seq    int
}

// newApprovalFixture builds the fixture with the seams a test passes; nil
// seams stay unwired.
func newApprovalFixture(t *testing.T, bridge *PromptSubmission, qa *QuestionApprovalSource) *approvalFixture {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	opts := []ServerOption{}
	if bridge != nil {
		opts = append(opts, WithPromptSubmission(bridge))
	}
	if qa != nil {
		opts = append(opts, WithQuestionApproval(qa))
	}
	return &approvalFixture{
		t:      t,
		server: NewServer("test-version", out, errOut, opts...),
		out:    out,
		errOut: errOut,
	}
}

// send dispatches one frame and decodes every protocol frame written for it.
func (f *approvalFixture) send(frame map[string]any) []map[string]any {
	f.t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		f.t.Fatalf("marshal request: %v", err)
	}
	before := f.out.Len()
	f.server.handleFrame(data)
	chunk := strings.TrimSuffix(f.out.String()[before:], "\n")
	if chunk == "" {
		f.t.Fatalf("no frames written for %v", frame)
	}
	parts := strings.Split(chunk, "\n")
	frames := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		var m map[string]any
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			f.t.Fatalf("decode frame %q: %v", p, err)
		}
		frames = append(frames, m)
	}
	return frames
}

// openSession initializes the server and opens one session on dir.
func (f *approvalFixture) openSession(dir string) string {
	f.t.Helper()
	f.seq++
	resps := f.send(map[string]any{"jsonrpc": "2.0", "id": f.seq, "method": MethodInitialize, "params": map[string]any{"protocolVersion": ProtocolVersion}})
	if len(resps) != 1 || resps[0]["error"] != nil {
		f.t.Fatalf("initialize wrote %v", resps)
	}
	f.seq++
	resps = f.send(map[string]any{"jsonrpc": "2.0", "id": f.seq, "method": MethodSessionNew, "params": map[string]any{"cwd": dir}})
	if len(resps) != 1 || resps[0]["error"] != nil {
		f.t.Fatalf("session/new wrote %v", resps)
	}
	result, _ := resps[0]["result"].(map[string]any)
	sess, _ := result["sessionId"].(string)
	if sess == "" {
		f.t.Fatalf("session/new returned no sessionId: %v", resps[0])
	}
	return sess
}

// prompt submits one text prompt and returns every frame written for it.
func (f *approvalFixture) prompt(sessionID, text, key string) []map[string]any {
	f.t.Helper()
	f.seq++
	return f.send(map[string]any{"jsonrpc": "2.0", "id": f.seq, "method": MethodSessionPrompt, "params": map[string]any{
		"sessionId":     sessionID,
		"prompt":        []map[string]any{{"type": "text", "text": text}},
		"submissionKey": key,
	}})
}

// resultOf returns a response frame's result object and fails when the frame
// carries an error member.
func resultOf(t *testing.T, frame map[string]any) map[string]any {
	t.Helper()
	if e, has := frame["error"]; has && e != nil {
		t.Fatalf("frame carries an error member: %v", frame)
	}
	res, ok := frame["result"].(map[string]any)
	if !ok {
		t.Fatalf("frame carries no result object: %v", frame)
	}
	return res
}

// noticeOf decodes the sop.notice kind and the display text of a notice
// frame.
func noticeOf(t *testing.T, frame map[string]any) (kind, text string) {
	t.Helper()
	params, ok := frame["params"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no params object: %v", frame)
	}
	sop, ok := params["sop"].(map[string]any)
	if !ok {
		t.Fatalf("frame params carry no sop notice member: %v", params)
	}
	kind, _ = sop["notice"].(string)
	update, _ := params["update"].(map[string]any)
	content, _ := update["content"].(map[string]any)
	text, _ = content["text"].(string)
	return kind, text
}

// wantTimeline asserts the exact deterministic event order across the seam
// and the governed-start bridge.
func wantTimeline(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event log[%d] = %q, want %q (full log %v)", i, got[i], want[i], want)
		}
	}
}

// approvalBlocker returns one deterministic pending approval blocker with the
// fixed identity id.
func approvalBlocker(id, action string) ApprovalBlocker {
	return ApprovalBlocker{
		ID:             id,
		Kind:           "human.approval_before_commit",
		Detail:         "the commit gate needs a human decision",
		OperatorAction: action,
		State:          approvalStatePending,
	}
}

// pendingQuestion returns one deterministic pending planner question.
func pendingQuestion(id string, revision int) PlannerQuestion {
	return PlannerQuestion{
		ID:       id,
		Identity: "ACP-005:blocker-identity",
		Kind:     "choice",
		Body:     "apply the reviewed plan change?",
		State:    QuestionStatePending,
		Revision: revision,
		Options:  []string{"apply", "discuss"},
	}
}

// TestApproval is ACP-005's deterministic acceptance test.
func TestApproval(t *testing.T) {
	t.Run("unchanged blocker stops without a retry", func(t *testing.T) {
		q := &qaStage{blockers: []ApprovalBlocker{approvalBlocker("apr-1", "run sop approvals, then record the decision on this gate with sop approve")}}
		fix := newApprovalFixture(t, q.bridge(), q.source())
		sess := fix.openSession(t.TempDir())

		// First prompt: the blocker is displayed once and governed work starts
		// once.
		first := fix.prompt(sess, "ship the change", "k1")
		if len(first) != 2 {
			t.Fatalf("first prompt wrote %d frames, want the response plus one blocker notice: %v", len(first), first)
		}
		res := resultOf(t, first[0])
		if got, _ := res["outcome"].(string); got != WorkOutcomeStarted {
			t.Fatalf("first outcome = %v, want %q", res["outcome"], WorkOutcomeStarted)
		}
		kind, text := noticeOf(t, first[1])
		if kind != NoticeApprovalBlocker {
			t.Fatalf("notice kind = %q, want %q", kind, NoticeApprovalBlocker)
		}
		for _, want := range []string{
			"Approval blocker apr-1 (human.approval_before_commit, state pending)",
			"To resolve: run sop approvals, then record the decision on this gate with sop approve",
			"existing SOP approval interface",
		} {
			if !strings.Contains(text, want) {
				t.Errorf("blocker display %q misses %q", text, want)
			}
		}

		// Second prompt against the same blocker identity: the adapter stops.
		// It retries nothing, so no attempt or retry budget is consumed.
		second := fix.prompt(sess, "ship the change", "k2")
		if len(second) != 1 {
			t.Fatalf("unchanged-blocker prompt wrote %d frames, want exactly the response: %v", len(second), second)
		}
		res2 := resultOf(t, second[0])
		if got, _ := res2["outcome"].(string); got != WorkOutcomeBlockedUnchanged {
			t.Fatalf("second outcome = %v, want %q", res2["outcome"], WorkOutcomeBlockedUnchanged)
		}
		if got, _ := res2["condition"].(string); got != "unchanged_approval_blocker" {
			t.Errorf("condition = %v, want unchanged_approval_blocker", res2["condition"])
		}
		if got, _ := res2["approvalState"].(string); got != approvalStatePending {
			t.Errorf("approvalState = %v, want pending: the blocker stays pending", res2["approvalState"])
		}
		if ids, _ := res2["blockerIds"].([]any); len(ids) != 1 || ids[0] != "apr-1" {
			t.Errorf("blockerIds = %v, want [apr-1]", res2["blockerIds"])
		}
		if msg, _ := res2["message"].(string); !strings.Contains(msg, "no attempt or retry budget was consumed") {
			t.Errorf("message %q must state that no attempt or retry budget was consumed", msg)
		}
		// No retry was issued: the governed start machinery ran exactly once,
		// on the first turn, and both prompts read fresh authoritative state.
		if q.starts != 1 || q.locks != 1 || q.persisted != 1 {
			t.Errorf("governed-start use = starts:%d locks:%d persisted:%d, want 1/1/1", q.starts, q.locks, q.persisted)
		}
		wantTimeline(t, q.log, []string{"seam-read", "lock", "persisted-read", "start", "seam-read"})
	})

	t.Run("stale or unrelated answer is refused and approval stays pending", func(t *testing.T) {
		q := &qaStage{questions: []PlannerQuestion{pendingQuestion("q-1", 2)}}
		fix := newApprovalFixture(t, q.bridge(), q.source())
		sess := fix.openSession(t.TempDir())

		// The displayed revision is 2; the prompt claims revision 1, so the
		// response is stale.
		stale := fix.prompt(sess, "answer:q-1@1 apply", "k1")
		if len(stale) != 1 {
			t.Fatalf("stale prompt wrote %d frames, want exactly the response: %v", len(stale), stale)
		}
		res := resultOf(t, stale[0])
		if got, _ := res["outcome"].(string); got != WorkOutcomeNotAuthorizing {
			t.Fatalf("stale outcome = %v, want %q: a stale response cannot authorize work", res["outcome"], WorkOutcomeNotAuthorizing)
		}
		if got, _ := res["condition"].(string); got != "unrelated_or_stale_response" {
			t.Errorf("condition = %v, want unrelated_or_stale_response", res["condition"])
		}
		if got, _ := res["approvalState"].(string); got != approvalStatePending {
			t.Errorf("approvalState = %v, want pending: the stale response leaves approval pending", res["approvalState"])
		}
		if msg, _ := res["message"].(string); !strings.Contains(msg, "stale") {
			t.Errorf("message %q must name the staleness", msg)
		}

		// An unrelated response is refused the same way and authorizes nothing.
		unrelated := fix.prompt(sess, "answer:unknown@2 apply", "k2")
		if len(unrelated) != 1 {
			t.Fatalf("unrelated prompt wrote %d frames, want exactly the response: %v", len(unrelated), unrelated)
		}
		res2 := resultOf(t, unrelated[0])
		if got, _ := res2["outcome"].(string); got != WorkOutcomeNotAuthorizing {
			t.Fatalf("unrelated outcome = %v, want %q", res2["outcome"], WorkOutcomeNotAuthorizing)
		}
		if got, _ := res2["approvalState"].(string); got != approvalStatePending {
			t.Errorf("approvalState = %v, want pending", res2["approvalState"])
		}
		// A refused response starts and authorizes nothing: the governed start
		// machinery never ran, and no approval or question state changed.
		if q.locks != 0 || q.persisted != 0 || q.starts != 0 {
			t.Errorf("governed-start use = locks:%d persisted:%d starts:%d, want none", q.locks, q.persisted, q.starts)
		}
		wantTimeline(t, q.log, []string{"seam-read", "seam-read"})
	})

	t.Run("continuation after external resolution needs a fresh state read", func(t *testing.T) {
		q := &qaStage{blockers: []ApprovalBlocker{approvalBlocker("apr-2", "record the decision on the task's active approval gate through the SOP approval interface")}}
		fix := newApprovalFixture(t, q.bridge(), q.source())
		sess := fix.openSession(t.TempDir())

		// First prompt: the blocker is reported; nothing continues past it.
		first := fix.prompt(sess, "continue the task", "k1")
		if len(first) != 2 {
			t.Fatalf("first prompt wrote %d frames, want the response plus one blocker notice: %v", len(first), first)
		}
		res1 := resultOf(t, first[0])
		if got, _ := res1["outcome"].(string); got != WorkOutcomeStarted {
			t.Fatalf("first outcome = %v, want %q", res1["outcome"], WorkOutcomeStarted)
		}
		if kind, _ := noticeOf(t, first[1]); kind != NoticeApprovalBlocker {
			t.Fatalf("first notice kind = %q, want %q", kind, NoticeApprovalBlocker)
		}

		// The operator resolves the blocker through the existing SOP approval
		// interface. ACP recorded nothing; the adapter's only memory is the
		// identity it reported, so continuation must come from a fresh
		// authoritative read, never from any in-memory value.
		q.blockers = nil

		second := fix.prompt(sess, "continue the task", "k2")
		if len(second) != 2 {
			t.Fatalf("resolution prompt wrote %d frames, want the response plus one resolution notice: %v", len(second), second)
		}
		res2 := resultOf(t, second[0])
		if got, _ := res2["outcome"].(string); got != WorkOutcomeStarted {
			t.Fatalf("continuation outcome = %v, want %q", res2["outcome"], WorkOutcomeStarted)
		}
		kind, text := noticeOf(t, second[1])
		if kind != NoticeApprovalResolved {
			t.Fatalf("notice kind = %q, want %q", kind, NoticeApprovalResolved)
		}
		for _, want := range []string{"fresh authoritative read", "continuation", "SOP approval interface"} {
			if !strings.Contains(text, want) {
				t.Errorf("resolution notice %q misses %q", text, want)
			}
		}
		// Ordering: the continuation start came after the fresh seam read that
		// observed the resolution, never from the in-memory blocker record.
		wantTimeline(t, q.log, []string{
			"seam-read", "lock", "persisted-read", "start",
			"seam-read", "lock", "persisted-read", "start",
		})
		if q.seamReads != 2 || q.starts != 2 {
			t.Errorf("seam reads = %d, starts = %d, want 2/2", q.seamReads, q.starts)
		}
	})
}

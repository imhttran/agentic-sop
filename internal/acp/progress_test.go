package acp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestProgress is ACP-004's deterministic acceptance coverage. It drives the
// real dispatch, translation, and delivery surfaces with deterministic fakes —
// no model, no network, no child process, and no sleeps or wall-clock reads —
// and proves:
//
//   - verified completion, verification failure, and blocked execution
//     produce update frames whose fields match the supplied persisted SOP
//     evidence exactly, and LOCAL_DONE and waiting-for-approval stay distinct
//     from verified completion;
//   - a transport response never labels unfinished work complete: the prompt
//     response carries only a protocol stop reason, and a running job's frame
//     never asserts completion;
//   - a server without a progress source emits no frames at all, and a
//     submission with no persisted record or a busy project answers with one
//     notice frame that repeats the result's own outcome instead of fabricating
//     progress (follow.go);
//   - the same snapshot builds byte-identical frames;
//   - a slow client cannot grow the buffer past EventBufferBound, intermediate
//     frames coalesce, and the terminal outcome survives coalescing and is
//     delivered last through whole-frame writes only.

// fakeProgress is a deterministic ProgressSource over a fixed snapshot map.
type fakeProgress struct {
	snaps map[string]ProgressSnapshot
	reads int
}

func (f *fakeProgress) source() *ProgressSource {
	return &ProgressSource{Progress: func(projectDir, jobID string) (ProgressSnapshot, bool, error) {
		f.reads++
		snap, ok := f.snaps[jobID]
		return snap, ok, nil
	}}
}

// progressFixture dispatches frames through one Server and decodes every
// protocol frame written for each request, so a test sees the response and the
// session/update notifications in order.
type progressFixture struct {
	t      *testing.T
	server *Server
	out    *bytes.Buffer
	errOut *bytes.Buffer
	seq    int
}

func newProgressFixture(t *testing.T, fb *fakeBridge, src *ProgressSource) *progressFixture {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	var opts []ServerOption
	if fb != nil {
		opts = append(opts, WithPromptSubmission((&recordedCalls{bridges: fb}).bridge()))
	}
	if src != nil {
		opts = append(opts, WithProgress(src))
	}
	return &progressFixture{t: t, server: NewServer("test-version", out, errOut, opts...), out: out, errOut: errOut}
}

// send marshals and dispatches one frame, then decodes every protocol frame
// the server wrote for it.
func (f *progressFixture) send(frame map[string]any) []map[string]any {
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
func (f *progressFixture) openSession(dir string) string {
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
func (f *progressFixture) prompt(sessionID, text string, extra map[string]any) []map[string]any {
	f.t.Helper()
	f.seq++
	params := map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}
	for k, v := range extra {
		params[k] = v
	}
	return f.send(map[string]any{"jsonrpc": "2.0", "id": f.seq, "method": MethodSessionPrompt, "params": params})
}

// decodeSOP returns the sop member of a session/update frame.
func decodeSOP(t *testing.T, frame map[string]any) map[string]any {
	t.Helper()
	params, ok := frame["params"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no params object: %v", frame)
	}
	sop, ok := params["sop"].(map[string]any)
	if !ok {
		t.Fatalf("frame params carry no sop member: %v", params)
	}
	return sop
}

// wantStringSlice compares a decoded JSON string slice against want.
func wantStringSlice(t *testing.T, got []any, want []string, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if s, _ := got[i].(string); s != want[i] {
			t.Fatalf("%s[%d] = %v, want %q", label, i, got[i], want[i])
		}
	}
}

func TestProgress(t *testing.T) {
	t.Run("verified completion matches the persisted evidence", func(t *testing.T) {
		snaps := map[string]ProgressSnapshot{
			"job-1": {
				JobID:    "job-1",
				TaskID:   "TASK-007",
				Stage:    "verify",
				State:    ProgressStateVerified,
				Verified: true,
				Evidence: []string{".agent-sdlc/runs/r1/jev.json", ".agent-sdlc/outcome.md"},
			},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", map[string]any{"submissionKey": "k1"})
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		resp, upd := frames[0], frames[1]
		if _, hasResult := resp["result"]; !hasResult {
			t.Fatalf("first frame is not the response: %v", resp)
		}
		if got, _ := upd["method"].(string); got != MethodSessionUpdate {
			t.Fatalf("update method = %v, want %q", upd["method"], MethodSessionUpdate)
		}
		params := upd["params"].(map[string]any)
		if got, _ := params["sessionId"].(string); got != sess {
			t.Errorf("sessionId = %q, want %q", got, sess)
		}
		update := params["update"].(map[string]any)
		if got, _ := update["sessionUpdate"].(string); got != UpdateAgentMessageChunk {
			t.Errorf("sessionUpdate = %v, want %q", update["sessionUpdate"], UpdateAgentMessageChunk)
		}
		content := update["content"].(map[string]any)
		text, _ := content["text"].(string)
		for _, want := range []string{"SOP job job-1", "task TASK-007", "is complete; verification passed", "Evidence: .agent-sdlc/runs/r1/jev.json, .agent-sdlc/outcome.md"} {
			if !strings.Contains(text, want) {
				t.Errorf("progress text %q misses %q", text, want)
			}
		}
		sop := decodeSOP(t, upd)
		if got, _ := sop["jobId"].(string); got != "job-1" {
			t.Errorf("sop.jobId = %v, want job-1", sop["jobId"])
		}
		if got, _ := sop["taskId"].(string); got != "TASK-007" {
			t.Errorf("sop.taskId = %v, want TASK-007", sop["taskId"])
		}
		if got, _ := sop["state"].(string); got != ProgressStateVerified {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateVerified)
		}
		if got, _ := sop["verified"].(bool); got != true {
			t.Errorf("sop.verified = %v, want true", sop["verified"])
		}
		if got, _ := sop["stage"].(string); got != "verify" {
			t.Errorf("sop.stage = %v, want verify", sop["stage"])
		}
		evidence, _ := sop["evidence"].([]any)
		wantStringSlice(t, evidence, snaps["job-1"].Evidence, "sop.evidence")
		if fp.reads != 1 {
			t.Errorf("ProgressSource reads = %d, want 1", fp.reads)
		}

		// The response never labels the work; the completion claim lives only
		// in the update payload, and the turn end uses the protocol value.
		result := resp["result"].(map[string]any)
		if got, _ := result["stopReason"].(string); got != StopReasonEndTurn {
			t.Errorf("response stopReason = %v, want %q", result["stopReason"], StopReasonEndTurn)
		}
		if _, has := result["sop"]; has {
			t.Errorf("response carried the sop progress member: %v", result)
		}
	})

	t.Run("verification failure reports the failure and never completion", func(t *testing.T) {
		code := 2
		snaps := map[string]ProgressSnapshot{
			"job-1": {
				JobID:    "job-1",
				Stage:    "verify",
				State:    ProgressStateVerificationFailed,
				Verified: false,
				ExitCode: &code,
			},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", nil)
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		sop := decodeSOP(t, frames[1])
		if got, _ := sop["state"].(string); got != ProgressStateVerificationFailed {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateVerificationFailed)
		}
		if got, _ := sop["verified"].(bool); got != false {
			t.Errorf("sop.verified = %v, want false", sop["verified"])
		}
		if got, _ := sop["exitCode"].(float64); got != 2 {
			t.Errorf("sop.exitCode = %v, want 2", sop["exitCode"])
		}
		text := frames[1]["params"].(map[string]any)["update"].(map[string]any)["content"].(map[string]any)["text"].(string)
		if !strings.Contains(text, "verification failed") || !strings.Contains(text, "work is not complete") {
			t.Errorf("progress text %q must report the failure, not completion", text)
		}

		result := frames[0]["result"].(map[string]any)
		if got, _ := result["stopReason"].(string); got != StopReasonEndTurn {
			t.Errorf("response stopReason = %v, want %q", result["stopReason"], StopReasonEndTurn)
		}
		if _, has := result["sop"]; has {
			t.Errorf("response carried a progress payload: %v", result)
		}
	})

	t.Run("blocked execution carries the blockers and never completion", func(t *testing.T) {
		snaps := map[string]ProgressSnapshot{
			"job-1": {
				JobID: "job-1",
				State: ProgressStateBlocked,
				Blockers: []string{
					"plan change needs human approval",
					"implementation agent returned LOCAL_DONE without evidence",
				},
			},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", nil)
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		sop := decodeSOP(t, frames[1])
		if got, _ := sop["state"].(string); got != ProgressStateBlocked {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateBlocked)
		}
		if got, _ := sop["verified"].(bool); got != false {
			t.Errorf("sop.verified = %v, want false", sop["verified"])
		}
		blockers, _ := sop["blockers"].([]any)
		wantStringSlice(t, blockers, snaps["job-1"].Blockers, "sop.blockers")
		text := frames[1]["params"].(map[string]any)["update"].(map[string]any)["content"].(map[string]any)["text"].(string)
		for _, want := range snaps["job-1"].Blockers {
			if !strings.Contains(text, want) {
				t.Errorf("progress text %q misses blocker %q", text, want)
			}
		}
		if !strings.Contains(text, "work is not complete") {
			t.Errorf("blocked text %q must not read as completion", text)
		}
		if _, has := sop["taskId"]; has {
			t.Errorf("a snapshot with no task id invented one: %v", sop)
		}
	})

	t.Run("waiting for approval stays distinct from completion", func(t *testing.T) {
		snaps := map[string]ProgressSnapshot{
			"job-1": {JobID: "job-1", Stage: "review", State: ProgressStateAwaitingApproval},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", nil)
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		sop := decodeSOP(t, frames[1])
		if got, _ := sop["state"].(string); got != ProgressStateAwaitingApproval {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateAwaitingApproval)
		}
		if got, _ := sop["verified"].(bool); got != false {
			t.Errorf("sop.verified = %v, want false", sop["verified"])
		}
		text := frames[1]["params"].(map[string]any)["update"].(map[string]any)["content"].(map[string]any)["text"].(string)
		if !strings.Contains(text, "human approval") || !strings.Contains(text, "work is not complete") {
			t.Errorf("approval-wait text %q must say the work still waits", text)
		}
	})

	t.Run("LOCAL_DONE is not verified completion", func(t *testing.T) {
		snaps := map[string]ProgressSnapshot{
			"job-1": {JobID: "job-1", State: ProgressStateLocalDone, Verified: false},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", nil)
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		sop := decodeSOP(t, frames[1])
		if got, _ := sop["state"].(string); got != ProgressStateLocalDone {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateLocalDone)
		}
		if got, _ := sop["state"].(string); got == ProgressStateVerified {
			t.Errorf("LOCAL_DONE leaked into the verified state")
		}
		if got, _ := sop["verified"].(bool); got != false {
			t.Errorf("sop.verified = %v, want false: local done is not verified", sop["verified"])
		}
		text := frames[1]["params"].(map[string]any)["update"].(map[string]any)["content"].(map[string]any)["text"].(string)
		if !strings.Contains(text, "LOCAL_DONE") || !strings.Contains(text, "work is not complete") {
			t.Errorf("local-done text %q must keep the work unfinished", text)
		}
	})

	t.Run("a running job never labels work complete and omits unavailable fields", func(t *testing.T) {
		snaps := map[string]ProgressSnapshot{
			"job-1": {JobID: "job-1", Stage: "implement", State: ProgressStateRunning},
		}
		fp := &fakeProgress{snaps: snaps}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())

		frames := fix.prompt(sess, "explain the workflow", nil)
		if len(frames) != 2 {
			t.Fatalf("wrote %d frames, want the response plus one session/update", len(frames))
		}
		sop := decodeSOP(t, frames[1])
		if got, _ := sop["state"].(string); got != ProgressStateRunning {
			t.Errorf("sop.state = %v, want %q", sop["state"], ProgressStateRunning)
		}
		if got, _ := sop["verified"].(bool); got != false {
			t.Errorf("a running job reported verified=true")
		}
		for _, missing := range []string{"taskId", "blockers", "evidence", "exitCode"} {
			if _, has := sop[missing]; has {
				t.Errorf("a snapshot with no %s evidence invented the field", missing)
			}
		}
		text := frames[1]["params"].(map[string]any)["update"].(map[string]any)["content"].(map[string]any)["text"].(string)
		if !strings.Contains(text, "not complete") {
			t.Errorf("running text %q must not claim completion", text)
		}

		result := frames[0]["result"].(map[string]any)
		if got, _ := result["outcome"].(string); got != WorkOutcomeStarted {
			t.Errorf("response outcome = %v, want %q", result["outcome"], WorkOutcomeStarted)
		}
		if got, _ := result["stopReason"].(string); got != StopReasonEndTurn {
			t.Errorf("response stopReason = %v, want %q", result["stopReason"], StopReasonEndTurn)
		}
		for _, never := range []string{"sop", "verified", "state"} {
			if _, has := result[never]; has {
				t.Errorf("response for unfinished work carries %q: %v", never, result)
			}
		}
	})

	t.Run("the same snapshot builds byte-identical frames", func(t *testing.T) {
		snap := ProgressSnapshot{
			JobID:    "job-1",
			TaskID:   "TASK-007",
			Stage:    "verify",
			State:    ProgressStateVerified,
			Verified: true,
			Evidence: []string{".agent-sdlc/outcome.md"},
		}
		first, outcome1, err := buildProgressFrame("sess_x", snap)
		if err != nil {
			t.Fatalf("build frame: %v", err)
		}
		second, outcome2, err := buildProgressFrame("sess_x", snap)
		if err != nil {
			t.Fatalf("build frame: %v", err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("frames differ: %s vs %s", first, second)
		}
		if outcome1 != outcome2 || !outcome1 {
			t.Fatalf("outcome classification = %v/%v, want true/true", outcome1, outcome2)
		}
		if _, outcome, err := buildProgressFrame("sess_x", ProgressSnapshot{JobID: "job-1", State: ProgressStateRunning}); err != nil || outcome {
			t.Fatalf("running snapshot: outcome=%v err=%v, want false/nil", outcome, err)
		}
	})

	t.Run("a missing seam, a missing record, and busy outcomes never fabricate progress", func(t *testing.T) {
		t.Run("no progress source", func(t *testing.T) {
			fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
			fix := newProgressFixture(t, fb, nil)
			sess := fix.openSession(t.TempDir())
			if frames := fix.prompt(sess, "explain", nil); len(frames) != 1 {
				t.Fatalf("wrote %d frames without a progress source, want exactly the response", len(frames))
			}
		})
		t.Run("source has no record for the job", func(t *testing.T) {
			fp := &fakeProgress{snaps: map[string]ProgressSnapshot{}}
			fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
			fix := newProgressFixture(t, fb, fp.source())
			sess := fix.openSession(t.TempDir())
			frames := fix.prompt(sess, "explain", nil)
			if len(frames) != 2 {
				t.Fatalf("wrote %d frames with no persisted record, want the response and one notice", len(frames))
			}
			notice := decodeSOP(t, frames[1])
			if got, _ := notice["notice"].(string); got != NoticePromptOutcome {
				t.Errorf("notice = %q, want %q", got, NoticePromptOutcome)
			}
			if _, ok := notice["state"]; ok {
				t.Errorf("notice carries a progress state (%v): a missing record must not become progress", notice)
			}
			if text := updateText(t, frames[1]); !strings.Contains(text, "no persisted job record for job-1") {
				t.Errorf("notice text = %q, want it to name the missing record", text)
			}
			if fp.reads != 1 {
				t.Errorf("ProgressSource reads = %d, want 1", fp.reads)
			}
		})
		t.Run("busy project answers with its outcome, not a fabricated frame", func(t *testing.T) {
			fp := &fakeProgress{snaps: map[string]ProgressSnapshot{}}
			fb := &fakeBridge{lockErr: ErrProjectBusy}
			fix := newProgressFixture(t, fb, fp.source())
			sess := fix.openSession(t.TempDir())
			frames := fix.prompt(sess, "explain", nil)
			if len(frames) != 2 {
				t.Fatalf("wrote %d frames for a busy project, want the response and one notice", len(frames))
			}
			notice := decodeSOP(t, frames[1])
			if _, ok := notice["state"]; ok {
				t.Errorf("busy notice carries a progress state (%v): a busy project is not work state", notice)
			}
			if _, ok := notice["jobId"]; ok {
				t.Errorf("busy notice carries a job identity (%v): nothing started, so none exists", notice)
			}
			if text := updateText(t, frames[1]); !strings.Contains(text, "nothing was started or queued") {
				t.Errorf("notice text = %q, want the busy result's own message", text)
			}
			result := frames[0]["result"].(map[string]any)
			if got, _ := result["outcome"].(string); got != WorkOutcomeBusy {
				errorf := result["outcome"]
				t.Fatalf("outcome = %v (%v), want %q", got, errorf, WorkOutcomeBusy)
			}
			if got, _ := result["stopReason"].(string); got != StopReasonEndTurn {
				t.Errorf("busy response stopReason = %v, want %q", result["stopReason"], StopReasonEndTurn)
			}
			if fp.reads != 0 {
				t.Errorf("ProgressSource reads = %d, want 0 for a busy project", fp.reads)
			}
		})
	})

	t.Run("an unusable snapshot is refused, not fabricated", func(t *testing.T) {
		fp := &fakeProgress{snaps: map[string]ProgressSnapshot{
			"job-1": {JobID: "job-1", State: "shipping_complete"},
		}}
		fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "job-1"}}
		fix := newProgressFixture(t, fb, fp.source())
		sess := fix.openSession(t.TempDir())
		if frames := fix.prompt(sess, "explain", nil); len(frames) != 1 {
			t.Fatalf("wrote %d frames for an unpinnable snapshot, want exactly the response", len(frames))
		}
		if diag := fix.errOut.String(); !strings.Contains(diag, "pinned SOP payload states") {
			t.Errorf("diagnostics = %q, want an actionable unknown-state note", diag)
		}
		if _, _, err := buildProgressFrame("sess_x", ProgressSnapshot{State: ProgressStateRunning}); err == nil {
			t.Errorf("a snapshot without a job id was accepted")
		}
	})

	t.Run("SOP states never enter the protocol stop-reason vocabulary", func(t *testing.T) {
		want := []string{StopReasonEndTurn, StopReasonMaxTokens, StopReasonMaxTurnRequests, StopReasonRefusal, StopReasonCancelled}
		if len(StopReasons) != len(want) {
			t.Fatalf("StopReasons = %v, want the pinned five", StopReasons)
		}
		for i := range want {
			if StopReasons[i] != want[i] {
				t.Errorf("StopReasons[%d] = %q, want %q", i, StopReasons[i], want[i])
			}
		}
		for _, state := range []string{
			ProgressStateRunning, ProgressStateAwaitingApproval, ProgressStateBlocked,
			ProgressStateLocalDone, ProgressStateVerified, ProgressStateVerificationFailed,
			ProgressStateCompleted, ProgressStateFailed, ProgressStateEnded,
		} {
			for _, reason := range StopReasons {
				if reason == state {
					t.Errorf("SOP state %q leaked into the protocol stop-reason vocabulary", state)
				}
			}
		}
		if got := MethodSessionUpdate; got != "session/update" {
			t.Errorf("MethodSessionUpdate = %q, want session/update", got)
		}
	})

	t.Run("a slow client cannot exceed the bound and the terminal outcome survives coalescing", func(t *testing.T) {
		fix := newProgressFixture(t, nil, nil)
		sessID := "sess_slow"

		// Sustained progress for a client that never reads: the newest
		// snapshots coalesce onto the oldest slots, so the buffered count pins
		// at the bound and the newest snapshot stays buffered.
		for i := 1; i <= 200; i++ {
			snap := ProgressSnapshot{JobID: "job-1", Stage: fmt.Sprintf("stage-%d", i), State: ProgressStateRunning}
			frame, outcome, err := buildProgressFrame(sessID, snap)
			if err != nil || outcome {
				t.Fatalf("build progress frame %d: outcome=%v err=%v", i, outcome, err)
			}
			if err := fix.server.EnqueueProgress(frame); err != nil {
				t.Fatalf("enqueue progress frame %d: %v", i, err)
			}
			if got := fix.server.events.Len(); got > EventBufferBound {
				t.Fatalf("buffered %d frames, bound %d: memory grew without limit", got, EventBufferBound)
			}
		}
		if got := fix.server.events.Len(); got != EventBufferBound {
			t.Fatalf("buffered %d frames, want the bound %d", got, EventBufferBound)
		}
		if diag := fix.errOut.String(); !strings.Contains(diag, "coalesced") {
			t.Errorf("diagnostics = %q, want a coalescing note", diag)
		}

		// The terminal outcome is admitted even though the buffer is full: the
		// oldest progress frames drop (diagnosed) to reserve its slot.
		terminal, _, err := buildProgressFrame(sessID, ProgressSnapshot{
			JobID:    "job-1",
			State:    ProgressStateVerified,
			Verified: true,
			Evidence: []string{".agent-sdlc/outcome.md"},
		})
		if err != nil {
			t.Fatalf("build terminal frame: %v", err)
		}
		if err := fix.server.EnqueueOutcome(terminal); err != nil {
			t.Fatalf("terminal outcome was rejected under coalescing: %v", err)
		}
		if got := fix.server.events.Len(); got > EventBufferBound {
			t.Fatalf("buffered %d frames after the outcome, bound %d", got, EventBufferBound)
		}
		if diag := fix.errOut.String(); !strings.Contains(diag, "terminal outcome") {
			t.Errorf("diagnostics = %q, want the slot-reservation note", diag)
		}

		// The drain delivers every buffered frame in order, whole frames only,
		// with the terminal outcome last.
		before := fix.out.Len()
		fix.server.PumpEvents()
		chunk := strings.TrimSuffix(fix.out.String()[before:], "\n")
		if chunk == "" {
			t.Fatal("the drain wrote nothing")
		}
		delivered := strings.Split(chunk, "\n")
		if len(delivered) != EventBufferBound {
			t.Fatalf("delivered %d frames, want the buffered %d", len(delivered), EventBufferBound)
		}
		if got := fix.server.events.Len(); got != 0 {
			t.Fatalf("buffer holds %d frames after the drain, want 0", got)
		}
		seenRunning := 0
		for i, line := range delivered {
			var frame map[string]any
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatalf("delivered line %d is not one whole JSON frame: %v (%q)", i, err, line)
			}
			if got, _ := frame["method"].(string); got != MethodSessionUpdate {
				t.Fatalf("delivered line %d has method %v, want %q", i, frame["method"], MethodSessionUpdate)
			}
			sop := decodeSOP(t, frame)
			state, _ := sop["state"].(string)
			switch state {
			case ProgressStateRunning:
				seenRunning++
			case ProgressStateVerified:
				if i != len(delivered)-1 {
					t.Fatalf("the terminal outcome was delivered at position %d of %d, want last", i, len(delivered))
				}
				if got, _ := sop["verified"].(bool); got != true {
					t.Errorf("the delivered outcome lost verified=true: %v", sop)
				}
				evidence, _ := sop["evidence"].([]any)
				wantStringSlice(t, evidence, []string{".agent-sdlc/outcome.md"}, "delivered sop.evidence")
			default:
				t.Fatalf("delivered line %d carries unexpected state %q", i, state)
			}
		}
		if seenRunning != EventBufferBound-1 {
			t.Fatalf("delivered %d progress frames, want %d", seenRunning, EventBufferBound-1)
		}
		// The newest progress snapshot survived coalescing; a superseded old
		// one did not (it is absent whole, not truncated).
		var newestFrame map[string]any
		if err := json.Unmarshal([]byte(delivered[len(delivered)-2]), &newestFrame); err != nil {
			t.Fatalf("decode the newest buffered frame: %v", err)
		}
		if _, hasParams := newestFrame["params"]; !hasParams {
			t.Fatal("the newest progress frame lost its params")
		}
		newest := decodeSOP(t, newestFrame)
		if got, _ := newest["stage"].(string); got != "stage-200" {
			t.Errorf("newest buffered snapshot stage = %q, want stage-200", got)
		}
	})
}

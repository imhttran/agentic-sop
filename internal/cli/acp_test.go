package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/acp"
	"github.com/imhttran/agentic-sop/internal/projectlock"
)

// This file is ACP-003's CLI-side coverage: it proves the `sop acp` server is
// wired to the SAME governed start path `sop prompt` and MCP sop_start_work use,
// and that the ACP bridge keeps the project lock and duplicate-start rules.
//
// The launcher is faked, so no child process, model, or network is involved; the
// job bookkeeping, locking, and fingerprinting are the real implementations.

// syncBuffer is a goroutine-safe writer for the protocol stream.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// recordingLauncher records every governed start it is asked to perform.
type recordingLauncher struct {
	mu    sync.Mutex
	calls int
	dirs  []string
	args  [][]string
	code  int
}

func (r *recordingLauncher) launch(dir string, args []string, _ string) (int, func() int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.dirs = append(r.dirs, dir)
	r.args = append(r.args, append([]string(nil), args...))
	code := r.code
	return 4200 + r.calls, func() int { return code }, nil
}

func (r *recordingLauncher) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

// fakeACPEnv is the production start-work environment with a fake launcher.
func fakeACPEnv(r *recordingLauncher) workEnv {
	return workEnv{
		launch: r.launch,
		alive:  func(int) bool { return false },
		now:    time.Now,
	}
}

// acpResponse is one decoded protocol frame.
type acpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// stream drives a real Serve loop over a pipe so a test can read the session id
// the server assigned and then submit a prompt to that same session.
type stream struct {
	t   *testing.T
	pw  *io.PipeWriter
	out *syncBuffer
	seq int
}

func startStream(t *testing.T, bridge *acp.PromptSubmission) (*stream, func()) {
	t.Helper()
	pr, pw := io.Pipe()
	out := &syncBuffer{}
	server := acp.NewServer(Version, out, &syncBuffer{}, acp.WithPromptSubmission(bridge))
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), pr) }()
	s := &stream{t: t, pw: pw, out: out}
	stop := func() {
		_ = pw.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Serve: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return after EOF")
		}
	}
	return s, stop
}

func (s *stream) call(method string, params map[string]any) acpResponse {
	s.t.Helper()
	s.seq++
	frame := map[string]any{"jsonrpc": "2.0", "id": s.seq, "method": method}
	if params != nil {
		frame["params"] = params
	}
	data, err := json.Marshal(frame)
	if err != nil {
		s.t.Fatalf("marshal: %v", err)
	}
	before := len(s.out.String())
	if _, err := s.pw.Write(append(data, '\n')); err != nil {
		s.t.Fatalf("write frame: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		chunk := s.out.String()[before:]
		if i := strings.IndexByte(chunk, '\n'); i >= 0 {
			var r acpResponse
			if err := json.Unmarshal([]byte(chunk[:i]), &r); err != nil {
				s.t.Fatalf("decode response %q: %v", chunk[:i], err)
			}
			return r
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("no response for %s within the deadline", method)
		}
		time.Sleep(time.Millisecond)
	}
}

// openSession initializes the stream and opens a session bound to dir.
func (s *stream) openSession(dir string) string {
	s.t.Helper()
	if got := s.call("initialize", map[string]any{"protocolVersion": acp.ProtocolVersion}); got.Error != nil {
		s.t.Fatalf("initialize: %+v", got.Error)
	}
	got := s.call("session/new", map[string]any{"cwd": dir})
	if got.Error != nil {
		s.t.Fatalf("session/new: %+v", got.Error)
	}
	var res struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(got.Result, &res); err != nil {
		s.t.Fatalf("decode session/new: %v", err)
	}
	return res.SessionID
}

// prompt submits one text prompt and returns its decoded result.
func (s *stream) prompt(sessionID, text string, extra map[string]any) (map[string]any, *acpResponse) {
	s.t.Helper()
	params := map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	}
	for k, v := range extra {
		params[k] = v
	}
	got := s.call("session/prompt", params)
	if got.Error != nil {
		return nil, &got
	}
	var res map[string]any
	if err := json.Unmarshal(got.Result, &res); err != nil {
		s.t.Fatalf("decode session/prompt: %v", err)
	}
	return res, nil
}

// TestACPPromptFingerprintMatchesGovernedStart proves the adapter's fingerprint is
// byte-for-byte the one `sop prompt` work records use. Without this, a resubmitted
// prompt could never reconcile against work started by the CLI.
func TestACPPromptFingerprintMatchesGovernedStart(t *testing.T) {
	for _, capability := range []string{"plan", "review", "design_tests", "implement"} {
		for _, text := range []string{"explain the workflow", "  padded text  ", "line one\nline two"} {
			want := workFingerprint(workKindPrompt, capability, text)
			if got := acp.PromptFingerprint(capability, text); got != want {
				t.Errorf("PromptFingerprint(%q, %q) = %s, want %s", capability, text, got, want)
			}
		}
	}
}

// newACPProjectDir returns an initialized project directory whose cleanup
// tolerates the asynchronous exit recorder a governed start leaves behind.
// t.TempDir's strict RemoveAll can fail with "directory not empty" when that
// recorder writes after the test body returns, so removal is retried instead of
// failing an otherwise passing test.
func newACPProjectDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sop-acp-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 100; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
	initProject(t, dir)
	return dir
}

// TestACPPromptSubmissionCLIIntegration drives the real Serve loop through the
// real bridge: one accepted submission starts exactly one governed job, a resent
// submission is replayed, and a different key for the same work reconciles
// against the persisted job instead of starting a second one.
func TestACPPromptSubmissionCLIIntegration(t *testing.T) {
	dir := newACPProjectDir(t)

	launcher := &recordingLauncher{}
	s, stop := startStream(t, acpSubmission(fakeACPEnv(launcher)))
	defer stop()

	sess := s.openSession(dir)
	res, rerr := s.prompt(sess, "explain the governed workflow", map[string]any{"submissionKey": "k1"})
	if rerr != nil {
		t.Fatalf("prompt: %+v", rerr.Error)
	}
	if got, _ := res["outcome"].(string); got != acp.WorkOutcomeStarted {
		t.Fatalf("outcome = %q, want %q (result %v)", got, acp.WorkOutcomeStarted, res)
	}
	if jobID, _ := res["jobId"].(string); jobID == "" {
		t.Fatalf("started result carries no jobId: %v", res)
	}
	if launcher.count() != 1 {
		t.Fatalf("launcher calls = %d, want 1", launcher.count())
	}
	if got := launcher.args[0]; len(got) < 2 || got[0] != "prompt" || got[1] != "--capability" {
		t.Errorf("launched argv = %v, want the governed `prompt --capability ...` form", got)
	}

	// Same submission key: the adapter replays its recorded result.
	again, rerr := s.prompt(sess, "explain the governed workflow", map[string]any{"submissionKey": "k1"})
	if rerr != nil {
		t.Fatalf("resent prompt: %+v", rerr.Error)
	}
	if replayed, _ := again["replayed"].(bool); !replayed {
		t.Errorf("resent submission was not replayed: %v", again)
	}
	if launcher.count() != 1 {
		t.Fatalf("launcher calls after replay = %d, want 1", launcher.count())
	}

	// A new key for the same text reconciles against the persisted job: still
	// exactly one work item.
	third, rerr := s.prompt(sess, "explain the governed workflow", map[string]any{"submissionKey": "k2"})
	if rerr != nil {
		t.Fatalf("reconciling prompt: %+v", rerr.Error)
	}
	switch outcome, _ := third["outcome"].(string); outcome {
	case acp.WorkOutcomeReconciled, acp.WorkOutcomeAlreadyRunning:
	default:
		t.Errorf("outcome for the same work under a new key = %v, want reconciled or already_running", third)
	}
	if launcher.count() != 1 {
		t.Fatalf("launcher calls after reconcile = %d, want 1 (no duplicate work item)", launcher.count())
	}
}

// TestACPPromptBridgeNotInitialized proves the bridge reports a project without
// SOP state instead of starting a child.
func TestACPPromptBridgeNotInitialized(t *testing.T) {
	dir := t.TempDir() // no .agent-sdlc/state.db
	launcher := &recordingLauncher{}
	bridge := acpSubmission(fakeACPEnv(launcher))

	res, err := bridge.Starter(acp.Submission{ProjectDir: dir, Capability: "plan", Text: "explain"})
	if err != nil {
		t.Fatalf("Starter: %v", err)
	}
	if res.Outcome != acp.WorkOutcomeNotInitialized {
		t.Errorf("outcome = %q, want %q", res.Outcome, acp.WorkOutcomeNotInitialized)
	}
	if launcher.count() != 0 {
		t.Errorf("launcher calls = %d, want 0", launcher.count())
	}
}

// TestACPPromptBridgeReportsBusyWhenProjectLockHeld proves the bridge maps a
// contended project writer lock to ErrProjectBusy instead of queueing.
func TestACPPromptBridgeReportsBusyWhenProjectLockHeld(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	lock, err := projectlock.Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold project lock: %v", err)
	}
	defer lock.Release()

	restore := acpBusyProbeTimeout
	acpBusyProbeTimeout = 50 * time.Millisecond
	defer func() { acpBusyProbeTimeout = restore }()

	bridge := acpSubmission(fakeACPEnv(&recordingLauncher{}))
	release, err := bridge.LockTaker(context.Background(), dir)
	if !errors.Is(err, acp.ErrProjectBusy) {
		if release != nil {
			release()
		}
		t.Fatalf("LockTaker error = %v, want ErrProjectBusy", err)
	}
	if release != nil {
		t.Error("a contended lock returned a release function")
	}
}

// TestACPPromptBridgeStateReaderReadsPersistedJobs proves the bridge exposes the
// governed job records the adapter reconciles against, including the state.
func TestACPPromptBridgeStateReaderReadsPersistedJobs(t *testing.T) {
	dir := newACPProjectDir(t)

	launcher := &recordingLauncher{}
	bridge := acpSubmission(fakeACPEnv(launcher))
	res, err := bridge.Starter(acp.Submission{ProjectDir: dir, Capability: "plan", Text: "explain"})
	if err != nil {
		t.Fatalf("Starter: %v", err)
	}
	if res.Outcome != acp.WorkOutcomeStarted || res.JobID == "" {
		t.Fatalf("Starter result = %+v, want a started job id", res)
	}

	records, err := bridge.StateReader(dir)
	if err != nil {
		t.Fatalf("StateReader: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("StateReader returned no persisted records")
	}
	found := false
	for _, r := range records {
		if r.JobID != res.JobID {
			continue
		}
		found = true
		if r.Fingerprint != acp.PromptFingerprint("plan", "explain") {
			t.Errorf("record fingerprint = %q, want the governed prompt fingerprint", r.Fingerprint)
		}
		switch r.State {
		case "running", "completed", "failed", "ended":
		default:
			t.Errorf("record state = %q, want a derived started-work state", r.State)
		}
	}
	if !found {
		t.Errorf("StateReader did not report the started job %s: %+v", res.JobID, records)
	}
}

// TestACPRunACPServesInitialization proves the `acp` subcommand serves the
// protocol over the injected stdin and exits cleanly at EOF.
func TestACPRunACPServesInitialization(t *testing.T) {
	dir := t.TempDir()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"v1"}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":%q}}`, dir),
	}, "\n") + "\n"

	var out, errOut bytes.Buffer
	d := deps{stdin: strings.NewReader(input)}
	if code := run([]string{"acp"}, &out, &errOut, d); code != 0 {
		t.Fatalf("sop acp exit = %d, stderr = %s", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d frames, want 2: %q", len(lines), out.String())
	}
	for _, line := range lines {
		var r acpResponse
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("stdout carried a non-frame line %q: %v", line, err)
		}
		if r.Error != nil {
			t.Fatalf("frame error: %+v", r.Error)
		}
	}
}

// TestACPPromptConcurrentStartsCreateOneWorkItem is the competing-execution
// regression this bridge's locking claim rests on. Two ACP sessions submit the
// same prompt at the same time, after the project-lock probe has already
// released, so the only thing that can keep them apart is startWork's
// cross-process start lock plus its re-read of the persisted jobs under it.
//
// Exactly one governed start may run. The loser either observes the winner's
// job (already_running / reconciled) or is refused the start lock; neither may
// launch a second work item.
func TestACPPromptConcurrentStartsCreateOneWorkItem(t *testing.T) {
	dir := newACPProjectDir(t)

	launcher := &recordingLauncher{}
	bridge := acpSubmission(fakeACPEnv(launcher))
	fp := acp.PromptFingerprint("plan", "concurrent submission")
	sub := acp.Submission{
		ProjectDir:    dir,
		Capability:    "plan",
		Text:          "concurrent submission",
		Fingerprint:   fp,
		SubmissionKey: fp,
	}

	const racers = 2
	results := make([]acp.SubmissionResult, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start // release both racers together
			results[i], errs[i] = bridge.Starter(sub)
		}(i)
	}
	close(start)
	wg.Wait()

	if got := launcher.count(); got != 1 {
		t.Fatalf("governed launches = %d, want 1: concurrent starts must not create a second work item", got)
	}
	started := 0
	for i := range results {
		if errs[i] != nil {
			// The start lock is held by the winner: a refusal is the other safe
			// outcome, and it must not be reported as a started job.
			continue
		}
		switch results[i].Outcome {
		case acp.WorkOutcomeStarted:
			started++
		case acp.WorkOutcomeAlreadyRunning, acp.WorkOutcomeReconciled, acp.WorkOutcomeBusy, acp.WorkOutcomeTaskInProgress:
		default:
			t.Errorf("racer %d outcome = %q, want started or a non-starting disposition", i, results[i].Outcome)
		}
	}
	if started != 1 {
		t.Errorf("started outcomes = %d, want exactly 1 (results=%+v errors=%v)", started, results, errs)
	}
}

// blockingLauncher keeps the governed child "running" until release is closed,
// so a cancellation test can act while the job has no exit record.
type blockingLauncher struct {
	pid     int
	release chan struct{}
}

func (l *blockingLauncher) launch(string, []string, string) (int, func() int, error) {
	return l.pid, func() int { <-l.release; return 0 }, nil
}

// cancelTestEnv builds a start-work environment whose child stays alive until the
// injected stopper runs, so the cancellation path is exercised without signalling
// a real process. It records whether each stop was forced.
func cancelTestEnv(launcher *blockingLauncher, alive *atomic.Bool, mu *sync.Mutex, signals *[]bool) workEnv {
	return workEnv{
		launch: launcher.launch,
		alive:  func(int) bool { return alive.Load() },
		now:    time.Now,
		stop: func(_ int, force bool) error {
			mu.Lock()
			*signals = append(*signals, force)
			mu.Unlock()
			alive.Store(false) // the child exits on the first signal
			return nil
		},
	}
}

// TestACPPromptCancelRecordsCancelledState proves the bridge records a confirmed
// cancellation in the job's own record, sends the unforced signal first, and the
// adapter's StateReader reports it as a distinct "cancelled" state.
func TestACPPromptCancelRecordsCancelledState(t *testing.T) {
	dir := newACPProjectDir(t)
	launcher := &blockingLauncher{pid: 9101, release: make(chan struct{})}
	t.Cleanup(func() { close(launcher.release) })
	var alive atomic.Bool
	alive.Store(true)
	var mu sync.Mutex
	var signals []bool
	bridge := acpSubmission(cancelTestEnv(launcher, &alive, &mu, &signals))

	res, err := bridge.Starter(acp.Submission{ProjectDir: dir, Capability: "plan", Text: "cancel me"})
	if err != nil {
		t.Fatalf("Starter: %v", err)
	}
	if res.Outcome != acp.WorkOutcomeStarted || res.JobID == "" {
		t.Fatalf("Starter result = %+v, want a started job", res)
	}
	if err := bridge.Cancel(context.Background(), dir, res.JobID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	mu.Lock()
	got := append([]bool(nil), signals...)
	mu.Unlock()
	if len(got) != 1 || got[0] {
		t.Fatalf("stop signals = %v, want exactly one unforced signal", got)
	}

	records, err := bridge.StateReader(dir)
	if err != nil {
		t.Fatalf("StateReader: %v", err)
	}
	state := ""
	for _, r := range records {
		if r.JobID == res.JobID {
			state = r.State
		}
	}
	if state != "cancelled" {
		t.Errorf("persisted state = %q, want %q (a stopped job must not read as failed or ended)", state, "cancelled")
	}
}

// TestACPPromptCancelEscalatesToForcedStop proves an unresponsive child gets the
// forced signal within the bound, and the cancellation is still recorded once.
func TestACPPromptCancelEscalatesToForcedStop(t *testing.T) {
	restoreStop, restorePoll := workCancelStop, workCancelPoll
	workCancelStop, workCancelPoll = 150*time.Millisecond, 10*time.Millisecond
	defer func() { workCancelStop, workCancelPoll = restoreStop, restorePoll }()

	dir := newACPProjectDir(t)
	launcher := &blockingLauncher{pid: 9102, release: make(chan struct{})}
	t.Cleanup(func() { close(launcher.release) })
	var alive atomic.Bool
	alive.Store(true)
	var mu sync.Mutex
	var signals []bool
	env := workEnv{
		launch: launcher.launch,
		alive:  func(int) bool { return alive.Load() },
		now:    time.Now,
		stop: func(_ int, force bool) error {
			mu.Lock()
			signals = append(signals, force)
			mu.Unlock()
			if force {
				alive.Store(false) // only the forced signal is obeyed
			}
			return nil
		},
	}
	bridge := acpSubmission(env)
	res, err := bridge.Starter(acp.Submission{ProjectDir: dir, Capability: "plan", Text: "stubborn"})
	if err != nil {
		t.Fatalf("Starter: %v", err)
	}
	if err := bridge.Cancel(context.Background(), dir, res.JobID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	mu.Lock()
	got := append([]bool(nil), signals...)
	mu.Unlock()
	if len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("stop signals = %v, want [false true]", got)
	}
}

// TestACPPromptCancelLeavesAFinishedJobAlone proves completion wins the race: a
// job that recorded its exit before the stop request keeps its own outcome and no
// cancellation marker is written.
func TestACPPromptCancelLeavesAFinishedJobAlone(t *testing.T) {
	dir := newACPProjectDir(t)
	launcher := &recordingLauncher{} // wait returns 0 immediately: the job completes
	var alive atomic.Bool
	alive.Store(false)
	var mu sync.Mutex
	var signals []bool
	bridge := acpSubmission(workEnv{
		launch: launcher.launch,
		alive:  func(int) bool { return alive.Load() },
		now:    time.Now,
		stop: func(_ int, force bool) error {
			mu.Lock()
			signals = append(signals, force)
			mu.Unlock()
			return nil
		},
	})

	res, err := bridge.Starter(acp.Submission{ProjectDir: dir, Capability: "plan", Text: "already done"})
	if err != nil {
		t.Fatalf("Starter: %v", err)
	}
	exitPath := filepath.Join(workRoot(dir), res.JobID, workExitFile)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, statErr := os.Stat(exitPath); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the job never recorded its exit at %s", exitPath)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := bridge.Cancel(context.Background(), dir, res.JobID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	mu.Lock()
	got := append([]bool(nil), signals...)
	mu.Unlock()
	if len(got) != 0 {
		t.Errorf("a finished job was signalled: %v", got)
	}
	records, err := bridge.StateReader(dir)
	if err != nil {
		t.Fatalf("StateReader: %v", err)
	}
	state := ""
	for _, r := range records {
		if r.JobID == res.JobID {
			state = r.State
		}
	}
	if state == "cancelled" {
		t.Errorf("a completed job was relabelled %q", state)
	}
	if state != "completed" {
		t.Errorf("persisted state = %q, want completed", state)
	}
}

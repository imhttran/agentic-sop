package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/mcp"
	"github.com/imhttran/agentic-sop/internal/planflow"
)

// fakeWork is a controllable start-work environment: launches are recorded, each
// launched job "runs" until finish is called, and liveness is explicit.
type fakeWork struct {
	mu        sync.Mutex
	launches  [][]string
	nextPID   int
	live      map[int]bool
	done      map[int]chan int
	launchErr error
	clock     time.Time
	// recorded notes, per launch, whether job.json already existed when the
	// process was started.
	recorded []bool
}

func newFakeWork() *fakeWork {
	return &fakeWork{nextPID: 4000, live: map[int]bool{}, done: map[int]chan int{}, clock: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
}

func (f *fakeWork) env() workEnv {
	return workEnv{
		launch: func(dir string, args []string, logPath string) (int, func() int, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.launchErr != nil {
				return 0, nil, f.launchErr
			}
			_, statErr := os.Stat(filepath.Join(filepath.Dir(logPath), workJobFile))
			f.recorded = append(f.recorded, statErr == nil)
			f.nextPID++
			pid := f.nextPID
			f.launches = append(f.launches, append([]string(nil), args...))
			f.live[pid] = true
			ch := make(chan int, 1)
			f.done[pid] = ch
			_ = os.WriteFile(logPath, []byte("SOP\nstarted "+args[0]+"\n"), 0o644)
			return pid, func() int { return <-ch }, nil
		},
		alive: func(pid int) bool {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.live[pid]
		},
		now: func() time.Time {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.clock = f.clock.Add(time.Second)
			return f.clock
		},
	}
}

// finish ends a job with code and waits until the server recorded the exit.
func (f *fakeWork) finish(t *testing.T, dir string, pid, code int) {
	t.Helper()
	f.mu.Lock()
	ch := f.done[pid]
	f.mu.Unlock()
	ch <- code
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := listWorkJobs(dir, f.env())
		for _, j := range jobs {
			if j.PID == pid && j.ExitCode != nil {
				f.mu.Lock()
				f.live[pid] = false
				f.mu.Unlock()
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("exit for pid %d was not recorded", pid)
}

// vanish makes a job's process disappear without an exit record.
func (f *fakeWork) vanish(pid int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.live[pid] = false
}

type startDoc struct {
	Outcome           string   `json:"outcome"`
	Message           string   `json:"message"`
	Tasks             []string `json:"tasks"`
	InProgress        []string `json:"in_progress"`
	ActivePlan        string   `json:"active_plan"`
	AllTasksSatisfied *bool    `json:"all_tasks_satisfied"`
	Job               *struct {
		ID          string   `json:"id"`
		Kind        string   `json:"kind"`
		Capability  string   `json:"capability"`
		Source      string   `json:"source"`
		Fingerprint string   `json:"fingerprint"`
		SourceSHA   string   `json:"source_sha256"`
		Args        []string `json:"args"`
		PID         int      `json:"pid"`
		State       string   `json:"state"`
		ExitCode    *int     `json:"exit_code"`
		Log         string   `json:"log"`
		LogTail     string   `json:"log_tail"`
	} `json:"job"`
	Jobs []map[string]any `json:"jobs"`
}

func callStart(t *testing.T, dir string, f *fakeWork, args map[string]any) (startDoc, error) {
	t.Helper()
	raw, _ := json.Marshal(args)
	out, err := mcpStartWorkTool(dir, f.env()).Handler(context.Background(), raw)
	if err != nil {
		return startDoc{}, err
	}
	var doc startDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("decode %s: %v", out, err)
	}
	return doc, nil
}

func mustStart(t *testing.T, dir string, f *fakeWork, args map[string]any, want string) startDoc {
	t.Helper()
	doc, err := callStart(t, dir, f, args)
	if err != nil {
		t.Fatalf("sop_start_work %v: %v", args, err)
	}
	if doc.Outcome != want {
		t.Fatalf("outcome = %q (%s), want %q", doc.Outcome, doc.Message, want)
	}
	return doc
}

func TestStartWorkToolsShape(t *testing.T) {
	tools := mcpStartWorkTools(t.TempDir(), newFakeWork().env())
	start := toolByName(t, tools, "sop_start_work")
	status := toolByName(t, tools, "sop_work_status")
	if start.ReadOnly {
		t.Error("sop_start_work must not be read-only")
	}
	if !status.ReadOnly {
		t.Error("sop_work_status must be read-only")
	}
	// The opt-in tools never appear on the default surfaces.
	for _, set := range [][]string{toolNames(mcpTools(t.TempDir(), deps{})), toolNames(mcpPlannerTools(t.TempDir(), deps{}))} {
		for _, n := range set {
			if n == "sop_start_work" || n == "sop_work_status" {
				t.Errorf("%s must be opt-in, found on a default surface", n)
			}
		}
	}
}

func toolNames(tools []mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	return names
}

func TestMCPStartWorkFlagParsing(t *testing.T) {
	var out, errOut bytes.Buffer
	d := deps{getwd: func() (string, error) { return t.TempDir(), nil }}
	if code := runMCP([]string{"--status-only", "--start-work"}, &out, &errOut, d); code != exitUsage {
		t.Fatalf("--status-only --start-work code = %d, want usage", code)
	}
	errOut.Reset()
	if code := runMCP([]string{"--bogus"}, &out, &errOut, d); code != exitUsage || !strings.Contains(errOut.String(), "--start-work") {
		t.Fatalf("unknown flag: code=%d usage=%q", code, errOut.String())
	}
}

func TestStartWorkPromptLifecycle(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	f := newFakeWork()

	doc := mustStart(t, dir, f, map[string]any{"prompt": "  Add a CSV export to the report page.  ", "capability": "implement"}, workOutcomeStarted)
	if doc.Job == nil || doc.Job.State != workStateRunning || doc.Job.Kind != workKindPrompt || doc.Job.Capability != "implement" {
		t.Fatalf("started job = %+v", doc.Job)
	}
	if len(f.launches) != 1 {
		t.Fatalf("launches = %v", f.launches)
	}
	args := f.launches[0]
	if args[0] != "prompt" || args[1] != "--capability" || args[2] != "implement" || args[3] != "--file" {
		t.Fatalf("launch args = %v", args)
	}
	// The prompt is passed as a project-local file, never as shell text.
	data, err := os.ReadFile(args[4])
	if err != nil || strings.TrimSpace(string(data)) != "Add a CSV export to the report page." {
		t.Fatalf("prompt file %s = %q, %v", args[4], data, err)
	}
	if _, err := resolvePromptFile(dir, args[4]); err != nil {
		t.Fatalf("prompt file must be confined to the project: %v", err)
	}
	if !strings.Contains(doc.Job.LogTail, "started prompt") {
		t.Errorf("log tail = %q", doc.Job.LogTail)
	}

	// The same prompt while it runs: status, no second launch. Whitespace does not
	// make it different work.
	again := mustStart(t, dir, f, map[string]any{"prompt": "Add a CSV export to the report page.", "capability": "implement"}, workOutcomeAlreadyRunning)
	if again.Job.ID != doc.Job.ID {
		t.Errorf("already_running job = %s, want %s", again.Job.ID, doc.Job.ID)
	}
	// Different work while a job runs: refused, with the running job's status.
	busy := mustStart(t, dir, f, map[string]any{"prompt": "Something else"}, workOutcomeBusy)
	if busy.Job.ID != doc.Job.ID {
		t.Errorf("busy names job %s, want the running %s", busy.Job.ID, doc.Job.ID)
	}
	// Even rerun never starts a second copy of running work.
	mustStart(t, dir, f, map[string]any{"prompt": "Add a CSV export to the report page.", "capability": "implement", "rerun": true}, workOutcomeAlreadyRunning)
	if len(f.launches) != 1 {
		t.Fatalf("no further launch expected, got %v", f.launches)
	}

	f.finish(t, dir, doc.Job.PID, 0)

	// Same work after it finished: status unless rerun.
	ran := mustStart(t, dir, f, map[string]any{"prompt": "Add a CSV export to the report page.", "capability": "implement"}, workOutcomeAlreadyRan)
	if ran.Job.State != workStateCompleted || ran.Job.ExitCode == nil || *ran.Job.ExitCode != 0 {
		t.Fatalf("already_ran job = %+v", ran.Job)
	}
	// A different capability is different work.
	mustStart(t, dir, f, map[string]any{"prompt": "Add a CSV export to the report page.", "capability": "review"}, workOutcomeStarted)
	f.finish(t, dir, f.nextPID, 1)
	rerun := mustStart(t, dir, f, map[string]any{"prompt": "Add a CSV export to the report page.", "capability": "implement", "rerun": true}, workOutcomeStarted)
	if rerun.Job.ID == doc.Job.ID {
		t.Fatal("rerun must create a new job")
	}
	if len(f.launches) != 3 {
		t.Fatalf("launches = %d, want 3", len(f.launches))
	}
}

func TestStartWorkFailedAndVanishedJobs(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	f := newFakeWork()

	first := mustStart(t, dir, f, map[string]any{"prompt": "review the store"}, workOutcomeStarted)
	f.finish(t, dir, first.Job.PID, 2)
	failed := mustStart(t, dir, f, map[string]any{"prompt": "review the store"}, workOutcomeAlreadyRan)
	if failed.Job.State != workStateFailed || *failed.Job.ExitCode != 2 {
		t.Fatalf("failed job = %+v", failed.Job)
	}

	second := mustStart(t, dir, f, map[string]any{"prompt": "diagnose the flake", "capability": "diagnose_failure"}, workOutcomeStarted)
	// The server stopped before the child finished: no exit record, process gone.
	f.vanish(second.Job.PID)
	ended := mustStart(t, dir, f, map[string]any{"prompt": "diagnose the flake", "capability": "diagnose_failure"}, workOutcomeAlreadyRan)
	if ended.Job.State != workStateEnded || ended.Job.ExitCode != nil {
		t.Fatalf("ended job = %+v", ended.Job)
	}
	// A vanished job no longer blocks new work.
	mustStart(t, dir, f, map[string]any{"prompt": "new work"}, workOutcomeStarted)
}

func TestStartWorkPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "export", Status: domain.PLANNED, MaxAttempts: 3})
	plan := "# Plan\n\n## T001 export\n"
	writeFileT(t, dir, "docs/PLAN.md", plan)
	f := newFakeWork()

	// A plan that is not the active plan starts `sop run PLAN`.
	doc := mustStart(t, dir, f, map[string]any{"plan_path": "docs/PLAN.md"}, workOutcomeStarted)
	if got := f.launches[0]; len(got) != 2 || got[0] != "run" || got[1] != "docs/PLAN.md" {
		t.Fatalf("launch args = %v", got)
	}
	if doc.Job.SourceSHA != planflow.SourceFingerprint([]byte(plan)) || doc.Job.Source != "docs/PLAN.md" {
		t.Fatalf("plan job = %+v", doc.Job)
	}
	if len(doc.Tasks) != 1 || !strings.HasPrefix(doc.Tasks[0], "T001 PLANNED") {
		t.Errorf("tasks = %v", doc.Tasks)
	}
	f.finish(t, dir, doc.Job.PID, 0)

	// Once SOP holds this exact plan, it reports status instead of starting it.
	writeFileT(t, dir, filepath.Join(config.DirName, planMetaName),
		`{"source":"docs/PLAN.md","source_kind":"plan","source_sha256":"`+planflow.SourceFingerprint([]byte(plan))+`","plan_id":"plan-x"}`)
	held := mustStart(t, dir, f, map[string]any{"plan_path": "docs/PLAN.md"}, workOutcomeInMemory)
	if held.AllTasksSatisfied == nil || *held.AllTasksSatisfied {
		t.Errorf("all_tasks_satisfied = %v, want false", held.AllTasksSatisfied)
	}
	if held.Job == nil || held.Job.ID != doc.Job.ID {
		t.Errorf("in_memory should include the earlier job, got %+v", held.Job)
	}
	// rerun hands it back to `sop run`, which resumes.
	mustStart(t, dir, f, map[string]any{"plan_path": "docs/PLAN.md", "rerun": true}, workOutcomeStarted)
	f.finish(t, dir, f.nextPID, 0)

	// An edited active plan would stop `sop run` at the plan-changed gate, so it
	// is reported up front instead of starting a job that fails.
	writeFileT(t, dir, "docs/PLAN.md", plan+"\n## T002 more\n")
	changed := mustStart(t, dir, f, map[string]any{"plan_path": "docs/PLAN.md", "rerun": true}, workOutcomePlanBlocked)
	if !strings.Contains(changed.Message, "plan changed") || changed.ActivePlan != "docs/PLAN.md" {
		t.Errorf("plan_blocked = %q (active %q)", changed.Message, changed.ActivePlan)
	}
	if len(f.launches) != 2 {
		t.Fatalf("launches = %d, want 2", len(f.launches))
	}
	for i, ok := range f.recorded {
		if !ok {
			t.Errorf("launch %d started before its job record was written", i)
		}
	}
}

func TestStartWorkPlanBlockedByActivePlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "AP-101", Title: "photos", Status: domain.PLANNED, MaxAttempts: 3})
	active := "# Plan\n\n## AP-101 photos\n"
	writeFileT(t, dir, "docs/plans/PLAN-ACTIVE.md", active)
	writeFileT(t, dir, filepath.Join(config.DirName, planMetaName),
		`{"source":"docs/plans/PLAN-ACTIVE.md","source_kind":"plan","source_sha256":"`+planflow.SourceFingerprint([]byte(active))+`","plan_id":"plan-active"}`)
	writeFileT(t, dir, "docs/plans/PLAN-NEXT.md", "# Plan\n\n## NX-1 next\n")
	f := newFakeWork()

	// A different plan while the active plan has unfinished work: blocked, with
	// the same NEEDS_HUMAN explanation `sop run` would print. rerun cannot skip it.
	for _, rerun := range []bool{false, true} {
		doc := mustStart(t, dir, f, map[string]any{"plan_path": "docs/plans/PLAN-NEXT.md", "rerun": rerun}, workOutcomePlanBlocked)
		if !strings.Contains(doc.Message, "different plan is already active") || doc.ActivePlan != "docs/plans/PLAN-ACTIVE.md" {
			t.Fatalf("plan_blocked = %q (active %q)", doc.Message, doc.ActivePlan)
		}
	}
	if len(f.launches) != 0 {
		t.Fatalf("a blocked plan must not launch, got %v", f.launches)
	}

	// Once the active plan is finished, `sop run` hands off, so the next plan starts.
	seedTask(t, dir, &domain.Task{ID: "AP-101", Title: "photos", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	mustStart(t, dir, f, map[string]any{"plan_path": "docs/plans/PLAN-NEXT.md"}, workOutcomeStarted)
}

func TestStartWorkRefusesWhileATaskIsMidRun(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "one", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "two", Status: domain.IMPLEMENTING, MaxAttempts: 3})
	writeFileT(t, dir, "docs/PLAN.md", "# Plan\n")
	f := newFakeWork()

	for _, args := range []map[string]any{
		{"prompt": "review the store", "capability": "review"},
		{"plan_path": "docs/PLAN.md"},
		{"plan_path": "docs/PLAN.md", "rerun": true},
	} {
		doc := mustStart(t, dir, f, args, workOutcomeTaskInProgress)
		if len(doc.InProgress) != 1 || doc.InProgress[0] != "T002 (IMPLEMENTING)" {
			t.Fatalf("in_progress = %v", doc.InProgress)
		}
		if !strings.Contains(doc.Message, "sop resume") {
			t.Errorf("message should point at recovery: %q", doc.Message)
		}
	}
	if len(f.launches) != 0 {
		t.Fatalf("nothing may launch while a task is mid-run, got %v", f.launches)
	}

	// Settled states never block.
	for _, st := range []domain.TaskStatus{domain.PLANNED, domain.READY, domain.BLOCKED, domain.NOT_REQUIRED, domain.LOCAL_DONE} {
		if workMidRunStatuses[st] {
			t.Errorf("%s must not count as mid-run", st)
		}
	}
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "two", Status: domain.BLOCKED, MaxAttempts: 3})
	mustStart(t, dir, f, map[string]any{"prompt": "review the store", "capability": "review"}, workOutcomeStarted)
}

func TestStartWorkPlanIdentityIncludesPath(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	same := "# Plan\n\n## T001 x\n"
	writeFileT(t, dir, "docs/plans/A.md", same)
	writeFileT(t, dir, "docs/plans/B.md", same)
	f := newFakeWork()

	a := mustStart(t, dir, f, map[string]any{"plan_path": "docs/plans/A.md"}, workOutcomeStarted)
	f.finish(t, dir, a.Job.PID, 0)
	// Same content at another path is different work, not "already ran".
	b := mustStart(t, dir, f, map[string]any{"plan_path": "docs/plans/B.md"}, workOutcomeStarted)
	if a.Job.Fingerprint == b.Job.Fingerprint || a.Job.SourceSHA != b.Job.SourceSHA {
		t.Fatalf("fingerprints a=%s b=%s, content a=%s b=%s", a.Job.Fingerprint, b.Job.Fingerprint, a.Job.SourceSHA, b.Job.SourceSHA)
	}
}

func TestStartWorkRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	outside := filepath.Join(t.TempDir(), "PLAN.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "LINK.md")); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, dir, "EMPTY.md", "  \n")
	f := newFakeWork()

	cases := map[string]map[string]any{
		"neither":             {},
		"both":                {"prompt": "x", "plan_path": "docs/PLAN.md"},
		"capability for plan": {"plan_path": "EMPTY.md", "capability": "plan"},
		"unknown capability":  {"prompt": "x", "capability": "fix"},
		"absolute outside":    {"plan_path": outside},
		"symlink outside":     {"plan_path": "LINK.md"},
		"missing plan":        {"plan_path": "docs/NOPE.md"},
		"empty plan":          {"plan_path": "EMPTY.md"},
		"oversized prompt":    {"prompt": strings.Repeat("a", maxWorkPromptBytes+1)},
	}
	for name, args := range cases {
		if _, err := callStart(t, dir, f, args); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(f.launches) != 0 {
		t.Fatalf("nothing may launch on bad input, got %v", f.launches)
	}
	if _, err := os.Stat(filepath.Join(workRoot(dir), workLockFile)); !os.IsNotExist(err) {
		t.Errorf("start lock left behind: %v", err)
	}
}

func TestStartWorkNotInitialized(t *testing.T) {
	dir := t.TempDir()
	f := newFakeWork()
	doc := mustStart(t, dir, f, map[string]any{"prompt": "x"}, "not_initialized")
	if doc.Message != notInitializedMsg {
		t.Errorf("message = %q", doc.Message)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName)); !os.IsNotExist(err) {
		t.Errorf("an uninitialized project must not gain state: %v", err)
	}
}

func TestStartWorkLaunchFailureLeavesNoJob(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	f := newFakeWork()
	f.launchErr = os.ErrPermission
	if _, err := callStart(t, dir, f, map[string]any{"prompt": "x"}); err == nil {
		t.Fatal("expected launch error")
	}
	if jobs, _ := listWorkJobs(dir, f.env()); len(jobs) != 0 {
		t.Fatalf("a failed launch must leave no job, got %d", len(jobs))
	}
}

func TestStartWorkLock(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	clock := func() time.Time { return now }
	unlock, err := acquireWorkLock(root, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireWorkLock(root, clock); err == nil {
		t.Fatal("a held lock must refuse a second start")
	}
	// A lock left by a dead server is broken once stale.
	later := func() time.Time { return now.Add(workLockStale + time.Minute) }
	unlock2, err := acquireWorkLock(root, later)
	if err != nil {
		t.Fatalf("stale lock not broken: %v", err)
	}
	unlock2()
	unlock()
}

func TestWorkStatusTool(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	f := newFakeWork()
	call := func(args string) startDoc {
		t.Helper()
		out, err := mcpWorkStatusTool(dir, f.env()).Handler(context.Background(), json.RawMessage(args))
		if err != nil {
			t.Fatalf("sop_work_status %s: %v", args, err)
		}
		var doc startDoc
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	if doc := call(`{}`); doc.Message != "no work has been started over MCP" {
		t.Errorf("empty status = %+v", doc)
	}
	a := mustStart(t, dir, f, map[string]any{"prompt": "a"}, workOutcomeStarted)
	f.finish(t, dir, a.Job.PID, 0)
	b := mustStart(t, dir, f, map[string]any{"prompt": "b"}, workOutcomeStarted)

	list := call(`{}`)
	if len(list.Jobs) != 2 || list.Jobs[0]["id"] != b.Job.ID || list.Jobs[0]["state"] != workStateRunning || list.Jobs[1]["state"] != workStateCompleted {
		t.Fatalf("jobs = %v", list.Jobs)
	}
	one := call(`{"job_id":"` + a.Job.ID + `"}`)
	if one.Job == nil || one.Job.ID != a.Job.ID || one.Job.State != workStateCompleted {
		t.Fatalf("job = %+v", one.Job)
	}
	if _, err := mcpWorkStatusTool(dir, f.env()).Handler(context.Background(), json.RawMessage(`{"job_id":"nope"}`)); err == nil {
		t.Error("unknown job id must be an error")
	}
}

func TestWorkPIDAlive(t *testing.T) {
	if !workPIDAlive(os.Getpid()) {
		t.Error("this process must be alive")
	}
	if workPIDAlive(0) || workPIDAlive(-1) {
		t.Error("non-positive pids are never alive")
	}
}

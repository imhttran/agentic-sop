package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/mcp"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// sop_start_work lets an MCP client hand SOP a unit of work — prompt text (for
// example the last prompt planned in a chat) or a PLAN.md inside the project —
// and have SOP start it in the background. It never runs the work itself and
// never reimplements the workflow: it launches the same `sop prompt` or `sop run`
// command an operator would type, as a detached child process, and records the
// launch under .agent-sdlc/runs/mcp-work/<job-id>/.
//
// Before launching it checks what SOP already holds. If the same work is
// running, already ran, or (for a plan) is the active plan SOP has loaded, it
// returns that status instead of starting it again. Only one MCP-started job may
// run per project at a time, so two `sop run` processes never drive the same
// state concurrently.
//
// Both tools are opt-in (`sop mcp --start-work`, or `sop mcp --planner
// --start-work`); no default surface changes. Starting work never approves a
// gate: the started command stops at the human approval boundary exactly as it
// does when typed in a terminal.

const (
	// workDirName is the run subdirectory holding MCP-started jobs.
	workDirName = "mcp-work"
	// workJobFile, workExitFile, workLogFile and workPromptFile are a job's files.
	workJobFile    = "job.json"
	workExitFile   = "exit.json"
	workLogFile    = "output.log"
	workPromptFile = "prompt.md"
	// workLockFile serializes the check-then-launch step across processes.
	workLockFile = ".start.lock"
	// workLockStale is how old a leftover start lock must be before it is broken.
	workLockStale = 2 * time.Minute
	// workLogTailLines bounds the log excerpt returned with a status.
	workLogTailLines = 20
	// maxWorkPromptBytes bounds prompt text accepted over MCP.
	maxWorkPromptBytes = 256 << 10
)

// Job kinds.
const (
	workKindPrompt = "prompt"
	workKindPlan   = "plan"
)

// Job states derived from the job's files and process liveness.
const (
	workStateRunning   = "running"
	workStateCompleted = "completed" // exited 0
	workStateFailed    = "failed"    // exited non-zero
	workStateEnded     = "ended"     // process gone, exit not recorded
)

// Outcomes reported by sop_start_work.
const (
	workOutcomeStarted        = "started"
	workOutcomeAlreadyRunning = "already_running"
	workOutcomeBusy           = "busy"
	workOutcomeAlreadyRan     = "already_ran"
	workOutcomeInMemory       = "in_memory"
)

// workJob is the durable record of one MCP-started job.
type workJob struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Capability  string    `json:"capability,omitempty"`
	Source      string    `json:"source,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	Args        []string  `json:"args"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
}

// workExit records how a job's process ended, when the server observed it.
type workExit struct {
	ExitCode int       `json:"exit_code"`
	EndedAt  time.Time `json:"ended_at"`
}

// workJobView is a job with its derived state, as returned to the client.
type workJobView struct {
	workJob
	State    string     `json:"state"`
	ExitCode *int       `json:"exit_code,omitempty"`
	EndedAt  *time.Time `json:"ended_at,omitempty"`
	Log      string     `json:"log"`
	LogTail  string     `json:"log_tail,omitempty"`
	dir      string
}

// workLauncher starts args as a detached child of the sop binary in dir, with
// stdout and stderr appended to logPath. It returns the child's pid and a wait
// function reporting its exit code. Tests inject a fake.
type workLauncher func(dir string, args []string, logPath string) (pid int, wait func() int, err error)

// workProcessAlive reports whether pid is still running. Tests inject a fake.
type workProcessAlive func(pid int) bool

// workEnv carries the start-work seams.
type workEnv struct {
	launch workLauncher
	alive  workProcessAlive
	now    func() time.Time
}

// defaultWorkEnv is the production start-work environment.
func defaultWorkEnv() workEnv {
	return workEnv{launch: launchWorkProcess, alive: workPIDAlive, now: time.Now}
}

// mcpStartWorkTools returns the opt-in start-work tools.
func mcpStartWorkTools(dir string, env workEnv) []mcp.Tool {
	return []mcp.Tool{mcpStartWorkTool(dir, env), mcpWorkStatusTool(dir, env)}
}

// startWorkArgs is the sop_start_work argument shape.
type startWorkArgs struct {
	Prompt     string `json:"prompt"`
	PlanPath   string `json:"plan_path"`
	Capability string `json:"capability"`
	Rerun      bool   `json:"rerun"`
}

// mcpStartWorkTool builds sop_start_work.
func mcpStartWorkTool(dir string, env workEnv) mcp.Tool {
	return mcp.Tool{
		Name: "sop_start_work",
		Description: "Start SOP work in the background from prompt text (runs `sop prompt`) or a PLAN.md inside the project (runs `sop run PLAN`). " +
			"If the same work is already running, already ran, or is the plan SOP already holds, returns its status instead of starting it again; " +
			"pass rerun=true to start it again once it is not running. Only one started job runs per project at a time. Never approves a gate.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{
					"type":        "string",
					"description": "Prompt text to run with `sop prompt`. Give this or plan_path, not both.",
				},
				"plan_path": map[string]any{
					"type":        "string",
					"description": "Path to a PLAN.md inside the project (relative to the project root) to run with `sop run`.",
				},
				"capability": map[string]any{
					"type":        "string",
					"enum":        []string{"plan", "design_tests", "diagnose_failure", "review", "implement"},
					"description": "Prompt capability; defaults to plan (read-only). Use implement for governed repository changes. Prompt only.",
				},
				"rerun": map[string]any{
					"type":        "boolean",
					"description": "Start the work again even though it already ran or is the active plan. Never starts a second copy of running work.",
				},
			},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			var in startWorkArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			doc, err := startWork(dir, env, in)
			if err != nil {
				return "", err
			}
			return marshalWorkDoc(doc)
		},
	}
}

// mcpWorkStatusTool builds sop_work_status, the read-only view of started jobs.
func mcpWorkStatusTool(dir string, env workEnv) mcp.Tool {
	return mcp.Tool{
		Name:        "sop_work_status",
		Description: "Report MCP-started SOP work: one job by job_id, or the most recent jobs, with state, exit code, a log excerpt, and the project's task status.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"job_id": map[string]any{"type": "string"},
			},
		},
		ReadOnly: true,
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			var in struct {
				JobID string `json:"job_id"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			doc, err := workStatus(dir, env, strings.TrimSpace(in.JobID))
			if err != nil {
				return "", err
			}
			return marshalWorkDoc(doc)
		},
	}
}

// startWork resolves the requested work, reports existing status when SOP
// already holds it, and otherwise launches it.
func startWork(dir string, env workEnv, in startWorkArgs) (map[string]any, error) {
	prompt := strings.TrimSpace(in.Prompt)
	planPath := strings.TrimSpace(in.PlanPath)
	switch {
	case prompt == "" && planPath == "":
		return nil, errors.New("give prompt or plan_path")
	case prompt != "" && planPath != "":
		return nil, errors.New("give prompt or plan_path, not both")
	case planPath != "" && strings.TrimSpace(in.Capability) != "":
		return nil, errors.New("capability applies to prompt only")
	case len(prompt) > maxWorkPromptBytes:
		return nil, fmt.Errorf("prompt is larger than %d bytes", maxWorkPromptBytes)
	}

	found, err := exists(statePath(dir))
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"outcome": "not_initialized", "message": notInitializedMsg}, nil
	}

	// Resolve the work and its fingerprint before taking the lock, so a bad
	// argument fails without touching state.
	job := workJob{Version: 1}
	var planData []byte
	if prompt != "" {
		capName := strings.ToLower(strings.TrimSpace(in.Capability))
		if capName == "" {
			capName = defaultPromptCapability
		}
		capability, err := parsePromptCapability(capName)
		if err != nil {
			return nil, err
		}
		job.Kind = workKindPrompt
		job.Capability = promptCapabilityName(capability)
		job.Fingerprint = workFingerprint(workKindPrompt, job.Capability, prompt)
	} else {
		resolved, err := planflow.ResolvePlanPath(dir, planPath)
		if err != nil {
			return nil, err
		}
		// A client-supplied path is untrusted: confine it to the project, against
		// symlink resolution, exactly as `sop prompt --file` does.
		confined, err := resolvePromptFile(dir, resolved)
		if err != nil {
			return nil, fmt.Errorf("plan_path: %w", err)
		}
		planData, err = os.ReadFile(confined)
		if err != nil {
			return nil, fmt.Errorf("read plan: %w", err)
		}
		if strings.TrimSpace(string(planData)) == "" {
			return nil, fmt.Errorf("plan %s is empty", planPath)
		}
		job.Kind = workKindPlan
		job.Source = planflow.SourceRel(dir, resolved)
		job.Fingerprint = planflow.SourceFingerprint(planData)
	}

	root := workRoot(dir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	unlock, err := acquireWorkLock(root, env.now)
	if err != nil {
		return nil, err
	}
	defer unlock()

	jobs, err := listWorkJobs(dir, env)
	if err != nil {
		return nil, err
	}

	// Never start work while a started job is still running.
	for _, j := range jobs {
		if j.State != workStateRunning {
			continue
		}
		if j.Fingerprint == job.Fingerprint {
			return workDoc(dir, workOutcomeAlreadyRunning, "This work is already running.", &j), nil
		}
		return workDoc(dir, workOutcomeBusy,
			"Another SOP job started over MCP is still running in this project; wait for it to finish before starting new work.", &j), nil
	}

	if !in.Rerun {
		prev := latestWithFingerprint(jobs, job.Fingerprint)
		if job.Kind == workKindPlan {
			if src, sha, ok := planflow.ActivePlanSource(dir); ok && src == job.Source && sha == job.Fingerprint {
				doc := workDoc(dir, workOutcomeInMemory,
					"SOP already holds this plan as its active plan. Pass rerun=true to run it again with `sop run`, which resumes unfinished tasks.", prev)
				if tasks, err := workTaskLines(dir); err == nil {
					doc["all_tasks_satisfied"] = tasks.allSatisfied
				}
				return doc, nil
			}
		}
		if prev != nil {
			return workDoc(dir, workOutcomeAlreadyRan,
				"This work already ran. Pass rerun=true to start it again.", prev), nil
		}
	}

	job.ID = workJobID(root, env.now())
	jobDir := filepath.Join(root, job.ID)
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		return nil, err
	}
	switch job.Kind {
	case workKindPrompt:
		promptPath := filepath.Join(jobDir, workPromptFile)
		if err := os.WriteFile(promptPath, []byte(prompt+"\n"), 0o644); err != nil {
			return nil, err
		}
		job.Args = []string{"prompt", "--capability", job.Capability, "--file", promptPath}
	case workKindPlan:
		job.Args = []string{"run", job.Source}
	}

	logPath := filepath.Join(jobDir, workLogFile)
	pid, wait, err := env.launch(dir, job.Args, logPath)
	if err != nil {
		_ = os.RemoveAll(jobDir)
		return nil, fmt.Errorf("start %s: %w", strings.Join(job.Args[:1], " "), err)
	}
	job.PID = pid
	job.StartedAt = env.now().UTC()
	if err := writeJSONFile(filepath.Join(jobDir, workJobFile), job); err != nil {
		return nil, err
	}
	// Record the exit while this server lives. A detached child outlives the
	// server; its exit is then reported as "ended" with no code.
	go func() {
		code := wait()
		_ = writeJSONFile(filepath.Join(jobDir, workExitFile), workExit{ExitCode: code, EndedAt: env.now().UTC()})
	}()

	view := workJobView{workJob: job, State: workStateRunning, Log: relDir(dir, logPath), dir: jobDir}
	return workDoc(dir, workOutcomeStarted,
		"Started. Call sop_work_status with this job_id (or sop_status) to follow it.", &view), nil
}

// workStatus reports one job, or the recent jobs, with the task status.
func workStatus(dir string, env workEnv, jobID string) (map[string]any, error) {
	found, err := exists(statePath(dir))
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"outcome": "not_initialized", "message": notInitializedMsg}, nil
	}
	jobs, err := listWorkJobs(dir, env)
	if err != nil {
		return nil, err
	}
	if jobID != "" {
		for i := range jobs {
			if jobs[i].ID == jobID {
				return workDoc(dir, "status", "", &jobs[i]), nil
			}
		}
		return nil, fmt.Errorf("no started job %q", jobID)
	}
	doc := workDoc(dir, "status", "", nil)
	const recent = 5
	list := make([]map[string]any, 0, recent)
	for i := 0; i < len(jobs) && i < recent; i++ {
		list = append(list, jobSummary(jobs[i]))
	}
	doc["jobs"] = list
	if len(jobs) == 0 {
		doc["message"] = "no work has been started over MCP"
	}
	return doc, nil
}

// workDoc assembles a response: the outcome, the job (with a log excerpt), and
// the project's task status.
func workDoc(dir, outcome, message string, j *workJobView) map[string]any {
	doc := map[string]any{"outcome": outcome}
	if message != "" {
		doc["message"] = message
	}
	if j != nil {
		v := *j
		v.LogTail = tailFile(filepath.Join(j.dir, workLogFile), workLogTailLines)
		doc["job"] = v
	}
	if tasks, err := workTaskLines(dir); err == nil {
		doc["tasks"] = tasks.lines
	} else {
		doc["tasks_error"] = err.Error()
	}
	return doc
}

// jobSummary is the short form of a job used in a list.
func jobSummary(j workJobView) map[string]any {
	s := map[string]any{"id": j.ID, "kind": j.Kind, "state": j.State, "started_at": j.StartedAt}
	if j.Capability != "" {
		s["capability"] = j.Capability
	}
	if j.Source != "" {
		s["source"] = j.Source
	}
	if j.ExitCode != nil {
		s["exit_code"] = *j.ExitCode
	}
	return s
}

// workTasks is the persisted task status in the shape sop_status prints.
type workTasks struct {
	lines        []string
	allSatisfied bool
}

// workTaskLines reads the persisted tasks; it never creates state.
func workTaskLines(dir string) (workTasks, error) {
	st, err := store.Open(statePath(dir))
	if err != nil {
		return workTasks{}, err
	}
	defer st.Close()
	tasks, err := st.List()
	if err != nil {
		return workTasks{}, err
	}
	out := workTasks{lines: make([]string, 0, len(tasks)), allSatisfied: len(tasks) > 0 && domain.AllSatisfied(tasks)}
	for _, t := range tasks {
		out.lines = append(out.lines, fmt.Sprintf("%s %s %s", t.ID, t.Status, t.Title))
	}
	return out, nil
}

// listWorkJobs reads every recorded job, newest first, with its derived state.
func listWorkJobs(dir string, env workEnv) ([]workJobView, error) {
	root := workRoot(dir)
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var jobs []workJobView
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jobDir := filepath.Join(root, e.Name())
		var j workJob
		if !readJSONFile(filepath.Join(jobDir, workJobFile), &j) || j.ID == "" {
			continue // incomplete or foreign directory
		}
		v := workJobView{workJob: j, Log: relDir(dir, filepath.Join(jobDir, workLogFile)), dir: jobDir}
		var ex workExit
		switch {
		case readJSONFile(filepath.Join(jobDir, workExitFile), &ex):
			code, ended := ex.ExitCode, ex.EndedAt
			v.ExitCode, v.EndedAt = &code, &ended
			v.State = workStateCompleted
			if code != 0 {
				v.State = workStateFailed
			}
		case j.PID > 0 && env.alive(j.PID):
			v.State = workStateRunning
		default:
			v.State = workStateEnded
		}
		jobs = append(jobs, v)
	}
	sort.SliceStable(jobs, func(a, b int) bool {
		if !jobs[a].StartedAt.Equal(jobs[b].StartedAt) {
			return jobs[a].StartedAt.After(jobs[b].StartedAt)
		}
		return jobs[a].ID > jobs[b].ID
	})
	return jobs, nil
}

// latestWithFingerprint returns the newest job for the same work, or nil.
func latestWithFingerprint(jobs []workJobView, fp string) *workJobView {
	for i := range jobs {
		if jobs[i].Fingerprint == fp {
			return &jobs[i]
		}
	}
	return nil
}

// workFingerprint identifies a prompt by its kind, capability, and trimmed text.
func workFingerprint(kind, capability, text string) string {
	sum := sha256.New()
	for _, part := range []string{kind, capability, strings.TrimSpace(text)} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// workRoot is the directory holding MCP-started jobs.
func workRoot(dir string) string {
	return filepath.Join(dir, stateDirName, "runs", workDirName)
}

// workJobID returns a unique, time-ordered job id.
func workJobID(root string, now time.Time) string {
	base := "work-" + now.UTC().Format("20060102-150405")
	id := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(root, id)); errors.Is(err, fs.ErrNotExist) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// acquireWorkLock takes the cross-process start lock, breaking a stale one.
func acquireWorkLock(root string, now func() time.Time) (func(), error) {
	path := filepath.Join(root, workLockFile)
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		info, serr := os.Stat(path)
		if serr != nil || now().Sub(info.ModTime()) < workLockStale {
			return nil, errors.New("another start request is in progress; try again shortly")
		}
		_ = os.Remove(path) // stale: a previous server died mid-start
	}
	return nil, errors.New("could not take the start lock")
}

// launchWorkProcess starts the current sop binary detached from this server.
func launchWorkProcess(dir string, args []string, logPath string) (int, func() int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, nil, err
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Stdin stays nil (the null device): the MCP stdio stream must never reach the
	// child, and the child must not read an approval decision from it.
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, nil, err
	}
	wait := func() int {
		defer logFile.Close()
		err := cmd.Wait()
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			return 0
		case errors.As(err, &exitErr):
			return exitErr.ExitCode()
		default:
			return -1
		}
	}
	return cmd.Process.Pid, wait, nil
}

// tailFile returns up to n trailing lines of a file, bounded in size.
func tailFile(path string, n int) string {
	const maxBytes = 16 << 10
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && len(buf) > 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// writeJSONFile writes v atomically.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readJSONFile reads path into v, reporting whether it succeeded.
func readJSONFile(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// marshalWorkDoc renders a response document.
func marshalWorkDoc(doc map[string]any) (string, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

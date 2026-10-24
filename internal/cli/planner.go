package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/question"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// plannerWorker is the identity the CLI records when it claims a continuation.
const plannerWorker = "sop-planner-cli"

// plannerLease bounds a CLI-side claim; the durable record, not the lease, is what
// recovery reconciles.
const plannerLease = 30 * time.Second

// plannerSettings resolves the opt-in planner settings from configuration, applying
// the built-in round-limit default.
func plannerSettings(cfg config.Config) question.PlannerSettings {
	limit := cfg.Planner.RoundLimit
	if limit == 0 {
		limit = 3
	}
	return question.PlannerSettings{
		Enabled:        cfg.Planner.Enabled,
		Mode:           cfg.Planner.Mode,
		RoundLimit:     limit,
		PauseOnTimeout: cfg.Planner.PauseOnTimeout,
	}
}

// plannerTimeout converts planner.timeout_seconds to a Duration for a newly created
// question's deadline. Zero (the default) means no deadline; a negative value, or a value
// whose Duration would overflow, is rejected so a misconfiguration fails closed instead of
// silently wrapping. The deadline is evaluated only on command re-entry, never by a timer.
func plannerTimeout(cfg config.Config) (time.Duration, error) {
	secs := cfg.Planner.TimeoutSeconds
	if secs == 0 {
		return 0, nil
	}
	if secs < 0 {
		return 0, fmt.Errorf("planner: timeout_seconds must not be negative, got %d", secs)
	}
	if int64(secs) > int64(math.MaxInt64)/int64(time.Second) {
		return 0, fmt.Errorf("planner: timeout_seconds %d overflows a duration", secs)
	}
	return time.Duration(secs) * time.Second, nil
}

// currentRunEpoch returns a task run's creation timestamp, or "" when none exists.
func currentRunEpoch(dir, taskID string) string {
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", taskID, "state.json"))
	if err != nil {
		return ""
	}
	var st struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return ""
	}
	return st.CreatedAt
}

// plannerHumanGate reports whether the task is at a human decision boundary.
// plannerHumanGate reports whether a human decision currently owns the task. It
// reads the authoritative approval record together with the task lifecycle, and
// fails closed on unknown state:
//
//   - a PENDING request is a gate whatever the stage says;
//   - a BLOCKED task has not left its boundary (a decline records the decision but
//     does not requeue the task), so it stays gated;
//   - a parked WAITING_FOR_HUMAN stage survives the decision that resolved it, so it
//     remains a gate unless an explicit APPROVED, DECLINED, or SUPERSEDED decision is
//     recorded. A missing record is unknown state and is not treated as resolved.
//
// The requeued case that motivated this is a needs-human outcome that returns the
// task to PLANNED while the stage stays parked and the operator declines: the task is
// runnable again and its stale stage must not keep blocking planner questions.
func plannerHumanGate(dir string, task *domain.Task) bool {
	head, have := runpkg.At(runpkg.Dir(dir, task.ID)).Approval()
	if have && head.Status == domain.ApprovalPending {
		return true
	}
	if task.IsBlocked() {
		return true
	}
	stage, ok := runpkg.Load(dir, task.ID)
	if !ok || stage != runpkg.WaitingForHuman {
		return false
	}
	if !have {
		return true
	}
	switch head.Status {
	case domain.ApprovalDeclined, domain.ApprovalApproved, domain.ApprovalSuperseded:
		return false
	default:
		return true
	}
}

// questionsForTask lists every question whose task identity is taskID, across states.
func questionsForTask(svc *question.Service, taskID string) ([]*question.Question, error) {
	var out []*question.Question
	for _, st := range question.AllStates() {
		qs, err := svc.ListQuestions(st)
		if err != nil {
			return nil, err
		}
		for _, q := range qs {
			if strings.TrimSpace(q.Identity) == taskID {
				out = append(out, q)
			}
		}
	}
	return out, nil
}

// activeQuestion picks the task's actionable question: a PENDING one first, then
// ANSWERED, then APPLIED. An APPLIED question whose continuation is terminal (COMPLETED
// or SUPERSEDED) is not actionable and is skipped, so a later question whose work is still
// owed is the active one.
func activeQuestion(svc *question.Service, qs []*question.Question) *question.Question {
	for _, want := range []question.State{question.StatePending, question.StateAnswered, question.StateApplied} {
		for _, q := range qs {
			if q.State != want {
				continue
			}
			if want == question.StateApplied {
				if c := continuationFor(svc, q); c != nil && c.State.Terminal() {
					continue
				}
			}
			return q
		}
	}
	return nil
}

// continuationFor returns the continuation for a question, or nil when absent.
func continuationFor(svc *question.Service, q *question.Question) *question.Continuation {
	if q == nil {
		return nil
	}
	c, err := svc.Continuation(q.ID, q.RunEpoch)
	if err != nil {
		return nil
	}
	return c
}

// runPlanner dispatches `sop planner <ask|status|continue>`.
func runPlanner(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sop planner <ask|status|continue> ...")
		return exitUsage
	}
	switch args[0] {
	case "ask":
		return runPlannerAsk(args[1:], stdout, stderr, d)
	case "status":
		return runPlannerStatus(args[1:], stdout, stderr, d)
	case "continue":
		return runPlannerContinue(args[1:], stdout, stderr, d)
	default:
		fmt.Fprintln(stderr, "usage: sop planner <ask|status|continue> ...")
		return exitUsage
	}
}

// runPlannerAsk creates an executor-style question bound to live state and parks the
// run at WAITING_FOR_PLANNER. It never executes or approves anything.
func runPlannerAsk(args []string, stdout, stderr io.Writer, d deps) int {
	usage := "usage: sop planner ask <task-id> --question TEXT [--kind K] [--option O]... [--return-stage S]"
	if len(args) < 1 {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	taskID := args[0]
	body, kind, stage := "", "", runpkg.Implementing
	var options []string
	for i := 1; i < len(args); i++ {
		need := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			return args[i+1], true
		}
		switch args[i] {
		case "--question":
			v, ok := need()
			if !ok {
				fmt.Fprintln(stderr, usage)
				return exitUsage
			}
			body, i = v, i+1
		case "--kind":
			v, ok := need()
			if !ok {
				fmt.Fprintln(stderr, usage)
				return exitUsage
			}
			kind, i = v, i+1
		case "--option":
			v, ok := need()
			if !ok {
				fmt.Fprintln(stderr, usage)
				return exitUsage
			}
			options, i = append(options, v), i+1
		case "--return-stage":
			v, ok := need()
			if !ok {
				fmt.Fprintln(stderr, usage)
				return exitUsage
			}
			stage, i = runpkg.Stage(v), i+1
		default:
			fmt.Fprintln(stderr, usage)
			return exitUsage
		}
	}
	if strings.TrimSpace(body) == "" {
		fmt.Fprintln(stderr, usage)
		return exitUsage
	}
	if !supportedReturnStage(string(stage)) {
		fmt.Fprintf(stderr, "planner: unsupported return stage %q\n", stage)
		return exitError
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	if !cfg.Planner.Enabled {
		fmt.Fprintln(stderr, "planner: planner mode is disabled (set planner.enabled: true)")
		return exitError
	}
	timeout, terr := plannerTimeout(cfg)
	if terr != nil {
		fmt.Fprintf(stderr, "%v\n", terr)
		return exitError
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	defer st.Close()
	task, err := st.Get(taskID)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	if task.IsSatisfied() {
		fmt.Fprintf(stderr, "planner: task %s is %s; nothing to ask\n", taskID, task.Status)
		return exitError
	}
	if plannerHumanGate(dir, task) {
		fmt.Fprintf(stderr, "planner: task %s is at a human gate; the human decision owns it\n", taskID)
		return exitError
	}

	svc, fs, err := plannerService(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	_ = fs
	existing, err := questionsForTask(svc, taskID)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(kind) == "" {
		kind = fmt.Sprintf("planner-round-%d", len(existing)+1)
	}
	if len(existing)+1 > plannerSettings(cfg).RoundLimit {
		fmt.Fprintf(stderr, "planner: task %s reached the planner round limit (%d)\n", taskID, plannerSettings(cfg).RoundLimit)
		return exitError
	}

	q, err := svc.CreateQuestion(&question.Question{
		Identity:        taskID,
		Kind:            kind,
		Body:            strings.TrimSpace(body),
		Options:         options,
		SubmittedBy:     "executor",
		Project:         projectIdentity(dir),
		RunID:           taskID,
		RunEpoch:        currentRunEpoch(dir, taskID),
		Attempt:         task.Attempt,
		PlanRevision:    currentPlanRevision(dir),
		RepoFingerprint: repoFingerprint(dir),
		ReturnStage:     string(stage),
		Revision:        1,
		DeadlineAfter:   timeout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	if err := runpkg.SetStageOnExisting(dir, taskID, runpkg.WaitingForPlanner); err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "asked %s (PENDING) task %s; run paused at WAITING_FOR_PLANNER\n", q.ID, taskID)
	return exitOK
}

// runPlannerStatus reports each task's planner action without mutating anything.
func runPlannerStatus(args []string, stdout, stderr io.Writer, d deps) int {
	jsonOut := false
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		fmt.Fprintln(stderr, "usage: sop planner status [--json]")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	defer st.Close()
	svc, _, err := plannerService(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	settings := plannerSettings(cfg)
	type row struct {
		Task       string `json:"task"`
		Question   string `json:"question,omitempty"`
		State      string `json:"question_state,omitempty"`
		Action     string `json:"action"`
		Reason     string `json:"reason,omitempty"`
		ReturnStag string `json:"return_stage,omitempty"`
	}
	var rows []row
	for _, task := range tasks {
		v, verr := plannerView(dir, svc, task)
		if verr != nil {
			fmt.Fprintf(stderr, "planner: %v\n", verr)
			return exitError
		}
		action, reason := question.Decide(settings, v)
		r := row{Task: task.ID, Action: string(action), Reason: reason}
		if v.Question != nil {
			r.Question = v.Question.ID
			r.State = string(v.Question.State)
			r.ReturnStag = v.Question.ReturnStage
		}
		rows = append(rows, r)
	}
	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"version": 1, "tasks": rows}); err != nil {
			fmt.Fprintf(stderr, "planner: %v\n", err)
			return exitError
		}
		return exitOK
	}
	for _, r := range rows {
		fmt.Fprintf(stdout, "%s  %s  %s  %s\n", r.Task, actionOrNone(r.Action), r.State, oneLine(r.Reason))
	}
	return exitOK
}

func actionOrNone(a string) string {
	if a == "" {
		return "-"
	}
	return a
}

// runPlannerContinue reconciles the planner state. With --apply it applies an
// ANSWERED answer (creating the durable continuation) and REPORTS an APPLIED continuation
// as owed work. It never claims a continuation, never modifies the run checkpoint, and
// never marks a continuation COMPLETED: only the guarded executor path (`sop run`) owns
// execution and may record completion after the resumed stage work durably succeeds.
func runPlannerContinue(args []string, stdout, stderr io.Writer, d deps) int {
	apply := false
	for _, a := range args {
		if a == "--apply" {
			apply = true
			continue
		}
		fmt.Fprintln(stderr, "usage: sop planner continue [--apply]")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	defer st.Close()
	svc, _, err := plannerService(dir)
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	settings := plannerSettings(cfg)
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "planner: %v\n", err)
		return exitError
	}
	ctx := context.Background()
	for _, task := range tasks {
		v, verr := plannerView(dir, svc, task)
		if verr != nil {
			fmt.Fprintf(stderr, "planner: %v\n", verr)
			return exitError
		}
		action, reason := question.Decide(settings, v)
		if !apply {
			fmt.Fprintf(stdout, "%s  %s  %s\n", task.ID, actionOrNone(string(action)), oneLine(reason))
			continue
		}
		if (action == question.ActionApplyThenContinue || action == question.ActionContinue) && v.Question != nil {
			stage := strings.TrimSpace(v.Question.ReturnStage)
			if stage == "" {
				stage = string(runpkg.Implementing)
			}
			if !supportedReturnStage(stage) {
				fmt.Fprintf(stderr, "planner: unsupported return stage %q for %s\n", stage, task.ID)
				return exitError
			}
		}
		switch action {
		case question.ActionApplyThenContinue:
			if _, aerr := svc.ApplyAnswer(ctx, v.Question.ID); aerr != nil {
				fmt.Fprintf(stderr, "planner: apply %s: %v\n", v.Question.ID, aerr)
				return exitError
			}
			fmt.Fprintf(stdout, "%s  APPLIED %s (continuation pending execution)\n", task.ID, v.Question.ID)
		case question.ActionContinue:
			// Reconciliation must NOT consume owed work: it does not claim the continuation,
			// does not modify the run checkpoint, and does not mark the continuation
			// COMPLETED. The guarded executor path (`sop run`) performs the stage work and
			// records completion, so a stage-label-only reconciliation leaves it owed.
			if v.Question == nil {
				fmt.Fprintf(stdout, "%s  nothing to continue\n", task.ID)
				continue
			}
			cont := continuationFor(svc, v.Question)
			if cont == nil {
				fmt.Fprintf(stdout, "%s  no continuation\n", task.ID)
				continue
			}
			fmt.Fprintf(stdout, "%s  %s PENDING execution (run `sop run` to continue)\n", task.ID, cont.ID)
		case question.ActionWait:
			if err := runpkg.SetStageOnExisting(dir, task.ID, runpkg.WaitingForPlanner); err != nil {
				fmt.Fprintf(stderr, "planner: %v\n", err)
				return exitError
			}
			fmt.Fprintf(stdout, "%s  WAITING at WAITING_FOR_PLANNER (%s)\n", task.ID, oneLine(reason))
		default:
			fmt.Fprintf(stdout, "%s  %s  %s\n", task.ID, actionOrNone(string(action)), oneLine(reason))
		}
	}
	return exitOK
}

// plannerService opens the durable question store and service.
func plannerService(dir string) (*question.Service, *question.FileStore, error) {
	fs, err := openQuestionStore(dir)
	if err != nil {
		return nil, nil, err
	}
	return question.NewService(fs, nil), fs, nil
}

// plannerView builds the authoritative reconcile view for a task.
func plannerView(dir string, svc *question.Service, task *domain.Task) (question.ReconcileView, error) {
	qs, err := questionsForTask(svc, task.ID)
	if err != nil {
		return question.ReconcileView{}, err
	}
	q := activeQuestion(svc, qs)
	return plannerViewFor(svc, dir, task, qs, q), nil
}

// plannerViewFor builds a reconcile view from already-loaded questions.
func plannerViewFor(svc *question.Service, dir string, task *domain.Task, qs []*question.Question, q *question.Question) question.ReconcileView {
	repo := repoFingerprint(dir)
	// The reconcile policy compares identities as strings. When the bound identity names
	// this repository by another spelling (recorded before identities were resolved),
	// present it in the bound spelling so the policy does not read it as a change.
	if q != nil && repo != "" && sameRepository(q.RepoFingerprint, repo) {
		repo = q.RepoFingerprint
	}
	return question.ReconcileView{
		TaskID:                 task.ID,
		TaskTerminal:           task.IsSatisfied(),
		HumanGate:              plannerHumanGate(dir, task),
		Rounds:                 len(qs),
		Question:               q,
		Continuation:           continuationFor(svc, q),
		CurrentPlanRevision:    currentPlanRevision(dir),
		CurrentRunEpoch:        currentRunEpoch(dir, task.ID),
		CurrentRepoFingerprint: repo,
		Now:                    time.Now().UTC(),
	}
}

// withPlannerAnswer appends an applied planner answer to the implementation input so
// the resumed stage's executor sees the decision.
func withPlannerAnswer(base, answer string) string {
	return withPlannerDecisions(base, nil, answer)
}

// plannerDecision is one earlier applied planner decision in the current run: the
// question it answered and the applied answer.
type plannerDecision struct {
	Question string
	Answer   string
}

// withPlannerDecisions appends the run's earlier applied planner decisions (oldest
// first) and then the latest one, so a resumed stage sees every decision made in this
// run, not only the one that resumed it. The latest decision keeps its "Planner
// decision:" line last. With no latest answer the input is unchanged.
func withPlannerDecisions(base string, earlier []plannerDecision, answer string) string {
	a := strings.TrimSpace(answer)
	if a == "" {
		return base
	}
	var lines []string
	if strings.TrimSpace(base) != "" {
		lines = append(lines, base)
	}
	if len(earlier) > 0 {
		lines = append(lines, "Earlier planner decisions in this run (oldest first):")
		for _, e := range earlier {
			lines = append(lines, "- Q: "+collapseSpace(e.Question)+" A: "+collapseSpace(e.Answer))
		}
	}
	lines = append(lines, "Planner decision: "+a)
	return strings.Join(lines, "\n")
}

// collapseSpace joins a body's words with single spaces so a multi-line question or
// answer stays on one history line without losing any of its text.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// earlierPlannerDecisions returns the applied decisions for the task's questions that
// precede current in the SAME run epoch and plan revision, oldest first. A question from
// another epoch or plan revision is never carried forward, so a reset or replanned run
// starts without stale decisions.
func earlierPlannerDecisions(svc *question.Service, qs []*question.Question, current *question.Question) []plannerDecision {
	var prior []*question.Question
	for _, q := range qs {
		if q.ID == current.ID || q.State != question.StateApplied {
			continue
		}
		if q.Identity != current.Identity || q.RunEpoch != current.RunEpoch || q.PlanRevision != current.PlanRevision {
			continue
		}
		if !q.CreatedAt.Before(current.CreatedAt) {
			continue
		}
		prior = append(prior, q)
	}
	sort.SliceStable(prior, func(i, j int) bool {
		if !prior[i].CreatedAt.Equal(prior[j].CreatedAt) {
			return prior[i].CreatedAt.Before(prior[j].CreatedAt)
		}
		return prior[i].ID < prior[j].ID
	})
	out := make([]plannerDecision, 0, len(prior))
	for _, q := range prior {
		if a := appliedAnswer(svc, q.ID); a != "" {
			out = append(out, plannerDecision{Question: q.Body, Answer: a})
		}
	}
	return out
}

// appliedAnswer returns the latest recorded answer body for a question.
func appliedAnswer(svc *question.Service, questionID string) string {
	_, answers, err := svc.ReadQuestion(questionID)
	if err != nil || len(answers) == 0 {
		return ""
	}
	return strings.TrimSpace(answers[len(answers)-1].Body)
}

// plannerLifecycleGate is the executor hook: before the destructive run setup it
// decides whether the planner owns the task. It parks a PENDING question without
// resetting the run, and resumes an applied answer from the saved stage. It returns
// (handled, exitCode): when handled is false the normal lifecycle owns the task (for
// example a human gate, a terminal task, a stale binding, or no question at all).
func plannerLifecycleGate(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, saver taskSaver, task *domain.Task, sess *runSession, stdout, stderr io.Writer) (bool, int) {
	settings := plannerSettings(cfg)
	if !settings.Enabled {
		return false, 0
	}
	svc, _, err := plannerService(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
		return true, exitError
	}
	qs, err := questionsForTask(svc, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
		return true, exitError
	}
	// Crash-gap recovery: terminalize any predecessor continuation whose owed work
	// demonstrably yielded to a LATER successor question (same task, run epoch, plan
	// binding) but whose supersession was never recorded because the process stopped. Each
	// predecessor passes the SAME guards that gate its resume (human gate, cancellation,
	// terminal task, stale binding, disabled mode), so recovery never resolves work the
	// planner itself would refuse to act on, and it never replays the predecessor's stage
	// work.
	plannerResolvePredecessors(svc, dir, cfg, task, qs)
	q := activeQuestion(svc, qs)
	if q == nil {
		return false, 0
	}
	action, reason := question.Decide(settings, plannerViewFor(svc, dir, task, qs, q))
	switch action {
	case question.ActionWait:
		// Park without New: the run epoch and saved stage are preserved.
		if err := runpkg.SetStageOnExisting(dir, task.ID, runpkg.WaitingForPlanner); err != nil {
			fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
			return true, exitError
		}
		fmt.Fprintf(stdout, "%s PAUSED: WAITING_FOR_PLANNER (%s)\n", task.ID, oneLine(reason))
		return true, exitError
	case question.ActionApplyThenContinue:
		if _, aerr := svc.ApplyAnswer(ctx, q.ID); aerr != nil {
			fmt.Fprintf(stderr, "%s planner: apply: %v\n", task.ID, aerr)
			return true, exitError
		}
		return true, plannerResume(ctx, dir, cfg, a, d, saver, task, sess, svc, q.ID, stdout, stderr)
	case question.ActionContinue:
		return true, plannerResume(ctx, dir, cfg, a, d, saver, task, sess, svc, q.ID, stdout, stderr)
	case question.ActionRoundsExhausted:
		// The planner round limit is exhausted. Preserve an explicit paused/human boundary
		// with evidence rather than falling through to the normal lifecycle (which would
		// retry the task) or looping. No approval is implied by this park.
		parkPlannerHuman(ctx, dir, task, q.ID, "planner round limit reached; the planner loop is paused for a human decision", stdout, stderr)
		return true, exitError
	default:
		// Human gate, cancellation, terminal task, or stale binding: the normal lifecycle
		// owns the task.
		return false, 0
	}
}

// plannerHasSuccessor reports whether a distinct, durably stored successor question exists
// for the task: a later question (created after cur) bound to the same task, run epoch, and
// plan revision, still live (PENDING/ANSWERED/APPLIED). Supersession requires this —
// WAITING_FOR_PLANNER alone must not be sufficient.
func plannerHasSuccessor(dir string, task *domain.Task, qs []*question.Question, cur *question.Question) bool {
	if cur == nil {
		return false
	}
	epoch := strings.TrimSpace(currentRunEpoch(dir, task.ID))
	plan := strings.TrimSpace(currentPlanRevision(dir))
	for _, s := range qs {
		if s.ID == cur.ID {
			continue
		}
		switch s.State {
		case question.StatePending, question.StateAnswered, question.StateApplied:
		default:
			continue // not a live successor
		}
		if strings.TrimSpace(s.RunEpoch) != epoch || strings.TrimSpace(s.PlanRevision) != plan {
			continue // different run/plan binding
		}
		if !s.CreatedAt.After(cur.CreatedAt) {
			continue // not a later question
		}
		return true
	}
	return false
}

// plannerPredecessorResolvable reports whether q is a predecessor whose supersession was
// never recorded and whose owed work demonstrably yielded: it is APPLIED with a non-terminal
// continuation, has a distinct, live successor bound to the same task/run/plan, and passes the
// same Decide guards that gate its resume. It never mutates anything.
func plannerPredecessorResolvable(svc *question.Service, dir string, cfg config.Config, task *domain.Task, qs []*question.Question, q *question.Question) bool {
	if q == nil || q.State != question.StateApplied {
		return false
	}
	c := continuationFor(svc, q)
	if c == nil || c.State.Terminal() {
		return false
	}
	if !plannerHasSuccessor(dir, task, qs, q) {
		return false
	}
	action, _ := question.Decide(plannerSettings(cfg), plannerViewFor(svc, dir, task, qs, q))
	return !plannerSkipAction(action)
}

// plannerResolvePredecessors is the crash-gap recovery: for every APPLIED question whose
// continuation is non-terminal and which has a distinct successor (see plannerHasSuccessor),
// it records the terminal, non-completion outcome (SUPERSEDED) so no predecessor is left
// CLAIMED. It never replays work and never completes a continuation.
//
// Execution ownership is acquired FIRST, exactly as plannerResume does (the per-continuation
// execution lock, then the question store), so recovery can never supersede work that is
// still in flight: a successor question can be persisted while the predecessor's own worker
// still holds execution ownership — `sop planner ask` records one during a resumed stage, and
// the executor raise persists one inside it — so a worker that cannot take ownership leaves
// the continuation CLAIMED for its owner to finish. The successor and binding guards are
// revalidated AFTER ownership is held, and a predecessor is resolved only when the SAME
// guards that gate its resume also pass.
func plannerResolvePredecessors(svc *question.Service, dir string, cfg config.Config, task *domain.Task, qs []*question.Question) {
	for _, q := range qs {
		c := continuationFor(svc, q)
		if c == nil || !plannerPredecessorResolvable(svc, dir, cfg, task, qs, q) {
			continue
		}
		release, ok, err := question.TryAcquireLock(plannerEffectPath(dir, task.ID, c.ID) + ".lock")
		if err != nil || !ok {
			continue // in flight (or locking unsupported): leave it CLAIMED for its owner
		}
		// Revalidate after acquiring ownership: the successor may have vanished, the bindings may
		// have changed, or the predecessor may itself have completed while we waited.
		if fresh, ferr := questionsForTask(svc, task.ID); ferr == nil {
			var cur *question.Question
			for _, f := range fresh {
				if f.ID == q.ID {
					cur = f
					break
				}
			}
			if plannerPredecessorResolvable(svc, dir, cfg, task, fresh, cur) {
				if fc := continuationFor(svc, cur); fc != nil {
					_, _, _ = svc.ResolveYieldedContinuation(fc.ID)
				}
			}
		}
		release()
	}
}

// plannerSkipAction reports whether a Decide action is one of the guards that must stop the
// planner from acting on a question: disabled mode, cancellation, a human gate, a terminal
// task, or a stale binding.
func plannerSkipAction(a question.Action) bool {
	switch a {
	case question.ActionDisabled, question.ActionSkipCancelled, question.ActionSkipHumanGate,
		question.ActionSkipTerminal, question.ActionSkipStale:
		return true
	default:
		return false
	}
}

// plannerResume resumes a task's actual stage work from the saved stage, in the same
// run epoch, with the applied answer available to the executor. The continuation is
// recorded COMPLETED only after that work durably succeeds.
//
// Ordering: execution ownership is acquired FIRST. A worker that cannot obtain it leaves
// the continuation and the run checkpoint unchanged (it neither claims nor mutates). Only
// once ownership is held does it claim, revalidate the current guards, and — for a first
// attempt — modify the checkpoint and run the work.
func plannerResume(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, saver taskSaver, task *domain.Task, sess *runSession, svc *question.Service, questionID string, stdout, stderr io.Writer) int {
	q, _, err := svc.ReadQuestion(questionID)
	if err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
		return exitError
	}
	// Load the run's earlier decisions before any checkpoint or marker is written, so a
	// read failure stops the resume without leaving an uncertain continuation.
	qs, err := questionsForTask(svc, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
		return exitError
	}
	earlier := earlierPlannerDecisions(svc, qs, q)
	cont, cerr := svc.Continuation(q.ID, q.RunEpoch)
	if cerr != nil || cont == nil {
		fmt.Fprintf(stdout, "%s planner: no continuation owed\n", task.ID)
		return exitError
	}
	if cont.State == question.ContinuationCompleted {
		fmt.Fprintf(stdout, "%s planner: continuation already completed\n", task.ID)
		return exitError
	}
	stage := cont.ReturnStage
	if strings.TrimSpace(stage) == "" {
		stage = string(runpkg.Implementing)
	}
	if !supportedReturnStage(stage) {
		fmt.Fprintf(stderr, "%s planner: unsupported return stage %q; refusing to resume\n", task.ID, stage)
		return exitError
	}

	// Execution ownership FIRST: hold the continuation's execution lock for the WHOLE work so
	// a second worker cannot run it concurrently, even after the claim lease expires. It is a
	// separate file from the store lock, so question creation/submission are not blocked. A
	// worker that cannot acquire it stands down WITHOUT touching the checkpoint or the claim.
	release, acquired, lerr := question.TryAcquireLock(plannerEffectPath(dir, task.ID, cont.ID) + ".lock")
	if lerr != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, lerr)
		return exitError
	}
	if !acquired {
		fmt.Fprintf(stdout, "%s planner: execution held by another worker; standing down (checkpoint unchanged)\n", task.ID)
		return exitError
	}
	defer release()

	claimed, ok, cerr := svc.ClaimContinuation(cont.ID, plannerWorker, plannerLease)
	if cerr != nil {
		fmt.Fprintf(stderr, "%s planner: claim: %v\n", task.ID, cerr)
		return exitError
	}
	if !ok || claimed == nil {
		fmt.Fprintf(stdout, "%s planner: continuation held elsewhere\n", task.ID)
		return exitError
	}

	// Revalidate the guards against CURRENT state now that execution ownership is held: a
	// human gate, a cancelled/terminal task, or changed bindings (plan revision / run epoch /
	// repository identity) must abort BEFORE the checkpoint is touched.
	if !plannerContinuationAllowed(svc, dir, cfg, task) {
		fmt.Fprintf(stdout, "%s planner: continuation no longer permitted (human gate, terminal task, or stale binding); standing down\n", task.ID)
		return exitError
	}

	// The success marker's state decides the recovery path. A marker that is present but
	// unreadable or malformed is UNCERTAIN — never "absent" — so a corrupt success marker
	// cannot license a replay.
	switch plannerEffectState(dir, task.ID, claimed.ID) {
	case markerOK:
		// The effect already succeeded in a prior attempt (recorded before its completion was
		// persisted): do NOT replay it; record completion only.
		if _, _, cerr := svc.CompleteContinuation(claimed.ID, claimed.Generation, plannerWorker); cerr != nil {
			fmt.Fprintf(stderr, "%s planner: complete: %v\n", task.ID, cerr)
			return exitError
		}
		// The resumed work durably succeeded, so finalize the task now (exactly as the
		// normal lifecycle path does). Otherwise it stays runnable and a second scheduler
		// pass finishes it, double-counting the completion.
		if cerr := completeTask(saver, task); cerr != nil {
			fmt.Fprintf(stderr, "%s planner: complete task: %v\n", task.ID, cerr)
			return exitError
		}
		// The run's PASSED artifacts already exist; reopen a handle so the RBV-011 handoff
		// capsule can read them. Best-effort, mirroring the normal success path.
		if hrn, herr := runpkg.Resume(dir, task.ID); herr == nil {
			plannerSaveHandoff(cfg, saver, task, hrn, lifeResult{stage: runpkg.Passed})
		}
		fmt.Fprintf(stdout, "%s planner: effect already present; completion recorded without replay\n", task.ID)
		return exitOK
	case markerUncertain:
		parkPlannerHuman(ctx, dir, task, claimed.ID, "uncertain planner continuation: the success marker is unreadable or malformed", stdout, stderr)
		return exitError
	}
	// A prior attempt BEGAN execution (start marker present or unreadable) and may have
	// completed its effect, but no success marker was persisted. Safe replay cannot be
	// established, so recovery stops for human review rather than replaying the effect or
	// marking completion.
	if plannerEffectStartedState(dir, task.ID, claimed.ID) != markerAbsent {
		parkPlannerHuman(ctx, dir, task, claimed.ID, "uncertain planner continuation: a prior resumed attempt did not record success", stdout, stderr)
		return exitError
	}

	// Checkpoint write happens ONLY now, under execution ownership and after the guards
	// revalidate: the run is advanced to the saved return stage.
	if err := svc.FenceAndMutate(claimed.ID, claimed.Generation, plannerWorker, func() error {
		return runpkg.SetStageOnExisting(dir, task.ID, runpkg.Stage(stage))
	}); err != nil {
		fmt.Fprintf(stderr, "%s planner: checkpoint: %v\n", task.ID, err)
		return exitError
	}

	// First attempt: durably record that execution is beginning BEFORE running any stage work,
	// so a crash AFTER the effect but BEFORE the success marker is detected as uncertain. If
	// this durable write fails, stage work does NOT begin.
	if merr := writePlannerEffectStarted(dir, task.ID, claimed.ID); merr != nil {
		fmt.Fprintf(stderr, "%s planner: effect-start marker: %v (no stage work performed)\n", task.ID, merr)
		return exitError
	}

	// Resume the EXISTING run: New would reset state.json and clear attempts.
	rn, err := runpkg.Resume(dir, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
		return exitError
	}
	spec := specFromTask(task)
	spec.Deliverables = planTaskDeliverables(dir, task)
	spec.DeliverableGroups = deliverableGroups(spec)
	spec.AcceptanceChecks = planTaskAcceptanceChecks(dir, task)
	spec.AcceptanceMapping = planTaskAcceptanceMapping(dir, task)
	_ = rn.Write("task.md", spec.Render())
	d.plannerAnswer = appliedAnswer(svc, q.ID)
	d.plannerEarlier = earlier
	d.resumeFrom = stage
	ctx = taskActivityContext(ctx, rn.Dir(), stdout, task.ID, task.Title)
	res, lerr := executeLifecycle(ctx, dir, cfg, a, d, spec, rn, sess, failure.ApprovalNone, earlyGateResult{}, stdout)

	// COMPLETION BOUNDARY (explicit): the continuation is completed only when the resumed
	// stage's work durably succeeded — the lifecycle returned no error AND the run reached
	// PASSED. A failure or a park leaves it CLAIMED (recoverable) and the checkpoint
	// resumable; a stage-label change alone never reaches here.
	if lerr == nil && res.stage == runpkg.Passed {
		// Make the run checkpoint (the PASSED stage and its recorded results) durable BEFORE
		// recording success, so the success marker never precedes the persisted checkpoint.
		if perr := rn.PersistDurable(); perr != nil {
			fmt.Fprintf(stderr, "%s planner: durable checkpoint: %v (success not recorded)\n", task.ID, perr)
			return exitError
		}
		// Record the durable effect marker BEFORE the completion CAS, so a crash in this
		// gap is recovered without replaying the (possibly non-idempotent) effect.
		if merr := writePlannerEffect(dir, task.ID, claimed.ID, claimed.Generation); merr != nil {
			fmt.Fprintf(stderr, "%s planner: effect marker: %v\n", task.ID, merr)
			return exitError
		}
		if _, _, cerr := svc.CompleteContinuation(claimed.ID, claimed.Generation, plannerWorker); cerr != nil {
			fmt.Fprintf(stderr, "%s planner: complete: %v\n", task.ID, cerr)
			return exitError
		}
		// Finalize the task now that the resumed work durably succeeded, exactly as the
		// normal lifecycle path does. Without this the task stays runnable, so the scheduler
		// finishes it in a second pass and reports two completions for one task.
		if cerr := completeTask(saver, task); cerr != nil {
			fmt.Fprintf(stderr, "%s planner: complete task: %v\n", task.ID, cerr)
			return exitError
		}
		// Persist the RBV-011 handoff capsule exactly as the normal success path does
		// (best-effort, metadata only, under context.handoff).
		plannerSaveHandoff(cfg, saver, task, rn, res)
		fmt.Fprintf(stdout, "%s planner: resumed from %s and completed (generation %d)\n", task.ID, stage, claimed.Generation)
		return exitOK
	}
	// The resumed work yielded to a NEW planner question: the run parked at WAITING_FOR_PLANNER
	// because the executor asked again. Supersession requires that a DISTINCT successor
	// question was durably persisted for the same task, run epoch, and plan binding —
	// WAITING_FOR_PLANNER alone is NOT sufficient. The continuation is then recorded
	// SUPERSEDED (terminal, explicitly NOT completion), so it is neither CLAIMED nor claimable
	// and is never replayed. A failure or a human gate is NOT superseded (below), so owed work
	// is never falsely completed by parking.
	if lerr == nil && res.stage == runpkg.WaitingForPlanner {
		if qs, qerr := questionsForTask(svc, task.ID); qerr == nil && plannerHasSuccessor(dir, task, qs, q) {
			if _, tr, cerr := svc.SupersedeContinuation(claimed.ID, claimed.Generation, plannerWorker); cerr != nil {
				fmt.Fprintf(stderr, "%s planner: supersede: %v\n", task.ID, cerr)
				return exitError
			} else if tr {
				fmt.Fprintf(stdout, "%s planner: resumed from %s; yielded to a new planner question, continuation superseded (generation %d)\n", task.ID, stage, claimed.Generation)
				return exitError
			}
		}
	}
	if lerr != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, lerr)
	}
	fmt.Fprintf(stdout, "%s planner: resumed from %s but NOT completed (stage %s); continuation retained\n", task.ID, stage, res.stage)
	return exitError
}

// plannerSaveHandoff persists the RBV-011 handoff capsule for a task a planner continuation
// completed, under the existing context.handoff flag. It mirrors the normal success path:
// best-effort, metadata only, and it never changes the run outcome.
func plannerSaveHandoff(cfg config.Config, saver taskSaver, task *domain.Task, rn *runpkg.Run, res lifeResult) {
	if cfg.ContextEfficiency.Handoff {
		saveTaskHandoff(saver, task, rn, res)
	}
}

// plannerContinuationAllowed re-reads the authoritative state after execution ownership is
// held and reports whether continuing is still permitted: the task is not terminal, not at
// a human gate, not cancelled, and the question's bindings still match the current plan
// revision, run epoch, and repository identity.
func plannerContinuationAllowed(svc *question.Service, dir string, cfg config.Config, task *domain.Task) bool {
	view, err := plannerView(dir, svc, task)
	if err != nil {
		return false
	}
	action, _ := question.Decide(plannerSettings(cfg), view)
	return action == question.ActionApplyThenContinue || action == question.ActionContinue
}

// parkPlannerHuman stops a planner decision for explicit human review: it parks the run at
// the existing WAITING_FOR_HUMAN boundary and records an explicit approval request. The
// continuation (if any) is neither replayed nor completed; the human owns the decision.
// subject names the record (a continuation or question id) in the diagnostic.
func parkPlannerHuman(ctx context.Context, dir string, task *domain.Task, subject, reason string, stdout, stderr io.Writer) {
	if err := runpkg.SetStageOnExisting(dir, task.ID, runpkg.WaitingForHuman); err != nil {
		fmt.Fprintf(stderr, "%s planner: %v\n", task.ID, err)
	}
	recordHumanApprovalRequest(ctx, runpkg.At(runpkg.Dir(dir, task.ID)), task, runpkg.WaitingForHuman, failure.NeedsHuman, reason, reason)
	fmt.Fprintf(stdout, "%s planner: stopped for human review (%s): %s\n", task.ID, subject, oneLine(reason))
}

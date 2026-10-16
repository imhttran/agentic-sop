package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/question"
	"github.com/imhttran/agentic-sop/internal/store"
)

// questionStoreName is the project-scoped durable store for questions, answers,
// and the audit outcomes.
const questionStoreName = "questions.json"

// planMetaName mirrors the plan provenance file the planflow package writes; the
// binding check reads it to compare a question's plan revision with the live one.
const planMetaName = "plan.meta.json"

// questionStorePath is the durable question store path for a project.
func questionStorePath(dir string) string {
	return filepath.Join(dir, config.DirName, questionStoreName)
}

// openQuestionStore opens the project's durable question store.
func openQuestionStore(dir string) (*question.FileStore, error) {
	return question.OpenFileStore(questionStorePath(dir))
}

// currentPlanRevision returns the live plan revision (plan id + source fingerprint)
// from the plan provenance file, or "" when there is no recorded plan.
func currentPlanRevision(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, planMetaName))
	if err != nil {
		return ""
	}
	var m struct {
		PlanID       string `json:"plan_id"`
		SourceSHA256 string `json:"source_sha256"`
	}
	if err := json.Unmarshal(data, &m); err != nil || m.PlanID == "" {
		return ""
	}
	return m.PlanID + "@" + m.SourceSHA256
}

// projectIdentity returns the SOP project identity: the configured project name,
// or the project directory's base name when none is configured.
func projectIdentity(dir string) string {
	if cfg, err := loadConfigOrDefault(dir); err == nil {
		if name := strings.TrimSpace(cfg.Project.Name); name != "" {
			return name
		}
	}
	return filepath.Base(dir)
}

// repoFingerprint returns a STABLE repository identity: the nearest ancestor
// directory containing a .git entry (a normal repository or a worktree pointer).
// It is deliberately not a working-tree content hash, so ordinary implementation
// edits and commits never invalidate a bound question. It returns "" when no
// repository is found, in which case the repository check is skipped.
func repoFingerprint(dir string) string {
	d, err := filepath.Abs(dir)
	if err != nil {
		d = dir
	}
	for {
		if found, _ := exists(filepath.Join(d, ".git")); found {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// bindingRunStale reports whether the question's bound run is no longer eligible:
// the run identity must agree with the task, and when the question pinned a run
// epoch (the run's creation timestamp) the run must not have been superseded by a
// newer run for the same task. It returns a reason and whether the binding is stale.
func bindingRunStale(dir string, q *question.Question) (string, bool) {
	runID := strings.TrimSpace(q.RunID)
	if runID == "" {
		return "", false
	}
	if runID != strings.TrimSpace(q.Identity) {
		return fmt.Sprintf("bound run %q does not match task %q", runID, q.Identity), true
	}
	epoch := strings.TrimSpace(q.RunEpoch)
	if epoch == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", runID, "state.json"))
	if err != nil {
		return fmt.Sprintf("bound run %q is not present", runID), true
	}
	var st struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &st); err != nil || st.CreatedAt == "" {
		return fmt.Sprintf("bound run %q has no readable identity", runID), true
	}
	if st.CreatedAt != epoch {
		return fmt.Sprintf("bound run %q was superseded (run epoch changed)", runID), true
	}
	return "", false
}

// questionBindingCheck validates a question's binding against authoritative current
// state: the project identity, the bound task's presence in SOP's live task store,
// the bound run's continued eligibility, the repository identity, and the plan
// revision must all still match. It returns a non-empty reason when the binding is
// stale. Caller-supplied identifiers are never trusted on their own.
func questionBindingCheck(dir string) question.BindingCheck {
	return func(_ context.Context, q *question.Question) (string, error) {
		taskID := strings.TrimSpace(q.Identity)
		if taskID == "" {
			return "question has no task binding", nil
		}
		if p := strings.TrimSpace(q.Project); p != "" {
			if cur := projectIdentity(dir); cur != "" && cur != p {
				return fmt.Sprintf("wrong project: bound %q, current %q", p, cur), nil
			}
		}
		if found, err := exists(statePath(dir)); err != nil {
			return "", err
		} else if !found {
			return "project state is not initialized; cannot validate the bound task", nil
		}
		st, err := store.Open(statePath(dir))
		if err != nil {
			return "", err
		}
		defer st.Close()
		if _, err := st.Get(taskID); err != nil {
			return fmt.Sprintf("bound task %q is not in current SOP state", taskID), nil
		}
		if reason, stale := bindingRunStale(dir, q); stale {
			return reason, nil
		}
		if fp := strings.TrimSpace(q.RepoFingerprint); fp != "" {
			if cur := repoFingerprint(dir); cur != "" && cur != fp {
				return fmt.Sprintf("repository changed: bound %q, current %q", fp, cur), nil
			}
		}
		if rev := strings.TrimSpace(q.PlanRevision); rev != "" {
			if cur := currentPlanRevision(dir); cur != "" && cur != rev {
				return fmt.Sprintf("plan revision changed: bound %q, current %q", rev, cur), nil
			}
		}
		return "", nil
	}
}

// submitPlannerAnswer records one planner answer through the shared service, with the
// authoritative binding check wired in. It never applies the answer, approves a task,
// or dispatches continuation.
func submitPlannerAnswer(ctx context.Context, dir string, in question.SubmitInput) (question.SubmitResult, error) {
	st, err := openQuestionStore(dir)
	if err != nil {
		return question.SubmitResult{}, err
	}
	svc := question.NewService(st, nil)
	return svc.SubmitAnswer(ctx, in, questionBindingCheck(dir))
}

// readQuestion loads a question and its answers from the durable store.
func readQuestion(dir, id string) (*question.Question, []*question.Answer, []*question.Outcome, error) {
	st, err := openQuestionStore(dir)
	if err != nil {
		return nil, nil, nil, err
	}
	svc := question.NewService(st, nil)
	q, answers, err := svc.ReadQuestion(id)
	if err != nil {
		return nil, nil, nil, err
	}
	outcomes, err := st.Outcomes(id)
	if err != nil {
		return nil, nil, nil, err
	}
	return q, answers, outcomes, nil
}

// listQuestions lists questions in a state from the durable store.
func listQuestions(dir string, state question.State) ([]*question.Question, error) {
	st, err := openQuestionStore(dir)
	if err != nil {
		return nil, err
	}
	return question.NewService(st, nil).ListQuestions(state)
}

// runQuestion is `sop question <id>` or `sop question list [--state S] [--json]`.
func runQuestion(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) >= 1 && args[0] == "list" {
		return runQuestionList(args[1:], stdout, stderr, d)
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop question <question-id>")
		fmt.Fprintln(stderr, "       sop question list [--state PENDING|ANSWERED|APPLIED|REJECTED|SUPERSEDED] [--json]")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	id := args[0]
	q, answers, outcomes, err := readQuestion(dir, id)
	if err != nil {
		fmt.Fprintf(stderr, "question: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "question %s  %s\n", q.ID, q.State)
	fmt.Fprintf(stdout, "task: %s  run: %s  plan: %s\n", oneLine(q.Identity), oneLine(q.RunID), oneLine(q.PlanRevision))
	if body := strings.TrimSpace(q.Body); body != "" {
		fmt.Fprintln(stdout, body)
	}
	fmt.Fprintf(stdout, "answers: %d\n", len(answers))
	for _, a := range answers {
		fmt.Fprintf(stdout, "  %s by %s: %s\n", a.ID, oneLine(a.AnsweredBy), oneLine(a.Body))
	}
	fmt.Fprintf(stdout, "outcomes: %d\n", len(outcomes))
	for _, o := range outcomes {
		fmt.Fprintf(stdout, "  %s %s\n", o.Kind, oneLine(o.Reason))
	}
	return exitOK
}

// runQuestionList lists questions for a state (default PENDING, so a planner can
// discover open questions).
func runQuestionList(args []string, stdout, stderr io.Writer, d deps) int {
	state := question.StatePending
	jsonOut := false
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--json":
			jsonOut = true
		case args[i] == "--state" && i+1 < len(args):
			i++
			st, err := question.ParseState(args[i])
			if err != nil {
				fmt.Fprintf(stderr, "question: %v\n", err)
				return exitUsage
			}
			state = st
		default:
			fmt.Fprintln(stderr, "usage: sop question list [--state STATE] [--json]")
			return exitUsage
		}
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	questions, err := listQuestions(dir, state)
	if err != nil {
		fmt.Fprintf(stderr, "question: %v\n", err)
		return exitError
	}
	if jsonOut {
		doc := questionListDoc{Version: 1, State: string(state), Questions: make([]questionView, 0, len(questions))}
		for _, q := range questions {
			doc.Questions = append(doc.Questions, toQuestionView(q))
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			fmt.Fprintf(stderr, "question: %v\n", err)
			return exitError
		}
		return exitOK
	}
	if len(questions) == 0 {
		fmt.Fprintf(stdout, "no %s questions\n", state)
		return exitOK
	}
	for _, q := range questions {
		fmt.Fprintf(stdout, "%s  %s  %s\n", q.ID, q.State, oneLine(q.Body))
	}
	return exitOK
}

// runAnswer is `sop answer <question-id> --body TEXT [--by WHO] [--idempotency-key KEY]
// [--expected-revision N] [--json]`. It records an answer (ANSWERED) only: it never
// applies the answer, approves a task, or resumes execution.
func runAnswer(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sop answer <question-id> --body TEXT [--by NAME] [--idempotency-key KEY] [--expected-revision N] [--json]")
		return exitUsage
	}
	id := args[0]
	in := question.SubmitInput{QuestionID: id}
	jsonOut := false
	fail := func() int {
		fmt.Fprintln(stderr, "usage: sop answer <question-id> --body TEXT [--by NAME] [--idempotency-key KEY] [--expected-revision N] [--json]")
		return exitUsage
	}
	need := func(i int, flag string) (string, bool) {
		if i+1 >= len(args) {
			return "", false
		}
		return args[i+1], true
	}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--body":
			v, ok := need(i, "--body")
			if !ok {
				return fail()
			}
			in.Body = v
			i++
		case "--by":
			v, ok := need(i, "--by")
			if !ok {
				return fail()
			}
			in.AnsweredBy = v
			i++
		case "--idempotency-key":
			v, ok := need(i, "--idempotency-key")
			if !ok {
				return fail()
			}
			in.IdempotencyKey = v
			i++
		case "--expected-revision":
			v, ok := need(i, "--expected-revision")
			if !ok {
				return fail()
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				fmt.Fprintf(stderr, "answer: invalid --expected-revision %q\n", v)
				return exitUsage
			}
			in.ExpectedRevision = n
			i++
		case "--json":
			jsonOut = true
		default:
			return fail()
		}
	}
	if strings.TrimSpace(in.Body) == "" {
		return fail()
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	res, err := submitPlannerAnswer(context.Background(), dir, in)
	if err != nil {
		fmt.Fprintf(stderr, "answer: %v\n", err)
		return exitError
	}
	if jsonOut {
		doc := submitDoc{Version: 1, Outcome: string(res.Outcome), Reason: res.Reason}
		if res.Question != nil {
			v := toQuestionView(res.Question)
			doc.Question = &v
		}
		if res.Answer != nil {
			doc.AnswerID = res.Answer.ID
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			fmt.Fprintf(stderr, "answer: %v\n", err)
			return exitError
		}
		return exitOK
	}
	switch res.Outcome {
	case question.OutcomeAccepted:
		fmt.Fprintf(stdout, "accepted %s (ANSWERED)\n", id)
	case question.OutcomeDuplicate:
		fmt.Fprintf(stdout, "duplicate %s: %s\n", id, oneLine(res.Reason))
	default:
		fmt.Fprintf(stdout, "%s %s: %s\n", strings.ToLower(string(res.Outcome)), id, oneLine(res.Reason))
	}
	return exitOK
}

// questionView is the wire projection of a question.
type questionView struct {
	ID           string   `json:"id"`
	TaskID       string   `json:"task_id"`
	Kind         string   `json:"kind"`
	RunID        string   `json:"run_id,omitempty"`
	PlanRevision string   `json:"plan_revision,omitempty"`
	Revision     int      `json:"revision"`
	State        string   `json:"state"`
	Body         string   `json:"body"`
	Options      []string `json:"options,omitempty"`
}

func toQuestionView(q *question.Question) questionView {
	return questionView{
		ID:           q.ID,
		TaskID:       q.Identity,
		Kind:         q.Kind,
		RunID:        q.RunID,
		PlanRevision: q.PlanRevision,
		Revision:     q.Revision,
		State:        string(q.State),
		Body:         q.Body,
		Options:      q.Options,
	}
}

type questionListDoc struct {
	Version   int            `json:"version"`
	State     string         `json:"state"`
	Questions []questionView `json:"questions"`
}

type submitDoc struct {
	Version  int           `json:"version"`
	Outcome  string        `json:"outcome"`
	Reason   string        `json:"reason,omitempty"`
	Question *questionView `json:"question,omitempty"`
	AnswerID string        `json:"answer_id,omitempty"`
}

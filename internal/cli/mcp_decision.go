package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/mcp"
	"github.com/imhttran/agentic-sop/internal/question"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskresult"
)

// decisionActions is the closed planner-decision vocabulary. It is advice, never
// authorization: recording one of these advances a question to ANSWERED and
// nothing else. The set is closed so an unrecognized action is rejected instead of
// being stored as an instruction SOP cannot interpret.
var decisionActions = []string{"continue", "correct", "revise", "escalate"}

// knownDecisionAction reports whether action is in the closed vocabulary.
func knownDecisionAction(action string) bool {
	for _, a := range decisionActions {
		if a == action {
			return true
		}
	}
	return false
}

// mcpTaskResultTool reads one task's persisted result evidence. It is read-only:
// it reports what SOP already recorded and never mutates a task, a run, or an
// approval.
func mcpTaskResultTool(dir string) mcp.Tool {
	return mcp.Tool{
		Name: "sop_task_result",
		Description: "Read one task's result evidence: status, attempts, plan revision, latest run stage and gate, " +
			"evidence paths, and its active planner question.",
		InputSchema: singleStringSchema("task_id", "Task id"),
		ReadOnly:    true,
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			id, err := requiredStringArg(raw, "task_id")
			if err != nil {
				return "", err
			}
			return taskResultText(dir, id)
		},
	}
}

// mcpDecisionListTool lists a task's questions with their recorded answers,
// submission outcomes, and continuation state. It is read-only, so an executor or
// planner can retrieve a decision without consuming it.
func mcpDecisionListTool(dir string) mcp.Tool {
	return mcp.Tool{
		Name: "sop_decision_list",
		Description: "List a task's planner questions with recorded answers, submission outcomes, and " +
			"continuation state. Read-only: it never applies, approves, or resumes anything.",
		InputSchema: singleStringSchema("task_id", "Task id"),
		ReadOnly:    true,
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			id, err := requiredStringArg(raw, "task_id")
			if err != nil {
				return "", err
			}
			return decisionListText(dir, id)
		},
	}
}

// mcpDecisionSubmitTool records one planner decision against a question. It
// delegates to the same application service as `sop answer`, so the binding check,
// the expected-revision check, idempotency, and the audit ledger are not
// reimplemented here. A recorded decision is ANSWERED only: it never approves a
// task, never declines one, and never resumes execution.
func mcpDecisionSubmitTool(dir string) mcp.Tool {
	return mcp.Tool{
		Name: "sop_decision_submit",
		Description: "Record a planner decision (continue, correct, revise, escalate) against a question. " +
			"Records ANSWERED only; never applies the decision, approves, or resumes execution.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question_id":            map[string]any{"type": "string"},
				"task_id":                map[string]any{"type": "string"},
				"action":                 map[string]any{"type": "string", "enum": decisionActions},
				"body":                   map[string]any{"type": "string"},
				"rationale":              map[string]any{"type": "string"},
				"answered_by":            map[string]any{"type": "string"},
				"idempotency_key":        map[string]any{"type": "string"},
				"expected_revision":      map[string]any{"type": "integer"},
				"expected_plan_revision": map[string]any{"type": "string"},
			},
			"required": []string{"question_id", "action", "body"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var in struct {
				QuestionID           string `json:"question_id"`
				TaskID               string `json:"task_id"`
				Action               string `json:"action"`
				Body                 string `json:"body"`
				Rationale            string `json:"rationale"`
				AnsweredBy           string `json:"answered_by"`
				IdempotencyKey       string `json:"idempotency_key"`
				ExpectedRevision     int    `json:"expected_revision"`
				ExpectedPlanRevision string `json:"expected_plan_revision"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			return decisionSubmitText(ctx, dir, in.QuestionID, in.TaskID, in.Action, question.SubmitInput{
				QuestionID:       in.QuestionID,
				Body:             in.Body,
				AnsweredBy:       in.AnsweredBy,
				IdempotencyKey:   in.IdempotencyKey,
				ExpectedRevision: in.ExpectedRevision,
				ChosenAction:     in.Action,
				Rationale:        in.Rationale,
			}, in.ExpectedPlanRevision)
		},
	}
}

// taskResultText renders one task's persisted result evidence. Evidence paths are
// reported relative to the project and only when the file exists, so the caller
// never receives a path SOP has not written.
func taskResultText(dir, taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	path := statePath(dir)
	found, err := exists(path)
	if err != nil {
		return "", err
	}
	if !found {
		return notInitializedMsg, nil
	}
	st, err := store.Open(path)
	if err != nil {
		return "", err
	}
	task, err := st.Get(taskID)
	st.Close()
	if err != nil {
		return "", fmt.Errorf("task %s: %w", taskID, err)
	}

	lines := []string{
		fmt.Sprintf("task: %s", task.ID),
		fmt.Sprintf("title: %s", task.Title),
		fmt.Sprintf("status: %s", string(task.Status)),
		fmt.Sprintf("attempt: %d/%d", task.Attempt, task.MaxAttempts),
	}
	if rev := currentPlanRevision(dir); rev != "" {
		lines = append(lines, "plan_revision: "+rev)
	}
	if epoch := currentRunEpoch(dir, task.ID); epoch != "" {
		lines = append(lines, "run_epoch: "+epoch)
	}
	runDir := filepath.Join(dir, config.DirName, "runs", task.ID)
	if stage := runStageState(runDir); stage != "" {
		lines = append(lines, "run_stage: "+stage)
	}
	if gate := runGateDecision(runDir); gate != "" {
		lines = append(lines, "gate: "+gate)
	}
	for _, name := range []string{"report.md", "validation.json", "diff.patch"} {
		p := filepath.Join(runDir, name)
		if found, _ := exists(p); found {
			if rel, rerr := filepath.Rel(dir, p); rerr == nil {
				lines = append(lines, "evidence: "+rel)
			}
		}
	}
	if q := activeQuestionForTask(dir, task.ID); q != nil {
		lines = append(lines, fmt.Sprintf("question: %s %s revision=%d", q.ID, q.State, q.Revision))
	}
	published, perr := taskresult.List(dir, task.ID)
	if perr != nil {
		return "", perr
	}
	for _, r := range published {
		lines = append(lines, fmt.Sprintf("result: %s attempt=%d by=%s: %s", r.Kind, r.Attempt, r.PublishedBy, oneLine(r.Summary)))
		for _, e := range r.Evidence {
			lines = append(lines, "result_evidence: "+e)
		}
	}
	return mcp.JoinLines(lines), nil
}

// runStageState reads a run's persisted stage.
func runStageState(runDir string) string {
	data, err := os.ReadFile(filepath.Join(runDir, "state.json"))
	if err != nil {
		return ""
	}
	var st struct {
		Stage string `json:"stage"`
	}
	if json.Unmarshal(data, &st) != nil {
		return ""
	}
	return st.Stage
}

// runGateDecision reads a run's persisted gate decision.
func runGateDecision(runDir string) string {
	data, err := os.ReadFile(filepath.Join(runDir, "gate.json"))
	if err != nil {
		return ""
	}
	var gate struct {
		Decision string `json:"Decision"`
	}
	if json.Unmarshal(data, &gate) != nil {
		return ""
	}
	return gate.Decision
}

// activeQuestionForTask returns the task's actionable question (PENDING first,
// then ANSWERED, then APPLIED), or nil when the task has none.
func activeQuestionForTask(dir, taskID string) *question.Question {
	svc, _, err := plannerService(dir)
	if err != nil {
		return nil
	}
	qs, err := questionsForTask(svc, taskID)
	if err != nil {
		return nil
	}
	return activeQuestion(svc, qs)
}

// decisionListText renders a task's questions with their answers, outcomes, and
// continuation state, oldest question first.
func decisionListText(dir, taskID string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return "", fmt.Errorf("task_id is required")
	}
	svc, fs, err := plannerService(dir)
	if err != nil {
		return "", err
	}
	qs, err := questionsForTask(svc, taskID)
	if err != nil {
		return "", err
	}
	lines := []string{fmt.Sprintf("task: %s", taskID)}
	if len(qs) == 0 {
		return mcp.JoinLines(append(lines, "decisions: 0")), nil
	}
	// The closed action vocabulary is reported once so a planner knows exactly what it may
	// submit without reading the tool schema or the source.
	lines = append(lines, "actions: "+strings.Join(decisionActions, ", "))
	check := questionBindingCheck(dir)
	for _, q := range qs {
		lines = append(lines, fmt.Sprintf("question: %s state=%s revision=%d attempt=%d plan_revision=%s",
			q.ID, q.State, q.Revision, q.Attempt, q.PlanRevision))
		// The decision context itself: what is being asked, which options were offered,
		// when it expires, and what stage a continuation would return to. Without these a
		// planner would have to copy from an operator log or guess.
		lines = append(lines, "question_body: "+oneLine(q.Body))
		if len(q.Options) > 0 {
			lines = append(lines, "question_options: "+strings.Join(q.Options, " | "))
		}
		if !q.Deadline.IsZero() {
			lines = append(lines, "question_deadline: "+q.Deadline.UTC().Format(time.RFC3339))
		}
		if rs := strings.TrimSpace(q.ReturnStage); rs != "" {
			lines = append(lines, "question_return_stage: "+rs)
		}
		// The run-epoch binding is what ties a decision to one run. Report it explicitly,
		// including its absence for records persisted before planner ask bound the epoch.
		if ep := strings.TrimSpace(q.RunEpoch); ep != "" {
			lines = append(lines, "question_run_epoch: "+ep)
		} else {
			lines = append(lines, "question_run_epoch: unbound (legacy)")
		}
		// Surface a decision that submission would refuse, using the SAME binding rule the
		// submit path applies, so the two can never disagree.
		if reason, berr := check(context.Background(), q); berr != nil {
			return "", berr
		} else if reason != "" {
			lines = append(lines, "question_stale: "+reason)
		}
		answers, aerr := fs.AnswersFor(q.ID)
		if aerr != nil {
			return "", aerr
		}
		for _, a := range answers {
			lines = append(lines, fmt.Sprintf("answer: %s by=%s action=%s key=%s body=%s",
				a.ID, a.AnsweredBy, a.ChosenAction, a.IdempotencyKey, oneLine(a.Body)))
		}
		outcomes, oerr := fs.Outcomes(q.ID)
		if oerr != nil {
			return "", oerr
		}
		for _, o := range outcomes {
			lines = append(lines, fmt.Sprintf("outcome: %s reason=%s", o.Kind, oneLine(o.Reason)))
		}
		if c := continuationFor(svc, q); c != nil {
			lines = append(lines, fmt.Sprintf("continuation: %s state=%s generation=%d", c.ID, c.State, c.Generation))
		}
	}
	return mcp.JoinLines(lines), nil
}

// decisionSubmitText records a planner decision through the shared answer service.
// Caller-supplied identifiers are only hints: the question's authoritative binding
// (task, attempt, run, plan revision, repository) decides whether the decision is
// current, and a stale decision is refused with the prior record intact.
func decisionSubmitText(ctx context.Context, dir, questionID, taskID, action string, in question.SubmitInput, expectedPlanRevision string) (string, error) {
	action = strings.TrimSpace(action)
	if !knownDecisionAction(action) {
		return "", fmt.Errorf("unknown decision action %q; want one of %s", action, strings.Join(decisionActions, ", "))
	}
	questionID = strings.TrimSpace(questionID)
	if questionID == "" {
		return "", fmt.Errorf("question_id is required")
	}
	q, _, _, err := readQuestion(dir, questionID)
	if err != nil {
		return "", err
	}
	if taskID = strings.TrimSpace(taskID); taskID != "" && taskID != strings.TrimSpace(q.Identity) {
		return fmt.Sprintf("STALE question=%s\nreason: question is bound to task %s, not %s", q.ID, q.Identity, taskID), nil
	}
	if expected := strings.TrimSpace(expectedPlanRevision); expected != "" {
		if cur := currentPlanRevision(dir); cur != "" && cur != expected {
			return fmt.Sprintf("STALE question=%s\nreason: plan revision changed: expected %s current %s", q.ID, expected, cur), nil
		}
	}

	res, err := submitPlannerAnswer(ctx, dir, in)
	if err != nil {
		return "", err
	}
	lines := []string{fmt.Sprintf("%s question=%s", res.Outcome, q.ID)}
	if res.Reason != "" {
		lines = append(lines, "reason: "+res.Reason)
	}
	if res.Question != nil {
		lines = append(lines, fmt.Sprintf("state: %s revision: %d attempt: %d", res.Question.State, res.Question.Revision, res.Question.Attempt))
		if res.Question.PlanRevision != "" {
			lines = append(lines, "plan_revision: "+res.Question.PlanRevision)
		}
	}
	if res.Answer != nil {
		lines = append(lines, fmt.Sprintf("answer: %s action: %s", res.Answer.ID, res.Answer.ChosenAction))
	}
	return mcp.JoinLines(lines), nil
}

// singleStringSchema builds the input schema of a tool taking one string.
func singleStringSchema(name, description string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			name: map[string]any{"type": "string", "description": description},
		},
		"required": []string{name},
	}
}

// requiredStringArg decodes one required string argument.
func requiredStringArg(raw json.RawMessage, name string) (string, error) {
	var in map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	value, _ := in[name].(string)
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

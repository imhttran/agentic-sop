package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/mcp"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/question"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/validate"
)

// mcpUsage is the one-line usage for `sop mcp`.
const mcpUsage = "usage: sop mcp [--status-only | --planner] [--start-work]"

// runMCP serves the Model Context Protocol over stdio. The exposed tools call
// the same application services as the CLI commands, so the workflow is never
// reimplemented for MCP. Requests are read from stdin and responses written to
// stdout; diagnostics go to stderr.
//
// With --status-only, only sop_status is registered: every other tool is absent
// from tools/list and rejected by tools/call, so the restriction is enforced
// server-side rather than by the client. This is the planner-safe surface for a
// connection spike, which must expose no human approval action.
//
// --start-work additionally registers sop_start_work and sop_work_status, which
// let a client start `sop prompt` / `sop run` in the background. It is opt-in for
// the default and planner surfaces and refused with --status-only.
func runMCP(args []string, stdout, stderr io.Writer, d deps) int {
	statusOnly, planner, startWork := false, false, false
	for _, a := range args {
		switch a {
		case "--status-only":
			statusOnly = true
		case "--planner":
			planner = true
		case "--start-work":
			startWork = true
		default:
			fmt.Fprintln(stderr, mcpUsage)
			return exitUsage
		}
	}
	if statusOnly && planner {
		fmt.Fprintln(stderr, "mcp: --status-only and --planner are mutually exclusive")
		return exitUsage
	}
	if statusOnly && startWork {
		fmt.Fprintln(stderr, "mcp: --start-work cannot be combined with --status-only")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	var tools []mcp.Tool
	switch {
	case planner:
		tools = mcpPlannerTools(dir, d)
	case statusOnly:
		tools = mcpToolsWithMode(dir, d, true)
	default:
		tools = mcpTools(dir, d)
	}
	if startWork {
		tools = append(tools, mcpStartWorkTools(dir, defaultWorkEnv())...)
	}

	server := mcp.NewServer(Version, tools...)
	if err := server.Serve(context.Background(), os.Stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "mcp: %v\n", err)
		return exitError
	}
	return exitOK
}

// mcpTools is the full tool set: status, validate, review, and the human
// approval boundary over the project. Every tool calls the same application
// service the CLI uses.
func mcpTools(dir string, d deps) []mcp.Tool {
	return mcpToolsWithMode(dir, d, false)
}

// mcpToolsWithMode builds the tool set for a mode. In status-only mode the
// approval, validate, and review tools are never registered, so they cannot be
// listed or called. sop_status is read-only. The full set is unchanged.
func mcpToolsWithMode(dir string, d deps, statusOnly bool) []mcp.Tool {
	status := mcpStatusTool(dir)
	if statusOnly {
		return []mcp.Tool{status}
	}
	return []mcp.Tool{
		status,
		mcp.TextTool("sop_validate", "Run the configured build/test/lint commands and report the result.", func(ctx context.Context) (string, error) {
			return mcpValidate(ctx, dir)
		}),
		mcp.TextTool("sop_review", "Review the current working-tree changes and report findings.", func(ctx context.Context) (string, error) {
			return mcpReview(ctx, dir, d)
		}),
		approvalTool("sop_approval", "Report SOP's approval request (if any) for a task.", dir, approvalRead),
		approvalTool("sop_approve", "Record an approval of a task's active human approval gate.", dir, approvalApprove),
		approvalTool("sop_decline", "Record a decline of a task's active human approval gate.", dir, approvalDecline),
	}
}

// mcpStatusTool is the read-only status tool shared by every mode.
func mcpStatusTool(dir string) mcp.Tool {
	return mcp.TextTool("sop_status", "List the persisted tasks for this project.", func(context.Context) (string, error) {
		return mcpStatus(dir)
	}).WithReadOnly()
}

// mcpPlannerTools is the planner-scoped surface: status, question list/get, answer
// submission, task-result retrieval, and the decision operations. It deliberately
// excludes the human approval actions (sop_approve, sop_decline), validation, and
// review, so a planner role can never approve, decline, or otherwise act on a human
// gate. Receiving an answer or a decision never applies it, approves a task, or
// resumes execution.
func mcpPlannerTools(dir string, d deps) []mcp.Tool {
	return []mcp.Tool{
		mcpStatusTool(dir),
		mcpQuestionListTool(dir),
		mcpQuestionGetTool(dir),
		mcpAnswerSubmitTool(dir),
		mcpTaskResultTool(dir),
		mcpDecisionListTool(dir),
		mcpDecisionSubmitTool(dir),
	}
}

// mcpQuestionListTool lists open (PENDING) questions so a planner can discover them.
func mcpQuestionListTool(dir string) mcp.Tool {
	return mcp.TextTool("sop_question_list", "List open (PENDING) planner questions for this project.", func(context.Context) (string, error) {
		questions, err := listQuestions(dir, question.StatePending)
		if err != nil {
			return "", err
		}
		if len(questions) == 0 {
			return "no pending questions", nil
		}
		lines := make([]string, 0, len(questions))
		for _, q := range questions {
			lines = append(lines, fmt.Sprintf("%s %s rev=%d task=%s %s", q.ID, q.State, q.Revision, q.Identity, q.Body))
		}
		return mcp.JoinLines(lines), nil
	}).WithReadOnly()
}

// mcpQuestionGetTool returns one question with its answers and audit outcomes as JSON.
func mcpQuestionGetTool(dir string) mcp.Tool {
	return mcp.Tool{
		Name:        "sop_question_get",
		Description: "Read one planner question and its independently recorded answers.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question_id": map[string]any{"type": "string"},
			},
			"required": []string{"question_id"},
		},
		ReadOnly: true,
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			var in struct {
				QuestionID string `json:"question_id"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			if strings.TrimSpace(in.QuestionID) == "" {
				return "", fmt.Errorf("question_id is required")
			}
			q, answers, outcomes, err := readQuestion(dir, in.QuestionID)
			if err != nil {
				return "", err
			}
			doc := map[string]any{"question": toQuestionView(q), "answers": answers, "outcomes": outcomes}
			data, err := json.Marshal(doc)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// mcpAnswerSubmitTool records a planner answer. It records ANSWERED only; it never
// applies the answer, approves a task, or resumes execution.
func mcpAnswerSubmitTool(dir string) mcp.Tool {
	return mcp.Tool{
		Name:        "sop_answer_submit",
		Description: "Submit a planner answer to a question (records ANSWERED only; never applies or approves).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question_id":       map[string]any{"type": "string"},
				"body":              map[string]any{"type": "string"},
				"answered_by":       map[string]any{"type": "string"},
				"idempotency_key":   map[string]any{"type": "string"},
				"expected_revision": map[string]any{"type": "integer"},
			},
			"required": []string{"question_id", "body"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var in struct {
				QuestionID       string `json:"question_id"`
				Body             string `json:"body"`
				AnsweredBy       string `json:"answered_by"`
				IdempotencyKey   string `json:"idempotency_key"`
				ExpectedRevision int    `json:"expected_revision"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			res, err := submitPlannerAnswer(ctx, dir, question.SubmitInput{
				QuestionID:       in.QuestionID,
				Body:             in.Body,
				AnsweredBy:       in.AnsweredBy,
				IdempotencyKey:   in.IdempotencyKey,
				ExpectedRevision: in.ExpectedRevision,
			})
			if err != nil {
				return "", err
			}
			doc := map[string]any{"outcome": string(res.Outcome), "reason": res.Reason}
			if res.Question != nil {
				doc["question"] = toQuestionView(res.Question)
			}
			if res.Answer != nil {
				doc["answer_id"] = res.Answer.ID
			}
			data, err := json.Marshal(doc)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// approvalCommand is one approval application operation exposed over MCP.
type approvalCommand func(svc *approval.Service, in approvalArgs) (string, error)

// approvalArgs is the MCP argument shape for the approval tools.
type approvalArgs struct {
	TaskID string `json:"taskId"`
	By     string `json:"by"`
	Note   string `json:"note"`
}

// approvalTool builds an MCP tool that delegates to the approval application
// boundary; the CLI exposes the same operations as `sop approval|approve|decline`.
func approvalTool(name, description, dir string, run approvalCommand) mcp.Tool {
	return mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{"type": "string"},
				"by":     map[string]any{"type": "string"},
				"note":   map[string]any{"type": "string"},
			},
			"required": []string{"taskId"},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (string, error) {
			var in approvalArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return "", fmt.Errorf("invalid arguments: %w", err)
				}
			}
			if strings.TrimSpace(in.TaskID) == "" {
				return "", fmt.Errorf("taskId is required")
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
			defer st.Close()
			return run(approvalService(st, dir), in)
		},
	}
}

// approvalRead renders SOP's approval projection as text.
func approvalRead(svc *approval.Service, in approvalArgs) (string, error) {
	view, err := svc.Approval(in.TaskID)
	if err != nil {
		return "", err
	}
	if !view.Present {
		return fmt.Sprintf("no approval request for %s", in.TaskID), nil
	}
	lines := []string{
		fmt.Sprintf("task: %s", view.TaskID),
		fmt.Sprintf("status: %s", view.Status),
		fmt.Sprintf("kind: %s", view.Kind),
	}
	if view.Stage != "" {
		lines = append(lines, "stage: "+view.Stage)
	}
	if view.Disposition != "" {
		lines = append(lines, "disposition: "+view.Disposition)
	}
	if view.Reason != "" {
		lines = append(lines, "reason: "+view.Reason)
	}
	return mcp.JoinLines(lines), nil
}

// approvalApprove records an approval through the boundary.
func approvalApprove(svc *approval.Service, in approvalArgs) (string, error) {
	res, err := svc.Approve(in.TaskID, approval.DecisionInput{By: in.By, Note: in.Note})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("approved %s (%s)", in.TaskID, res.View.Status), nil
}

// approvalDecline records a decline through the boundary.
func approvalDecline(svc *approval.Service, in approvalArgs) (string, error) {
	res, err := svc.Decline(in.TaskID, approval.DecisionInput{By: in.By, Note: in.Note})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("declined %s (%s)", in.TaskID, res.View.Status), nil
}

// mcpStatus lists persisted tasks; it never creates state.
func mcpStatus(dir string) (string, error) {
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
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "no tasks", nil
	}

	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		lines = append(lines, fmt.Sprintf("%s %s %s", task.ID, task.Status, task.Title))
	}
	return mcp.JoinLines(lines), nil
}

// mcpValidate runs the configured validation commands.
func mcpValidate(ctx context.Context, dir string) (string, error) {
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return "", err
	}
	if !validate.Enabled(cfg.Validation) {
		return "no validation commands configured", nil
	}

	res := validate.Run(ctx, dir, cfg.Validation)
	lines := make([]string, 0, len(res.Results)+1)
	for _, r := range res.Results {
		lines = append(lines, fmt.Sprintf("%s %s %s", r.Status, r.Category, r.Command))
	}
	lines = append(lines, "validation: "+passFail(res.Passed()))
	return mcp.JoinLines(lines), nil
}

// mcpReview reviews the working-tree changes.
func mcpReview(ctx context.Context, dir string, d deps) (string, error) {
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return "", err
	}

	diff, err := d.readDiff(ctx, dir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(diff) == "" {
		return "no changes to review", nil
	}

	provider, err := reviewProvider(cfg, d)
	if err != nil {
		return "", err
	}
	report, err := provider.Review(ctx, review.Request{Task: "Review the current working changes.", Diff: diff})
	if err != nil {
		return "", err
	}

	lines := []string{}
	if summary := strings.TrimSpace(report.Summary); summary != "" {
		lines = append(lines, summary)
	}
	for _, f := range report.Findings {
		lines = append(lines, fmt.Sprintf("%s %s — %s", f.Severity, f.File, f.Title))
	}
	blocking := quality.BlockingFindings(cfg.Quality.FailOn, report.Findings)
	lines = append(lines, fmt.Sprintf("review: %d finding(s), %d blocking", len(report.Findings), blocking))
	return mcp.JoinLines(lines), nil
}

// passFail renders a boolean result.
func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

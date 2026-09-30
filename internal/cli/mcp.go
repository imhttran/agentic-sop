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
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/validate"
)

// runMCP serves the Model Context Protocol over stdio. The exposed tools call
// the same application services as the CLI commands, so the workflow is never
// reimplemented for MCP. Requests are read from stdin and responses written to
// stdout; diagnostics go to stderr.
func runMCP(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop mcp")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	server := mcp.NewServer(Version, mcpTools(dir, d)...)
	if err := server.Serve(context.Background(), os.Stdin, stdout); err != nil {
		fmt.Fprintf(stderr, "mcp: %v\n", err)
		return exitError
	}
	return exitOK
}

// mcpTools is the tool set: status, validate, review, and the human approval
// boundary over the project. Every tool calls the same application service the
// CLI uses.
func mcpTools(dir string, d deps) []mcp.Tool {
	return []mcp.Tool{
		mcp.TextTool("sop_status", "List the persisted tasks for this project.", func(context.Context) (string, error) {
			return mcpStatus(dir)
		}),
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

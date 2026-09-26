package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

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

// mcpTools is the tool set: status, validate, and review over the project.
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
	}
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

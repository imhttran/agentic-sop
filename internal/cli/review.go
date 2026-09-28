package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
)

// runReview reviews the current working-tree changes with the configured review
// engine and reports findings. What blocks is deterministic: the quality
// policy's fail_on severities decide the exit code, not the model.
func runReview(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop review")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	// A missing configuration is fine: review falls back to the built-in policy.
	cfg := config.Default()
	if loaded, err := config.LoadDir(dir); err == nil {
		cfg = *loaded
	} else if !errors.Is(err, config.ErrNotFound) {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return exitError
	}

	diff, err := d.readDiff(context.Background(), dir)
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(diff) == "" {
		fmt.Fprintln(stdout, "no changes to review")
		return exitOK
	}

	provider, err := reviewProvider(cfg, d)
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return exitError
	}

	report, err := provider.Review(context.Background(), review.Request{
		Task: "Review the current working changes.",
		Diff: diff,
	})
	if err != nil {
		fmt.Fprintf(stderr, "review: %v\n", err)
		return exitError
	}

	if summary := strings.TrimSpace(report.Summary); summary != "" {
		fmt.Fprintln(stdout, summary)
	}
	for _, f := range report.Findings {
		location := f.File
		if f.Line > 0 {
			location = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(stdout, "%-8s %-24s %s\n", f.Severity, location, f.Title)
	}

	blocking := quality.BlockingFindings(cfg.Quality.FailOn, report.Findings)
	fmt.Fprintf(stdout, "review: %d finding(s), %d blocking\n", len(report.Findings), blocking)
	if blocking > 0 {
		return exitError
	}
	return exitOK
}

// reviewProvider selects the configured review engine. The open-code-review
// engine requires an external command; a missing command is an error, never a
// silent fallback to the internal reviewer.
func reviewProvider(cfg config.Config, d deps) (review.Provider, error) {
	if strings.TrimSpace(cfg.Review.Engine) == "open-code-review" {
		provider, err := review.ProviderFromEnv()
		if err != nil {
			return nil, fmt.Errorf("%w (set %s)", err, review.EnvReviewCommand)
		}
		return provider, nil
	}

	a, err := d.newAgent(cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		return nil, err
	}
	return review.NewAgentProvider(a), nil
}

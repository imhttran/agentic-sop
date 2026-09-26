package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runPr pushes a task branch and opens a pull request for it. It requires a task
// file so the branch name, title, and body are deterministic; it honours the
// human gate (--yes) and never merges.
func runPr(args []string, stdout, stderr io.Writer, d deps) int {
	path, approve, ok := parseGateArgs("pr", args, stderr)
	if !ok {
		return exitUsage
	}
	if path == "" {
		fmt.Fprintln(stderr, "usage: sop pr TASK.md [--yes]")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "pr: %v\n", err)
		return exitError
	}
	if cfg.Human.RequiresApprovalBeforeCommit() && !approve {
		fmt.Fprintln(stderr, "pr: human approval required; re-run with --yes")
		return exitError
	}

	spec, err := loadTaskFile(dir, path)
	if err != nil {
		fmt.Fprintf(stderr, "pr: %v\n", err)
		return exitError
	}
	if spec.ID == "" {
		fmt.Fprintln(stderr, "pr: task file needs an id to name the branch")
		return exitError
	}

	branch, err := git.TaskBranchName(spec.ID, spec.Title)
	if err != nil {
		fmt.Fprintf(stderr, "pr: %v\n", err)
		return exitError
	}

	ctx := context.Background()
	client := d.newGitHub(dir)
	if err := client.PushBranch(ctx, branch); err != nil {
		fmt.Fprintf(stderr, "pr: push %s: %v\n", branch, err)
		return exitError
	}

	pr, err := client.CreatePullRequest(ctx, github.CreateRequest{
		Base:  cfg.Project.IntegrationBranch,
		Head:  branch,
		Title: prTitle(spec),
		Body:  spec.Render(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "pr: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "opened %s\n", pr.URL)
	return exitOK
}

// prTitle builds the deterministic pull-request title from a task file.
func prTitle(spec *taskfile.Spec) string {
	if spec.ID != "" {
		return fmt.Sprintf("[Task %s] %s", spec.ID, spec.Title)
	}
	return spec.Title
}

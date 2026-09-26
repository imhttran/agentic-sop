package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// CommandClient implements Client over the gh and git CLIs using structured
// arguments (never a shell). The command runner is injectable for tests.
type CommandClient struct {
	dir string
	run runnerFunc
}

// NewCommandClient returns a CommandClient that runs gh/git in dir.
func NewCommandClient(dir string) *CommandClient {
	return &CommandClient{dir: dir, run: execRunner(dir)}
}

func execRunner(dir string) runnerFunc {
	return func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			detail := strings.TrimSpace(stderr.String())
			if detail != "" {
				return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
			}
			return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return stdout.Bytes(), nil
	}
}

// PushBranch pushes branch to origin and sets its upstream.
func (c *CommandClient) PushBranch(ctx context.Context, branch string) error {
	if err := validateBranchish(branch); err != nil {
		return err
	}
	_, err := c.run(ctx, "git", "push", "-u", "origin", branch)
	return err
}

// CreatePullRequest opens a pull request and returns its current state.
func (c *CommandClient) CreatePullRequest(ctx context.Context, req CreateRequest) (PullRequest, error) {
	if strings.TrimSpace(req.Base) == "" || strings.TrimSpace(req.Head) == "" || strings.TrimSpace(req.Title) == "" {
		return PullRequest{}, errors.New("github: base, head, and title are required")
	}
	args := []string{"pr", "create", "--base", req.Base, "--head", req.Head, "--title", req.Title, "--body", req.Body}
	if _, err := c.run(ctx, "gh", args...); err != nil {
		return PullRequest{}, err
	}
	return c.GetPullRequest(ctx, req.Head)
}

// GetPullRequest returns the pull request for a head branch.
func (c *CommandClient) GetPullRequest(ctx context.Context, head string) (PullRequest, error) {
	if strings.TrimSpace(head) == "" {
		return PullRequest{}, errors.New("github: head branch is required")
	}
	out, err := c.run(ctx, "gh", "pr", "view", head, "--json", "number,url,state,headRefName,baseRefName,mergeable")
	if err != nil {
		return PullRequest{}, err
	}
	var wire struct {
		Number    int    `json:"number"`
		URL       string `json:"url"`
		State     string `json:"state"`
		Head      string `json:"headRefName"`
		Base      string `json:"baseRefName"`
		Mergeable string `json:"mergeable"`
	}
	if err := json.Unmarshal(out, &wire); err != nil {
		return PullRequest{}, fmt.Errorf("github: parse pull request: %w", err)
	}
	return PullRequest{
		Number:    wire.Number,
		URL:       wire.URL,
		State:     wire.State,
		Head:      wire.Head,
		Base:      wire.Base,
		Mergeable: wire.Mergeable,
	}, nil
}

// Checks returns the PR checks with normalized states.
func (c *CommandClient) Checks(ctx context.Context, prNumber int) ([]Check, error) {
	out, err := c.run(ctx, "gh", "pr", "checks", strconv.Itoa(prNumber), "--json", "name,state,link")
	if err != nil && len(out) == 0 {
		return nil, err
	}
	var wire []struct {
		Name  string `json:"name"`
		State string `json:"state"`
		Link  string `json:"link"`
	}
	if err := json.Unmarshal(out, &wire); err != nil {
		return nil, fmt.Errorf("github: parse checks: %w", err)
	}
	checks := make([]Check, 0, len(wire))
	for _, w := range wire {
		checks = append(checks, Check{Name: w.Name, State: normalizeState(w.State), Link: w.Link})
	}
	return checks, nil
}

// Merge merges a pull request using the given method (squash, merge, rebase).
func (c *CommandClient) Merge(ctx context.Context, prNumber int, method string) error {
	if method == "" {
		method = "squash"
	}
	switch method {
	case "squash", "merge", "rebase":
	default:
		return fmt.Errorf("github: unsupported merge method %q", method)
	}
	_, err := c.run(ctx, "gh", "pr", "merge", strconv.Itoa(prNumber), "--"+method)
	return err
}

func normalizeState(state string) CheckState {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "SUCCESS", "PASS", "PASSED":
		return CheckPass
	case "FAILURE", "FAIL", "FAILED", "ERROR", "ACTION_REQUIRED", "STARTUP_FAILURE", "TIMED_OUT":
		return CheckFail
	case "PENDING", "QUEUED", "IN_PROGRESS", "WAITING", "REQUESTED", "EXPECTED":
		return CheckPending
	case "SKIPPED", "NEUTRAL", "STALE":
		return CheckSkipping
	case "CANCELLED", "CANCELED":
		return CheckCancel
	default:
		return CheckUnknown
	}
}

func validateBranchish(branch string) error {
	if strings.TrimSpace(branch) == "" {
		return errors.New("github: branch name is empty")
	}
	if strings.ContainsAny(branch, " ~^:?*[\\") || strings.ContainsAny(branch, "\n\r\t") {
		return fmt.Errorf("github: invalid branch name %q", branch)
	}
	return nil
}

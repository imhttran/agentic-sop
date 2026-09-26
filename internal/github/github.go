// Package github provides SOP's remote-workflow boundary: pushing branches,
// managing pull requests, inspecting checks, and merging. It owns remote
// operations only; workflow state stays with the orchestrator. A Client
// interface keeps the boundary provider-agnostic and testable.
package github

import "context"

// CheckState is a normalized PR check state.
type CheckState string

const (
	CheckPass     CheckState = "PASS"
	CheckFail     CheckState = "FAIL"
	CheckPending  CheckState = "PENDING"
	CheckSkipping CheckState = "SKIPPING"
	CheckCancel   CheckState = "CANCEL"
	CheckUnknown  CheckState = "UNKNOWN"
)

// PullRequest is the remote pull request for a task branch.
type PullRequest struct {
	Number    int
	URL       string
	State     string
	Head      string
	Base      string
	Mergeable string
}

// Check is a single PR check.
type Check struct {
	Name  string
	State CheckState
	Link  string
}

// CreateRequest describes a pull request to open.
type CreateRequest struct {
	Base  string
	Head  string
	Title string
	Body  string
}

// Client is the remote-workflow boundary.
type Client interface {
	PushBranch(ctx context.Context, branch string) error
	CreatePullRequest(ctx context.Context, req CreateRequest) (PullRequest, error)
	GetPullRequest(ctx context.Context, head string) (PullRequest, error)
	Checks(ctx context.Context, prNumber int) ([]Check, error)
	Merge(ctx context.Context, prNumber int, method string) error
}

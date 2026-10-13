package sopclient

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrTaskNotFound    = errors.New("task not found")
	ErrProjectNotFound = errors.New("project not found")
)

// Client is the concrete SOP boundary over one or more project roots.
type Client struct {
	stores  map[string]*Store
	order   []string
	cmd     *Commander
	timeout time.Duration
}

// New opens the SOP state for each root and returns a boundary client.
func New(roots []string, sopBin string, timeout time.Duration) (*Client, error) {
	c := &Client{stores: map[string]*Store{}, cmd: NewCommander(sopBin, timeout), timeout: timeout}
	for _, root := range roots {
		st, err := OpenStore(root)
		if err != nil {
			return nil, err
		}
		id := st.ProjectID()
		if _, dup := c.stores[id]; dup {
			return nil, fmt.Errorf("duplicate project id %q (project roots need distinct directory names)", id)
		}
		c.stores[id] = st
		c.order = append(c.order, id)
	}
	return c, nil
}

// Close releases the underlying state databases.
func (c *Client) Close() error {
	var err error
	for _, st := range c.stores {
		if e := st.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// Root returns the filesystem root for a project id.
func (c *Client) Root(id string) (string, bool) {
	st, ok := c.stores[id]
	if !ok {
		return "", false
	}
	return st.root, true
}

func (c *Client) Projects(ctx context.Context) ([]ProjectSummary, error) {
	out := make([]ProjectSummary, 0, len(c.order))
	for _, id := range c.order {
		changed, _ := c.ChangedTasks(ctx, id)
		// A failed listing read is an absence (no gate), never a fabricated one.
		approvals, _ := c.Approvals(ctx, id)
		s, err := c.stores[id].Summary(ctx, changed, approvals)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *Client) Project(ctx context.Context, id string) (ProjectDetail, error) {
	st, ok := c.stores[id]
	if !ok {
		return ProjectDetail{}, ErrProjectNotFound
	}
	changed, _ := c.ChangedTasks(ctx, id)
	approvals, _ := c.Approvals(ctx, id)
	sum, err := st.Summary(ctx, changed, approvals)
	if err != nil {
		return ProjectDetail{}, err
	}
	tasks, err := st.Tasks(ctx, approvals)
	if err != nil {
		return ProjectDetail{}, err
	}
	plan, _ := st.PlanSource()
	// ApprovalsReported carries whether SOP reported a listing (possibly empty) or the
	// read failed, so a consumer can show an absence rather than "no gates".
	return ProjectDetail{
		Summary:           sum,
		Tasks:             tasks,
		PlanSource:        plan,
		Plan:              st.Plan(),
		PlanPerformance:   st.PlanPerformance(),
		ApprovalsReported: approvals.Reported,
	}, nil
}

func (c *Client) Task(ctx context.Context, projectID, taskID string) (TaskDetail, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return TaskDetail{}, ErrProjectNotFound
	}
	approvals, _ := c.Approvals(ctx, projectID)
	return st.Task(ctx, taskID, approvals)
}

// PlanSource returns the active plan path SOP recorded for a project, if any.
func (c *Client) PlanSource(projectID string) (string, bool) {
	st, ok := c.stores[projectID]
	if !ok {
		return "", false
	}
	return st.PlanSource()
}

// ChangedTasks runs `sop reconcile <PLAN.md> --list-changed --json` (a pure read)
// with the plan path from plan.meta.json. No active plan is ErrNoActivePlan; a
// failed or unparsable listing is an error, never an empty set.
func (c *Client) ChangedTasks(ctx context.Context, projectID string) (ChangedTasks, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return ChangedTasks{}, ErrProjectNotFound
	}
	planPath, ok := st.PlanSource()
	if !ok {
		return ChangedTasks{}, ErrNoActivePlan
	}
	out, err := c.exec(ctx, projectID, "reconcile", planPath, "--list-changed", "--json")
	if err != nil {
		return ChangedTasks{}, err
	}
	doc, ok := decodeListing([]byte(out))
	if !ok {
		return ChangedTasks{}, fmt.Errorf("sop reconcile --list-changed --json: unparsable SOP changed-task listing")
	}
	return st.changedTasksFromListing(doc), nil
}

// ErrApprovalsUnavailable: `sop approvals --json` failed or was unparsable.
// Callers must surface it, never treat it as "no gates".
var ErrApprovalsUnavailable = errors.New("SOP did not report an approval listing")

// Approvals runs `sop approvals --json` and decodes SOP's listing verbatim.
// An empty listing is Reported with no error. A failed or unparsable run returns
// an error wrapping ErrApprovalsUnavailable, so a failing SOP is never mistaken
// for "no gates". The controller never reconstructs a gate from other state.
func (c *Client) Approvals(ctx context.Context, projectID string) (ApprovalsListing, error) {
	out, err := c.exec(ctx, projectID, "approvals", "--json")
	if errors.Is(err, ErrProjectNotFound) {
		return ApprovalsListing{}, err
	}
	if err != nil {
		return ApprovalsListing{}, fmt.Errorf("%w: sop approvals --json: %v", ErrApprovalsUnavailable, err)
	}
	listing, ok := decodeApprovals([]byte(out))
	if !ok {
		return ApprovalsListing{}, fmt.Errorf("%w: sop approvals --json: unparsable SOP approval listing", ErrApprovalsUnavailable)
	}
	return listing, nil
}

// --- commands (FR-8): each delegates to the SOP CLI ---

func (c *Client) Run(ctx context.Context, projectID string) error {
	_, err := c.exec(ctx, projectID, "run")
	return err
}

func (c *Client) Resume(ctx context.Context, projectID string) error {
	_, err := c.exec(ctx, projectID, "resume")
	return err
}

// Retry requeues a BLOCKED task (`sop retry <id>`); SOP decides eligibility.
func (c *Client) Retry(ctx context.Context, projectID, taskID string) error {
	_, err := c.exec(ctx, projectID, "retry", taskID)
	return err
}

// RetryForce raises max_attempts (`sop retry <id> --force`).
func (c *Client) RetryForce(ctx context.Context, projectID, taskID string) error {
	_, err := c.exec(ctx, projectID, "retry", taskID, "--force")
	return err
}

// RetryAll requeues every BLOCKED task with budget left (`sop retry --all`).
func (c *Client) RetryAll(ctx context.Context, projectID string) error {
	_, err := c.exec(ctx, projectID, "retry", "--all")
	return err
}

// Reconcile applies a plan change (`sop reconcile <PLAN.md>`).
func (c *Client) Reconcile(ctx context.Context, projectID, planPath string) (string, error) {
	return c.exec(ctx, projectID, "reconcile", planPath)
}

// ReportTask returns `sop report <task>`.
func (c *Client) ReportTask(ctx context.Context, projectID, taskID string) (string, error) {
	return c.exec(ctx, projectID, "report", taskID)
}

// Validate runs `sop validate` (FR-6).
func (c *Client) Validate(ctx context.Context, projectID string) (string, error) {
	return c.exec(ctx, projectID, "validate")
}

// RunReview runs `sop review` (FR-5).
func (c *Client) RunReview(ctx context.Context, projectID string) (string, error) {
	return c.exec(ctx, projectID, "review")
}

// RunReport runs `sop report` for the project.
func (c *Client) RunReport(ctx context.Context, projectID string) (string, error) {
	return c.exec(ctx, projectID, "report")
}

func (c *Client) exec(ctx context.Context, projectID, verb string, args ...string) (string, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return "", ErrProjectNotFound
	}
	return c.cmd.Exec(ctx, st.root, verb, args...)
}

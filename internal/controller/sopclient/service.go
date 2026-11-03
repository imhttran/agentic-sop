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

// projectNotFound names the missing project while keeping the sentinel
// matchable with errors.Is.
func projectNotFound(id string) error {
	return fmt.Errorf("project %q: %w", id, ErrProjectNotFound)
}

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
			return nil, errors.Join(err, c.Close())
		}
		id := st.ProjectID()
		if _, dup := c.stores[id]; dup {
			return nil, errors.Join(
				fmt.Errorf("duplicate project id %q (project roots need distinct directory names)", id),
				st.Close(),
				c.Close(),
			)
		}
		c.stores[id] = st
		c.order = append(c.order, id)
	}
	return c, nil
}

// Close releases the underlying state databases.
func (c *Client) Close() error {
	var errs []error
	for _, st := range c.stores {
		if err := st.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
		changed, changedErr := c.ChangedTasks(ctx, id)
		// A failed listing read is an absence (no gate), never a fabricated one.
		approvals, _ := c.Approvals(ctx, id)
		s, err := c.stores[id].Summary(ctx, changed, approvals)
		if err != nil {
			return nil, fmt.Errorf("project %q: %w", id, err)
		}
		s.ChangedTasksError = changedTasksError(changedErr)
		out = append(out, s)
	}
	return out, nil
}

func (c *Client) Project(ctx context.Context, id string) (ProjectDetail, error) {
	st, ok := c.stores[id]
	if !ok {
		return ProjectDetail{}, projectNotFound(id)
	}
	changed, changedErr := c.ChangedTasks(ctx, id)
	approvals, _ := c.Approvals(ctx, id)
	sum, err := st.Summary(ctx, changed, approvals)
	if err != nil {
		return ProjectDetail{}, err
	}
	sum.ChangedTasksError = changedTasksError(changedErr)
	tasks, err := st.Tasks(ctx, approvals)
	if err != nil {
		return ProjectDetail{}, err
	}
	plan, _, _ := st.PlanSource() // a damaged file is reported through Plan.Error
	return ProjectDetail{
		Summary:         sum,
		Tasks:           tasks,
		PlanSource:      plan,
		Plan:            st.Plan(),
		PlanPerformance: st.PlanPerformance(),
	}, nil
}

func (c *Client) Task(ctx context.Context, projectID, taskID string) (TaskDetail, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return TaskDetail{}, projectNotFound(projectID)
	}
	approvals, _ := c.Approvals(ctx, projectID)
	return st.Task(ctx, taskID, approvals)
}

// PlanSource returns the active plan path SOP recorded for a project, if any. A
// plan.meta.json that cannot be read is an error, never an absence.
func (c *Client) PlanSource(projectID string) (string, bool, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return "", false, projectNotFound(projectID)
	}
	return st.PlanSource()
}

// ChangedTasks runs `sop reconcile <PLAN.md> --list-changed --json` (a pure read)
// with the plan path from plan.meta.json. No active plan is ErrNoActivePlan; a
// failed or unparsable listing is an error, never an empty set.
func (c *Client) ChangedTasks(ctx context.Context, projectID string) (ChangedTasks, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return ChangedTasks{}, projectNotFound(projectID)
	}
	planPath, ok, err := st.PlanSource()
	if err != nil {
		return ChangedTasks{}, err
	}
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
		return ApprovalsListing{}, fmt.Errorf("%w: sop approvals --json: %w", ErrApprovalsUnavailable, err)
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

// exec runs one SOP verb for a project and attributes any failure to that project,
// so a multi-project failure says which project it came from. The cause stays
// wrapped, so errors.Is/As still reach a timeout or a *CommandError.
func (c *Client) exec(ctx context.Context, projectID, verb string, args ...string) (string, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return "", projectNotFound(projectID)
	}
	out, err := c.cmd.Exec(ctx, st.root, verb, args...)
	if err != nil {
		return out, fmt.Errorf("project %q: %w", projectID, err)
	}
	return out, nil
}

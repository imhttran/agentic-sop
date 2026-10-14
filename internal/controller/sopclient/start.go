package sopclient

import "context"

// StartOrContinueResult reports which SOP verb a start request delegated to.
type StartOrContinueResult struct {
	// Verb is "run" or "resume", or "" when the plan was already complete.
	Verb     string
	Complete bool
	Message  string
}

// StartOrContinue executes (or continues) the plan with the supported execution
// command. A complete plan (every task terminal) dispatches nothing; otherwise the
// controller delegates `sop run`, which continues a started-but-incomplete graph and
// selects the runnable task. `sop resume` is a read-only next-action inspection and is
// never used here to execute; SOP still owns any approval or blocker gate, which the
// command layer surfaces as "blocked" from SOP's own state rather than a fabricated
// transition.
func (c *Client) StartOrContinue(ctx context.Context, projectID string) (StartOrContinueResult, error) {
	detail, err := c.Project(ctx, projectID)
	if err != nil {
		return StartOrContinueResult{}, err
	}
	if planComplete(detail) {
		return StartOrContinueResult{
			Complete: true,
			Message:  "Plan complete: SOP reports every task finished; nothing to run.",
		}, nil
	}
	if err := c.Run(ctx, projectID); err != nil {
		return StartOrContinueResult{Verb: "run"}, err
	}
	return StartOrContinueResult{
		Verb:    "run",
		Message: "Ran the plan (sop run); SOP selected the runnable task.",
	}, nil
}

// planComplete reports whether every task is terminal.
func planComplete(detail ProjectDetail) bool {
	return detail.Summary.Total > 0 && detail.Summary.Completed == detail.Summary.Total
}

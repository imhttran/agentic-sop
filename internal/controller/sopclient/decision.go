package sopclient

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

// decide runs `sop <verb> <task-id> [--by] [--note]` (argv, no shell) and turns a
// non-zero exit into a *DecisionRejection with SOP's message. SOP validates the
// gate; no --run is passed.
func (c *Client) decide(ctx context.Context, projectID, verb, taskID string, opts DecisionOptions) error {
	st, ok := c.stores[projectID]
	if !ok {
		return projectNotFound(projectID)
	}
	out, err := c.cmd.Exec(ctx, st.root, verb, decisionArgs(taskID, opts)...)
	if sopRefused(err) {
		return &DecisionRejection{Verb: verb, TaskID: taskID, Message: decisionMessage(out, err), Err: err}
	}
	return err
}

// sopRefused reports whether SOP ran and exited non-zero. A timeout,
// cancellation, or spawn failure never reached SOP, so it is not a refusal.
func sopRefused(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

// decisionMessage prefers SOP's own output, else the command error.
func decisionMessage(out string, err error) string {
	if msg := strings.TrimSpace(out); msg != "" {
		return msg
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

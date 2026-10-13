package web

import (
	"context"
	"net/http"
	"strings"

	"sop-controller/internal/sopclient"
)

// Approve/decline delegate to `sop approve` / `sop decline`. SOP validates the
// gate at command time; the controller does not pre-judge it from its own read,
// and a SOP refusal is surfaced as a conflict, never a 500 or a fake success.

const (
	approveVerb = "approve"
	declineVerb = "decline"
)

func (h *Handlers) approve(w http.ResponseWriter, r *http.Request) {
	h.approvalCommand(w, r, approveVerb, func(ctx context.Context, project, taskID string, opts sopclient.DecisionOptions) (string, error) {
		if err := h.sop.ApproveTask(ctx, project, taskID, opts); err != nil {
			return "", err
		}
		return "SOP recorded the approval decision.", nil
	})
}

func (h *Handlers) approveStatus(w http.ResponseWriter, r *http.Request) {
	h.approvalCommandStatus(w, r, approveVerb)
}

func (h *Handlers) decline(w http.ResponseWriter, r *http.Request) {
	h.approvalCommand(w, r, declineVerb, func(ctx context.Context, project, taskID string, opts sopclient.DecisionOptions) (string, error) {
		if err := h.sop.DeclineTask(ctx, project, taskID, opts); err != nil {
			return "", err
		}
		return "SOP recorded the decline/withhold decision.", nil
	})
}

func (h *Handlers) declineStatus(w http.ResponseWriter, r *http.Request) {
	h.approvalCommandStatus(w, r, declineVerb)
}

// approvalOptions reads the optional actor/note. They reach SOP as discrete argv
// elements, never a shell string. When a decision actor is configured
// (SOP_CONTROLLER_OPERATOR) it is authoritative and the browser-supplied field is
// ignored: an editable browser field is a label, never a verified identity. The note is
// always taken from the request.
func (h *Handlers) approvalOptions(r *http.Request) sopclient.DecisionOptions {
	by := strings.TrimSpace(h.operator)
	if by == "" {
		by = strings.TrimSpace(r.FormValue("by"))
	}
	return sopclient.DecisionOptions{
		By:   by,
		Note: r.FormValue("note"),
	}
}

// approvalCommand starts the decision in the background. The task is read
// through the project-scoped Task read, so a (project, task) pair that does not
// match yields ErrTaskNotFound rather than acting on a mismatched URL.
func (h *Handlers) approvalCommand(w http.ResponseWriter, r *http.Request, verb string, fn func(ctx context.Context, project, taskID string, opts sopclient.DecisionOptions) (string, error)) {
	project, taskID := r.PathValue("project"), r.PathValue("task")
	detail, err := h.sop.Task(r.Context(), project, taskID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	if detail.ID != taskID {
		h.notFound(w, r, "Task not found")
		return
	}
	opts := h.approvalOptions(r)
	key := taskCommandKey(verb, taskID)
	_, _ = h.runner.Start(project, key, func(ctx context.Context) (string, error) {
		return fn(ctx, project, taskID, opts)
	})
	h.renderCommand(w, r, project, key, taskCommandURL(project, taskID, verb))
}

// approvalCommandStatus reports a recorded decision (running or finished) on its
// own merits; with no recorded command the URL is a 404.
func (h *Handlers) approvalCommandStatus(w http.ResponseWriter, r *http.Request, verb string) {
	project, taskID := r.PathValue("project"), r.PathValue("task")
	key := taskCommandKey(verb, taskID)
	if _, ok := h.runner.Status(project, key); ok {
		h.renderCommand(w, r, project, key, taskCommandURL(project, taskID, verb))
		return
	}
	h.notFound(w, r, "No approval command for this task")
}

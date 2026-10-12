package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"sop-controller/internal/config"
	"sop-controller/internal/sopclient"
)

// Handlers holds the dashboard's HTTP dependencies.
type Handlers struct {
	sop             *sopclient.Client
	views           *Views
	poll            time.Duration
	attention       time.Duration
	runner          *CommandRunner
	discoveryReport config.DiscoveryReport
}

type baseData struct {
	CSRF string
	Poll time.Duration
	Nav  string
}

type projectsPage struct {
	baseData
	Projects []sopclient.ProjectSummary
}

// approvalsPage feeds the cross-project pending-approvals inbox (P0).
type approvalsPage struct {
	baseData
	Projects []sopclient.ProjectApprovals
	// Pending is the number of applicable gates across all projects.
	Pending int
	// Unread is the number of projects whose approval listing could not be read
	// (an absence, not "no gates").
	Unread int
}

type discoveryPage struct {
	baseData
	Discovery config.DiscoveryReport
}

type projectPage struct {
	baseData
	Project sopclient.ProjectDetail
	// ChangedTasks is SOP's changed-executed-task listing, verbatim.
	ChangedTasks sopclient.ChangedTasks
	// ChangedTasksError is set when the listing could not be read; the panel shows
	// the failure instead of an empty success.
	ChangedTasksError string
	// Decisions is built from the same reads as this page (no extra SOP call).
	Decisions decisionsData
}

type taskPage struct {
	baseData
	ProjectID string
	Task      sopclient.TaskDetail
	Events    []sopclient.ActivityView
	// Approval mirrors Task.Approval so the approval partial can render from this page.
	Approval sopclient.Approval
}

type errorPage struct {
	baseData
	Status  int
	Message string
}

// activityData feeds activity.html; events carry Cursor/Seq (CTRL007).
type activityData struct {
	baseData
	Events []sopclient.ActivityView
}

// cmdStatusData is the shared shape for command_status.html.
type cmdStatusData struct {
	baseData
	Command    CommandState
	CommandURL string
}

func (h *Handlers) base(r *http.Request, nav string) baseData {
	return baseData{CSRF: csrfToken(r), Poll: h.poll, Nav: nav}
}

// baseForProject is base for a project fragment. r may be nil for an internal
// projection, in which case CSRF is empty (the fragment only renders state).
func (h *Handlers) baseForProject(project string) baseData {
	return baseData{CSRF: "", Poll: h.poll, Nav: "projects"}
}

func (h *Handlers) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.views.Render(w, name, data); err != nil {
		log.Printf("[render] %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (h *Handlers) notFound(w http.ResponseWriter, r *http.Request, msg string) {
	h.render(w, http.StatusNotFound, "error.html",
		errorPage{baseData: h.base(r, ""), Status: http.StatusNotFound, Message: msg})
}

func (h *Handlers) renderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sopclient.ErrProjectNotFound):
		h.notFound(w, r, "Project not found")
	case errors.Is(err, sopclient.ErrTaskNotFound):
		h.notFound(w, r, "Task not found")
	default:
		h.render(w, http.StatusInternalServerError, "error.html",
			errorPage{baseData: h.base(r, ""), Status: http.StatusInternalServerError, Message: err.Error()})
	}
}

func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/projects", http.StatusFound)
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handlers) projects(w http.ResponseWriter, r *http.Request) {
	list, err := h.sop.Projects(r.Context())
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "projects.html", projectsPage{baseData: h.base(r, "projects"), Projects: list})
}

// approvals renders the cross-project pending-approvals inbox (P0). It is a
// read-only projection of SOP's `sop approvals --json` across the observed
// projects; each control POSTs to the existing task-scoped approve/decline
// routes, so SOP still validates the gate at command time. GET is side-effect
// free.
func (h *Handlers) approvals(w http.ResponseWriter, r *http.Request) {
	projects := h.sop.PendingApprovals(r.Context())
	page := approvalsPage{baseData: h.base(r, "approvals"), Projects: projects}
	for _, p := range projects {
		page.Pending += len(p.Entries)
		if !p.Reported {
			page.Unread++
		}
	}
	h.render(w, http.StatusOK, "approvals.html", page)
}

// discovery renders the read-only discovery report (roots, depth, projects, skips).
func (h *Handlers) discovery(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "discovery.html", discoveryPage{baseData: h.base(r, "discovery"), Discovery: h.discoveryReport})
}

func (h *Handlers) project(w http.ResponseWriter, r *http.Request) {
	detail, err := h.sop.Project(r.Context(), r.PathValue("project"))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// One listing read feeds both the changed-task and decisions panels; a failure is
	// shown in the panel and does not break the page.
	changed, changedErr := h.readChangedTasks(r.Context(), detail)
	base := h.base(r, "projects")
	h.render(w, http.StatusOK, "project.html", projectPage{
		baseData:          base,
		Project:           detail,
		ChangedTasks:      changed,
		ChangedTasksError: changedErr,
		Decisions:         decisions(base, detail, changed, changedErr),
	})
}

// projectTasks is the polled task-list fragment (FR-2, FR-10).
func (h *Handlers) projectTasks(w http.ResponseWriter, r *http.Request) {
	detail, err := h.sop.Project(r.Context(), r.PathValue("project"))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "task_list.html", struct {
		baseData
		Project sopclient.ProjectDetail
	}{h.base(r, ""), detail})
}

// projectActivity renders the same cursor/seq views the live stream sends (FR-4).
func (h *Handlers) projectActivity(w http.ResponseWriter, r *http.Request) {
	win, err := h.sop.ActivityWindow(r.Context(), r.PathValue("project"), "", 50)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "activity.html", activityData{h.base(r, ""), win.Events})
}

func (h *Handlers) task(w http.ResponseWriter, r *http.Request) {
	projectID, taskID := r.PathValue("project"), r.PathValue("task")
	detail, err := h.sop.Task(r.Context(), projectID, taskID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "task.html", taskPage{
		baseData:  h.base(r, "projects"),
		ProjectID: projectID,
		Task:      detail,
		Events:    h.taskActivityViews(r.Context(), projectID, taskID, detail),
		Approval:  detail.Approval,
	})
}

// taskActivityViews reads the task's own activity, falling back to
// detail.Activity if that read fails.
func (h *Handlers) taskActivityViews(ctx context.Context, projectID, taskID string, detail sopclient.TaskDetail) []sopclient.ActivityView {
	views, err := h.sop.TaskActivityView(ctx, projectID, taskID)
	if err == nil {
		return views
	}
	log.Printf("[activity] task %s/%s window read: %v (falling back to task activity)", projectID, taskID, err)
	out := make([]sopclient.ActivityView, 0, len(detail.Activity))
	for _, e := range detail.Activity {
		out = append(out, sopclient.ActivityView{ActivityEvent: e})
	}
	return out
}

func (h *Handlers) taskActivity(w http.ResponseWriter, r *http.Request) {
	projectID, taskID := r.PathValue("project"), r.PathValue("task")
	detail, err := h.sop.Task(r.Context(), projectID, taskID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "activity.html", activityData{h.base(r, ""), h.taskActivityViews(r.Context(), projectID, taskID, detail)})
}

// taskRecovery renders the recovery panel fragment (classification + actions).
func (h *Handlers) taskRecovery(w http.ResponseWriter, r *http.Request) {
	projectID, taskID := r.PathValue("project"), r.PathValue("task")
	detail, err := h.sop.Task(r.Context(), projectID, taskID)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, "recovery.html", taskPage{
		baseData:  h.base(r, ""),
		ProjectID: projectID,
		Task:      detail,
		Approval:  detail.Approval,
	})
}

// taskReview/taskCI/taskHandoff pass the TaskDetail so task.html can inline the
// same templates.
func (h *Handlers) taskReview(w http.ResponseWriter, r *http.Request) {
	h.taskFragment(w, r, "review.html")
}

func (h *Handlers) taskCI(w http.ResponseWriter, r *http.Request) {
	h.taskFragment(w, r, "ci.html")
}

func (h *Handlers) taskHandoff(w http.ResponseWriter, r *http.Request) {
	h.taskFragment(w, r, "handoff.html")
}

func (h *Handlers) taskFragment(w http.ResponseWriter, r *http.Request, name string) {
	detail, err := h.sop.Task(r.Context(), r.PathValue("project"), r.PathValue("task"))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	h.render(w, http.StatusOK, name, detail)
}

// startVerb is start-or-continue (CTRL004): one command key for run/resume, so a
// double click cannot launch two runs.
const startVerb = "start"

// reconcileCommandKey serializes plain reconcile per project. Accept-changed keeps
// its own per-task key.
const reconcileCommandKey = "reconcile"

// command starts a SOP command (FR-8) and returns its status fragment.
func (h *Handlers) command(w http.ResponseWriter, r *http.Request) {
	project, verb := r.PathValue("project"), r.PathValue("verb")
	if _, ok := h.sop.Root(project); !ok {
		h.notFound(w, r, "Project not found")
		return
	}
	var fn func(context.Context) (string, error)
	switch verb {
	case startVerb, "run":
		// `run` and `start` both mean start-or-continue: StartOrContinue picks
		// `sop run` or `sop resume` from SOP's state.
		fn = func(ctx context.Context) (string, error) {
			res, err := h.sop.StartOrContinue(ctx, project)
			return res.Message, err
		}
	case "resume":
		fn = func(ctx context.Context) (string, error) { return "", h.sop.Resume(ctx, project) }
	case "validate":
		fn = func(ctx context.Context) (string, error) { return h.sop.Validate(ctx, project) }
	case "review":
		fn = func(ctx context.Context) (string, error) { return h.sop.RunReview(ctx, project) }
	case "report":
		fn = func(ctx context.Context) (string, error) { return h.sop.RunReport(ctx, project) }
	case "retry-all":
		fn = func(ctx context.Context) (string, error) { return "", h.sop.RetryAll(ctx, project) }
	case "reconcile":
		plan, ok := h.sop.PlanSource(project)
		if !ok {
			h.render(w, http.StatusConflict, "error.html", errorPage{
				baseData: h.base(r, ""),
				Status:   http.StatusConflict,
				Message:  "No active plan recorded by SOP; there is nothing to reconcile.",
			})
			return
		}
		// CTRL012: refuse to start reconcile while SOP reports pending changed tasks,
		// so the human boundary cannot be stepped around.
		changed, err := h.sop.ChangedTasks(r.Context(), project)
		if err != nil {
			h.render(w, http.StatusConflict, "error.html", errorPage{
				baseData: h.base(r, ""),
				Status:   http.StatusConflict,
				Message:  "SOP did not report a changed-executed-task listing; reconcile is refused: " + err.Error(),
			})
			return
		}
		if pending := changed.Pending(); len(pending) > 0 {
			h.render(w, http.StatusConflict, "error.html", errorPage{
				baseData: h.base(r, ""),
				Status:   http.StatusConflict,
				Message:  "SOP reports changed executed tasks still pending approval: " + pendingTaskIDs(pending) + ". Reconcile is refused until each is explicitly accepted.",
			})
			return
		}
		fn = func(ctx context.Context) (string, error) { return h.sop.Reconcile(ctx, project, plan) }
	default:
		http.Error(w, "unknown command", http.StatusBadRequest)
		return
	}
	// start/run share one key; reconcile uses the project reconcile key.
	key := verb
	if verb == startVerb {
		key = "run"
	}
	if verb == "reconcile" {
		key = reconcileCommandKey
	}
	h.runner.Start(project, key, fn)
	h.renderCommand(w, r, project, key, commandURL(project, key))
}

// commandStatus: an unknown project is a 404, not a phantom "idle".
func (h *Handlers) commandStatus(w http.ResponseWriter, r *http.Request) {
	project, verb := r.PathValue("project"), r.PathValue("verb")
	if _, ok := h.sop.Root(project); !ok {
		h.notFound(w, r, "Project not found")
		return
	}
	if verb == startVerb {
		verb = "run"
	}
	if verb == "reconcile" {
		verb = reconcileCommandKey
	}
	h.renderCommand(w, r, project, verb, commandURL(project, verb))
}

func commandURL(project, verb string) string {
	return "/projects/" + project + "/commands/" + verb
}

// pendingTaskIDs joins pending ids for the refusal message.
func pendingTaskIDs(pending []sopclient.ChangedExecutedTask) string {
	ids := make([]string, len(pending))
	for i, t := range pending {
		ids[i] = t.TaskID
	}
	return strings.Join(ids, ", ")
}

// acceptChangedTask delegates to Client.AcceptChangedTask, which checks every
// precondition before calling SOP (CTRL012).
func (h *Handlers) acceptChangedTask(w http.ResponseWriter, r *http.Request) {
	h.taskCommand(w, r, "accept-changed", func(ctx context.Context, project, taskID string) (string, error) {
		return "", h.sop.AcceptChangedTask(ctx, project, taskID)
	})
}

func (h *Handlers) acceptChangedTaskStatus(w http.ResponseWriter, r *http.Request) {
	h.taskCommandStatus(w, r, "accept-changed")
}

// retry requeues a single task through SOP (FR-8).
func (h *Handlers) retry(w http.ResponseWriter, r *http.Request) {
	h.taskCommand(w, r, "retry", func(ctx context.Context, project, taskID string) (string, error) {
		return "", h.sop.Retry(ctx, project, taskID)
	})
}

func (h *Handlers) retryStatus(w http.ResponseWriter, r *http.Request) {
	h.taskCommandStatus(w, r, "retry")
}

// retryForce requeues a task whose retry budget is spent (`sop retry --force`).
func (h *Handlers) retryForce(w http.ResponseWriter, r *http.Request) {
	h.taskCommand(w, r, "retry-force", func(ctx context.Context, project, taskID string) (string, error) {
		return "", h.sop.RetryForce(ctx, project, taskID)
	})
}

func (h *Handlers) retryForceStatus(w http.ResponseWriter, r *http.Request) {
	h.taskCommandStatus(w, r, "retry-force")
}

// reportTask returns SOP's concise summary of one task's latest run.
func (h *Handlers) reportTask(w http.ResponseWriter, r *http.Request) {
	h.taskCommand(w, r, "report", h.sop.ReportTask)
}

func (h *Handlers) reportTaskStatus(w http.ResponseWriter, r *http.Request) {
	h.taskCommandStatus(w, r, "report")
}

// taskCommandKey gives every task command its own (verb, task) key.
func taskCommandKey(verb, taskID string) string {
	return verb + ":" + taskID
}

// taskCommand runs a task command in the background and renders its polling
// status fragment.
func (h *Handlers) taskCommand(w http.ResponseWriter, r *http.Request, verb string, fn func(ctx context.Context, project, taskID string) (string, error)) {
	project, taskID := r.PathValue("project"), r.PathValue("task")
	if _, ok := h.sop.Root(project); !ok {
		h.notFound(w, r, "Project not found")
		return
	}
	key := taskCommandKey(verb, taskID)
	_, _ = h.runner.Start(project, key, func(ctx context.Context) (string, error) {
		return fn(ctx, project, taskID)
	})
	h.renderTaskCommand(w, r, project, taskID, key, taskCommandURL(project, taskID, verb))
}

// taskCommandStatus: an unknown project is a 404.
func (h *Handlers) taskCommandStatus(w http.ResponseWriter, r *http.Request, verb string) {
	project, taskID := r.PathValue("project"), r.PathValue("task")
	if _, ok := h.sop.Root(project); !ok {
		h.notFound(w, r, "Project not found")
		return
	}
	h.renderTaskCommand(w, r, project, taskID, taskCommandKey(verb, taskID), taskCommandURL(project, taskID, verb))
}

// renderTaskCommand adds the task's Stage/Attempt when the read succeeds.
func (h *Handlers) renderTaskCommand(w http.ResponseWriter, r *http.Request, project, taskID, key, url string) {
	st := h.commandState(project, key)
	if detail, err := h.sop.Task(r.Context(), project, taskID); err == nil {
		st.TaskID = taskID
		st.Stage = detail.Stage
		st.Attempt = detail.Attempt
	}
	h.render(w, http.StatusOK, "command_status.html", cmdStatusData{
		baseData:   h.base(r, ""),
		Command:    st,
		CommandURL: url,
	})
}

func taskCommandURL(project, taskID, verb string) string {
	return "/projects/" + project + "/tasks/" + taskID + "/commands/" + verb
}

func (h *Handlers) renderCommand(w http.ResponseWriter, r *http.Request, project, key, url string) {
	st := h.commandState(project, key)
	if detail, err := h.sop.Project(r.Context(), project); err == nil {
		if taskID, ok := detail.ActiveTask(); ok {
			for _, t := range detail.Tasks {
				if t.ID == taskID {
					st.TaskID, st.Stage, st.Attempt = t.ID, t.Stage, t.Attempt
					break
				}
			}
		}
	}
	h.render(w, http.StatusOK, "command_status.html", cmdStatusData{
		baseData:   h.base(r, ""),
		Command:    st,
		CommandURL: url,
	})
}

// commandState returns the recorded status for project/key, or an idle
// placeholder when no command has run yet.
func (h *Handlers) commandState(project, key string) CommandState {
	st, ok := h.runner.Status(project, key)
	if !ok {
		return CommandState{Verb: key, State: "idle"}
	}
	return st
}

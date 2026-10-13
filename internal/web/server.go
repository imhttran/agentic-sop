package web

import (
	"io/fs"
	"net/http"
	"time"

	"sop-controller/internal/config"
	"sop-controller/internal/sopclient"
)

// Options wires the dashboard server.
type Options struct {
	SOP      *sopclient.Client
	Views    *Views
	StaticFS fs.FS
	Poll     time.Duration
	// Attention is the gate re-read cadence for live transports; zero uses the
	// short default.
	Attention      time.Duration
	CommandTimeout time.Duration
	AllowNetwork   bool
	AccessToken    string
	// SessionTTL bounds an authenticated session; zero uses the default.
	SessionTTL time.Duration
	// SessionFile persists remembered-device sessions across restarts; empty keeps them
	// in memory only.
	SessionFile string
	// Operator is the configured decision actor; when set it is recorded for
	// approve/decline instead of the browser-provided actor field.
	Operator string
	// Discovery feeds /discovery; zero renders an empty report.
	Discovery config.DiscoveryReport
}

// NewServer builds the HTTP handler for the dashboard.
func NewServer(opts Options) http.Handler {
	h := &Handlers{sop: opts.SOP, views: opts.Views, poll: opts.Poll, attention: opts.Attention, runner: NewCommandRunner(opts.CommandTimeout), discoveryReport: opts.Discovery}
	h.accessToken = opts.AccessToken
	h.operator = opts.Operator
	h.sessions = newSessionStore(opts.AccessToken, opts.SessionTTL, opts.SessionFile)
	h.loginLimiter = newAttemptLimiter(loginMaxAttempts, loginWindow)

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(opts.StaticFS))))

	mux.HandleFunc("GET /healthz", h.health)
	// Token login (network mode): the sign-in page, its submission, and logout. The
	// token is validated server-side and never appears in a URL.
	mux.HandleFunc("GET /login", h.loginPage)
	mux.HandleFunc("POST /login", h.loginSubmit)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /projects", h.projects)
	// P0 cross-project pending-approvals inbox (read-only; controls POST to the
	// task-scoped approve/decline routes above).
	mux.HandleFunc("GET /approvals", h.approvals)
	mux.HandleFunc("GET /discovery", h.discovery)
	mux.HandleFunc("GET /projects/{project}", h.project)
	mux.HandleFunc("GET /projects/{project}/tasks", h.projectTasks)
	mux.HandleFunc("GET /projects/{project}/activity", h.projectActivity)
	// C2-006 decisions surface (read-only; actions POST to the task routes below).
	mux.HandleFunc("GET /projects/{project}/decisions", h.projectDecisions)
	// Live activity (CTRL007): SSE stream and bounded-poll fallback.
	mux.HandleFunc("GET /projects/{project}/activity/stream", h.activityStream)
	mux.HandleFunc("GET /projects/{project}/activity/window", h.activityWindow)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}", h.task)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/activity", h.taskActivity)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/recovery", h.taskRecovery)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/review", h.taskReview)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/ci", h.taskCI)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/handoff", h.taskHandoff)
	mux.HandleFunc("POST /projects/{project}/commands/{verb}", h.command)
	mux.HandleFunc("GET /projects/{project}/commands/{verb}", h.commandStatus)
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/retry", h.retry)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/retry", h.retryStatus)
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/retry-force", h.retryForce)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/retry-force", h.retryForceStatus)
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/report", h.reportTask)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/report", h.reportTaskStatus)
	// CTRL011 approval controls delegate to `sop approve` / `sop decline`; SOP
	// validates the gate. GET status routes match the fragment's polling URL.
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/approve", h.approve)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/approve", h.approveStatus)
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/decline", h.decline)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/decline", h.declineStatus)
	// CTRL012 per-task accept-changed; see Client.AcceptChangedTask.
	mux.HandleFunc("POST /projects/{project}/tasks/{task}/commands/accept-changed", h.acceptChangedTask)
	mux.HandleFunc("GET /projects/{project}/tasks/{task}/commands/accept-changed", h.acceptChangedTaskStatus)

	var handler http.Handler = mux
	handler = csrfProtect(handler)
	if opts.AllowNetwork {
		handler = authGate(opts.AccessToken, h.sessions, handler)
	}
	handler = logRequests(handler)
	return handler
}

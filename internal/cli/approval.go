package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/approval"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// approvalService builds the human approval application boundary over the
// project's task store and each task's run directory. The CLI, MCP, and the
// lifecycle all delegate to this one service, so approval semantics live in a
// single place.
func approvalService(st *store.Store, projectDir string) *approval.Service {
	return approval.New(st, func(taskID string) approval.Record {
		return runpkg.At(runpkg.Dir(projectDir, taskID))
	})
}

// runApprovals lists every task SOP reports at an applicable human approval gate.
// It is read-only: it classifies nothing itself, creates no request, and resolves
// nothing. A task whose boundary cannot be read is an error, so a listing never
// silently omits a gate the human needs to see.
func runApprovals(args []string, stdout, stderr io.Writer, d deps) int {
	jsonOut := false
	for _, a := range args {
		switch a {
		case "--json":
			jsonOut = true
		default:
			fmt.Fprintln(stderr, "usage: sop approvals [--json]")
			return exitUsage
		}
	}

	dir, st, ok := openApprovalState(d, stderr, "approvals")
	if !ok {
		return exitError
	}
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "approvals: %v\n", err)
		return exitError
	}

	svc := approvalService(st, dir)
	items := []approvalListingItem{}
	for _, task := range tasks {
		view, err := svc.Approval(task.ID)
		if err != nil {
			fmt.Fprintf(stderr, "approvals: %s: %v\n", task.ID, err)
			return exitError
		}
		if !view.Applicable {
			continue
		}
		items = append(items, approvalListingItem{
			TaskID:      view.TaskID,
			Kind:        string(view.Kind),
			Target:      view.Target,
			Reason:      view.Reason,
			Evidence:    view.Evidence,
			Stage:       view.Stage,
			Disposition: view.Disposition,
			Status:      string(view.Status),
			RequestedAt: view.RequestedAt,
			TaskStatus:  string(view.TaskStatus),
		})
	}

	if jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(approvalListingDoc{Version: 1, Approvals: items}); err != nil {
			fmt.Fprintf(stderr, "approvals: %v\n", err)
			return exitError
		}
		return exitOK
	}

	if len(items) == 0 {
		fmt.Fprintln(stdout, "no pending approvals")
		return exitOK
	}
	for _, it := range items {
		fmt.Fprintf(stdout, "%s  %s  %s  %s\n", it.TaskID, it.Kind, it.Stage, oneLine(it.Reason))
	}
	fmt.Fprintln(stdout, "\nInspect one with: sop approval <task-id>")
	fmt.Fprintln(stdout, "Decide one with:  sop approve <task-id> | sop decline <task-id>")
	return exitOK
}

// approvalListingItem is one pending gate in the machine-readable listing: the
// same fields the single-task view projects, so a client renders a gate identically
// whether it read one or listed many.
type approvalListingItem struct {
	TaskID      string    `json:"task_id"`
	Kind        string    `json:"kind,omitempty"`
	Target      string    `json:"target,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Evidence    string    `json:"evidence,omitempty"`
	Stage       string    `json:"stage,omitempty"`
	Disposition string    `json:"disposition,omitempty"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requested_at,omitempty"`
	TaskStatus  string    `json:"task_status,omitempty"`
}

// approvalListingDoc is the stable JSON document for `sop approvals --json`.
type approvalListingDoc struct {
	Version   int                   `json:"version"`
	Approvals []approvalListingItem `json:"approvals"`
}

// runApprovalStatus prints SOP's authoritative approval boundary for a task: the
// present-or-absent request SOP recorded. It never classifies SOP state itself
// and never creates or modifies anything.
func runApprovalStatus(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop approval <task-id>")
		return exitUsage
	}
	id := args[0]

	dir, st, ok := openApprovalState(d, stderr, "approval")
	if !ok {
		return exitError
	}
	defer st.Close()

	view, err := approvalService(st, dir).Approval(id)
	if err != nil {
		fmt.Fprintf(stderr, "approval: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "Task: %s\n", id)
	fmt.Fprintf(stdout, "Task status: %s\n", view.TaskStatus)
	if !view.Present {
		// A task status alone is never an approval boundary: SOP reports none.
		fmt.Fprintln(stdout, "Approval: none")
		return exitOK
	}
	writeApprovalView(stdout, view)
	return exitOK
}

// runApprove records an approval of the task's active approval request. It
// delegates entirely to the application boundary; it holds no lifecycle logic.
// With no <task-id> (or --select) it chooses among the applicable gates at a
// terminal (Phase 6). With --run it starts the ordinary run only AFTER the decision
// is persisted, so a failure to start the run never loses the recorded decision.
func runApprove(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseDecisionArgs(args, true, stderr)
	if !ok {
		return exitUsage
	}
	if opts.id == "" || opts.choose {
		return runInteractiveDecision(true, opts, stdout, stderr, d)
	}

	dir, st, ok := openApprovalState(d, stderr, "approve")
	if !ok {
		return exitError
	}
	defer st.Close()

	res, err := approvalService(st, dir).Approve(opts.id, approval.DecisionInput{By: opts.by, Note: opts.note})
	if err != nil {
		fmt.Fprintf(stderr, "approve: %v\n", err)
		return exitError
	}
	recordApprovalDecisionActivity(dir, opts.id, true, opts.note)
	if res.Idempotent {
		fmt.Fprintf(stdout, "already approved: %s\n", opts.id)
	} else {
		fmt.Fprintf(stdout, "approved %s\n", opts.id)
		writeApprovalView(stdout, res.View)
	}
	if opts.run {
		// The decision above is already persisted, so a failure to start the run
		// leaves the approval recorded and the task runnable. The continuation is the
		// ordinary run — the same path `sop run` uses — and never bypasses a gate.
		return runRun(nil, stdout, stderr, d)
	}
	return exitOK
}

// runDecline records a decline of the task's active approval request. It
// preserves truthful lifecycle state and never manufactures completion. With no
// <task-id> (or --select) it chooses among the applicable gates at a terminal.
func runDecline(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parseDecisionArgs(args, false, stderr)
	if !ok {
		return exitUsage
	}
	if opts.id == "" || opts.choose {
		return runInteractiveDecision(false, opts, stdout, stderr, d)
	}

	dir, st, ok := openApprovalState(d, stderr, "decline")
	if !ok {
		return exitError
	}
	defer st.Close()

	res, err := approvalService(st, dir).Decline(opts.id, approval.DecisionInput{By: opts.by, Note: opts.note})
	if err != nil {
		fmt.Fprintf(stderr, "decline: %v\n", err)
		return exitError
	}
	recordApprovalDecisionActivity(dir, opts.id, false, opts.note)
	if res.Idempotent {
		fmt.Fprintf(stdout, "already declined: %s\n", opts.id)
		return exitOK
	}
	fmt.Fprintf(stdout, "declined %s\n", opts.id)
	writeApprovalView(stdout, res.View)
	return exitOK
}

// openApprovalState resolves the project directory and opens the state store,
// reporting a diagnostic through stderr. It is the shared preamble of the
// approval commands.
func openApprovalState(d deps, stderr io.Writer, label string) (string, *store.Store, bool) {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return "", nil, false
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return "", nil, false
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", label, err)
		return "", nil, false
	}
	return dir, st, true
}

// decisionOptions are the parsed arguments of `sop approve` / `sop decline`.
type decisionOptions struct {
	id     string
	by     string
	note   string
	choose bool
	run    bool
}

// decisionUsage is the one-line usage for a decision command.
func decisionUsage(allowRun bool) string {
	if allowRun {
		return "usage: sop approve [<task-id> | --select] [--by NAME] [--note TEXT] [--run]"
	}
	return "usage: sop decline [<task-id> | --select] [--by NAME] [--note TEXT]"
}

// parseDecisionArgs parses `[<task-id> | --select] [--by NAME] [--note TEXT] [--run]`.
// An omitted task id and --select are equivalent: both ask for an interactive choice
// among the applicable gates, which the caller MUST fail closed on when stdin is not
// a terminal. --run is accepted only for approve (allowRun); for a decline it is a
// usage error, because a decline never continues execution.
func parseDecisionArgs(args []string, allowRun bool, stderr io.Writer) (decisionOptions, bool) {
	var opts decisionOptions
	fail := func() (decisionOptions, bool) {
		fmt.Fprintln(stderr, decisionUsage(allowRun))
		return decisionOptions{}, false
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--by":
			if i+1 >= len(args) {
				return fail()
			}
			i++
			opts.by = args[i]
		case strings.HasPrefix(a, "--by="):
			opts.by = strings.TrimPrefix(a, "--by=")
		case a == "--note":
			if i+1 >= len(args) {
				return fail()
			}
			i++
			opts.note = args[i]
		case strings.HasPrefix(a, "--note="):
			opts.note = strings.TrimPrefix(a, "--note=")
		case a == "--select":
			opts.choose = true
		case a == "--run":
			if !allowRun {
				return fail()
			}
			opts.run = true
		case strings.HasPrefix(a, "-"):
			return fail()
		default:
			if opts.id != "" {
				return fail()
			}
			opts.id = a
		}
	}
	// An explicit id and --select are mutually exclusive intents: one names the
	// task, the other asks to choose one. Neither is silently ignored.
	if opts.id != "" && opts.choose {
		return fail()
	}
	return opts, true
}

// writeApprovalView renders SOP's approval projection for display.
func writeApprovalView(w io.Writer, v approval.View) {
	fmt.Fprintf(w, "Approval: %s\n", v.Status)
	if v.Kind != "" {
		fmt.Fprintf(w, "Kind: %s\n", v.Kind)
	}
	if v.Target != "" {
		fmt.Fprintf(w, "Target: %s\n", v.Target)
	}
	if v.Stage != "" {
		fmt.Fprintf(w, "Stage: %s\n", v.Stage)
	}
	if v.Disposition != "" {
		fmt.Fprintf(w, "Disposition: %s\n", v.Disposition)
	}
	if v.Reason != "" {
		fmt.Fprintf(w, "Reason: %s\n", v.Reason)
	}
	if v.Evidence != "" {
		fmt.Fprintf(w, "Evidence: %s\n", v.Evidence)
	}
	if !v.RequestedAt.IsZero() {
		fmt.Fprintf(w, "Requested: %s\n", v.RequestedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if !v.DecidedAt.IsZero() {
		fmt.Fprintf(w, "Decided: %s", v.DecidedAt.Format("2006-01-02T15:04:05Z07:00"))
		if v.LifecycleAction != "" {
			fmt.Fprintf(w, " (lifecycle: %s)", v.LifecycleAction)
		}
		fmt.Fprintln(w)
	}
	if v.DecidedBy != "" {
		fmt.Fprintf(w, "Decided by: %s\n", v.DecidedBy)
	}
	if v.Note != "" {
		fmt.Fprintf(w, "Note: %s\n", v.Note)
	}
}

// recordApprovalDecisionActivity appends a decision event to the task's activity
// stream, so a controller/CLI reading the stream sees the human decision beside
// the rest of the task's lifecycle. It is best-effort and never affects the
// command's outcome.
func recordApprovalDecisionActivity(projectDir, taskID string, approved bool, note string) {
	runDir := runpkg.Dir(projectDir, taskID)
	action := "DECLINED"
	if approved {
		action = "APPROVED"
	}
	rec := activity.New(taskID, newActivityArtifact(filepath.Join(runDir, activityArtifactName)))
	rec.Emit(activity.StageApproval, action, oneLine(note))
}

// currentApprovalBoundary reports the CURRENT task run's structured approval
// boundary, if any: an active (unresolved) approval request SOP recorded for the
// task, or a persisted WAITING_FOR_HUMAN run stage from the previous invocation.
// It is the ONLY input that can produce APPROVAL_REQUIRED; free-form prose, a task
// title, an acceptance criterion, or inspected/domain/fixture state never can.
func currentApprovalBoundary(rn *runpkg.Run, priorStage runpkg.Stage) failure.ApprovalBoundary {
	if req, ok := rn.Approval(); ok && req.Status == domain.ApprovalPending {
		return failure.ApprovalRequest
	}
	if priorStage == runpkg.WaitingForHuman {
		return failure.ApprovalWaitingForHuman
	}
	return failure.ApprovalNone
}

// humanBoundary reports whether a lifecycle result parked the task at a genuine
// human decision boundary, so an explicit approval request is warranted. It is
// driven by the classifier's human disposition, the autonomy policy's human
// decision, or the WAITING_FOR_HUMAN stage — never by a task status or prose.
func humanBoundary(stage runpkg.Stage, cls failure.Classification, d autonomy.Decision) bool {
	return d.RequiresHuman || d.Action == autonomy.ActionHumanApproval ||
		cls.Disposition == failure.NeedsHuman || stage == runpkg.WaitingForHuman
}

// recordHumanApprovalRequest records SOP's explicit, resolvable approval request
// for a task parked at a human boundary, so a client can resolve it through the
// approval application boundary. It is best-effort provenance: a write failure is
// dropped and never changes the run outcome. It records only SOP's own decision,
// never a client-supplied gate.
func recordHumanApprovalRequest(ctx context.Context, rn *runpkg.Run, task *domain.Task, stage runpkg.Stage, disp failure.Disposition, reason, evidence string) {
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return
	}
	svc := approval.New(nil, func(string) approval.Record { return rn })
	if _, err := svc.Request(approval.RequestInput{
		TaskID:      task.ID,
		Kind:        domain.ApprovalNeedsHuman,
		Target:      task.ID,
		Reason:      oneLine(reason),
		Evidence:    oneLine(evidence),
		Stage:       string(stage),
		Disposition: string(disp),
		RequestedBy: "sop",
	}); err != nil {
		return
	}
	activity.FromContext(ctx).Emit(activity.StageApproval, "REQUESTED", oneLine(reason))
}

// runInteractiveDecision lets a human choose among the applicable gates at a terminal
// and then records the decision through the SAME application boundary the explicit
// commands use. It is one authoritative decision operation, not a second mechanism.
//
// It fails closed: off a TTY it prints the usage and the gates and records nothing,
// and a cancel (or a closed stdin) records nothing and exits non-zero. What the human
// types is a DECISION to relay to the boundary — never a command, a capability, or a
// lifecycle transition — and the boundary revalidates the gate at mutation time, so a
// gate that went stale between the listing and the choice is refused.
func runInteractiveDecision(approve bool, opts decisionOptions, stdout, stderr io.Writer, d deps) int {
	label, verb := "decline", "Decline"
	if approve {
		label, verb = "approve", "Approve"
	}

	dir, st, ok := openApprovalState(d, stderr, label)
	if !ok {
		return exitError
	}
	defer st.Close()

	svc := approvalService(st, dir)
	gates, err := listApplicableGates(svc, st)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", label, err)
		return exitError
	}
	if len(gates) == 0 {
		fmt.Fprintln(stderr, "no pending approvals")
		return exitError
	}

	in := stdinOf(d)
	if !interactiveInput(d) {
		// Fail closed: an interactive choice requires a human at a terminal. Print
		// the usage and the enumerated gates, record nothing, and never guess a
		// default — a pipe, a CI run, or a redirect is not a decision.
		fmt.Fprintln(stderr, decisionUsage(approve))
		fmt.Fprintln(stderr, "interactive selection requires a terminal (stdin is not a TTY); record nothing:")
		writeGateList(stderr, gates)
		fmt.Fprintf(stderr, "decide one explicitly with: sop %s <task-id>\n", label)
		return exitUsage
	}

	writeGateList(stdout, gates)
	br := bufio.NewReader(in)

	id := gates[0].TaskID
	if len(gates) > 1 {
		fmt.Fprintf(stdout, "%s which task? [1-%d or <task-id>]: ", verb, len(gates))
		line, _ := readDecisionLine(br)
		selected, ok := selectGate(strings.TrimSpace(line), gates)
		if !ok {
			fmt.Fprintln(stderr, "cancelled: no decision recorded")
			return exitError
		}
		id = selected
	}

	fmt.Fprintf(stdout, "%s %s? [y/N]: ", verb, id)
	line, _ := readDecisionLine(br)
	if !confirmed(line) {
		fmt.Fprintln(stderr, "cancelled: no decision recorded")
		return exitError
	}

	var res approval.Result
	if approve {
		res, err = svc.Approve(id, approval.DecisionInput{By: opts.by, Note: opts.note})
	} else {
		res, err = svc.Decline(id, approval.DecisionInput{By: opts.by, Note: opts.note})
	}
	if err != nil {
		// A selection that is not (or is no longer) applicable is refused with the
		// boundary's own error; the CLI never resolves it itself.
		fmt.Fprintf(stderr, "%s: %v\n", label, err)
		return exitError
	}
	recordApprovalDecisionActivity(dir, id, approve, opts.note)
	if approve {
		if res.Idempotent {
			fmt.Fprintf(stdout, "already approved: %s\n", id)
		} else {
			fmt.Fprintf(stdout, "approved %s\n", id)
			writeApprovalView(stdout, res.View)
		}
	} else {
		if res.Idempotent {
			fmt.Fprintf(stdout, "already declined: %s\n", id)
			return exitOK
		}
		fmt.Fprintf(stdout, "declined %s\n", id)
		writeApprovalView(stdout, res.View)
	}
	if approve && opts.run {
		return runRun(nil, stdout, stderr, d)
	}
	return exitOK
}

// listApplicableGates enumerates every task SOP reports at an applicable gate, in
// store order. It classifies nothing itself: it asks the application boundary, so the
// listing can never disagree with what a decision would accept.
func listApplicableGates(svc *approval.Service, st *store.Store) ([]approval.View, error) {
	tasks, err := st.List()
	if err != nil {
		return nil, err
	}
	gates := []approval.View{}
	for _, task := range tasks {
		view, err := svc.Approval(task.ID)
		if err != nil {
			return nil, err
		}
		if view.Applicable {
			gates = append(gates, view)
		}
	}
	return gates, nil
}

// writeGateList renders the applicable gates for a human, numbered when there is
// more than one so a selection has an unambiguous target.
func writeGateList(w io.Writer, gates []approval.View) {
	for i, g := range gates {
		if len(gates) > 1 {
			fmt.Fprintf(w, "%d) %s  %s  %s  %s\n", i+1, g.TaskID, g.Kind, g.Stage, oneLine(g.Reason))
			continue
		}
		fmt.Fprintf(w, "%s  %s  %s  %s\n", g.TaskID, g.Kind, g.Stage, oneLine(g.Reason))
	}
}

// selectGate resolves an interactive selection to a task id. A number selects by
// position; anything else is taken as a task id verbatim, so the application boundary
// — not the CLI — decides whether it is applicable.
func selectGate(input string, gates []approval.View) (string, bool) {
	if input == "" {
		return "", false
	}
	if n, err := strconv.Atoi(input); err == nil {
		if n < 1 || n > len(gates) {
			return "", false
		}
		return gates[n-1].TaskID, true
	}
	return input, true
}

// readDecisionLine reads one line of interactive input. A missing trailing newline
// (EOF) still yields the line, so a piped `y` is read the same as a typed one; the
// caller decides what the text means.
func readDecisionLine(br *bufio.Reader) (string, error) {
	line, err := br.ReadString('\n')
	return strings.TrimSpace(line), err
}

// confirmed reports whether an interactive answer is an explicit yes. Only "y" or
// "yes" (case-insensitive) confirms; everything else — including an empty line and
// EOF — is not a decision.
func confirmed(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// stdinOf returns the interactive input, defaulting to os.Stdin.
func stdinOf(d deps) io.Reader {
	if d.stdin != nil {
		return d.stdin
	}
	return os.Stdin
}

// interactiveInput reports whether the approval commands may ask a human. It is the
// real terminal check unless a test injected one.
func interactiveInput(d deps) bool {
	r := stdinOf(d)
	if d.interactive != nil {
		return d.interactive(r)
	}
	return isTerminalReader(r)
}

// isTerminalReader reports whether r is an interactive terminal (a character
// device). A bytes.Buffer, a pipe, or a regular file is not, so a piped, redirected,
// or CI stdin fails closed.
func isTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// printParkedHumanGate renders the human boundary a run stopped at, so an operator
// needs no SOP internals to know what to do next. It is presentation only: it reads
// the request SOP recorded and prints the commands that resolve it. When the run has
// no resolvable gate — an ad-hoc `--task`/`prompt` run whose id is not a stored task —
// it says so rather than naming a command that would fail.
func printParkedHumanGate(w io.Writer, id, stage, reason, continuation string, resolvable bool) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run paused: human approval required")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Task: %s\n", id)
	if stage != "" {
		fmt.Fprintf(w, "Stage: %s\n", stage)
	}
	if r := oneLine(reason); r != "" {
		fmt.Fprintf(w, "Reason: %s\n", r)
	}
	fmt.Fprintln(w)
	if !resolvable {
		fmt.Fprintln(w, "This run has no stored task, so it has no resolvable approval gate.")
		fmt.Fprintln(w, "Continue with:")
		fmt.Fprintf(w, "  %s\n", continuation)
		return
	}
	fmt.Fprintln(w, "Inspect:")
	fmt.Fprintf(w, "  sop approval %s\n", id)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approve:")
	fmt.Fprintf(w, "  sop approve %s\n", id)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Decline:")
	fmt.Fprintf(w, "  sop decline %s\n", id)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "After approving, explicitly continue with:")
	fmt.Fprintf(w, "  %s\n", continuation)
}

// printLeftoverGates lists the gates a run left behind, with a pointer at
// `sop approvals`, so an operator who stops a run can see what still waits on them.
// It prints nothing when no gate is applicable, and it is best-effort: a listing
// failure never changes the run's outcome.
func printLeftoverGates(w io.Writer, dir string, st *store.Store) {
	gates, err := listApplicableGates(approvalService(st, dir), st)
	if err != nil || len(gates) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approvals left behind:")
	for _, g := range gates {
		fmt.Fprintf(w, "  %s  %s  %s  %s\n", g.TaskID, g.Kind, g.Stage, oneLine(g.Reason))
	}
	fmt.Fprintln(w, "List them with: sop approvals")
}

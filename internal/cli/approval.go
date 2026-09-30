package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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
func runApprove(args []string, stdout, stderr io.Writer, d deps) int {
	id, by, note, ok := parseDecisionArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, st, ok := openApprovalState(d, stderr, "approve")
	if !ok {
		return exitError
	}
	defer st.Close()

	res, err := approvalService(st, dir).Approve(id, approval.DecisionInput{By: by, Note: note})
	if err != nil {
		fmt.Fprintf(stderr, "approve: %v\n", err)
		return exitError
	}
	recordApprovalDecisionActivity(dir, id, true, note)
	if res.Idempotent {
		fmt.Fprintf(stdout, "already approved: %s\n", id)
		return exitOK
	}
	fmt.Fprintf(stdout, "approved %s\n", id)
	writeApprovalView(stdout, res.View)
	return exitOK
}

// runDecline records a decline of the task's active approval request. It
// preserves truthful lifecycle state and never manufactures completion.
func runDecline(args []string, stdout, stderr io.Writer, d deps) int {
	id, by, note, ok := parseDecisionArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, st, ok := openApprovalState(d, stderr, "decline")
	if !ok {
		return exitError
	}
	defer st.Close()

	res, err := approvalService(st, dir).Decline(id, approval.DecisionInput{By: by, Note: note})
	if err != nil {
		fmt.Fprintf(stderr, "decline: %v\n", err)
		return exitError
	}
	recordApprovalDecisionActivity(dir, id, false, note)
	if res.Idempotent {
		fmt.Fprintf(stdout, "already declined: %s\n", id)
		return exitOK
	}
	fmt.Fprintf(stdout, "declined %s\n", id)
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

// parseDecisionArgs parses `<task-id> [--by NAME] [--note TEXT]`.
func parseDecisionArgs(args []string, stderr io.Writer) (id, by, note string, ok bool) {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--by" && i+1 < len(args):
			i++
			by = args[i]
		case args[i] == "--note" && i+1 < len(args):
			i++
			note = args[i]
		case strings.HasPrefix(args[i], "-"):
			fmt.Fprintln(stderr, "usage: sop approve|decline <task-id> [--by NAME] [--note TEXT]")
			return "", "", "", false
		case id == "":
			id = args[i]
		default:
			fmt.Fprintln(stderr, "usage: sop approve|decline <task-id> [--by NAME] [--note TEXT]")
			return "", "", "", false
		}
	}
	if id == "" {
		fmt.Fprintln(stderr, "usage: sop approve|decline <task-id> [--by NAME] [--note TEXT]")
		return "", "", "", false
	}
	return id, by, note, true
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

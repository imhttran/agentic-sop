package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runPlan reads PRD.md, asks the configured agent for a plan, then writes both
// PLAN.md (human) and .agent-sdlc/plan.json (machine). An existing PLAN.md is
// never overwritten, so a human-edited plan is safe.
func runPlan(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) >= 1 {
		switch args[0] {
		case "activate":
			return runPlanActivate(args[1:], stdout, stderr, d)
		case "supersede":
			return runPlanSupersede(args[1:], stdout, stderr, d)
		case "complete":
			return runPlanComplete(args[1:], stdout, stderr, d)
		case "historicalize":
			return runPlanHistoricalize(args[1:], stdout, stderr, d)
		}
	}
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: sop plan [TASK.md] | sop plan activate <PLAN.md> | sop plan supersede <PLAN.md> | sop plan complete | sop plan historicalize [<PLAN.md>]")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	planPath := filepath.Join(dir, "PLAN.md")
	planExists, err := exists(planPath)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}
	if planExists {
		fmt.Fprintln(stderr, "PLAN.md already exists; refusing to overwrite")
		return exitError
	}

	// The planning input is PRD.md by default, or a Markdown task file when one
	// is given. A task file is parsed and normalized so the agent reasons over a
	// stable shape rather than raw prose.
	sourceName := "PRD.md"
	if len(args) == 1 {
		sourceName = args[0]
	}
	sourcePath := sourceName
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(dir, sourceName)
	}

	source, err := os.ReadFile(sourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(stderr, "%s not found in project directory\n", sourceName)
			return exitError
		}
		fmt.Fprintf(stderr, "plan: read %s: %v\n", sourceName, err)
		return exitError
	}
	if strings.TrimSpace(string(source)) == "" {
		fmt.Fprintf(stderr, "%s is empty\n", sourceName)
		return exitError
	}

	input := string(source)
	if len(args) == 1 {
		spec, err := taskfile.Parse(source)
		if err != nil {
			fmt.Fprintf(stderr, "plan: %v\n", err)
			return exitError
		}
		input = spec.Render()
	}

	harness, provider, model, err := configuredAgent(dir)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	a, err := d.newAgent(harness, provider, model)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	plan, err := planner.New(a).OnRepair(func(attempt int, cause error) {
		fmt.Fprintf(stderr, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
	}).Generate(context.Background(), input)
	if err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	if err := writePlanArtifacts(dir, plan); err != nil {
		fmt.Fprintf(stderr, "plan: %v\n", err)
		return exitError
	}

	fmt.Fprintln(stdout, "wrote PLAN.md")
	return exitOK
}

// writePlanArtifacts writes the human PLAN.md and the machine plan.json from
// the same validated Plan. Payloads are built first and each file is written to
// a temporary file and renamed, so a failure never leaves a half-written file.
func writePlanArtifacts(dir string, plan *planner.Plan) error {
	markdown := plan.RenderMarkdown()
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("encode plan.json: %w", err)
	}
	data = append(data, '\n')

	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(dir, "PLAN.md"), markdown); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(stateDir, "plan.json"), string(data)); err != nil {
		return err
	}
	return nil
}

// writeFileAtomic writes content to path via a temporary file and rename, so a
// failure part-way through does not leave a truncated file behind.
func writeFileAtomic(path, content string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// exists reports whether path exists, distinguishing "not found" from other
// filesystem errors. It is the single os.Stat wrapper for the CLI package.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// --- Plan lifecycle: activation and supersession ----------------------------
//
// Activation and execution are separate operator actions. `sop plan activate`
// installs a validated plan's task graph as the ACTIVE plan and stops; it never
// runs the first task. `sop run` then executes the next runnable task. Supersession
// explicitly replaces an active plan, even one with unresolved work, preserving its
// evidence as history.

// runPlanActivate resolves and validates a PLAN and installs its task graph as the
// active plan WITHOUT executing any task. It invokes no lifecycle capability for a
// task and records no task attempt: the first task is left READY for a later
// `sop run`. The configured agent is consulted only when the document is not a
// recognizable plan and must be normalized; a well-formed PLAN.md (or an
// already-recorded machine plan) activates without one.
func runPlanActivate(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop plan activate <PLAN.md>")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	source, err := planflow.ResolvePlanPath(dir, args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "plan activate: %v\n", err)
		return exitError
	}
	defer st.Close()

	res, err := planflow.Prepare(context.Background(), planflow.Options{
		Dir:        dir,
		PlanSource: source,
		Agent:      lifecycleAgent(dir, d),
		Store:      st,
		OnRepair:   lifecycleRepair(stderr),
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	printActivation(stdout, res, st)
	return exitOK
}

// runPlanSupersede explicitly replaces the active plan with a validated new plan,
// even when the active plan still has unresolved work, preserving the previous
// plan's task records, run evidence, and approvals as a SUPERSEDED archive. It
// never executes a task.
func runPlanSupersede(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop plan supersede <PLAN.md>")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	source, err := planflow.ResolvePlanPath(dir, args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "plan supersede: %v\n", err)
		return exitError
	}
	defer st.Close()

	res, err := planflow.Supersede(context.Background(), planflow.Options{
		Dir:        dir,
		PlanSource: source,
		Agent:      lifecycleAgent(dir, d),
		Store:      st,
		OnRepair:   lifecycleRepair(stderr),
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	if res.SupersededPlanID != "" || res.SupersededSource != "" {
		fmt.Fprintf(stdout, "Superseded plan: %s\n", displayOr(res.SupersededSource, "(unknown source)"))
		if res.SupersededPlanID != "" {
			fmt.Fprintf(stdout, "Superseded plan id: %s\n", res.SupersededPlanID)
		}
		if res.Archived != "" {
			fmt.Fprintf(stdout, "Archived (SUPERSEDED): %s\n", res.Archived)
		}
		fmt.Fprintln(stdout, "Its unfinished, FAILED, and NEEDS_HUMAN states are preserved as history (no PASS or approval fabricated).")
	} else {
		fmt.Fprintln(stdout, "No active plan to supersede; activating the requested plan.")
	}
	printActivation(stdout, planflow.Result{Source: res.Source, PlanID: res.PlanID, PlanRebuilt: true, TasksCreated: res.TasksCreated}, st)
	return exitOK
}

// runPlanComplete archives the active plan as COMPLETE and releases it, without
// installing a successor. It fails closed when the plan still has unresolved work.
// It never executes a task and never runs a model.
func runPlanComplete(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop plan complete")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "plan complete: %v\n", err)
		return exitError
	}
	defer st.Close()

	res, err := planflow.Complete(planflow.Options{Dir: dir, Store: st})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	if res.Source != "" {
		fmt.Fprintf(stdout, "Completed plan: %s\n", res.Source)
	} else {
		fmt.Fprintln(stdout, "Completed active plan")
	}
	if res.PlanID != "" {
		fmt.Fprintf(stdout, "Plan id: %s\n", res.PlanID)
	}
	if res.Archived != "" {
		fmt.Fprintf(stdout, "Archived (COMPLETE): %s\n", res.Archived)
	}
	fmt.Fprintf(stdout, "Preserved %d task record(s) as history.\n", res.Tasks)
	return exitOK
}

// lifecycleAgent builds the configured agent best-effort for a plan-lifecycle
// command. It returns nil when none is configured or it cannot be built: a
// lifecycle operation proceeds for a well-formed plan and reports the missing
// agent only if the document must be normalized. Activation must not require a
// model/provider merely to activate a well-formed plan.
func lifecycleAgent(dir string, d deps) agent.Agent {
	if d.newAgent == nil {
		return nil
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return nil
	}
	a, err := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		return nil
	}
	return a
}

// lifecycleRepair mirrors the reconcile repair observer so a normalized plan is
// reported the same way.
func lifecycleRepair(stderr io.Writer) planner.RepairFunc {
	return func(attempt int, cause error) {
		fmt.Fprintf(stderr, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
	}
}

// printActivation reports the active plan and its task states after an activation
// or supersession, stating plainly that nothing executed.
func printActivation(w io.Writer, res planflow.Result, st *store.Store) {
	fmt.Fprintf(w, "Activated %s\n", res.Source)
	if res.PlanID != "" {
		fmt.Fprintf(w, "Plan ID: %s\n", res.PlanID)
	}
	if res.PlanRebuilt {
		fmt.Fprintln(w, "Plan compiled and installed.")
	}
	fmt.Fprintln(w, "State: ACTIVE")
	fmt.Fprintln(w, "Nothing executed: the first task is READY for a later `sop run`.")
	fmt.Fprintln(w)

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(w, "plan: %v\n", err)
		return
	}
	for _, t := range tasks {
		if t.Status == domain.PLANNED {
			fmt.Fprintf(w, "%s %s %s\n", t.ID, displayStatus(t), t.Title)
			continue
		}
		fmt.Fprintf(w, "%s %s %s\n", t.ID, string(t.Status), t.Title)
	}
}

// displayStatus renders a task's persisted status for the activation summary.
func displayStatus(t *domain.Task) string {
	if t.Status == "" {
		return string(domain.PLANNED)
	}
	return string(t.Status)
}

// displayOr returns s when non-empty, otherwise fallback.
func displayOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// --- Plan historicalization -------------------------------------------------
//
// Historicalization is the deterministic, model-free lifecycle transition that
// moves a completed (or explicitly disposed) plan from ACTIVE into its historical
// record. `sop plan historicalize` performs it; `--check` reports readiness only.
// The operation validates the plan's terminal state, task dispositions, pending
// approvals, and verification evidence before mutating, is idempotent
// (ALREADY_HISTORICALIZED), and refuses a plan that changed since readiness. It
// never executes a task and never runs a model.

// runPlanHistoricalize archives the active plan, preserving its task outcomes,
// verification evidence, approvals, and terminal disposition. It reuses the
// planflow archive the plan lifecycle already owns; it never archives the wrong
// plan and never fabricates completion.
func runPlanHistoricalize(args []string, stdout, stderr io.Writer, d deps) int {
	const usage = "usage: sop plan historicalize [<PLAN.md>] [--disposition COMPLETE|SUPERSEDED] [--check] [--json]"
	var plan, disposition string
	check, jsonOut := false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--check":
			check = true
		case a == "--json":
			jsonOut = true
		case a == "--disposition":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "plan historicalize: --disposition requires a value")
				fmt.Fprintln(stderr, usage)
				return exitUsage
			}
			disposition = args[i+1]
			i++
		case strings.HasPrefix(a, "--disposition="):
			disposition = strings.TrimPrefix(a, "--disposition=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "plan historicalize: unknown flag %s\n", a)
			fmt.Fprintln(stderr, usage)
			return exitUsage
		case plan == "":
			plan = a
		default:
			fmt.Fprintln(stderr, usage)
			return exitUsage
		}
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}
	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "plan historicalize: %v\n", err)
		return exitError
	}
	defer st.Close()

	if check {
		ready, err := planflow.EvaluateHistoricalization(dir, st, plan, disposition)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		if jsonOut {
			writeHistoricalizationJSON(stdout, ready)
		} else {
			printHistoricalizationReadiness(stdout, ready)
		}
		if !ready.Eligible {
			fmt.Fprintf(stderr, "plan historicalize: the plan is not eligible; see the readiness report (%s)\n", ready.Reason)
			return exitError
		}
		return exitOK
	}

	res, err := planflow.Historicalize(planflow.HistoricalizeOptions{Dir: dir, Store: st, Plan: plan, Disposition: disposition})
	if err != nil {
		if errors.Is(err, planflow.ErrHistoricalizationIneligible) && res.Readiness.State != "" {
			if jsonOut {
				writeHistoricalizationJSON(stdout, res.Readiness)
			} else {
				printHistoricalizationReadiness(stdout, res.Readiness)
				fmt.Fprintln(stdout, "No mutation performed.")
			}
		}
		fmt.Fprintln(stderr, err)
		return exitError
	}
	if jsonOut {
		writeHistoricalizationResultJSON(stdout, res)
		return exitOK
	}
	printHistoricalizationResult(stdout, res)
	return exitOK
}

// printHistoricalizationReadiness renders a readiness evaluation for an operator.
func printHistoricalizationReadiness(w io.Writer, r planflow.HistoricalizationReadiness) {
	fmt.Fprintf(w, "Plan: %s\n", displayOr(r.PlanID, "(unknown)"))
	if r.Source != "" {
		fmt.Fprintf(w, "Source: %s\n", r.Source)
	}
	fmt.Fprintf(w, "State: %s\n", r.State)
	fmt.Fprintf(w, "Disposition: %s\n", r.Disposition)
	fmt.Fprintf(w, "Tasks: %d\n", r.TotalTasks)
	if len(r.TaskCounts) > 0 {
		fmt.Fprintf(w, "By state: %s\n", formatHistoricalizationCounts(r.TaskCounts))
	}
	fmt.Fprintf(w, "Eligible: %s\n", historicalizationYesNo(r.Eligible))
	if r.Reason != "" {
		fmt.Fprintf(w, "Reason: %s\n", r.Reason)
	}
	if len(r.UnresolvedTasks) > 0 {
		fmt.Fprintf(w, "Unresolved tasks: %s\n", strings.Join(r.UnresolvedTasks, ", "))
	}
	if len(r.UnresolvedApprovals) > 0 {
		fmt.Fprintf(w, "Unresolved approvals: %s\n", strings.Join(r.UnresolvedApprovals, ", "))
	}
	if len(r.UnresolvedVerification) > 0 {
		fmt.Fprintf(w, "Unresolved verification: %s\n", strings.Join(r.UnresolvedVerification, ", "))
	}
	if r.Archived != "" {
		fmt.Fprintf(w, "Archive: %s\n", r.Archived)
	}
}

// printHistoricalizationResult renders the outcome of a historicalization.
func printHistoricalizationResult(w io.Writer, res planflow.HistoricalizeResult) {
	printHistoricalizationReadiness(w, res.Readiness)
	switch res.Outcome {
	case planflow.OutcomeAlreadyHistoricalized:
		fmt.Fprintln(w, "Already historicalized: no change.")
	default:
		if res.Archived != "" {
			fmt.Fprintf(w, "Historicalized (%s): %s\n", res.Readiness.Disposition, res.Archived)
		}
		fmt.Fprintf(w, "Preserved %d task record(s) as history.\n", res.Tasks)
		fmt.Fprintln(w, "State: HISTORICALIZED (removed from ACTIVE selection).")
	}
}

// formatHistoricalizationCounts renders task counts by state in a stable order.
func formatHistoricalizationCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

func historicalizationYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// writeHistoricalizationJSON renders a readiness evaluation as a JSON document.
func writeHistoricalizationJSON(w io.Writer, r planflow.HistoricalizationReadiness) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(historicalizationDocFrom(r))
}

// writeHistoricalizationResultJSON renders a historicalization outcome as JSON.
func writeHistoricalizationResultJSON(w io.Writer, res planflow.HistoricalizeResult) {
	doc := struct {
		planflow.HistoricalizationReadiness
		Outcome string `json:"outcome"`
		Tasks   int    `json:"tasks"`
	}{historicalizationDocFrom(res.Readiness), res.Outcome, res.Tasks}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(doc)
}

// historicalizationDocFrom normalizes a readiness value so empty categories render
// as [] rather than null.
func historicalizationDocFrom(r planflow.HistoricalizationReadiness) planflow.HistoricalizationReadiness {
	r.UnresolvedTasks = idsOrEmpty(r.UnresolvedTasks)
	r.UnresolvedApprovals = idsOrEmpty(r.UnresolvedApprovals)
	r.UnresolvedVerification = idsOrEmpty(r.UnresolvedVerification)
	if r.TaskCounts == nil {
		r.TaskCounts = map[string]int{}
	}
	return r
}

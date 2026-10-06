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
		}
	}
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: sop plan [TASK.md] | sop plan activate <PLAN.md> | sop plan supersede <PLAN.md>")
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

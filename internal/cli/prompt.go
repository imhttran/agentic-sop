package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/workitem"
)

// sop prompt: ad-hoc, governed work submitted directly by an operator (Phase 5.4).
//
// A prompt is a second input form for SOP's existing execution machinery, not a
// second workflow engine. It becomes a workitem.WorkItem, passes through the SAME
// capability check, JEV evidence seam, deterministic SMALL/MEDIUM/LARGE router,
// model resolution, and opt-in provider validation a task does, and then runs on one
// of two paths:
//
//	read-only capability (PLAN, DESIGN_TESTS, DIAGNOSE_FAILURE, REVIEW)
//	    -> one bounded Generate call; the model only produces text
//	mutating capability (IMPLEMENT)
//	    -> the governed implementation lifecycle (plan, implement, validate,
//	       review, gate, fix, approval), exactly as a task runs
//
// SOP remains the sole authority. Prompt text is untrusted data: it is passed to the
// selected agent, never interpreted as a shell command, a lifecycle transition, a
// model class, or configuration. Routing is deterministic and, as everywhere, never
// chosen by the model; no keyword in the prompt can select a class.
//
// The prompt path is OFF from a policy standpoint in the same way routing is: with
// routing disabled and no --model-class, the configured/default execution stack is
// used, and no automatic escalation is performed for prompts (Phase 5.4 37).

// promptUsage is the one-line usage for `sop prompt`.
const promptUsage = "usage: sop prompt [--capability CAP] [--model-class small|medium|large] [--json] [--file PATH | PROMPT]"

// defaultPromptCapability is the conservative, read-only default: a bare prompt is
// never treated as an implementation request (Phase 5.4 8).
const defaultPromptCapability = "plan"

// promptCapabilities maps the accepted CLI capability names to the canonical
// agent.Capability values. The set is deliberately the operator-facing five:
// IMPLEMENT is the only mutating one, and FIX (an internal lifecycle capability)
// is not offered as a prompt capability.
var promptCapabilities = map[string]agent.Capability{
	"plan":             agent.Plan,
	"design_tests":     agent.DesignTests,
	"diagnose_failure": agent.DiagnoseFailure,
	"review":           agent.Review,
	"implement":        agent.Implement,
}

// promptCapabilityList renders the accepted capability names for messages.
func promptCapabilityList() string {
	return "plan, design_tests, diagnose_failure, review, implement"
}

// parsePromptCapability validates and normalizes a capability name.
func parsePromptCapability(name string) (agent.Capability, error) {
	c, ok := promptCapabilities[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("unknown capability %q: want %s", name, promptCapabilityList())
	}
	return c, nil
}

// promptOptions are the parsed arguments of `sop prompt`.
type promptOptions struct {
	capability string
	file       string
	prompt     string
	modelClass string
	json       bool
}

// parsePromptArgs parses the prompt command's flags and positional prompt. At most
// one of --file and a positional prompt may be given.
func parsePromptArgs(args []string, stderr io.Writer) (promptOptions, bool) {
	opts := promptOptions{capability: defaultPromptCapability}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--capability":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, promptUsage)
				return promptOptions{}, false
			}
			opts.capability = args[i+1]
			i++
		case strings.HasPrefix(a, "--capability="):
			opts.capability = strings.TrimPrefix(a, "--capability=")
		case a == "--model-class":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, promptUsage)
				return promptOptions{}, false
			}
			opts.modelClass = args[i+1]
			i++
		case strings.HasPrefix(a, "--model-class="):
			opts.modelClass = strings.TrimPrefix(a, "--model-class=")
		case a == "--file":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, promptUsage)
				return promptOptions{}, false
			}
			opts.file = args[i+1]
			i++
		case strings.HasPrefix(a, "--file="):
			opts.file = strings.TrimPrefix(a, "--file=")
		case a == "--json":
			opts.json = true
		case strings.HasPrefix(a, "-") && a != "-":
			fmt.Fprintf(stderr, "unknown flag %s\n%s\n", a, promptUsage)
			return promptOptions{}, false
		default:
			if opts.prompt != "" {
				fmt.Fprintln(stderr, promptUsage)
				return promptOptions{}, false
			}
			opts.prompt = a
		}
	}
	if opts.file != "" && opts.prompt != "" {
		fmt.Fprintf(stderr, "use either a positional prompt or --file, not both\n%s\n", promptUsage)
		return promptOptions{}, false
	}
	if opts.file == "" && strings.TrimSpace(opts.prompt) == "" {
		fmt.Fprintln(stderr, promptUsage)
		return promptOptions{}, false
	}
	return opts, true
}

// runPrompt implements `sop prompt`: it turns direct prompt text or a prompt file
// into a governed WorkItem and executes it through SOP's existing machinery.
func runPrompt(args []string, stdout, stderr io.Writer, d deps) int {
	opts, ok := parsePromptArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	capability, err := parsePromptCapability(opts.capability)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	text, err := promptText(dir, opts)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}

	wi, err := workitem.FromPrompt(promptRunID(dir), capability, text)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitUsage
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}
	d.modelClass = opts.modelClass
	routing, err := applyModelRouting(&cfg, d.modelClass)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}
	d.routing = routing
	if err := applyRoutingEnabled(&d, cfg); err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}
	// Bounded execution recovery (Phase 5) applies to a prompt exactly as it does to a
	// task. The policy is OFF unless the operator opts in (models.escalation_enabled),
	// and it can only take effect on an `implement` prompt, which runs the governed
	// implementation lifecycle (runAttempts). A read-only prompt is a single bounded
	// call with no quality gate, so escalation never applies to it.
	if err := applyEscalationEnabled(&d, cfg); err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}

	// Prompt progress goes to stderr in machine mode so stdout carries only the
	// structured result document.
	progress := stdout
	if opts.json {
		progress = stderr
	}

	if agent.IsRepositoryMutation(wi.Capability) {
		return runPromptImplement(dir, cfg, d, wi, opts, progress, stdout, stderr)
	}
	return runPromptReadOnly(dir, cfg, d, wi, opts, progress, stdout, stderr)
}

// promptText returns the prompt content from a file or the positional argument. A
// missing or empty file is a clear error, and a file path MUST resolve inside the
// project directory: prompt input is untrusted, so it MUST NOT become an arbitrary
// local-file read.
func promptText(dir string, opts promptOptions) (string, error) {
	if opts.file == "" {
		return opts.prompt, nil
	}
	path, err := resolvePromptFile(dir, opts.file)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read prompt file: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("prompt file %s is empty", opts.file)
	}
	return string(data), nil
}

// resolvePromptFile resolves a prompt file path and confines it to the project
// directory. An absolute path inside the project is allowed; anything outside is
// rejected, so skill-supplied input cannot read an arbitrary file.
//
// The confinement is enforced against the symlink-RESOLVED paths, not only
// lexically: a project-local symlink (for example `prompts/input.md` -> a file in
// /tmp) passes a lexical check but reads outside the project, so both the project
// root and the target are resolved with filepath.EvalSymlinks before the
// containment test. The project root is resolved too, so a project reached through
// a symlink (for example /tmp -> /private/tmp on macOS) does not make a legitimate
// project-local file look like it escapes.
func resolvePromptFile(dir, file string) (string, error) {
	path := file
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	realDir := absDir
	if r, rerr := filepath.EvalSymlinks(absDir); rerr == nil {
		realDir = r
	}
	// A missing file is reported by the caller's read, so a resolution failure here
	// (a non-existent path cannot be resolved) is not itself a confinement failure.
	if r, rerr := filepath.EvalSymlinks(absPath); rerr == nil {
		absPath = r
	}
	rel, err := filepath.Rel(realDir, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prompt file %s is outside the project directory", file)
	}
	return absPath, nil
}

// promptRunID builds a run identity for a prompt. It is used only for run/report
// organization: it never influences routing, policy, or capability. It is unique
// within the run root, so two prompts in the same second never share a directory.
func promptRunID(dir string) string {
	base := "prompt-" + time.Now().UTC().Format("20060102-150405")
	id := base
	for i := 2; runDirExists(dir, id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
	}
	return id
}

// runDirExists reports whether a run directory already exists for id.
func runDirExists(dir, id string) bool {
	_, err := os.Stat(filepath.Join(dir, stateDirName, "runs", promptsDirName, id))
	return err == nil
}

// runPromptReadOnly executes a read-only prompt: one bounded agent call, with the
// routed selection validated (when opted in) and the capability guarded first. It
// never mutates the repository and never runs the implementation lifecycle.
func runPromptReadOnly(dir string, cfg config.Config, d deps, wi workitem.WorkItem, opts promptOptions, progress, stdout, stderr io.Writer) int {
	rid := promptRunDir(wi.ID)
	rn, err := runpkg.New(dir, rid)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}
	_ = rn.Write("prompt.md", renderPromptDoc(wi, nil))

	ctx := context.Background()

	// Optional typed JEV triage (Phase 5.4 12). Reused unchanged: JEV provides
	// evidence only and never selects the class.
	tri := runPromptJEV(ctx, cfg, d, wi, rn)
	// Early JEV triage owns a human boundary exactly as it does for a task: a policy
	// escalation stops the prompt at that boundary rather than running it (for a
	// mutating prompt, before any repository change).
	if tri.Escalate {
		return stopPromptForTriage(rn, wi, tri, opts, progress, stdout, stderr)
	}

	tr, _, err := promptRouting(cfg, d, tri)
	if err != nil {
		return failPrompt(rn, stderr, err)
	}
	sel := promptSelection(cfg, tr)
	if err := validateSelection(ctx, cfg, sel); err != nil {
		return failPrompt(rn, stderr, fmt.Errorf("prompt selection: %w", err))
	}
	if d.newAgent == nil {
		return failPrompt(rn, stderr, errors.New("prompt: no agent factory is available"))
	}
	a, err := d.newAgent(cfg.Agent.Harness, sel.Provider, sel.Model)
	if err != nil {
		return failPrompt(rn, stderr, fmt.Errorf("prompt: build agent: %w", err))
	}
	// Capability enforcement: an unsupported capability fails clearly instead of
	// being reinterpreted or silently forwarded to a provider that cannot serve it.
	if err := guardCapability(a, wi.Capability); err != nil {
		return failPrompt(rn, stderr, err)
	}

	renderPromptRouting(progress, wi, tr, sel)

	resp, err := a.Generate(ctx, agent.Request{
		Capability:         wi.Capability,
		Task:               wi.Content,
		OutputRequirements: promptOutputRequirements(wi.Capability),
	})
	if err != nil {
		if opts.json {
			_ = writePromptJSON(stdout, stderr, promptResultDoc{
				Version:    promptResultVersion,
				WorkItemID: wi.ID,
				Kind:       string(wi.Kind),
				Capability: promptCapabilityName(wi.Capability),
				Status:     "failed",
				Routing:    promptRoutingDoc(tr, sel),
				ReportPath: promptRunDirRel(wi.ID),
			})
		}
		_ = writePromptMetadata(rn, promptResultDoc{
			Version:    promptResultVersion,
			WorkItemID: wi.ID,
			Kind:       string(wi.Kind),
			Capability: promptCapabilityName(wi.Capability),
			Status:     "failed",
			Routing:    promptRoutingDoc(tr, sel),
			ReportPath: promptRunDirRel(wi.ID),
		})
		return failPrompt(rn, stderr, fmt.Errorf("prompt: %s: %w", wi.Capability, err))
	}

	_ = rn.Write("result.md", resp.Content)
	writePromptRoutingArtifact(rn, wi, tr, sel)

	doc := promptResultDoc{
		Version:    promptResultVersion,
		WorkItemID: wi.ID,
		Kind:       string(wi.Kind),
		Capability: promptCapabilityName(wi.Capability),
		Status:     "completed",
		Routing:    promptRoutingDoc(tr, sel),
		ReportPath: promptRunDirRel(wi.ID),
		ResultPath: filepath.Join(promptRunDirRel(wi.ID), "result.md"),
		Result:     resp.Content,
	}
	_ = writePromptMetadata(rn, doc)

	if opts.json {
		return writePromptJSON(stdout, stderr, doc)
	}
	fmt.Fprint(stdout, renderPromptResult(wi, tr, sel, resp.Content))
	return exitOK
}

// runPromptImplement executes a mutating prompt (IMPLEMENT) through SOP's existing
// governed implementation lifecycle: the same planning, implementation, validation,
// review, gate, fix, and approval path a task runs. It never shortcuts to a bare
// Generate call, so an implementation prompt keeps SOP's safety boundaries.
func runPromptImplement(dir string, cfg config.Config, d deps, wi workitem.WorkItem, opts promptOptions, progress, stdout, stderr io.Writer) int {
	// The prompt is projected into a task file for the existing lifecycle. This is
	// the same adapter boundary workitem.FromTask documents; no new lifecycle is
	// introduced.
	spec := &taskfile.Spec{ID: wi.ID, Title: wi.Title, Description: wi.Content}

	rid := promptRunDir(wi.ID)
	priorStage, _ := runpkg.Load(dir, rid)
	rn, err := runpkg.New(dir, rid)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return exitError
	}
	_ = rn.Write("prompt.md", renderPromptDoc(wi, spec))
	_ = rn.Write("task.md", spec.Render())

	if d.newAgent == nil {
		return failPrompt(rn, stderr, errors.New("prompt: no agent factory is available"))
	}
	a, err := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		return failPrompt(rn, stderr, err)
	}
	// Capability enforcement and validation apply to the agent that will actually
	// execute. With the automatic per-task router ON, the routed class builds,
	// validates, and guards its OWN agent at the final-selection seam
	// (applyTaskRouting), so the default agent MUST NOT be rejected here: it may be a
	// text-only provider while routing selects a tool-capable one, and rejecting it
	// before routing would refuse an implementation the routed class could run.
	// With the router off, the default agent is the executing agent and is guarded
	// and validated here. Either way the executing agent is guarded exactly once.
	if !d.routingEnabled {
		if err := guardCapability(a, agent.Implement); err != nil {
			return failPrompt(rn, stderr, err)
		}
		if err := validateSelectedModel(context.Background(), cfg, d.routing); err != nil {
			return failPrompt(rn, stderr, fmt.Errorf("prompt: %w", err))
		}
	}

	ctx := taskActivityContext(context.Background(), rn.Dir(), progress, rn.State().ID, wi.Title)
	tri := runPromptJEV(ctx, cfg, d, wi, rn)
	// Early JEV triage owns a human boundary exactly as it does for a task, and
	// BEFORE any implementation work, so a policy escalation cannot mutate the
	// repository (Phase 5.4 6, 8).
	if tri.Escalate {
		return stopPromptForTriage(rn, wi, tri, opts, progress, stdout, stderr)
	}

	res, err := runAttempts(ctx, dir, cfg, a, d, spec, rn, newRunSession(), currentApprovalBoundary(rn, priorStage), tri, progress)
	emitClassificationActivity(ctx, res.classification, res.decision)
	if err != nil {
		return failRun(rn, stderr, err)
	}
	code := emitRunSummary(progress, dir, cfg, rn, res)
	// A governed `implement` prompt that stops at a human boundary names it too, but
	// the prompt is not a stored task, so there is no resolvable approval gate: it
	// prints the boundary and how to continue instead of a command that would fail.
	if humanBoundary(res.stage, res.classification, res.decision) {
		printParkedHumanGate(progress, rn.State().ID, string(res.stage),
			firstNonBlank(firstReason(res.gate), res.classification.Reason),
			"sop prompt --capability implement", false)
	}

	sel := model.Selection{Provider: cfg.Agent.Provider, Model: cfg.Agent.Model}
	if res.routing != nil {
		sel = res.routing.Selection
	}
	doc := promptResultDoc{
		Version:    promptResultVersion,
		WorkItemID: wi.ID,
		Kind:       string(wi.Kind),
		Capability: promptCapabilityName(wi.Capability),
		Status:     promptStatus(res.gate.Decision),
		Routing:    promptRoutingDoc(res.routing, sel),
		ReportPath: promptRunDirRel(wi.ID),
	}
	_ = writePromptMetadata(rn, doc)

	if opts.json {
		if jc := writePromptJSON(stdout, stderr, doc); jc != exitOK {
			return jc
		}
	}
	return code
}

// promptsDirName is the subdirectory of the runs root holding prompt runs, so they
// are distinguishable from task runs. It is also the id prefix latestRun uses when
// a report is addressed by id.
const promptsDirName = "prompts"

// promptRunDir is the run directory id for a prompt run: prompts are grouped under
// runs/prompts/ so they are distinguishable from task runs. It uses the existing
// run storage layout rather than a new one.
func promptRunDir(id string) string { return promptsDirName + "/" + id }

// promptRunDirRel is the project-relative path of a prompt run directory, for
// metadata and CLI output.
func promptRunDirRel(id string) string {
	return filepath.Join(stateDirName, "runs", promptsDirName, id)
}

// runPromptJEV runs the optional task-triage JEV checkpoint over the prompt's
// bounded context. It is a strict no-op unless the triage gate is enabled and an
// analyzer is wired. JEV provides typed evidence only.
func runPromptJEV(ctx context.Context, cfg config.Config, d deps, wi workitem.WorkItem, rn *runpkg.Run) earlyGateResult {
	inv := runpkg.JEVInvocation{Purpose: jev.PurposeTaskTriage, Task: wi.Content}
	return runJEVCheckpoint(ctx, cfg, d, runpkg.CheckpointTaskTriage, inv, wi.ID, rn)
}

// promptRouting computes the prompt's model-class decision, mirroring the task
// router's seam. It returns ok=false when no routing applies (routing disabled and
// no manual override), in which case the configured execution stack is used.
//
// It is deterministic and never reads the prompt text: only typed JEV evidence
// participates, and without evidence the router falls back to the safe default
// (MEDIUM) rather than guessing a class from prose.
func promptRouting(cfg config.Config, d deps, tri earlyGateResult) (*taskRouting, bool, error) {
	if strings.TrimSpace(d.modelClass) != "" {
		if d.routing.Active {
			res, applied := applyLocalFallback(cfg, d, d.routing)
			return &taskRouting{
				Class:           res.Selection.Class,
				Source:          runpkg.RoutingSourceManual,
				Reasons:         routingReasons([]string{model.RoutingReasonManual}, res.Selection),
				Selection:       res.Selection,
				ExecutionTarget: executionTargetFor(res, applied),
			}, true, nil
		}
		return nil, false, nil
	}
	if !d.routingEnabled {
		return nil, false, nil
	}

	sig := router.SignalsFrom(nil, router.TaskSignals{})
	var checkpoints []string
	if ev := evidenceFrom(tri); ev != nil {
		sig = sig.Merge(router.SignalsFrom(ev, router.TaskSignals{}))
		checkpoints = append(checkpoints, "task_triage")
	}
	dec := router.Decide(sig)
	res, err := model.Resolve(model.Inputs{
		Config:       cfg.Models,
		Lookup:       os.Getenv,
		RoutedClass:  dec.Class,
		RoutedReason: router.ReasonsText(dec.Reasons),
	})
	if err != nil {
		return nil, false, err
	}
	if !res.Active {
		return nil, false, nil
	}
	res, applied := applyLocalFallback(cfg, d, res)
	return &taskRouting{
		Class:           dec.Class,
		Source:          runpkg.RoutingSourcePolicy,
		Reasons:         routingReasons(dec.Reasons, res.Selection),
		Selection:       res.Selection,
		ExecutionTarget: executionTargetFor(res, applied),
		Signals:         sig,
		Checkpoints:     checkpoints,
	}, true, nil
}

// promptSelection returns the selection the prompt will execute on: the routing
// decision's selection when routing applied, otherwise the configured execution
// stack's provider and model.
func promptSelection(cfg config.Config, tr *taskRouting) model.Selection {
	if tr != nil {
		return tr.Selection
	}
	stack := resolveExecutionStack(cfg)
	return model.Selection{Provider: stack.Provider, Model: stack.Model}
}

// promptOutputRequirements is the capability-appropriate output instruction passed
// to the agent. It is a shape hint, never a policy.
func promptOutputRequirements(c agent.Capability) string {
	switch c {
	case agent.Plan:
		return "Return a concrete, structured plan. Do not modify the repository."
	case agent.Review:
		return "Return concise review findings with severities. Do not modify the repository."
	case agent.DiagnoseFailure:
		return "Explain the likely cause and the next diagnostic steps. Do not modify the repository."
	case agent.DesignTests:
		return "Return a test plan with concrete cases. Do not modify the repository."
	default:
		return "Return a concise, structured answer."
	}
}

// promptCapabilityName renders a capability for the prompt result document. The
// value matches the CLI's input vocabulary.
func promptCapabilityName(c agent.Capability) string { return strings.ToLower(string(c)) }

// promptStatus maps the lifecycle gate decision to a prompt result status.
func promptStatus(d quality.Decision) string {
	switch d {
	case quality.Pass:
		return "completed"
	case quality.NeedsHuman:
		return "needs_human"
	case quality.Continue:
		return "incomplete"
	case quality.Fail:
		return "failed"
	default:
		return "failed"
	}
}

// renderPromptDoc renders the prompt.md artifact: the operator's prompt plus its
// kind and explicit capability. It records the request, not a decision.
func renderPromptDoc(wi workitem.WorkItem, spec *taskfile.Spec) string {
	var b strings.Builder
	b.WriteString("# Prompt\n\n")
	fmt.Fprintf(&b, "- Work item: %s\n", wi.ID)
	fmt.Fprintf(&b, "- Kind: %s\n", wi.Kind)
	fmt.Fprintf(&b, "- Capability: %s\n", wi.Capability)
	b.WriteString("\n## Content\n\n")
	b.WriteString(wi.Content)
	b.WriteString("\n")
	if spec != nil {
		b.WriteString("\n## Task projection\n\n")
		b.WriteString(spec.Render())
	}
	return b.String()
}

// renderPromptRouting prints the concise routing block for a prompt: the class, the
// resolved provider/model, and the deterministic reason. It prints nothing when no
// routing applied, so an unrouted prompt is silent (matching the task path).
func renderPromptRouting(w io.Writer, wi workitem.WorkItem, tr *taskRouting, sel model.Selection) {
	fmt.Fprintf(w, "Prompt: %s\n", wi.Title)
	fmt.Fprintf(w, "  %-11s %s\n", "Capability:", wi.Capability)
	if tr == nil {
		fmt.Fprintf(w, "  %-11s %s\n", "Provider:", sel.Provider)
		if sel.Model != "" {
			fmt.Fprintf(w, "  %-11s %s\n", "Model:", sel.Model)
		}
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Model routing:")
	fmt.Fprintf(w, "  %-17s %s\n", "Class:", tr.Class)
	fmt.Fprintf(w, "  %-17s %s\n", "Provider:", sel.Provider)
	if sel.Model != "" {
		fmt.Fprintf(w, "  %-17s %s\n", "Model:", sel.Model)
	}
	fmt.Fprintf(w, "  %-17s %s\n", "Execution source:", executionSourceFor(tr))
	if len(tr.Reasons) > 0 {
		fmt.Fprintf(w, "  %-17s %s\n", "Reason:", router.ReasonsText(tr.Reasons))
	}
	fmt.Fprintln(w)
}

// renderPromptResult renders the human-readable read-only result block.
func renderPromptResult(wi workitem.WorkItem, tr *taskRouting, sel model.Selection, result string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prompt:\n  Capability: %s\n", wi.Capability)
	if tr != nil {
		fmt.Fprintf(&b, "  Class: %s\n", tr.Class)
	} else {
		b.WriteString("  Class: (unrouted)\n")
	}
	fmt.Fprintf(&b, "  Provider: %s\n", sel.Provider)
	if sel.Model != "" {
		fmt.Fprintf(&b, "  Model: %s\n", sel.Model)
	}
	b.WriteString("\nResult:\n\n")
	b.WriteString(result)
	if !strings.HasSuffix(result, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// promptResultVersion is the schema version of the prompt result document.
const promptResultVersion = 1

// promptResultDoc is the machine-readable result of a prompt run. It is the same
// document persisted as metadata.json and printed by --json, so the skill and a
// human read one contract. It carries no secret and no hidden reasoning.
type promptResultDoc struct {
	Version    int                   `json:"version"`
	WorkItemID string                `json:"work_item_id"`
	Kind       string                `json:"kind"`
	Capability string                `json:"capability"`
	Status     string                `json:"status"`
	Routing    *promptRoutingSection `json:"routing,omitempty"`
	ReportPath string                `json:"report_path,omitempty"`
	ResultPath string                `json:"result_path,omitempty"`
	Result     string                `json:"result,omitempty"`
}

// promptRoutingSection is the prompt's routing evidence in the result document. It
// reuses the same fields the task routing artifact records, including the typed
// execution source so the routing CLASS is distinguishable from the actual
// EXECUTION target.
type promptRoutingSection struct {
	Class           string   `json:"class,omitempty"`
	Provider        string   `json:"provider,omitempty"`
	Model           string   `json:"model,omitempty"`
	Source          string   `json:"source,omitempty"`
	ExecutionSource string   `json:"execution_source,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
}

// promptRoutingDoc projects a routing decision and selection into the result
// document's routing section. It renders the selection even when routing did not
// apply, so the effective provider/model are always visible.
func promptRoutingDoc(tr *taskRouting, sel model.Selection) *promptRoutingSection {
	out := &promptRoutingSection{Provider: sel.Provider, Model: sel.Model}
	if tr != nil {
		out.Class = string(tr.Class)
		out.Source = string(tr.Source)
		out.ExecutionSource = executionSourceFor(tr)
		out.Reasons = tr.Reasons
	}
	return out
}

// writePromptRoutingArtifact persists the prompt's routing decision using the
// existing routing artifact contract (no prompt-specific schema). It is skipped when
// no routing applied, matching the task path.
func writePromptRoutingArtifact(rn *runpkg.Run, wi workitem.WorkItem, tr *taskRouting, sel model.Selection) {
	if tr == nil {
		return
	}
	art := runpkg.RoutingArtifact{
		Version:         runpkg.RoutingArtifactVersion,
		Task:            wi.ID,
		Class:           string(tr.Class),
		Source:          tr.Source,
		Reasons:         tr.Reasons,
		Provider:        sel.Provider,
		Model:           sel.Model,
		Locality:        string(sel.Locality),
		ExecutionSource: executionSourceFor(tr),
		Checkpoints:     tr.Checkpoints,
		Signals:         routingSignalsDoc(tr.Signals),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}
	if err := rn.WriteRoutingArtifact(art); err != nil {
		// Diagnostic only: a contract violation never changes the run.
		_ = err
	}
}

// writePromptMetadata persists the prompt result document as metadata.json.
func writePromptMetadata(rn *runpkg.Run, doc promptResultDoc) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return rn.Write("metadata.json", string(append(data, '\n')))
}

// writePromptJSON prints the structured result document.
func writePromptJSON(stdout, stderr io.Writer, doc promptResultDoc) int {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "prompt: encode result: %v\n", err)
		return exitError
	}
	fmt.Fprintln(stdout, string(data))
	return exitOK
}

// stopPromptForTriage stops a prompt at the early-JEV human boundary, mirroring the
// task path (`sop run`): it records a needs_human status and returns a non-zero exit
// without performing any agent work. It never bypasses the boundary.
func stopPromptForTriage(rn *runpkg.Run, wi workitem.WorkItem, tri earlyGateResult, opts promptOptions, progress, stdout, stderr io.Writer) int {
	doc := promptResultDoc{
		Version:    promptResultVersion,
		WorkItemID: wi.ID,
		Kind:       string(wi.Kind),
		Capability: promptCapabilityName(wi.Capability),
		Status:     "needs_human",
		ReportPath: promptRunDirRel(wi.ID),
	}
	_ = writePromptMetadata(rn, doc)
	fmt.Fprintf(progress, "%s NEEDS_HUMAN (early JEV triage)\n  %s\n", rn.State().ID, tri.Reason)
	if opts.json {
		// A machine consumer still receives the document, but the boundary is exit
		// non-zero: needs_human is not success.
		_ = writePromptJSON(stdout, stderr, doc)
	}
	return exitError
}

// failPrompt records the failed run stage and reports the error through stderr.
func failPrompt(rn *runpkg.Run, stderr io.Writer, err error) int {
	if rn != nil {
		_ = rn.SetStage(runpkg.Failed)
	}
	fmt.Fprintf(stderr, "prompt: %v\n", err)
	return exitError
}

// writePromptReport renders a read-only prompt's result document in `sop report`,
// reusing the same metadata.json a prompt run writes. It returns false when the run
// has no prompt metadata (a task or implement-prompt run), so the caller falls back
// to the standard report.
func writePromptReport(w io.Writer, runDir string) bool {
	data, err := os.ReadFile(filepath.Join(runDir, "metadata.json"))
	if err != nil {
		return false
	}
	var doc promptResultDoc
	if err := json.Unmarshal(data, &doc); err != nil || doc.Kind != string(workitem.KindPrompt) {
		return false
	}
	fmt.Fprintf(w, "Run: %s\n", doc.WorkItemID)
	fmt.Fprintf(w, "Kind: %s\n", doc.Kind)
	fmt.Fprintf(w, "Capability: %s\n", doc.Capability)
	fmt.Fprintf(w, "Status: %s\n", doc.Status)
	if r := doc.Routing; r != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Model routing:")
		if r.Class != "" {
			fmt.Fprintf(w, "  %-17s %s\n", "Class:", r.Class)
		}
		fmt.Fprintf(w, "  %-17s %s\n", "Provider:", r.Provider)
		if r.Model != "" {
			fmt.Fprintf(w, "  %-17s %s\n", "Model:", r.Model)
		}
		if r.Source != "" {
			fmt.Fprintf(w, "  %-17s %s\n", "Source:", r.Source)
		}
		if r.ExecutionSource != "" {
			fmt.Fprintf(w, "  %-17s %s\n", "Execution source:", r.ExecutionSource)
		}
		if len(r.Reasons) > 0 {
			fmt.Fprintf(w, "  %-17s %s\n", "Reason:", router.ReasonsText(r.Reasons))
		}
	}
	if doc.ResultPath != "" {
		fmt.Fprintf(w, "\nResult: %s\n", doc.ResultPath)
	}
	return true
}

// Package cli implements the sop command-line interface. It parses
// arguments and drives the domain and store layers; workflow/orchestration
// logic belongs to later tasks.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/recovery"
	"github.com/imhttran/agentic-sop/internal/resume"
)

// Version is the CLI version reported by the version command.
const Version = "dev"

const (
	// stateDirName is the per-project directory holding sop state. Its value
	// comes from the config package, the single owner of the on-disk layout.
	stateDirName = config.DirName
	// stateFileName is the SQLite database file name inside stateDirName.
	stateFileName = "state.db"
)

// Exit codes returned by Run.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// deps holds the external boundaries the CLI depends on so commands can be
// driven with fakes in tests: the working-directory resolver, the agent
// constructor (parameterised by the configured harness, provider, and model),
// the resume observer, and the working-tree diff reader.
type deps struct {
	getwd        func() (string, error)
	newAgent     func(harness, provider, model string) (agent.Agent, error)
	newResources func(dir string) (resume.Observer, error)
	readDiff     func(ctx context.Context, dir string) (string, error)
	// snapshotRepository observes file content around IMPLEMENT/FIX for providers
	// which do not return invocation-scoped mutation paths. It is read-only.
	snapshotRepository func(context.Context, string) (map[string]string, error)
	commit             func(ctx context.Context, dir, message string) error
	newGitHub          func(dir string) github.Client
	// newJEVAnalyzer builds the optional JEV analyzer from configuration. It is
	// consulted only when JEV is enabled; a nil factory (or a nil analyzer, or
	// an error) leaves JEV absent, which is never fatal to the lifecycle.
	newJEVAnalyzer func(cfg config.Config) (jev.Analyzer, error)
	// modelClass is the --model-class override for this invocation, if any. It is
	// the highest-precedence input to the optional model-routing layer and is
	// empty for every command that does not accept the flag.
	modelClass string
	// taskInputs is optional, caller-supplied observation text keyed by task ID.
	// It supplies context only; it cannot grant tools, mutation or completion.
	taskInputs map[string]string
	// routing is the resolved, non-secret model-routing evidence for this
	// invocation. The run commands set it after resolution; each task run records
	// it in its artifacts so the model choice stays auditable after the process
	// exits. It is zero when routing is inactive (no models: block, SOP_MODEL_*
	// variable, or --model-class) and carries no credential.
	routing model.Result
	// routingEnabled is the automatic model-class router feature flag for this
	// invocation (SOP_MODEL_ROUTING_ENABLED / models.routing_enabled). It is OFF by
	// default; the run commands set it after resolution, and only they consult it.
	routingEnabled bool
	// escalation is the bounded execution-recovery policy for this invocation
	// (Phase 5): whether SOP may escalate a failed attempt to the next larger model
	// class, and how many escalations it may take. It is OFF by default, so an
	// existing installation's execution behavior is unchanged.
	escalation recovery.Policy
	// attempt is the escalation provenance for the lifecycle attempt currently
	// running: nil for a first attempt (the class comes from routing), and set only
	// for an attempt the recovery policy selected. It is per-attempt, so the run
	// loop resets it before each lifecycle call.
	attempt *escalationAttempt
	// stdin is the interactive input the approval commands read when they must
	// choose among gates (Phase 6). It is os.Stdin in production and a buffer in
	// tests; prompt text read from it is a DECISION, never a command.
	stdin io.Reader
	// interactive reports whether the approval commands may ask a human at all. It
	// is true only for a real terminal on stdin, and it is injectable so a test can
	// exercise the interactive path without a pseudo-terminal. A nil value falls
	// back to the real terminal check, so an unwired process fails closed.
	interactive func(io.Reader) bool
	// localProbe reports whether a local class's resolved model cannot be served
	// by its runtime, with a short non-secret detail. It is the read-only
	// availability observation behind the local-first cloud fallback. A nil value
	// means "no observation", which model resolution never treats as
	// unavailability, so an unwired process keeps the local model.
	localProbe func(cfg config.Config, sel model.Selection) (unavailable bool, detail string)
}

func defaultDeps() deps {
	return deps{
		getwd: os.Getwd,
		newAgent: func(harness, provider, model string) (agent.Agent, error) {
			// This is the composition root: it resolves the effective
			// harness/provider pair the same way the startup summary does, then
			// builds the matching agent. Environment keeps the highest precedence.
			h, _ := agent.EffectiveHarness(harness)
			p, _ := agent.EffectiveProvider(provider)
			if h == agent.HarnessTool && p == agent.ProviderOllama {
				// The native path: the Ollama tool loop runs in-process against
				// the working tree, with all capabilities (including IMPLEMENT and
				// FIX). SOP remains the workflow authority; the loop owns only
				// tool-bounded execution.
				a, err := ollamaagent.NativeAgentFromEnv(model)
				if err != nil {
					return nil, err
				}
				return agent.NewChecked(a), nil
			}
			if p == agent.ProviderOpenAICompatible {
				// The generic OpenAI-compatible provider owns its endpoint with the
				// documented precedence environment > configuration > default. The
				// agent layer resolves the environment and default tiers; the
				// composition root adds the project-configuration tier, which the
				// factory cannot otherwise see (the agent layer cannot import
				// internal/config). It never substitutes another provider.
				dir, _ := os.Getwd()
				a, err := agent.NewOpenAICompatibleExecution(openAICompatibleExecutionEndpoint(dir), model)
				if err != nil {
					return nil, err
				}
				return agent.NewChecked(a), nil
			}
			a, err := agent.FromConfig(provider, model)
			if err != nil {
				return nil, err
			}
			return agent.NewChecked(a), nil
		},
		newResources: func(dir string) (resume.Observer, error) {
			return &gitHubResources{branches: git.New(dir), prs: github.NewCommandClient(dir)}, nil
		},
		readDiff: func(ctx context.Context, dir string) (string, error) {
			// Include untracked files (new files the agent created) so change
			// detection is not blind to them; exclude SOP's own output.
			return git.New(dir).DiffAll(ctx, config.DirName, planflow.ReportsDir)
		},
		snapshotRepository: snapshotRepository,
		commit: func(ctx context.Context, dir, message string) error {
			g := git.New(dir)
			if err := g.Add(ctx); err != nil {
				return err
			}
			return g.Commit(ctx, message)
		},
		newGitHub: func(dir string) github.Client { return github.NewCommandClient(dir) },
		newJEVAnalyzer: func(cfg config.Config) (jev.Analyzer, error) {
			// The default analyzer drives the existing Ollama provider path. An
			// absent/misconfigured provider leaves JEV absent, which the caller
			// treats as non-fatal (never failing the lifecycle).
			return jev.NewOllamaAnalyzerFromEnv()
		},
		stdin:       os.Stdin,
		interactive: isTerminalReader,
		localProbe:  localRuntimeProbe,
	}
}

// openAICompatibleExecutionEndpoint resolves the generic OpenAI-compatible
// provider's execution endpoint with the documented precedence: environment >
// configuration > default. The environment and default tiers come from the agent
// layer; the configuration tier is read from the project configuration at dir,
// which the agent factory receives no other way. It is used only by the
// production composition root, so a test that injects its own newAgent keeps its
// seam.
func openAICompatibleExecutionEndpoint(dir string) string {
	cfg, err := config.LoadDir(dir)
	if err != nil {
		// No configuration (or an unreadable one) leaves the environment > default
		// tiers; the caller has already validated a present configuration.
		return agent.OpenAICompatibleEndpointFromEnv()
	}
	return cfg.ResolveOpenAICompatibleEndpoint()
}

// Run parses args and executes a single command, writing normal output to
// stdout and diagnostics to stderr. It returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr, defaultDeps())
}

// run is Run with injectable boundaries.
func run(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		writeHelp(stdout)
		return exitOK
	}

	// Apply the optional project .env before any command resolves configuration or
	// the environment, so a .env-supplied value (for example model routing) is
	// visible everywhere. A missing .env is fine; a malformed one fails clearly.
	if d.getwd != nil {
		if dir, err := d.getwd(); err == nil {
			if err := loadDotEnv(dir); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return exitError
			}
		}
	}

	command, rest := args[0], args[1:]
	switch command {
	case "help", "-h", "--help":
		writeHelp(stdout)
		return exitOK
	case "version":
		return runVersion(rest, stdout, stderr)
	case "init":
		return runInit(rest, stdout, stderr, d.getwd)
	case "status":
		return runStatus(rest, stdout, stderr, d.getwd)
	case "task":
		return runTask(rest, stdout, stderr, d.getwd)
	case "plan":
		return runPlan(rest, stdout, stderr, d)
	case "tasks":
		return runTasks(rest, stdout, stderr, d)
	case "validate":
		return runValidate(rest, stdout, stderr, d.getwd)
	case "review":
		return runReview(rest, stdout, stderr, d)
	case "run":
		return runRun(rest, stdout, stderr, d)
	case "prompt":
		return runPrompt(rest, stdout, stderr, d)
	case "commit":
		return runCommit(rest, stdout, stderr, d)
	case "pr":
		return runPr(rest, stdout, stderr, d)
	case "mcp":
		return runMCP(rest, stdout, stderr, d)
	case "report":
		return runReport(rest, stdout, stderr, d.getwd)
	case "eval":
		return runEval(rest, stdout, stderr, d)
	case "resume":
		return runResume(rest, stdout, stderr, d)
	case "retry":
		return runRetry(rest, stdout, stderr, d.getwd)
	case "approve":
		return runApprove(rest, stdout, stderr, d)
	case "decline":
		return runDecline(rest, stdout, stderr, d)
	case "approval":
		return runApprovalStatus(rest, stdout, stderr, d)
	case "approvals":
		return runApprovals(rest, stdout, stderr, d)
	case "reconcile":
		return runReconcile(rest, stdout, stderr, d)
	case "index":
		return runIndex(rest, stdout, stderr, d)
	case "retrieve":
		return runRetrieve(rest, stdout, stderr, d)
	case "gate":
		return runGate(rest, stdout, stderr, d)
	case "providers":
		return runProviders(rest, stdout, stderr, d)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", command)
		fmt.Fprintln(stderr, "run `sop --help` for usage")
		return exitUsage
	}
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop version")
		return exitUsage
	}
	fmt.Fprintf(stdout, "sop %s\n", Version)
	return exitOK
}

func writeHelp(w io.Writer) {
	fmt.Fprint(w, `sop — a standard operating procedure for agentic software development

Usage:
  sop <command> [arguments]

Commands:
  init      initialize project state (.agent-sdlc/state.db)
  status    list persisted tasks
  task <id> show details; task complete <id> --external records an external completion
  plan      generate PLAN.md, or sop plan activate|supersede|complete PLAN.md
  tasks     build and persist tasks from .agent-sdlc/plan.json
  validate  run the configured build/test/lint commands
  review    review the current changes with the configured engine
  run       run [PLAN.md | --task TASK.md]  (normal entry point)
  prompt    run an ad-hoc prompt through SOP (--capability CAP, --file PATH, --json)
  commit    commit the current changes (needs --yes when the human gate is on)
  pr        push a task branch and open a pull request (needs --yes)
  mcp       serve tools over the Model Context Protocol (stdio)
  report    print a concise summary of the latest run
  retry     requeue a BLOCKED task so the next run retries it (--all for every task)
  approve   record an approval decision on a task's active human approval gate
            (no <task-id> or --select chooses interactively; --run continues after)
  decline   record a decline decision on a task's active human approval gate
            (no <task-id> or --select chooses interactively)
  approval  show SOP's approval request (if any) for a task
  approvals list every task waiting at an approval gate (--json)
  index     build the deterministic Structural Repository Index (.agent-sdlc/context/index.json)
  retrieve  rank repository evidence lexically (deterministic BM25)
  gate      evaluate Phase 8 evidence gates (sop gate retrieve|vector), model-free
  reconcile reconcile an intentional PLAN change (--accept-changed <id>, --list-changed)
  providers inspect configured provider runtimes and their models (--models)
  eval      run a corpus of task files and report benchmark metrics
  resume    report the next legal action for interrupted work
  version   print the CLI version
  help      show this help
`)
}

// projectDir resolves the working directory, which T003 treats as the project
// root. It reports failure through stderr.
func projectDir(getwd func() (string, error), stderr io.Writer) (string, bool) {
	dir, err := getwd()
	if err != nil {
		fmt.Fprintf(stderr, "error: cannot determine working directory: %v\n", err)
		return "", false
	}
	return dir, true
}

// statePath returns the SQLite state database path for a project directory.
// It is the single place that knows the on-disk state layout.
func statePath(projectDir string) string {
	return filepath.Join(projectDir, stateDirName, stateFileName)
}

// configuredAgent returns the agent harness, provider and model named by the
// project configuration, or ("", "", "") when there is no configuration (the
// environment then decides). A present but invalid configuration is an error,
// not a silent fallback.
func configuredAgent(projectDir string) (harness, provider, modelName string, err error) {
	cfg, err := config.LoadDir(projectDir)
	if errors.Is(err, config.ErrNotFound) {
		// No configuration file: only the model-routing environment (which a .env
		// file may supply) can select an agent. Otherwise the environment decides.
		res, rerr := model.Resolve(model.Inputs{Lookup: os.Getenv})
		if rerr != nil {
			return "", "", "", rerr
		}
		if res.Active {
			return "", res.Selection.Provider, res.Selection.Model, nil
		}
		return "", "", "", nil
	}
	if err != nil {
		return "", "", "", err
	}
	if _, err := applyModelRouting(cfg, ""); err != nil {
		return "", "", "", err
	}
	return cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model, nil
}

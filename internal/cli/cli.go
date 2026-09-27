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
// constructor (parameterised by the configured provider), the resume observer,
// and the working-tree diff reader.
type deps struct {
	getwd        func() (string, error)
	newAgent     func(provider string) (agent.Agent, error)
	newResources func(dir string) (resume.Observer, error)
	readDiff     func(ctx context.Context, dir string) (string, error)
	commit       func(ctx context.Context, dir, message string) error
	newGitHub    func(dir string) github.Client
}

func defaultDeps() deps {
	return deps{
		getwd: os.Getwd,
		newAgent: func(provider string) (agent.Agent, error) {
			a, err := agent.FromConfig(provider)
			if err != nil {
				return nil, err
			}
			return agent.NewChecked(a), nil
		},
		newResources: func(dir string) (resume.Observer, error) {
			return &gitHubResources{branches: git.New(dir), prs: github.NewCommandClient(dir)}, nil
		},
		readDiff: func(ctx context.Context, dir string) (string, error) {
			return git.New(dir).Diff(ctx)
		},
		commit: func(ctx context.Context, dir, message string) error {
			g := git.New(dir)
			if err := g.Add(ctx); err != nil {
				return err
			}
			return g.Commit(ctx, message)
		},
		newGitHub: func(dir string) github.Client { return github.NewCommandClient(dir) },
	}
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
  task <id> show details for a single task
  plan      generate PLAN.md from PRD.md or a task file
  tasks     build and persist tasks from .agent-sdlc/plan.json
  validate  run the configured build/test/lint commands
  review    review the current changes with the configured engine
  run       run the project (normal entry point); also runs one TASK.md
  commit    commit the current changes (needs --yes when the human gate is on)
  pr        push a task branch and open a pull request (needs --yes)
  mcp       serve tools over the Model Context Protocol (stdio)
  report    print a concise summary of the latest run
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

// configuredProvider returns the agent provider named by the project
// configuration, or "" when there is no configuration (the environment then
// decides). A present but invalid configuration is an error, not a silent
// fallback.
func configuredProvider(projectDir string) (string, error) {
	cfg, err := config.LoadDir(projectDir)
	if errors.Is(err, config.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return cfg.Agent.Provider, nil
}

package cli

// SOP-002 — pre-implementation preflight.
//
// Before the implementation model is invoked, the controller authorizes every
// declared acceptance command on the path that will actually run it, and checks
// the declared execution prerequisites: tools on PATH, manifests inside the
// repository, and required environment variables for presence only. A failure is
// reported as ONE structured setup blocker instead of calling the model.
//
// Capability inventory:
//
//   - EXISTS acceptance-check authorization: authorizeAcceptanceCommandWith
//     (internal/cli/outcome.go). Preflight calls it with
//     config.CargoAcceptanceCheckEnabled, so the dangerous-form rejection and the
//     Cargo classifier apply exactly as at execution time.
//   - EXISTS agent-tool authorization: Harness.checkCommandIn
//     (internal/toolharness/harness.go). Preflight applies the same command-policy
//     rules (CargoClassify with config.CargoAgentToolEnabled, Classify otherwise).
//   - EXISTS independent Cargo permissions: config.CargoAcceptanceCheckEnabled and
//     config.CargoAgentToolEnabled (internal/config/cargo.go). A declared Cargo
//     command whose two permissions disagree is a command-policy blocker.
//   - EXISTS plan acceptance checks: planTaskAcceptanceChecks
//     (internal/cli/report_deliverable.go), projected into spec.AcceptanceChecks.
//   - EXISTS validation commands: config.Validation. They run through the
//     operator-trusted shell runner (internal/testrunner), which applies no command
//     policy, so there is no authorization path to check them against and they are
//     not declared to preflight.
//   - EXISTS continuation budget: continueTask (internal/cli/drive.go) with
//     runpkg.Run.ReadContinuations / RecordContinuations. Preflight runs in
//     runScheduledTask before the run is created, so a setup failure records no
//     continuation and no attempt.
//   - MISSING configuration key for declared tools, manifests, and environment
//     variables (owner: internal/config). They stay an explicit, empty-by-default
//     input to PreflightDeclarations.
//   - MISSING setup-specific domain.BlockedReason (owner: internal/domain). A setup
//     blocker therefore changes neither the task status nor its attempt count.
//
// Secret handling: a declared environment variable is checked for presence only.
// Its value is never read into a message, so it can never be printed.
//
// No check here executes a task command. Readiness is decided from tokenizing and
// authorizing, PATH lookups, repository path checks, and environment presence.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// PreflightAuthorizationPath names the authorization path that will run a
// declared command. The two paths are independent (an acceptance check and an
// agent tool are authorized separately), so a command is authorized per path.
type PreflightAuthorizationPath string

const (
	// PreflightAcceptance is the acceptance-check authorization path
	// (authorizeAcceptanceCommandWith).
	PreflightAcceptance PreflightAuthorizationPath = "acceptance"
	// PreflightAgentTool is the agent-tool authorization path (internal/toolharness).
	PreflightAgentTool PreflightAuthorizationPath = "agent-tool"
)

// PreflightCommand is one declared command together with the authorization path
// that will run it.
type PreflightCommand struct {
	// Command is the declared command string (never a shell string; it is
	// tokenized by the same splitter the runner uses).
	Command string
	// Path is the authorization path that will execute the command.
	Path PreflightAuthorizationPath
}

// PreflightDeclarations is the declared prerequisite contract. Every field may be
// empty; an empty contract passes, so a run with no declared prerequisites
// reaches the model call with unchanged behaviour.
type PreflightDeclarations struct {
	// Commands are the declared commands, each with the authorization path that
	// will run it.
	Commands []PreflightCommand
	// Tools are executable names that must resolve on PATH.
	Tools []string
	// Manifests are repository-relative file paths that must exist inside the
	// repository root.
	Manifests []string
	// EnvVars are environment variable names that must be present (presence only;
	// the value is never read into a message).
	EnvVars []string
	// CargoAcceptanceCheck and CargoAgentTool are the independent Cargo
	// permissions of the two authorization paths.
	CargoAcceptanceCheck bool
	CargoAgentTool       bool
}

// PreflightBlocker is the single structured setup blocker reported when the
// preflight fails. It names the check, the subject, and the reason, so an
// operator can fix the setup instead of guessing.
type PreflightBlocker struct {
	// Check names the failed check: "command-policy", "tool", "manifest", or
	// "env".
	Check string
	// Subject names the specific command, tool, manifest path, or variable name.
	Subject string
	// Reason states why the check failed.
	Reason string
}

// Error renders the blocker as one actionable line.
func (b PreflightBlocker) Error() string {
	return fmt.Sprintf("setup blocker (%s): %s: %s", b.Check, b.Subject, b.Reason)
}

// PreflightResult is the outcome of a preflight pass. Blocker is nil when the
// preflight passed.
type PreflightResult struct {
	Blocker *PreflightBlocker
}

// OK reports whether the preflight passed.
func (r PreflightResult) OK() bool { return r.Blocker == nil }

// PreflightDeclarationsFromConfig builds the declaration contract from the
// project's Cargo permissions and the task's plan-declared acceptance checks,
// which run on the acceptance path. Tool, manifest, and environment-variable
// declarations have no configuration key today, so they default to empty.
func PreflightDeclarationsFromConfig(cfg config.Config, acceptanceChecks []string) PreflightDeclarations {
	decl := PreflightDeclarations{
		CargoAcceptanceCheck: cfg.CargoAcceptanceCheckEnabled(),
		CargoAgentTool:       cfg.CargoAgentToolEnabled(),
	}
	for _, command := range acceptanceChecks {
		decl.Commands = append(decl.Commands, PreflightCommand{Command: command, Path: PreflightAcceptance})
	}
	return decl
}

// Preflight evaluates every declared prerequisite without executing any task
// command. It returns the first setup blocker it finds, in a stable order, so the
// reported blocker is reproducible. An empty declaration contract passes.
func Preflight(dir string, decl PreflightDeclarations) PreflightResult {
	if blocker := checkCommandPolicy(dir, decl); blocker != nil {
		return PreflightResult{Blocker: blocker}
	}
	if blocker := checkTools(decl.Tools); blocker != nil {
		return PreflightResult{Blocker: blocker}
	}
	if blocker := checkManifests(dir, decl.Manifests); blocker != nil {
		return PreflightResult{Blocker: blocker}
	}
	if blocker := checkEnvVars(decl.EnvVars); blocker != nil {
		return PreflightResult{Blocker: blocker}
	}
	return PreflightResult{}
}

// checkCommandPolicy authorizes every declared command on the path that will run
// it. The two Cargo permissions are independent: an acceptance check is judged
// against acceptance_check only, and an agent tool against agent_tool only. A
// disagreement between the two permissions is therefore NOT a blocker; requiring
// them to agree would make acceptance-only Cargo configuration unusable. The
// command is never executed.
func checkCommandPolicy(dir string, decl PreflightDeclarations) *PreflightBlocker {
	for _, declared := range decl.Commands {
		command := strings.TrimSpace(declared.Command)
		if command == "" {
			continue
		}
		path := declared.Path
		if path == "" {
			path = PreflightAcceptance
		}
		if _, err := authorizeForPath(command, path, decl, dir); err != nil {
			return &PreflightBlocker{
				Check:   "command-policy",
				Subject: command,
				Reason:  fmt.Sprintf("not authorized on the %s path: %v", path, err),
			}
		}
	}
	return nil
}

// authorizeForPath authorizes a declared command on one authorization path without
// executing it, returning its argument vector. The acceptance path is the runner's
// own authorizer; the agent-tool path applies the command-policy rules of
// Harness.checkCommandIn.
func authorizeForPath(command string, path PreflightAuthorizationPath, decl PreflightDeclarations, dir string) ([]string, error) {
	if path == PreflightAcceptance {
		return authorizeAcceptanceCommandWith(command, decl.CargoAcceptanceCheck, dir)
	}
	argv, err := toolharness.SplitCommand(command)
	if err != nil {
		return nil, err
	}
	// Use the harness's own authorization path (the same one that executes an
	// agent tool) rather than a parallel approximation: it applies the command
	// policy, protected-path rules, directory-override rejection, path confinement,
	// read-only-root rules, and the Cargo permission together.
	h := toolharness.New(dir, toolharness.Config{CargoAgentToolEnabled: decl.CargoAgentTool}, nil)
	if err := h.CheckCommand(argv); err != nil {
		return nil, err
	}
	return argv, nil
}

// checkTools verifies every declared tool resolves on PATH. It performs a PATH
// lookup only and never runs the tool.
func checkTools(tools []string) *PreflightBlocker {
	for _, tool := range tools {
		name := strings.TrimSpace(tool)
		if name == "" {
			continue
		}
		if _, err := exec.LookPath(name); err != nil {
			return &PreflightBlocker{
				Check:   "tool",
				Subject: name,
				Reason:  "not found on PATH",
			}
		}
	}
	return nil
}

// checkManifests verifies every declared manifest exists inside the repository
// root. An absolute path or a path escaping the root is rejected, so a manifest
// check can never inspect outside the repository.
func checkManifests(dir string, manifests []string) *PreflightBlocker {
	for _, manifest := range manifests {
		rel := strings.TrimSpace(manifest)
		if rel == "" {
			continue
		}
		if filepath.IsAbs(rel) {
			return &PreflightBlocker{
				Check:   "manifest",
				Subject: rel,
				Reason:  "declared manifest must be a repository-relative path",
			}
		}
		clean := filepath.Clean(filepath.FromSlash(rel))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return &PreflightBlocker{
				Check:   "manifest",
				Subject: rel,
				Reason:  "declared manifest escapes the repository root",
			}
		}
		info, err := os.Stat(filepath.Join(dir, clean))
		if err != nil {
			return &PreflightBlocker{
				Check:   "manifest",
				Subject: rel,
				Reason:  "manifest file is missing inside the repository",
			}
		}
		if info.IsDir() {
			return &PreflightBlocker{
				Check:   "manifest",
				Subject: rel,
				Reason:  "declared manifest is a directory, not a file",
			}
		}
	}
	return nil
}

// checkEnvVars verifies every declared environment variable is present. It uses
// presence only: the value is never read into a message, so a secret value can
// never be printed.
func checkEnvVars(names []string) *PreflightBlocker {
	for _, name := range names {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		if _, ok := os.LookupEnv(key); !ok {
			return &PreflightBlocker{
				Check:   "env",
				Subject: key,
				Reason:  "required environment variable is not set",
			}
		}
	}
	return nil
}

// runPreflight evaluates the declared prerequisites for a project directory and,
// when a setup blocker is found, prints it and returns a non-zero exit code. It
// is called before the implementation model is invoked and before any
// continuation-budget increment, so a setup failure consumes no budget. It
// executes no task command.
//
// exitSetupBlocker is distinct from a task-local failure so the graph driver can
// stop immediately: a setup blocker leaves the task runnable by design (it does
// not spend a retry or continuation), and re-selecting it in the same invocation
// would only repeat the same blocker.
const exitSetupBlocker = -1

func runPreflight(dir string, decl PreflightDeclarations, stdout, stderr io.Writer) int {
	res := Preflight(dir, decl)
	if res.OK() {
		return exitOK
	}
	fmt.Fprintf(stderr, "run: %s\n", res.Blocker.Error())
	fmt.Fprintln(stdout, "setup blocker: resolve the declared prerequisite before the model is invoked")
	return exitSetupBlocker
}

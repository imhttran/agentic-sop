package hipcompare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CommandOptions are the parsed arguments of the opt-in `sop compare` subcommand.
type CommandOptions struct {
	ManifestPath   string
	BaselinePath   string
	CandidatePath  string
	OutDir         string
	StartingCommit string
	// Executor settings are shared by both arms. They are provided on the command
	// line so both arms carry the identical settings into the fingerprint.
	Executor        string
	Model           string
	Settings        string
	ToolPermissions []string
	// ExecutorCommand is the operator-provided command that actually runs one
	// arm's agent work. It is the same command for both arms; the harness runs it
	// inside the arm's disposable worktree and passes the arm's instructions through
	// the environment. When it is empty, the arm is treated as a no-op agent run and
	// only the manifest's verification commands decide acceptance. The harness never
	// invents a result.
	ExecutorCommand string
	// Budget is a caller-supplied budget. Unset fields fall back to the fixed
	// default, so a caller may lower counts (for example to run one pair) but never
	// raise a ceiling.
	Budget Budget
}

// Command runs the opt-in comparison harness end to end. It stays off the default
// `sop run` path: it is only reachable as `sop compare`. It never reads or writes
// credentials, never pushes, merges, or changes any external state, and it never
// retries a failed arm automatically.
func Command(ctx context.Context, args []string, stdout io.Writer) int {
	opts, err := parseCommandArgs(args)
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n%s\n", err, commandUsage)
		return 2
	}

	man, err := LoadTaskManifest(opts.ManifestPath)
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}
	base, err := LoadInstructionSet(opts.BaselinePath)
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}
	cand, err := LoadInstructionSet(opts.CandidatePath)
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}
	if err := base.Validate(); err != nil {
		fmt.Fprintf(stdout, "compare: baseline: %v\n", err)
		return 1
	}
	if len(base.Files) != 0 {
		fmt.Fprintln(stdout, "compare: baseline must not declare edits: only the candidate may apply a prompt or skill change")
		return 1
	}
	if err := cand.ValidateFor(man); err != nil {
		fmt.Fprintf(stdout, "compare: candidate: %v\n", err)
		return 1
	}

	budget, err := ResolveBudget(opts.Budget)
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}

	outDir := opts.OutDir
	if outDir == "" {
		outDir = "."
	}

	// The repository is the current checkout, not the output directory: the
	// starting commit and the arm worktrees must come from the same repository.
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}

	// Every fingerprint component is derived from task inputs, the starting commit,
	// instruction hashes, executor/model settings, tool permissions, and verification
	// commands. Both arms share every component except their own instructions.
	commit := opts.StartingCommit
	if commit == "" {
		head, herr := gitOutput(ctx, root, "rev-parse", "HEAD")
		if herr != nil {
			fmt.Fprintf(stdout, "compare: resolve starting commit: %v\n", herr)
			return 1
		}
		commit = strings.TrimSpace(head)
	}
	taskInputs, _ := json.Marshal(man)
	settings := CanonicalSettings(opts.Executor, opts.Model, opts.Settings, opts.ToolPermissions)
	fp := DeriveFingerprint(FingerprintInputs{
		TaskInputs:           string(taskInputs),
		StartingCommit:       commit,
		InstructionHashes:    map[string]string{"baseline": InstructionHash(base), "candidate": InstructionHash(cand)},
		Executor:             opts.Executor,
		Model:                opts.Model,
		Settings:             settings,
		ToolPermissions:      strings.Join(sortedCopy(opts.ToolPermissions), ","),
		VerificationCommands: man.VerificationCommands,
	})

	// Resume rule: a stored COMPLETED comparison is resumed only when every
	// fingerprint component matches. A change to any component is refused, naming
	// the component. A stored incomplete comparison carries its prior arms and
	// executions forward, so repeated invocations never exceed the fixed ceiling.
	priorExecutions := 0
	var priorArms []ArmResult
	if dec, rerr := ResumeFingerprint(outDir, fp); rerr == nil && dec.Stored != nil {
		if !dec.Resumed {
			fmt.Fprintf(stdout, "compare: refusing to resume: fingerprint component %q differs\n", dec.DifferingComponent)
			return 1
		}
		if !dec.Stored.Completed {
			priorExecutions = dec.Stored.Executions
			priorArms = dec.Stored.Arms
		} else {
			fmt.Fprintf(stdout, "compare: resuming completed comparison %s (fingerprint %s matches)\n", dec.Stored.TaskID, fp.Digest)
			return 0
		}
	}

	cfg := RunnerConfig{
		Manifest:        man,
		Baseline:        base,
		Candidate:       cand,
		Budget:          budget,
		Executor:        opts.Executor,
		Model:           opts.Model,
		Settings:        settings,
		ToolPermissions: opts.ToolPermissions,
		StartingCommit:  commit,
		Worktrees:       gitWorktrees{dir: root},
		WorktreeRoot:    filepath.Join(os.TempDir(), "hipcompare-worktrees"),
		Exec:            newCommandExecutor(opts),
		Repository:      root,
		EvidenceRoot:    filepath.Join(outDir, "evidence"),
		Now:             time.Now,
		PriorExecutions: priorExecutions,
		PriorArms:       priorArms,
	}

	cmp, err := Run(ctx, cfg, fp)
	if err != nil {
		// Persist whatever was recorded (every executed arm has a record) before
		// reporting the error, so a rejected comparison still leaves evidence.
		_ = WriteArtifacts(outDir, cmp)
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}
	if err := WriteArtifacts(outDir, cmp); err != nil {
		fmt.Fprintf(stdout, "compare: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "compare: wrote %s and %s\n", filepath.Join(outDir, ResultsFileName), filepath.Join(outDir, ComparisonMarkdownFileName))
	return 0
}

// commandUsage documents the invocation and the fixed limits.
const commandUsage = `usage: sop compare --manifest MANIFEST.json --baseline BASE.json --candidate CAND.json
                   [--out DIR] [--commit SHA] [--executor NAME] [--model NAME]
                   [--executor-command CMD] [--setting K=V] [--allow-tool NAME]...
                   [--pairs N] [--held-out-pairs N] [--per-run-seconds S]
                   [--per-run-tokens N]

Fixed limits (not configurable upward): 3 pairs, 1 held-out pair, 8 executions
maximum, per-run time and token limits, no automatic retries. A caller may lower
the counts. The candidate may edit only the manifest's allowed_candidate_paths;
any other edit aborts the comparison. A completed comparison resumes only when
every fingerprint component matches.`

func parseCommandArgs(args []string) (CommandOptions, error) {
	var opts CommandOptions
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s needs a value", a)
			}
			i++
			return args[i], nil
		}
		intValue := func() (int, error) {
			v, err := value()
			if err != nil {
				return 0, err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return 0, fmt.Errorf("flag %s needs an integer, got %q", a, v)
			}
			return n, nil
		}
		switch a {
		case "--manifest":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.ManifestPath = v
		case "--baseline":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.BaselinePath = v
		case "--candidate":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.CandidatePath = v
		case "--out":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.OutDir = v
		case "--commit":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.StartingCommit = v
		case "--executor":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.Executor = v
		case "--model":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.Model = v
		case "--executor-command":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.ExecutorCommand = v
		case "--setting":
			v, err := value()
			if err != nil {
				return opts, err
			}
			if opts.Settings != "" {
				opts.Settings += "&"
			}
			opts.Settings += v
		case "--allow-tool":
			v, err := value()
			if err != nil {
				return opts, err
			}
			opts.ToolPermissions = append(opts.ToolPermissions, v)
		case "--pairs":
			n, err := intValue()
			if err != nil {
				return opts, err
			}
			opts.Budget.Pairs = n
		case "--held-out-pairs":
			n, err := intValue()
			if err != nil {
				return opts, err
			}
			opts.Budget.HeldOutPairs = n
		case "--per-run-seconds":
			n, err := intValue()
			if err != nil {
				return opts, err
			}
			opts.Budget.PerRunTimeSeconds = n
		case "--per-run-tokens":
			n, err := intValue()
			if err != nil {
				return opts, err
			}
			opts.Budget.PerRunTokenLimit = n
		case "-h", "--help":
			return CommandOptions{}, errors.New("help requested")
		default:
			return opts, fmt.Errorf("unknown argument %s", a)
		}
	}
	if opts.ManifestPath == "" || opts.BaselinePath == "" || opts.CandidatePath == "" {
		return opts, errors.New("--manifest, --baseline and --candidate are required")
	}
	return opts, nil
}

// gitWorktrees is the production disposable-worktree lifecycle. It uses the Git
// CLI directly and never a shell.
type gitWorktrees struct{ dir string }

func (g gitWorktrees) Create(ctx context.Context, path, commit, branch string) error {
	return runGit(ctx, g.dir, "worktree", "add", "-b", branch, path, commit)
}

func (g gitWorktrees) Remove(ctx context.Context, path string) error {
	return runGit(ctx, g.dir, "worktree", "remove", "--force", path)
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// commandExecutor is the production arm executor. It runs the operator's
// --executor-command (if any) as the arm's agent, then runs the manifest's trusted
// verification commands in the same worktree to decide acceptance. Both arms get
// the same executor, settings, tools, and verification commands. The executor
// environment is a scrubbed copy of the operator environment that drops any
// clearly-secret variable, so a credential never reaches the experimental agent.
type commandExecutor struct {
	opts CommandOptions
}

func newCommandExecutor(opts CommandOptions) ArmExecutor {
	return commandExecutor{opts: opts}
}

func (c commandExecutor) Execute(ctx context.Context, in ExecutorInput) (ExecutorResult, error) {
	res := ExecutorResult{}
	env := scrubbedEnv(in)

	// Step 0: apply the arm's instruction set to the worktree. The baseline
	// declares no edits; the candidate may touch only the manifest's allowed
	// prompt or skill paths. The runner checks the resulting diff again, but this
	// pre-write check means an out-of-list instruction never reaches the worktree.
	for _, f := range in.Instructions.Files {
		if bad := disallowed([]string{f.Path}, in.AllowedPaths); bad != "" {
			return res, &DisallowedEditError{Path: bad}
		}
		dest := filepath.Join(in.Worktree, filepath.FromSlash(f.Path))
		if !pathWithin(dest, in.Worktree) {
			return res, fmt.Errorf("instruction path escapes the worktree: %s", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return res, fmt.Errorf("create instruction directory for %s: %w", f.Path, err)
		}
		if err := os.WriteFile(dest, []byte(f.Content), 0o644); err != nil {
			return res, fmt.Errorf("apply instruction %s: %w", f.Path, err)
		}
		res.InstructionsApplied = append(res.InstructionsApplied, f.Path)
	}

	// Step 1: the arm's agent work, if a command was configured. An operator that
	// wants no model call passes an empty command; then only verification decides.
	if strings.TrimSpace(c.opts.ExecutorCommand) != "" {
		cmd := exec.CommandContext(ctx, "sh", "-c", c.opts.ExecutorCommand)
		cmd.Dir = in.Worktree
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if s := string(out); s != "" {
			res.Output += s
		}
		if err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("executor command failed: %v", err))
			return res, nil
		}
		res.Completed = true
	} else {
		res.Completed = true
	}

	// Step 2: the manifest's trusted verification commands, identical for both arms,
	// decide acceptance. The harness never reports acceptance without running them.
	if len(in.VerificationCommands) == 0 {
		res.Failures = append(res.Failures, "no verification commands: acceptance cannot be decided")
		return res, nil
	}
	for i, vc := range in.VerificationCommands {
		if ctx.Err() != nil {
			res.Failures = append(res.Failures, "verification aborted by context deadline")
			return res, ctx.Err()
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", vc)
		cmd.Dir = in.Worktree
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if s := string(out); s != "" {
			res.Output += fmt.Sprintf("verification[%d]: %s\n", i, s)
		}
		if err != nil {
			res.Failures = append(res.Failures, fmt.Sprintf("verification command %d failed: %v", i, err))
			res.AcceptancePassed = false
			return res, nil
		}
	}
	res.AcceptancePassed = true
	return res, nil
}

// scrubbedEnv is the operator environment without clearly-secret variables, plus
// the arm's shared inputs passed as HIPCOMPARE_* variables.
func scrubbedEnv(in ExecutorInput) []string {
	taskFile := ""
	if in.Manifest != nil {
		taskFile = in.Manifest.TaskFile
	}
	out := make([]string, 0, len(os.Environ())+12)
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		if isSecretEnvName(kv[:i]) {
			continue
		}
		out = append(out, kv)
	}
	out = append(out,
		"HIPCOMPARE_WORKTREE="+in.Worktree,
		"HIPCOMPARE_STARTING_COMMIT="+in.StartingCommit,
		"HIPCOMPARE_TASK_FILE="+taskFile,
		"HIPCOMPARE_INSTRUCTION_NAME="+in.Instructions.Name,
		"HIPCOMPARE_INSTRUCTION_TEXT="+in.Instructions.Instructions,
		"HIPCOMPARE_VERIFICATION="+strings.Join(in.VerificationCommands, "\n"),
		"HIPCOMPARE_ALLOWED_PATHS="+strings.Join(in.AllowedPaths, "\n"),
		"HIPCOMPARE_EXECUTOR="+in.Executor,
		"HIPCOMPARE_MODEL="+in.Model,
		"HIPCOMPARE_SETTINGS="+in.Settings,
		"HIPCOMPARE_TOOLS="+strings.Join(in.ToolPermissions, ","),
	)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// timeLimitMillis converts the per-run time limit to milliseconds for comparison
// with the observed elapsed time.
func timeLimitMillis(seconds int) int64 {
	return int64(seconds) * 1000
}

// pathWithin reports whether child is inside parent after cleaning. It is used
// before writing a candidate instruction so a path can never escape the arm
// worktree.
func pathWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

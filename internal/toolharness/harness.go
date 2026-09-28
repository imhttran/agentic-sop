// Package toolharness is SOP's controlled tool harness: the single place where a
// model may read and modify repository files, run policy-checked commands, and
// inspect Git state. It is an execution adapter, never a workflow authority:
// SOP keeps ownership of task state, validation, review, retries, and human
// gates, and this harness never touches .agent-sdlc/state.db, never commits,
// pushes, merges, resets, or cleans, and reports a structured outcome for
// IMPLEMENT and FIX.
//
// The harness exposes exactly eight tools — read_file, write_file, create_file,
// list_files, search_files, run_command, git_status, and git_diff — and every
// invocation is policy checked and audited.
package toolharness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/commandpolicy"
)

// Tool names, kept in one place so the registry and the prompt cannot drift.
const (
	ToolReadFile    = "read_file"
	ToolWriteFile   = "write_file"
	ToolCreateFile  = "create_file"
	ToolListFiles   = "list_files"
	ToolSearchFiles = "search_files"
	ToolRunCommand  = "run_command"
	ToolGitStatus   = "git_status"
	ToolGitDiff     = "git_diff"
)

// Tools returns the canonical tool names in a stable order. This is the harness's
// advertised surface; a request for any other tool is refused.
func Tools() []string {
	return []string{
		ToolReadFile, ToolWriteFile, ToolCreateFile, ToolListFiles,
		ToolSearchFiles, ToolRunCommand, ToolGitStatus, ToolGitDiff,
	}
}

// Bounds keep a single tool call from exhausting memory or producing an
// unbounded result.
const (
	maxListEntries     = 500
	maxSearchMatches   = 200
	maxSearchFileBytes = 512 << 10
)

// Config is the harness's execution settings.
type Config struct {
	// CommandTimeout bounds a single run_command execution.
	CommandTimeout time.Duration
	// MaxOutputBytes bounds the output kept from a tool call.
	MaxOutputBytes int
}

// DefaultConfig returns the built-in execution bounds.
func DefaultConfig() Config {
	return Config{
		CommandTimeout: 120 * time.Second,
		MaxOutputBytes: 256 << 10,
	}
}

// Sentinel errors, so callers can classify a refusal without matching on an
// error message. Policy denials (a disallowed command, a protected path) are
// distinguishable from an execution failure, which is what the audit uses to
// record an allow/deny decision.
var (
	// errCommandNotAllowed is the sentinel for a command-policy refusal.
	errCommandNotAllowed = errors.New("command not allowed")
	// ErrProtectedPath is the sentinel for a refusal to touch SOP's state
	// database. It is exported so adapters and tests can classify a denial with
	// errors.Is instead of string matching.
	ErrProtectedPath = errors.New("refusing to access SOP state")
)

// Harness dispatches controlled tool calls inside a repository root. It is safe
// for sequential use by one agent loop; it is not designed for concurrent calls.
type Harness struct {
	root    string // canonical repository root
	cfg     Config
	auditor Auditor
}

// New returns a Harness rooted at root, using cfg and auditing through auditor.
// A nil auditor discards audit records.
func New(root string, cfg Config, auditor Auditor) *Harness {
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		canonical = root
	}
	if abs, err := filepath.Abs(canonical); err == nil {
		canonical = abs
	}
	return &Harness{root: canonical, cfg: cfg, auditor: auditor}
}

// Root returns the canonical repository root the harness works in.
func (h *Harness) Root() string { return h.root }

// Run dispatches one tool call by name and records an audit entry for the
// decision and the outcome. A policy denial is returned as an error but is
// audited as a denial, so refusals are as visible as executions.
func (h *Harness) Run(ctx context.Context, name string, args map[string]any) (string, error) {
	result, err := h.dispatch(ctx, name, args)
	h.audit(name, args, result, err)
	return result, err
}

// dispatch routes a tool call to its implementation.
func (h *Harness) dispatch(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case ToolReadFile:
		return h.readFile(args)
	case ToolWriteFile:
		return h.writeFile(args, false)
	case ToolCreateFile:
		return h.writeFile(args, true)
	case ToolListFiles:
		return h.listFiles(args)
	case ToolSearchFiles:
		return h.searchFiles(args)
	case ToolRunCommand:
		return h.runCommand(ctx, args)
	case ToolGitStatus:
		return h.git(ctx, "status", "--short", "--branch")
	case ToolGitDiff:
		return h.git(ctx, "diff")
	default:
		return "", fmt.Errorf("unsupported tool %q", name)
	}
}

// audit records one tool decision/outcome. A denied or unsupported tool is
// recorded with Action=deny so the audit answers "what was refused", not only
// "what ran". Denials are recognized by sentinel error, so rewording a message
// cannot silently reclassify them.
func (h *Harness) audit(name string, args map[string]any, result string, err error) {
	if h.auditor == nil {
		return
	}
	rec := AuditRecord{Tool: name, Action: ActionAllow, Request: SummarizeRequest(name, args)}
	switch {
	case err == nil:
		rec.Outcome = OutcomeOK
	case isDenial(err):
		rec.Action = ActionDeny
		rec.Outcome = OutcomeDenied
		rec.Detail = err.Error()
	default:
		rec.Outcome = OutcomeError
		rec.Detail = err.Error()
	}
	_ = result // result content is not audited: it can be large and sensitive
	h.auditor.Record(rec)
}

// isDenial reports whether err is a policy refusal rather than an execution
// failure, using sentinel errors so classification does not depend on wording.
func isDenial(err error) bool {
	return errors.Is(err, errCommandNotAllowed) || errors.Is(err, ErrProtectedPath)
}

// RecordDenied records a refusal the caller decided (for example a
// capability-level tool restriction applied above this harness), so the audit
// reports every refusal, not only the ones this harness makes itself. It never
// records file content.
func (h *Harness) RecordDenied(name string, args map[string]any, detail string) {
	if h.auditor == nil {
		return
	}
	h.auditor.Record(AuditRecord{
		Tool:    name,
		Action:  ActionDeny,
		Outcome: OutcomeDenied,
		Request: SummarizeRequest(name, args),
		Detail:  detail,
	})
}

// SummarizeRequest renders the request arguments that matter for audit and
// diagnostics, never including file content.
func SummarizeRequest(name string, args map[string]any) string {
	switch name {
	case ToolRunCommand:
		command, _ := args["command"].(string)
		return strings.TrimSpace(command)
	default:
		var parts []string
		for _, key := range []string{"path", "pattern"} {
			if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, key+"="+strings.TrimSpace(v))
			}
		}
		return strings.Join(parts, " ")
	}
}

// git runs a fixed, read-only git command. The subcommand is allow-listed by
// construction, so git inspection cannot be turned into a destructive operation.
func (h *Harness) git(ctx context.Context, args ...string) (string, error) {
	return h.exec(ctx, append([]string{"git"}, args...))
}

// WorkingTreeChanged reports whether the repository working tree differs from
// HEAD — a modified, staged, deleted, or untracked path — ignoring SOP's own state
// directory. It runs a fixed, read-only `git status --porcelain`, so it reports
// the source tree the model actually touched rather than trusting a claim the
// model makes about its own work. A failure to inspect (for example outside a git
// repository) is returned as an error, so a caller can decline to act on it.
func (h *Harness) WorkingTreeChanged(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, h.cfg.CommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = h.root
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return hasSourceChange(string(out)), nil
}

// hasSourceChange reports whether porcelain output names any path outside SOP's
// state directory.
func hasSourceChange(porcelain string) bool {
	for _, line := range strings.Split(porcelain, "\n") {
		path := porcelainPath(line)
		if path == "" {
			continue
		}
		if path == stateDirRel || strings.HasPrefix(path, stateDirRel+"/") {
			continue
		}
		return true
	}
	return false
}

// porcelainPath extracts the path from one `git status --porcelain` line, or ""
// for a blank line. A rename entry ("R  old -> new") reports its new path.
func porcelainPath(line string) string {
	line = strings.TrimRight(line, "\r")
	if len(line) < 4 {
		return ""
	}
	rest := line[3:] // two status columns and the separating space
	if i := strings.Index(rest, " -> "); i >= 0 {
		rest = rest[i+len(" -> "):]
	}
	return strings.Trim(rest, `"`)
}

// runCommand tokenizes and policy-checks a command before executing it.
func (h *Harness) runCommand(ctx context.Context, args map[string]any) (string, error) {
	command, _ := args["command"].(string)
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New(`missing required argument "command"`)
	}
	argv, err := SplitCommand(command)
	if err != nil {
		return "", err
	}
	if err := h.CheckCommand(argv); err != nil {
		return "", err
	}
	return h.exec(ctx, argv)
}

// SplitCommand tokenizes a command string, rejecting shell metacharacters and
// globs so nothing can be reinterpreted by a shell. It is deliberately small: no
// variable expansion, no redirection, no command substitution.
func SplitCommand(s string) ([]string, error) {
	if strings.ContainsAny(s, ";&|<>$`\n\r*?[]{}()~") {
		return nil, fmt.Errorf("%w: shell metacharacters or globs are not allowed in %q", errCommandNotAllowed, s)
	}

	var argv []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			argv = append(argv, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("%w: unterminated quote", errCommandNotAllowed)
	}
	flush()
	if len(argv) == 0 {
		return nil, errors.New("command is empty")
	}
	return argv, nil
}

// CheckCommand applies SOP's existing command policy. It refuses repository
// location overrides, absolute paths outside the repository, commands that touch
// the state database, and any command SOP does not classify SAFE. Destructive Git
// operations (commit, push, merge, reset, clean, and the rest) are not SAFE, so
// they are denied unless an SOP-controlled workflow explicitly authorizes them
// elsewhere; this harness never authorizes them.
func (h *Harness) CheckCommand(argv []string) error {
	if len(argv) == 0 {
		return errors.New("command is empty")
	}
	if hasDirOverride(argv) {
		return fmt.Errorf("%w: -C/--git-dir/--work-tree overrides are not allowed", errCommandNotAllowed)
	}
	for _, a := range argv {
		if filepath.IsAbs(a) {
			if _, err := h.resolve(a, accessRead); err != nil {
				return fmt.Errorf("%w: %v", errCommandNotAllowed, err)
			}
		}
		if namesStateDB(a) {
			return fmt.Errorf("%w: %s is protected", ErrProtectedPath, stateDBRel)
		}
	}

	if filepath.Base(argv[0]) == "gofmt" {
		return nil
	}
	class, err := commandpolicy.Classify(argv)
	if err != nil {
		return err
	}
	if class != commandpolicy.Safe {
		return fmt.Errorf("%w: %q is %s", errCommandNotAllowed, strings.Join(argv, " "), class)
	}
	return nil
}

// namesStateDB reports whether an argument references the state database by a
// path variant (with either separator or an absolute reference).
func namesStateDB(arg string) bool {
	normalized := strings.ReplaceAll(arg, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	return normalized == stateDBRel || strings.HasSuffix(normalized, "/"+stateDBRel)
}

// hasDirOverride reports whether argv would make a command operate outside the
// repository root.
func hasDirOverride(argv []string) bool {
	for _, a := range argv {
		switch a {
		case "-C", "--git-dir", "--work-tree", "--exec-path", "--namespace":
			return true
		}
		if strings.HasPrefix(a, "--git-dir=") || strings.HasPrefix(a, "--work-tree=") ||
			strings.HasPrefix(a, "--exec-path=") || strings.HasPrefix(a, "--namespace=") {
			return true
		}
	}
	return false
}

// exec runs argv in the repository root with a bounded timeout, capturing stdout
// and stderr. A non-zero exit is reported in the result (the model should see
// failing tests); only a timeout is a tool error.
func (h *Harness) exec(ctx context.Context, argv []string) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, h.cfg.CommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...)
	cmd.Dir = h.root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	out := buf.String()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return truncate(out, h.cfg.MaxOutputBytes), fmt.Errorf("command timed out after %s", h.cfg.CommandTimeout)
	}
	return fmt.Sprintf("%s\n%s", exitStatus(err), truncate(out, h.cfg.MaxOutputBytes)), nil
}

// exitStatus renders a command's exit state for the model.
func exitStatus(err error) string {
	if err == nil {
		return "exit 0"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return fmt.Sprintf("exit %d", ee.ExitCode())
	}
	return fmt.Sprintf("error: %v", err)
}

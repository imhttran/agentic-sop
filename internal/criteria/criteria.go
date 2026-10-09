// Package criteria implements HARDEN-001b: trusted, caller-owned acceptance
// criterion verification.
//
// Trust model: a criterion is MET only when SOP executes an OPERATOR-OWNED
// verifier command and observes a passing result. Model output can never supply,
// influence, or authorize a verifier. Authorisation is provenance-based, not a
// caller-set flag: only bindings produced by ParseBindings (an operator-authored
// source) are executable; a Binding literal a caller constructs is inert
// (unauthorised). A model's claim (e.g. CompletionEvidence.Acceptance) is never
// trusted, and evidence that merely contains "MET" is not accepted unless it is
// well-formed and matches its execution context (see Evidence.Valid and Fresh).
//
// This package is self-contained and additive: it does not change existing
// lifecycle behavior. HARDEN-001 wires its evidence into the quality gate to make
// enforcement default-on; HARDEN-001b only produces trusted criterion outcomes.
package criteria

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/toolharness"
	"gopkg.in/yaml.v3"
)

// State is the deterministic outcome of verifying one criterion.
type State string

const (
	// Met: an authorised verifier executed and passed (exit 0 and, when declared,
	// the required output substring was present).
	Met State = "MET"
	// NotMet: an authorised verifier executed and failed its declared assertion.
	NotMet State = "NOT_MET"
	// Unavailable: no usable verifier — missing, invalid, unauthorised, mismatched,
	// timed out, canceled, or otherwise not executable. Never treated as Met.
	Unavailable State = "UNAVAILABLE"
)

// Binding binds a required criterion (by stable id) to an operator-owned verifier.
//
// Authorisation: a Binding is executable only when it was produced by
// ParseBindings. The authorisation is an unexported provenance bit, so a caller
// (or agent-authored content) cannot grant it by setting an exported field.
type Binding struct {
	// CriterionID is the stable identifier for the criterion (an explicit id, or
	// the normalised legacy criterion text).
	CriterionID string
	// Command is the verifier argv. It is executed WITHOUT a shell.
	Command []string
	// OutputMustContain is an optional assertion on the verifier's stdout.
	OutputMustContain string
	// digest is the verifier-definition digest (argv + assertion). Stored so a
	// changed definition is detectable.
	digest string
	// authorised is set ONLY by ParseBindings (operator-authoritative source).
	authorised bool
}

// Digest returns the verifier-definition digest (argv + assertion).
func (b Binding) Digest() string {
	if b.digest != "" {
		return b.digest
	}
	return verifierDigest(b.Command, b.OutputMustContain)
}

// Authorized reports whether the binding came from the operator source.
func (b Binding) Authorized() bool { return b.authorised }

// Outcome is the trusted result for one criterion.
type Outcome struct {
	CriterionID    string   `json:"criterion_id"`
	Command        []string `json:"command"`
	VerifierDigest string   `json:"verifier_digest"`
	ExitCode       int      `json:"exit_code"`
	State          State    `json:"state"`
	Reason         string   `json:"reason,omitempty"`
}

// Evidence is the machine-readable audit artifact for one task attempt.
type Evidence struct {
	Version        int       `json:"version"`
	TaskID         string    `json:"task_id"`
	Attempt        int       `json:"attempt"`
	AttemptID      string    `json:"attempt_id"`
	Revision       string    `json:"revision"`
	Workspace      string    `json:"workspace"`
	WorkspaceState string    `json:"workspace_state,omitempty"`
	BindingsDigest string    `json:"bindings_digest"`
	Outcomes       []Outcome `json:"criteria"`
	RecordedAt     time.Time `json:"recorded_at"`
}

// EvidenceVersion is the schema version of the evidence artifact.
const EvidenceVersion = 1

// ErrNoBindings is returned by ParseBindings when the operator source is empty.
var ErrNoBindings = errors.New("criteria: no operator bindings supplied")

// yamlDoc is the operator-owned binding document.
type yamlDoc struct {
	Version  int           `yaml:"version"`
	Bindings []yamlBinding `yaml:"bindings"`
}

type yamlBinding struct {
	ID                string `yaml:"id"`
	Criterion         string `yaml:"criterion"`
	Command           string `yaml:"command"`
	OutputMustContain string `yaml:"output_must_contain"`
}

// NormalizeCriterion returns the stable identifier for a legacy free-text
// criterion: lowercased, whitespace-collapsed text. It is deterministic and
// model-independent, so an operator binding and a declared criterion agree on
// identity without a new task-syntax requirement. Newer bindings may instead pin
// an explicit id (see the yaml "id" field).
func NormalizeCriterion(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// ParseBindings reads OPERATOR-OWNED bindings from a YAML document and returns
// executable (authorised) bindings. It fails closed on an empty document, a
// malformed or unauthorised command, an empty criterion, or a duplicate id. The
// only way to obtain an authorised Binding is through this function.
func ParseBindings(data []byte) ([]Binding, error) {
	if strings.TrimSpace(string(data)) == "" {
		return nil, ErrNoBindings
	}
	var doc yamlDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("criteria: parse bindings: %w", err)
	}
	if len(doc.Bindings) == 0 {
		return nil, ErrNoBindings
	}
	specs := make([]BindingSpec, 0, len(doc.Bindings))
	for _, b := range doc.Bindings {
		specs = append(specs, BindingSpec{ID: b.ID, Criterion: b.Criterion, Command: b.Command, OutputMustContain: b.OutputMustContain})
	}
	return ParseBindingSpecs(specs)
}

// BindingSpec is one operator-authored binding in typed form: the typed analogue
// of the YAML document, and the ONLY other source of authorised bindings.
type BindingSpec struct {
	ID                string
	Criterion         string
	Command           string
	OutputMustContain string
}

// ParseBindingSpecs authorises typed operator binding specs. Like ParseBindings,
// it is the only way to obtain executable bindings: authorisation is set here and
// nowhere else, so operator-controlled configuration (which supplies the specs) is
// the sole source of verifier commands.
func ParseBindingSpecs(specs []BindingSpec) ([]Binding, error) {
	if len(specs) == 0 {
		return nil, ErrNoBindings
	}
	seen := make(map[string]bool, len(specs))
	out := make([]Binding, 0, len(specs))
	for i, b := range specs {
		id := NormalizeCriterion(b.ID)
		if id == "" {
			id = NormalizeCriterion(b.Criterion)
		}
		if id == "" {
			return nil, fmt.Errorf("criteria: binding %d has an empty criterion id", i)
		}
		if seen[id] {
			return nil, fmt.Errorf("criteria: duplicate criterion id %q", id)
		}
		seen[id] = true
		argv, err := tokenizeCommand(b.Command)
		if err != nil {
			return nil, fmt.Errorf("criteria: binding %q: %w", id, err)
		}
		out = append(out, Binding{
			CriterionID:       id,
			Command:           argv,
			OutputMustContain: b.OutputMustContain,
			digest:            verifierDigest(argv, b.OutputMustContain),
			authorised:        true,
		})
	}
	return out, nil
}

// tokenizeCommand tokenises an operator command using SOP's existing trusted
// command policy (toolharness.SplitCommand: no shell metacharacters, no globs, no
// command substitution) and then rejects workspace-escape elements (absolute
// paths and "..", which SplitCommand — a tokeniser, not a path escaper — does not
// itself reject). This is the documented reuse-plus-extension of the policy.
func tokenizeCommand(s string) ([]string, error) {
	argv, err := toolharness.SplitCommand(s)
	if err != nil {
		return nil, err
	}
	for _, a := range argv {
		if filepath.IsAbs(a) || a == ".." || strings.HasPrefix(a, "../") ||
			strings.Contains(a, "/../") || strings.HasSuffix(a, "/..") {
			return nil, fmt.Errorf("criteria: command element escapes the workspace: %q", a)
		}
	}
	return argv, nil
}

// verifierDigest is the sha256 of the argv and the optional output assertion. A
// changed verifier definition therefore changes the digest.
func verifierDigest(argv []string, assertion string) string {
	h := sha256.New()
	for _, a := range argv {
		io.WriteString(h, a)
		h.Write([]byte{0})
	}
	h.Write([]byte{1})
	io.WriteString(h, assertion)
	return hex.EncodeToString(h.Sum(nil))
}

// BindingsDigest is the aggregate digest of an authorised binding set, so a
// changed verifier definition is detectable from the evidence alone.
func BindingsDigest(bindings []Binding) string {
	lines := make([]string, 0, len(bindings))
	for _, b := range bindings {
		lines = append(lines, b.CriterionID+"\x00"+b.Digest())
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		io.WriteString(h, l)
		h.Write([]byte{1})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Options configures one verification pass for a task attempt.
type Options struct {
	// Dir is the working directory verifiers run in (the task workspace).
	Dir string
	// Workspace overrides the recorded workspace identity; when empty it is the
	// absolute form of Dir.
	Workspace string
	// WorkspaceState is an operator/caller-supplied descriptor of the relevant
	// workspace state (for example a git cleanliness/revision digest).
	WorkspaceState string
	// TaskID, Attempt, and AttemptID bind the result to its execution context.
	TaskID    string
	Attempt   int
	AttemptID string
	// Revision is the repository revision the verification is bound to (git HEAD).
	Revision string
	// Criteria are the task's required criteria (explicit ids or legacy text).
	Criteria []string
	// Bindings are the operator-owned verifiers.
	Bindings []Binding
	// Timeout bounds each verifier; zero uses DefaultTimeout.
	Timeout time.Duration
	// MaxOutputBytes caps captured stdout; zero uses DefaultMaxOutputBytes.
	MaxOutputBytes int
	// Now, when set, supplies the timestamp (deterministic tests).
	Now func() time.Time
}

// DefaultTimeout and DefaultMaxOutputBytes bound a verifier's resources.
const (
	DefaultTimeout        = 60 * time.Second
	DefaultMaxOutputBytes = 64 * 1024
)

// Verify executes the operator-owned verifier bound to each required criterion and
// returns trusted Evidence bound to the task, attempt, workspace, revision, and
// binding set. Missing, unauthorised, duplicate, mismatched, timed-out, canceled,
// or unexecutable verifiers are UNAVAILABLE. Nothing is fabricated.
func Verify(ctx context.Context, opts Options) (Evidence, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxOut := opts.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = DefaultMaxOutputBytes
	}
	workspace := strings.TrimSpace(opts.Workspace)
	if workspace == "" {
		if abs, err := filepath.Abs(opts.Dir); err == nil {
			workspace = abs
		} else {
			workspace = opts.Dir
		}
	}

	byID := make(map[string]Binding, len(opts.Bindings))
	dupe := make(map[string]bool, len(opts.Bindings))
	for _, b := range opts.Bindings {
		if _, ok := byID[b.CriterionID]; ok {
			dupe[b.CriterionID] = true
		}
		byID[b.CriterionID] = b
	}

	ev := Evidence{
		Version:        EvidenceVersion,
		TaskID:         opts.TaskID,
		Attempt:        opts.Attempt,
		AttemptID:      opts.AttemptID,
		Revision:       opts.Revision,
		Workspace:      workspace,
		WorkspaceState: opts.WorkspaceState,
		BindingsDigest: BindingsDigest(opts.Bindings),
		RecordedAt:     now().UTC(),
	}
	seenReq := make(map[string]bool, len(opts.Criteria))
	for _, c := range opts.Criteria {
		id := criterionID(c)
		if id == "" || seenReq[id] {
			continue
		}
		seenReq[id] = true
		ev.Outcomes = append(ev.Outcomes, verifyOne(ctx, opts.Dir, id, byID, dupe, timeout, maxOut))
	}
	return ev, nil
}

// criterionID resolves a declared criterion to its id: an explicit id is used
// verbatim; otherwise the legacy free text is normalised.
func criterionID(c string) string {
	t := strings.TrimSpace(c)
	return NormalizeCriterion(t)
}

func verifyOne(ctx context.Context, dir, id string, byID map[string]Binding, dupe map[string]bool, timeout time.Duration, maxOut int) Outcome {
	b, ok := byID[id]
	if !ok {
		return Outcome{CriterionID: id, State: Unavailable, Reason: "no operator verifier bound to this criterion"}
	}
	if dupe[id] {
		return Outcome{CriterionID: id, Command: b.Command, VerifierDigest: b.Digest(), State: Unavailable, Reason: "duplicate operator bindings for this criterion"}
	}
	if !b.Authorized() {
		return Outcome{CriterionID: id, VerifierDigest: b.Digest(), State: Unavailable, Reason: "binding is not operator-authorised"}
	}
	if len(b.Command) == 0 {
		return Outcome{CriterionID: id, State: Unavailable, Reason: "binding has an empty command"}
	}

	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, b.Command[0], b.Command[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &out, limit: maxOut}
	cmd.Stderr = io.Discard

	err := cmd.Run()
	outcome := Outcome{CriterionID: id, Command: b.Command, VerifierDigest: b.Digest()}
	switch {
	case errors.Is(cctx.Err(), context.DeadlineExceeded):
		outcome.State = Unavailable
		outcome.Reason = "verifier timed out"
		return outcome
	case ctx.Err() != nil:
		outcome.State = Unavailable
		outcome.Reason = "verification canceled"
		return outcome
	}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			outcome.ExitCode = ee.ExitCode()
			outcome.State = NotMet
			outcome.Reason = "verifier failed its declared assertion"
			return outcome
		}
		outcome.State = Unavailable
		outcome.Reason = "verifier could not execute: " + err.Error()
		return outcome
	}
	// Exit code is the default assertion: a verified pass needs only exit 0 unless
	// an explicit output assertion was also declared.
	if b.OutputMustContain != "" && !strings.Contains(out.String(), b.OutputMustContain) {
		outcome.State = NotMet
		outcome.Reason = fmt.Sprintf("verifier output did not contain %q", b.OutputMustContain)
		return outcome
	}
	outcome.State = Met
	outcome.Reason = "verifier passed"
	return outcome
}

// WorkspaceDigest computes a deterministic fingerprint of the workspace: one
// sha256 over each file's workspace-relative path and content, in lexical order.
// It includes tracked AND untracked files, so an untracked change also invalidates
// the fingerprint, and it EXCLUDES the SOP audit directory (".agent-sdlc") and
// ".git", so generated audit artifacts never self-invalidate the fingerprint.
func WorkspaceDigest(dir string) (string, error) {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			if path != dir && (d.Name() == ".git" || d.Name() == ".agent-sdlc") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		io.WriteString(h, filepath.ToSlash(rel))
		h.Write([]byte{0})
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		defer f.Close()
		if _, cerr := io.Copy(h, f); cerr != nil {
			return cerr
		}
		h.Write([]byte{1})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// limitedWriter caps how many bytes are captured from a verifier's stdout.
type limitedWriter struct {
	w     io.Writer
	limit int
	n     int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n >= l.limit {
		return len(p), nil
	}
	room := l.limit - l.n
	if len(p) > room {
		if _, err := l.w.Write(p[:room]); err != nil {
			return 0, err
		}
		l.n = l.limit
		return len(p), nil
	}
	if _, err := l.w.Write(p); err != nil {
		return 0, err
	}
	l.n += len(p)
	return len(p), nil
}

// WriteEvidence writes the evidence artifact to <dir>/criteria.json (JSON),
// creating the directory if needed. Secrets are never captured: only the command
// argv, digest, exit code, state, and reason are recorded (never the environment).
func WriteEvidence(dir string, ev Evidence) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "criteria.json")
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Valid reports whether the evidence is well-formed enough to be considered at
// all: it must carry its full execution identity, and every MET outcome must have
// a verifier digest, a command, and a zero exit code. Evidence that merely
// contains "MET" therefore cannot be accepted.
func (e Evidence) Valid() error {
	if e.Version != EvidenceVersion {
		return fmt.Errorf("criteria: unknown evidence version %d", e.Version)
	}
	for _, f := range []struct{ name, val string }{
		{"task id", e.TaskID}, {"attempt id", e.AttemptID}, {"revision", e.Revision},
		{"workspace", e.Workspace}, {"bindings digest", e.BindingsDigest},
	} {
		if strings.TrimSpace(f.val) == "" {
			return fmt.Errorf("criteria: evidence is missing its %s", f.name)
		}
	}
	for _, o := range e.Outcomes {
		if strings.TrimSpace(o.CriterionID) == "" {
			return errors.New("criteria: outcome is missing its criterion id")
		}
		if strings.TrimSpace(o.VerifierDigest) == "" {
			return fmt.Errorf("criteria: outcome %q is missing its verifier digest", o.CriterionID)
		}
		if len(o.Command) == 0 {
			return fmt.Errorf("criteria: outcome %q is missing its command", o.CriterionID)
		}
		if o.State == Met && o.ExitCode != 0 {
			return fmt.Errorf("criteria: outcome %q is MET with non-zero exit code", o.CriterionID)
		}
	}
	return nil
}

// AllMet reports whether every outcome is MET. An empty outcome set is not "all
// met": a task with required criteria and no results has nothing verified.
func (e Evidence) AllMet() bool {
	if len(e.Outcomes) == 0 {
		return false
	}
	for _, o := range e.Outcomes {
		if o.State != Met {
			return false
		}
	}
	return true
}

// AllMetVerified reports whether the evidence is valid AND every criterion is MET.
// This is the only form a consumer (HARDEN-001) should treat as "acceptance
// satisfied": a bare MET string in a malformed artifact fails Valid.
func (e Evidence) AllMetVerified() bool {
	return e.Valid() == nil && e.AllMet()
}

// Freshness is the expected execution context for accepting evidence.
type Freshness struct {
	TaskID         string
	Attempt        int
	AttemptID      string
	Revision       string
	Workspace      string
	WorkspaceState string
	BindingsDigest string
}

// Fresh reports whether the evidence matches the expected execution context, so a
// stale artifact, a different task/attempt, a different workspace, or a changed
// verifier definition is never reused as fresh verification. Empty expectations
// never match (no trivially-true freshness).
func (e Evidence) Fresh(exp Freshness) bool {
	if exp.TaskID == "" || exp.Revision == "" || exp.Workspace == "" || exp.AttemptID == "" || exp.BindingsDigest == "" {
		return false
	}
	return e.TaskID == exp.TaskID &&
		e.Attempt == exp.Attempt &&
		e.AttemptID == exp.AttemptID &&
		e.Revision == exp.Revision &&
		e.Workspace == exp.Workspace &&
		e.WorkspaceState == exp.WorkspaceState &&
		e.BindingsDigest == exp.BindingsDigest
}

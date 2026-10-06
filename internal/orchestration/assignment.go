package orchestration

// This file defines the ORCH-002 canonical, provider-neutral Worker contract.
//
// CONTRACT OVERVIEW
//
// A WorkAssignment is the harness-owned description of one unit of work handed
// to a worker adapter. A WorkResult is what that worker returns. Both are pure
// value types composed only of domain/agent-neutral values: task identifiers,
// agent.Capability labels, plain strings and lists, declared scope paths, and
// an opaque repository-identity token. Neither type names a provider, model,
// transport, or vendor.
//
// SINGLE-LIFECYCLE-AUTHORITY INVARIANT
//
// The contract grants workers NO lifecycle authority. It carries no transition,
// approval, commit, budget-extension, or completion method. Acceptance of an
// assignment and acceptance of a result remain harness-side deterministic
// decisions delegated to internal/domain. A WorkResult is a claim; it is never
// itself success.
//
// SCOPE AND REPOSITORY IDENTITY AT THE CONTRACT BOUNDARY
//
// Scope and RepositoryIdentity are enforced here (presence/shape, changed-file
// containment, identity match). They are deliberately NOT the disjoint-write
// policy: write ownership and conflict arbitration belong to ORCH-005, and
// stale-identity/rebase rejection belongs to ORCH-006. ORCH-002 only fails
// safely on an already-visible mismatch or malformed result.
//
// FAIL-SAFE SEMANTICS
//
// A malformed or out-of-scope result is rejected with an explicit typed error
// (AssignmentError / ResultError) and a Diagnostic list. It is never coerced
// into agent.OutcomeCompleted. A result that reports an explicit failed status
// is likewise rejected as not acceptable: the boundary never reinterprets a
// failure (for example an unparsable payload) as success. Validation functions
// have no side effects.

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Scope declares the repository-relative paths a worker is permitted to touch.
// It is expressed as repository-relative path prefixes and/or globs. An empty
// Scope denies all writes: a worker may reason and report, but may not claim a
// changed file.
//
// Scope is a provider-neutral value type. It contains no filesystem handle and
// performs no I/O.
type Scope struct {
	// Paths are repository-relative path prefixes (for example "internal/agent")
	// or file paths (for example "README.md"). A result file is in scope when it
	// equals a path or is nested beneath it.
	Paths []string `json:"paths,omitempty"`
	// Globs are repository-relative glob patterns (for example "cmd/**/*.go")
	// matched against a result file with path.Match semantics on cleaned,
	// slash-separated paths.
	Globs []string `json:"globs,omitempty"`
}

// RepositoryIdentity is an opaque, provider-neutral token that identifies the
// repository revision an assignment was built against (for example a git HEAD
// commit or a caller-supplied revision name). It is a value, never derived from
// model output. Populating it from git and rejecting staleness belong to
// ORCH-006; the contract only checks presence and match.
type RepositoryIdentity struct {
	// Revision is the opaque revision token. It must be non-empty on a valid
	// assignment.
	Revision string `json:"revision"`
}

// IsZero reports whether the identity carries no revision token.
func (r RepositoryIdentity) IsZero() bool { return strings.TrimSpace(r.Revision) == "" }

// WorkAssignment is the canonical, provider-neutral assignment handed to a
// worker. Every field is domain/agent-neutral; none names a provider, model, or
// transport.
type WorkAssignment struct {
	// AssignmentID is the caller-owned correlation ID. It is not model output.
	AssignmentID string `json:"assignment_id"`
	// TaskID is the domain task the assignment executes.
	TaskID string `json:"task_id"`
	// Capability is the deterministic capability requested (agent.Capability).
	Capability agent.Capability `json:"capability"`
	// Task is the human-readable task statement.
	Task string `json:"task"`
	// Scope is the declared write scope enforced at the boundary.
	Scope Scope `json:"scope"`
	// Context is caller-supplied, provider-neutral background for the worker.
	Context string `json:"context,omitempty"`
	// AllowedTools lists the deterministic tool names the worker may use.
	AllowedTools []string `json:"allowed_tools,omitempty"`
	// Budget bounds the worker's effort. A zero value means "unbounded"; callers
	// that require a bound must set at least one field.
	Budget Budget `json:"budget,omitempty"`
	// OutputContract describes the required shape of the worker's output.
	OutputContract OutputContract `json:"output_contract,omitempty"`
	// RepositoryIdentity identifies the revision the assignment was built against.
	RepositoryIdentity RepositoryIdentity `json:"repository_identity"`
	// AcceptanceCriteria and ValidationCommands are the caller-owned verification
	// contract, carried through to the agent boundary.
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	ValidationCommands []string `json:"validation_commands,omitempty"`
}

// Budget bounds worker effort without granting authority to extend it. It is
// deliberately token-free: no token budget and no provider-specific budget is
// introduced during this phase (see internal/orchestration/budget.go and the
// AGENT-004 invocation budget in internal/budget, which remains authoritative).
type Budget struct {
	// MaxSteps is the maximum number of worker steps; zero means unbounded.
	MaxSteps int `json:"max_steps,omitempty"`
	// TimeoutSeconds is a wall-clock bound; zero means unbounded.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// OutputContract describes the shape a WorkResult must satisfy. It is caller
// data, never model output.
type OutputContract struct {
	// Format names the expected result format (for example "json" or "markdown").
	Format string `json:"format,omitempty"`
	// RequiredFields lists result fields the caller requires to be present.
	RequiredFields []string `json:"required_fields,omitempty"`
}

// Diagnostic is one provider-neutral observation about a rejected assignment or
// result. It carries no provider field.
type Diagnostic struct {
	// Code is a stable, machine-checkable identifier (for example "scope_violation").
	Code string `json:"code"`
	// Message is a human-readable explanation.
	Message string `json:"message"`
	// Path optionally names the repository-relative path the diagnostic concerns.
	Path string `json:"path,omitempty"`
}

// Progress is a provider-neutral statement of how far a worker got. It is
// informational only and grants no lifecycle authority.
type Progress struct {
	// StepsCompleted counts finished worker steps.
	StepsCompleted int `json:"steps_completed"`
	// Summary is a short human-readable progress note.
	Summary string `json:"summary,omitempty"`
	// Complete is the worker's own claim that it finished. The harness still
	// verifies; a false value is never treated as success.
	Complete bool `json:"complete,omitempty"`
}

// WorkResult is the canonical, provider-neutral result a worker returns. It
// carries only domain/agent-neutral values and is a claim, not a success.
type WorkResult struct {
	// AssignmentID must match the WorkAssignment it answers.
	AssignmentID string `json:"assignment_id"`
	// Status is one of the explicit agent.OutcomeStatus values.
	Status agent.OutcomeStatus `json:"status"`
	// Findings summarizes what the worker concluded.
	Findings []string `json:"findings,omitempty"`
	// ProposedChanges describes intended changes in prose/structured text. It is
	// advisory and never applied by this contract.
	ProposedChanges []string `json:"proposed_changes,omitempty"`
	// ChangedFiles lists the repository-relative paths the worker claims to have
	// changed. Every entry must lie within the assignment Scope.
	ChangedFiles []string `json:"changed_files,omitempty"`
	// Evidence lists caller-verifiable evidence references (for example commands
	// run or files inspected).
	Evidence []string `json:"evidence,omitempty"`
	// Progress reports how far the worker got.
	Progress Progress `json:"progress,omitempty"`
	// Diagnostics records provider-neutral rejection/observation details.
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	// RepositoryIdentity must match the assignment's identity token.
	RepositoryIdentity RepositoryIdentity `json:"repository_identity,omitempty"`
}

// AssignmentError is the explicit typed failure returned when a WorkAssignment
// is malformed. It is never a success.
type AssignmentError struct {
	Diagnostics []Diagnostic
}

func (e *AssignmentError) Error() string {
	return "invalid work assignment: " + joinDiagnostics(e.Diagnostics)
}

// ResultError is the explicit typed failure returned when a WorkResult is
// malformed or out of scope. It is never a success.
type ResultError struct {
	Diagnostics []Diagnostic
}

func (e *ResultError) Error() string {
	return "invalid work result: " + joinDiagnostics(e.Diagnostics)
}

// joinDiagnostics renders diagnostics deterministically.
func joinDiagnostics(diags []Diagnostic) string {
	if len(diags) == 0 {
		return "unspecified"
	}
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Path != "" {
			parts = append(parts, fmt.Sprintf("%s (%s): %s", d.Code, d.Path, d.Message))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", d.Code, d.Message))
	}
	return strings.Join(parts, "; ")
}

// ValidateAssignment enforces the assignment contract boundary: non-empty
// assignment and task IDs and task, a valid capability, well-formed declared
// scope, and a present repository identity. It performs no I/O and mutates
// nothing. A failure is an explicit *AssignmentError, never a default success.
func ValidateAssignment(a WorkAssignment) error {
	var diags []Diagnostic

	if strings.TrimSpace(a.AssignmentID) == "" {
		diags = append(diags, Diagnostic{Code: "missing_assignment_id", Message: "assignment id is empty"})
	}
	if strings.TrimSpace(a.TaskID) == "" {
		diags = append(diags, Diagnostic{Code: "missing_task_id", Message: "task id is empty"})
	}
	if strings.TrimSpace(a.Task) == "" {
		diags = append(diags, Diagnostic{Code: "missing_task", Message: "task statement is empty"})
	}
	if !knownCapability(a.Capability) {
		diags = append(diags, Diagnostic{Code: "invalid_capability", Message: fmt.Sprintf("capability %q is not a known agent capability", a.Capability)})
	}
	diags = append(diags, validateScope(a.Scope)...)
	if a.RepositoryIdentity.IsZero() {
		diags = append(diags, Diagnostic{Code: "missing_repository_identity", Message: "repository identity is empty"})
	}
	if a.Budget.MaxSteps < 0 || a.Budget.TimeoutSeconds < 0 {
		diags = append(diags, Diagnostic{Code: "invalid_budget", Message: "budget bounds must not be negative"})
	}

	if len(diags) > 0 {
		return &AssignmentError{Diagnostics: diags}
	}
	return nil
}

// ValidateResult enforces the result contract boundary against its assignment:
// non-empty and matching AssignmentID, an explicit known outcome status,
// changed files contained in the declared scope, and a repository identity that
// matches the assignment. It performs no I/O, mutates nothing, and never
// reinterprets a malformed/out-of-scope result as success. A result that reports
// an explicit failed status (for example an unparsable payload) is rejected as
// well: the boundary accepts only a completed or needs-human claim.
func ValidateResult(a WorkAssignment, r WorkResult) error {
	var diags []Diagnostic

	if strings.TrimSpace(r.AssignmentID) == "" {
		diags = append(diags, Diagnostic{Code: "missing_assignment_id", Message: "result assignment id is empty"})
	} else if r.AssignmentID != a.AssignmentID {
		diags = append(diags, Diagnostic{Code: "assignment_id_mismatch", Message: fmt.Sprintf("result assignment id %q does not match assignment %q", r.AssignmentID, a.AssignmentID)})
	}
	if !knownOutcomeStatus(r.Status) {
		diags = append(diags, Diagnostic{Code: "unknown_status", Message: fmt.Sprintf("status %q is not a known outcome status", r.Status)})
	} else if r.Status == agent.OutcomeFailed {
		// A result the worker itself reports as failed (for example an unparsable
		// payload) is never accepted: it must not be reinterpreted as success.
		diags = append(diags, Diagnostic{Code: "result_failed", Message: "result reports an explicit failed status"})
	}
	for _, file := range r.ChangedFiles {
		if !scopeContains(a.Scope, file) {
			diags = append(diags, Diagnostic{Code: "out_of_scope", Path: file, Message: "changed file lies outside the declared scope"})
		}
	}
	if r.RepositoryIdentity.IsZero() {
		diags = append(diags, Diagnostic{Code: "missing_repository_identity", Message: "result repository identity is empty"})
	} else if r.RepositoryIdentity.Revision != a.RepositoryIdentity.Revision {
		diags = append(diags, Diagnostic{Code: "repository_identity_mismatch", Message: fmt.Sprintf("result revision %q does not match assignment revision %q", r.RepositoryIdentity.Revision, a.RepositoryIdentity.Revision)})
	}

	if len(diags) > 0 {
		return &ResultError{Diagnostics: diags}
	}
	return nil
}

// validateScope checks the declared scope shape: every declared path and glob
// must be repository-relative, non-empty, and free of traversal.
func validateScope(s Scope) []Diagnostic {
	for _, p := range s.Paths {
		if diags := validateScopeEntry(p, false); len(diags) > 0 {
			return diags
		}
	}
	for _, g := range s.Globs {
		if diags := validateScopeEntry(g, true); len(diags) > 0 {
			return diags
		}
	}
	return nil
}

// validateScopeEntry validates one scope path or glob.
func validateScopeEntry(entry string, isGlob bool) []Diagnostic {
	kind := "scope path"
	if isGlob {
		kind = "scope glob"
	}
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return []Diagnostic{{Code: "malformed_scope", Message: kind + " is empty"}}
	}
	if isGlob {
		if _, err := path.Match(trimmed, ""); err != nil {
			return []Diagnostic{{Code: "malformed_scope", Path: trimmed, Message: "scope glob is invalid: " + err.Error()}}
		}
	}
	clean := path.Clean(trimmed)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(trimmed) {
		return []Diagnostic{{Code: "malformed_scope", Path: trimmed, Message: kind + " must be repository-relative and must not escape the repository"}}
	}
	return nil
}

// scopeContains reports whether a repository-relative file lies within the
// declared scope. An empty scope denies every file. A file equal to a declared
// path, or nested beneath it, is in scope; otherwise declared globs are tried.
func scopeContains(s Scope, file string) bool {
	cleanFile := path.Clean(strings.TrimSpace(file))
	if cleanFile == "" || cleanFile == "." || cleanFile == ".." || strings.HasPrefix(cleanFile, "../") || path.IsAbs(cleanFile) {
		return false
	}
	for _, p := range s.Paths {
		cleanPath := path.Clean(strings.TrimSpace(p))
		if cleanFile == cleanPath || strings.HasPrefix(cleanFile, cleanPath+"/") {
			return true
		}
	}
	for _, g := range s.Globs {
		pattern := strings.TrimSpace(g)
		if ok, err := path.Match(pattern, cleanFile); err == nil && ok {
			return true
		}
		if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
			cleanPrefix := path.Clean(prefix)
			if cleanFile == cleanPrefix || strings.HasPrefix(cleanFile, cleanPrefix+"/") {
				return true
			}
		}
	}
	return false
}

// knownCapability reports whether c is one of the canonical agent capabilities.
func knownCapability(c agent.Capability) bool {
	return agent.AllCapabilities().Supports(c)
}

// knownOutcomeStatus reports whether s is one of the explicit outcome statuses.
func knownOutcomeStatus(s agent.OutcomeStatus) bool {
	switch s {
	case agent.OutcomeCompleted, agent.OutcomeNeedsHuman, agent.OutcomeFailed:
		return true
	default:
		return false
	}
}

// ErrNoAssignment is returned when an adapter is asked for a result without an
// assignment. It is an explicit failure, never success.
var ErrNoAssignment = errors.New("no work assignment provided")

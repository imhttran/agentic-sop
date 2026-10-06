package orchestration

// This file implements ORCH-003: deterministic work decomposition.
//
// CONTRACT OVERVIEW
//
// DecomposeWork takes an ordered set of already-loaded work candidates and
// emits a bounded set of scoped WorkAssignments. It is pure, provider-neutral,
// performs no I/O, consults no provider or model, and mutates no lifecycle
// state. It reuses the ORCH-002 WorkAssignment/Scope contract and the
// ORCH-002.2 AssignmentBuilder.
//
// DECOMPOSITION BOUND
//
// Decomposition is bounded by an explicit, deterministic limit. The limit is a
// caller-supplied value (DecomposeRequest.Limit) when positive, otherwise the
// package default DefaultDecompositionLimit. It is NEVER derived from model
// output or from the input size. Models must not create unlimited subagents:
// the harness owns the decomposition bound, so an input with more candidates
// than the limit is deterministically clamped (subject to the over-bound
// policy below).
//
// OVER-BOUND POLICY
//
// DecompositionOverBoundClamp (the default) emits exactly the limit and records
// a provider-neutral Diagnostic describing the truncation. The alternative
// DecompositionOverBoundReject returns an explicit *AssignmentError and emits
// no assignments. Both policies are deterministic for identical input. A
// negative limit is always an explicit error, never a silent default.
//
// STABLE IDENTITY RULE
//
// Every emitted assignment receives a stable AssignmentID derived purely from
// decomposition inputs: the candidate task ID, the candidate capability, and a
// deterministic ordinal. The derivation is a canonical FNV-1a hash with no
// randomness, time, I/O, or provider/model input, and is independent of caller
// map or slice order, so identical inputs yield byte-identical identities
// across runs. A caller-supplied candidate ID is ignored in favour of the
// deterministic derivation so identity cannot be destabilised by a
// non-deterministic caller or model.
//
// EXPLICIT SCOPE RULE
//
// Every emitted assignment carries an explicit Scope value. A candidate that
// supplies a scope is validated with the existing scope checks; a candidate
// that supplies none receives an explicit empty (deny-all) Scope, never a
// missing scope. Emission without a declared scope is therefore impossible: an
// unscoped assignment cannot be produced by this step, and a malformed caller
// scope produces an explicit *AssignmentError instead of silent omission.
//
// LIFECYCLE AUTHORITY
//
// The decomposition step grants no lifecycle authority. It defines no state,
// transition, approval, or completion API and never mutates domain state,
// consistent with ORCH-002 and model.go's single-lifecycle-authority invariant.
//
// The internal/scheduler and internal/run runnable-task selection contracts are
// UNCONFIRMED, so this step takes already-loaded, caller-supplied candidate data
// and does not depend on scheduler selection semantics.

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// DefaultDecompositionLimit is the explicit, deterministic default bound on the
// number of assignments a single decomposition step may emit. It is a harness
// constant, never derived from model output or input size.
const DefaultDecompositionLimit = 16

// DecompositionOverBoundPolicy selects the deterministic behaviour when the
// number of candidates exceeds the decomposition limit.
type DecompositionOverBoundPolicy string

const (
	// DecompositionOverBoundClamp emits exactly the limit (the bound) and records
	// a deterministic Diagnostic describing the truncation. It is the default
	// (zero-value) policy.
	DecompositionOverBoundClamp DecompositionOverBoundPolicy = "CLAMP"
	// DecompositionOverBoundReject emits no assignments and returns an explicit
	// *AssignmentError when the candidates exceed the limit.
	DecompositionOverBoundReject DecompositionOverBoundPolicy = "REJECT"
)

// DecompositionCandidate is one unit of work offered to the decomposition step.
// It carries only domain/agent-neutral values: a task ID, a capability, a task
// statement, and a declared scope. It names no provider, model, or transport.
type DecompositionCandidate struct {
	// TaskID is the domain task this candidate executes.
	TaskID string
	// Capability is the deterministic capability requested.
	Capability agent.Capability
	// Task is the human-readable task statement.
	Task string
	// Scope is the declared write scope for this candidate. A zero Scope is an
	// explicit deny-all scope: the emitted assignment still carries a Scope field.
	Scope Scope
}

// DecomposeRequest is the pure, deterministic input to DecomposeWork. Every
// field is domain/agent-neutral.
type DecomposeRequest struct {
	// Candidates lists the already-loaded work to decompose. Order does not affect
	// the emitted identity set.
	Candidates []DecompositionCandidate
	// Limit is the explicit deterministic bound on emitted assignments. When zero,
	// DefaultDecompositionLimit applies. A negative value is an explicit error.
	Limit int
	// Policy selects the deterministic over-bound behaviour. The zero value is
	// DecompositionOverBoundClamp.
	Policy DecompositionOverBoundPolicy
	// Context, AllowedTools, Budget, OutputContract, RepositoryIdentity, and the
	// verification contract are applied uniformly to every emitted assignment.
	Context            string
	AllowedTools       []string
	Budget             Budget
	OutputContract     OutputContract
	RepositoryIdentity RepositoryIdentity
	AcceptanceCriteria []string
	ValidationCommands []string
}

// DecomposeResult is the deterministic output of DecomposeWork.
type DecomposeResult struct {
	// Assignments are the bounded, deterministically ordered emitted assignments.
	Assignments []WorkAssignment
	// Diagnostics records provider-neutral observations such as truncation. It is
	// empty when nothing was truncated.
	Diagnostics []Diagnostic
}

// DecomposeWork deterministically decomposes work into a bounded set of scoped
// assignments with stable identities.
//
// It applies the explicit deterministic bound from the request (or the package
// default), assigns every emitted assignment a stable derived AssignmentID and
// an explicit Scope, builds each via the existing AssignmentBuilder, and orders
// the output deterministically. It performs no I/O, consults no provider or
// model, and never mutates lifecycle state.
//
// On a negative limit, a malformed candidate, or (under
// DecompositionOverBoundReject) an over-bound candidate set, it returns an
// explicit *AssignmentError and emits no assignments.
func DecomposeWork(req DecomposeRequest) (DecomposeResult, error) {
	limit, err := resolveDecompositionLimit(req.Limit)
	if err != nil {
		return DecomposeResult{}, err
	}

	candidates, err := normalizeCandidates(req.Candidates)
	if err != nil {
		return DecomposeResult{}, err
	}

	policy := req.Policy
	if policy == "" {
		policy = DecompositionOverBoundClamp
	}

	truncated := false
	if len(candidates) > limit {
		switch policy {
		case DecompositionOverBoundReject:
			return DecomposeResult{}, &AssignmentError{Diagnostics: []Diagnostic{{
				Code:    "over_decomposition_limit",
				Message: fmt.Sprintf("candidate count %d exceeds decomposition limit %d", len(candidates), limit),
			}}}
		case DecompositionOverBoundClamp:
			candidates = candidates[:limit]
			truncated = true
		default:
			return DecomposeResult{}, &AssignmentError{Diagnostics: []Diagnostic{{
				Code:    "invalid_over_bound_policy",
				Message: fmt.Sprintf("unknown decomposition over-bound policy %q", policy),
			}}}
		}
	}

	assignments := make([]WorkAssignment, 0, len(candidates))
	for ordinal, candidate := range candidates {
		id := stableAssignmentID(candidate, ordinal)
		builder := AssignmentBuilder{
			AssignmentID:       id,
			TaskID:             candidate.TaskID,
			Capability:         candidate.Capability,
			Task:               candidate.Task,
			Scope:              copyScope(candidate.Scope),
			Context:            req.Context,
			AllowedTools:       copyStrings(req.AllowedTools),
			Budget:             req.Budget,
			OutputContract:     copyOutputContract(req.OutputContract),
			RepositoryIdentity: req.RepositoryIdentity,
			AcceptanceCriteria: copyStrings(req.AcceptanceCriteria),
			ValidationCommands: copyStrings(req.ValidationCommands),
		}
		assignment := builder.Build()
		if err := ValidateAssignment(assignment); err != nil {
			return DecomposeResult{}, err
		}
		assignments = append(assignments, assignment)
	}

	result := DecomposeResult{Assignments: assignments}
	if truncated {
		result.Diagnostics = []Diagnostic{{
			Code:    "decomposition_truncated",
			Message: fmt.Sprintf("candidate count exceeded decomposition limit %d; emitted %d", limit, limit),
		}}
	}
	return result, nil
}

// resolveDecompositionLimit returns the explicit deterministic bound for the
// request: the caller limit when positive, the package default when zero, and
// an explicit error when negative.
func resolveDecompositionLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, &AssignmentError{Diagnostics: []Diagnostic{{
			Code:    "invalid_decomposition_limit",
			Message: fmt.Sprintf("decomposition limit must not be negative: %d", limit),
		}}}
	}
	if limit == 0 {
		return DefaultDecompositionLimit, nil
	}
	return limit, nil
}

// normalizeCandidates validates every candidate and returns a deterministically
// ordered copy. Candidates are ordered by (TaskID, Capability) so the emitted
// identity set is independent of caller slice or map order. Every candidate's
// scope is validated with the existing scope checks; an empty scope is an
// explicit deny-all scope, not an error.
func normalizeCandidates(in []DecompositionCandidate) ([]DecompositionCandidate, error) {
	out := make([]DecompositionCandidate, len(in))
	copy(out, in)
	var diags []Diagnostic
	for i := range out {
		if out[i].Scope.Paths == nil {
			out[i].Scope.Paths = []string{}
		}
		if out[i].Scope.Globs == nil {
			out[i].Scope.Globs = []string{}
		}
		diags = append(diags, validateScope(out[i].Scope)...)
	}
	if len(diags) > 0 {
		return nil, &AssignmentError{Diagnostics: diags}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TaskID != out[j].TaskID {
			return out[i].TaskID < out[j].TaskID
		}
		return out[i].Capability < out[j].Capability
	})
	return out, nil
}

// stableAssignmentID derives a stable AssignmentID from decomposition inputs:
// the candidate task ID, the candidate capability, and a deterministic ordinal.
// It is a canonical FNV-1a hash with no randomness, time, or I/O, so identical
// inputs yield byte-identical identities across runs.
func stableAssignmentID(candidate DecompositionCandidate, ordinal int) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte("orch-003\x00"))
	_, _ = h.Write([]byte(candidate.TaskID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(string(candidate.Capability)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strconv.Itoa(ordinal)))
	return "assign-" + strconv.FormatUint(h.Sum64(), 16)
}

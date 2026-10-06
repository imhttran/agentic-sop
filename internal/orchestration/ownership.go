package orchestration

// This file implements ORCH-005: the write-ownership model and deterministic
// enforcement of disjoint write scopes.
//
// CONTRACT OVERVIEW
//
// Concurrent repository mutation is the highest-risk part of orchestration, so
// Phase 9 defaults to a single writer/integrator rather than unrestricted
// parallel writes. This step models that choice explicitly and enforces it in
// pure Go code paths, never by prompt instructions or caller convention.
//
// SINGLE-WRITER / INTEGRATOR DEFAULT
//
// WriteOwnership zero value is OwnershipSingleWriter: at most one
// repository-mutating assignment may be admitted per run. A run with more than
// one potential writer under the default is rejected before any worker
// executes.
//
// FAIL-CLOSED BACKSTOP
//
// The single-writer default is a safety invariant: it must not be silently
// bypassed by a capability that actually mutates the repository but is not
// classified as mutating by agent.IsRepositoryMutation. Admission therefore
// fails closed: a capability whose classification is not positively known to be
// non-mutating is treated as a potential writer. Only a capability that is both
// in the canonical agent capability set AND classified by
// agent.IsRepositoryMutation as non-mutating is exempt from the single-writer
// count. An unknown capability is conservatively counted as a writer, so a
// misclassified or newly added mutating capability cannot defeat the default and
// admit two real writers.
//
// DISJOINT MULTI-WRITER
//
// OwnershipDisjointMultiWriter permits more than one potential writer only when
// every pair of write Scopes is provably disjoint. Overlap is detected
// deterministically by WriteScopesOverlap, which is a pure predicate over the
// ORCH-002 Scope value type (Path/Path, Path/Glob, and Glob/Glob, including the
// "/**" prefix semantics used by scopeContains). A non-mutating (analysis)
// assignment is exempt: it may share or omit scope without conflict.
//
// SOUNDNESS OF GLOB OVERLAP
//
// Two scopes are reported disjoint only when their disjointness is proven. When
// the overlap of two globs cannot be decided soundly (for example two general
// globs with no fixed prefix), the predicate reports OVERLAP: admission is
// conservative in the rejecting direction, never the admitting direction. A
// malformed glob that cannot be interpreted is likewise treated as overlapping.
// This guarantees that an overlapping write scope is never admitted, even at the
// cost of occasionally rejecting scopes that happen to be disjoint.
//
// DETERMINISTIC DIAGNOSTICS
//
// Overlap admission reports every conflicting pair with stable AssignmentIDs
// and stable diagnostic codes, independent of assignment order: pairs are
// canonicalized by (min, max) AssignmentID and the report is sorted, so
// identical inputs always yield byte-identical diagnostics.
//
// LIFECYCLE AUTHORITY
//
// The ownership model grants NO lifecycle authority. It defines no transition,
// approval, commit, budget-extension, or completion operation and holds no
// handle to workflow state, providers, models, transports, or filesystems. It
// names no provider, model, transport, or filesystem handle.
//
// SCOPE OF ENFORCEMENT
//
// ORCH-005 enforces ownership within one coordinator run over one assignment
// set. Stale identity/rebase rejection is ORCH-006; centralized verification and
// repository application are ORCH-007. This file does neither.

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// WriteOwnership names who is permitted to mutate the repository during one
// coordinator run. The zero value is OwnershipSingleWriter, which is the safe
// default: no more than one repository-mutating writer is admitted. It is a
// provider-neutral value: it names no provider, model, transport, or filesystem
// handle and grants no lifecycle authority.
type WriteOwnership string

const (
	// OwnershipSingleWriter admits at most one repository-mutating assignment per
	// run. It is the default (zero-value) ownership mode: mutation is routed to a
	// single writer/integrator rather than unrestricted parallel writes. A run with
	// more than one mutating assignment is rejected before any worker executes.
	OwnershipSingleWriter WriteOwnership = "SINGLE_WRITER"
	// OwnershipDisjointMultiWriter permits more than one repository-mutating
	// assignment, but only when every pair of mutating write Scopes is provably
	// disjoint. Overlapping mutating scopes are rejected before any worker
	// executes.
	OwnershipDisjointMultiWriter WriteOwnership = "DISJOINT_MULTI_WRITER"
)

// OwnershipDiagnosticCode is a stable, machine-checkable identifier for an
// ownership admission observation. Codes never imply success.
type OwnershipDiagnosticCode string

const (
	// OwnershipCodeSingleWriterExceeded: under the single-writer default more than
	// one repository-mutating assignment was offered.
	OwnershipCodeSingleWriterExceeded OwnershipDiagnosticCode = "single_writer_exceeded"
	// OwnershipCodeOverlappingScope: two repository-mutating assignments declared
	// overlapping write scopes under disjoint multi-writer.
	OwnershipCodeOverlappingScope OwnershipDiagnosticCode = "overlapping_write_scope"
	// OwnershipCodeInvalidMode: the ownership mode is not a known value.
	OwnershipCodeInvalidMode OwnershipDiagnosticCode = "invalid_ownership_mode"
)

// OwnershipError is the explicit typed failure returned when the ownership
// policy rejects an assignment set. It is never a success, and it is returned
// before any assignment executes so no partial dispatch can occur.
type OwnershipError struct {
	// Mode is the ownership mode that rejected the set.
	Mode WriteOwnership
	// Diagnostics are the stable, order-independent ownership observations. Every
	// conflicting pair is reported.
	Diagnostics []OwnershipDiagnostic
}

func (e *OwnershipError) Error() string {
	return "write ownership rejected: " + joinOwnershipDiagnostics(e.Diagnostics)
}

// OwnershipDiagnostic is one provider-neutral observation about a rejected
// ownership admission. It carries no provider, model, or transport field.
type OwnershipDiagnostic struct {
	// Code is the stable machine-checkable identifier.
	Code OwnershipDiagnosticCode
	// Message is a human-readable explanation.
	Message string
	// AssignmentA and AssignmentB are the two caller-owned assignment IDs the
	// diagnostic concerns, canonicalized so AssignmentA <= AssignmentB. They are
	// empty for diagnostics that do not concern a specific pair.
	AssignmentA string
	AssignmentB string
}

// joinOwnershipDiagnostics renders diagnostics deterministically.
func joinOwnershipDiagnostics(diags []OwnershipDiagnostic) string {
	if len(diags) == 0 {
		return "unspecified"
	}
	parts := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.AssignmentA != "" || d.AssignmentB != "" {
			parts = append(parts, fmt.Sprintf("%s (%s,%s): %s", d.Code, d.AssignmentA, d.AssignmentB, d.Message))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", d.Code, d.Message))
	}
	return strings.Join(parts, "; ")
}

// isKnownNonMutating reports whether capability is positively known to be
// non-mutating. It is true only for a capability in the canonical agent
// capability set that agent.IsRepositoryMutation classifies as non-mutating.
// Any capability that is unknown, or that is known to be mutating, is NOT
// known-non-mutating. This is the fail-closed classification used by the
// single-writer backstop.
func isKnownNonMutating(capability agent.Capability) bool {
	if !agent.AllCapabilities().Supports(capability) {
		// Unknown capability: cannot prove it is non-mutating. Treat as a
		// potential writer (fail closed).
		return false
	}
	return !agent.IsRepositoryMutation(capability)
}

// isPotentialWriter reports whether an assignment may write the repository. It
// treats a capability as a writer when agent.IsRepositoryMutation classifies it
// as mutating OR when it is not positively known to be non-mutating. The
// positive classification is taken from the canonical predicate; the fallback
// is the fail-closed backstop that prevents a misclassified or newly added
// mutating capability from defeating the single-writer default.
func isPotentialWriter(a WorkAssignment) bool {
	return agent.IsRepositoryMutation(a.Capability) || !isKnownNonMutating(a.Capability)
}

// mutatingAssignments returns the subset of assignments that may write the
// repository. It uses agent.IsRepositoryMutation for the positive
// classification and the fail-closed backstop for capabilities that are neither
// known-mutating nor known-non-mutating. Non-mutating (analysis) assignments are
// exempt from ownership admission.
func mutatingAssignments(assignments []WorkAssignment) []WorkAssignment {
	out := make([]WorkAssignment, 0, len(assignments))
	for _, a := range assignments {
		if isPotentialWriter(a) {
			out = append(out, a)
		}
	}
	return out
}

// AdmitWriteOwnership deterministically applies the write-ownership policy to an
// assignment set before any worker executes. It returns an explicit
// *OwnershipError (and never nil-error-plus-partial-admission) when the policy is
// violated. It performs no I/O and mutates nothing.
//
// Under OwnershipSingleWriter at most one repository-mutating assignment is
// admitted. Under OwnershipDisjointMultiWriter every pair of mutating write
// scopes must be provably disjoint. Non-mutating assignments never conflict.
// The decision and its diagnostics are independent of assignment order.
func AdmitWriteOwnership(mode WriteOwnership, assignments []WorkAssignment) error {
	if mode == "" {
		mode = OwnershipSingleWriter
	}
	switch mode {
	case OwnershipSingleWriter:
		return admitSingleWriter(assignments)
	case OwnershipDisjointMultiWriter:
		return admitDisjointMultiWriter(assignments)
	default:
		return &OwnershipError{Mode: mode, Diagnostics: []OwnershipDiagnostic{{
			Code:    OwnershipCodeInvalidMode,
			Message: fmt.Sprintf("unknown write ownership mode %q", mode),
		}}}
	}
}

// admitSingleWriter admits at most one repository-mutating assignment. With zero
// or one mutating assignment it admits the set; with more it rejects the set
// deterministically, naming the offending mutating assignment IDs.
func admitSingleWriter(assignments []WorkAssignment) error {
	mutating := mutatingAssignments(assignments)
	if len(mutating) <= 1 {
		return nil
	}

	ids := make([]string, 0, len(mutating))
	for _, a := range mutating {
		ids = append(ids, a.AssignmentID)
	}
	sort.Strings(ids)

	diags := make([]OwnershipDiagnostic, 0, len(ids)-1)
	for i := 1; i < len(ids); i++ {
		diags = append(diags, OwnershipDiagnostic{
			Code:        OwnershipCodeSingleWriterExceeded,
			Message:     fmt.Sprintf("single-writer policy admits one repository-mutating assignment; found %d", len(mutating)),
			AssignmentA: ids[0],
			AssignmentB: ids[i],
		})
	}
	return &OwnershipError{Mode: OwnershipSingleWriter, Diagnostics: diags}
}

// admitDisjointMultiWriter admits any number of repository-mutating assignments
// provided every pair of their write scopes is provably disjoint. Every
// conflicting pair is reported; pairs are canonicalized by
// (min, max) AssignmentID and the report is sorted, so identical inputs yield
// identical diagnostics regardless of input order.
func admitDisjointMultiWriter(assignments []WorkAssignment) error {
	mutating := mutatingAssignments(assignments)

	type pair struct {
		a, b string
	}
	conflicts := make([]pair, 0)
	for i := 0; i < len(mutating); i++ {
		for j := i + 1; j < len(mutating); j++ {
			if !WriteScopesOverlap(mutating[i].Scope, mutating[j].Scope) {
				continue
			}
			a, b := mutating[i].AssignmentID, mutating[j].AssignmentID
			if b < a {
				a, b = b, a
			}
			conflicts = append(conflicts, pair{a: a, b: b})
		}
	}
	if len(conflicts) == 0 {
		return nil
	}

	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].a != conflicts[j].a {
			return conflicts[i].a < conflicts[j].a
		}
		return conflicts[i].b < conflicts[j].b
	})

	diags := make([]OwnershipDiagnostic, 0, len(conflicts))
	for _, c := range conflicts {
		diags = append(diags, OwnershipDiagnostic{
			Code:        OwnershipCodeOverlappingScope,
			Message:     "repository-mutating assignments declare overlapping write scopes",
			AssignmentA: c.a,
			AssignmentB: c.b,
		})
	}
	return &OwnershipError{Mode: OwnershipDisjointMultiWriter, Diagnostics: diags}
}

// WriteScopesOverlap reports whether two declared write scopes overlap, that is
// whether there exists at least one repository-relative file both scopes permit
// a worker to write. It is a pure, deterministic predicate over the ORCH-002
// Scope value type and performs no I/O.
//
// The predicate covers Path/Path, Path/Glob, and Glob/Glob combinations,
// including the "/**" prefix semantics used by scopeContains. An empty
// (deny-all) scope never overlaps any scope, including another empty scope: a
// deny-all scope permits no file, so it cannot conflict.
//
// The predicate is conservative in the rejecting direction: when glob overlap
// cannot be soundly decided it reports overlap. It therefore never fails to
// detect a genuine overlap; two scopes are reported disjoint only when their
// disjointness is proven.
func WriteScopesOverlap(a, b Scope) bool {
	if scopeIsDenyAll(a) || scopeIsDenyAll(b) {
		return false
	}
	for _, ap := range a.Paths {
		for _, bp := range b.Paths {
			if pathsOverlap(ap, bp) {
				return true
			}
		}
	}
	for _, ap := range a.Paths {
		for _, bg := range b.Globs {
			if pathAndGlobOverlap(ap, bg) {
				return true
			}
		}
	}
	for _, ag := range a.Globs {
		for _, bp := range b.Paths {
			if pathAndGlobOverlap(bp, ag) {
				return true
			}
		}
	}
	for _, ag := range a.Globs {
		for _, bg := range b.Globs {
			if globsOverlap(ag, bg) {
				return true
			}
		}
	}
	return false
}

// scopeIsDenyAll reports whether a scope permits no file at all.
func scopeIsDenyAll(s Scope) bool {
	return len(s.Paths) == 0 && len(s.Globs) == 0
}

// normalizeScopePath cleans a declared scope path for comparison.
func normalizeScopePath(p string) string { return path.Clean(strings.TrimSpace(p)) }

// containsWithin reports whether file equals dir or is nested beneath dir, using
// the same containment rule as scopeContains.
func containsWithin(dir, file string) bool {
	return file == dir || strings.HasPrefix(file, dir+"/")
}

// pathsOverlap reports whether two declared path prefixes overlap. Two path
// prefixes overlap when either is equal to or nested within the other.
func pathsOverlap(a, b string) bool {
	ca, cb := normalizeScopePath(a), normalizeScopePath(b)
	if ca == "" || cb == "" {
		return false
	}
	return containsWithin(ca, cb) || containsWithin(cb, ca)
}

// globMatches reports whether a single repository-relative file matches a
// declared glob, reusing the "/**" prefix semantics of scopeContains.
func globMatches(pattern, file string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if ok, err := path.Match(pattern, file); err == nil && ok {
		return true
	}
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		cleanPrefix := path.Clean(prefix)
		if cleanPrefix == "." || cleanPrefix == ".." || strings.HasPrefix(cleanPrefix, "../") {
			// A non-repository-relative prefix cannot be interpreted under the
			// repository. scope validation rejects such entries before admission.
			return false
		}
		return containsWithin(cleanPrefix, file)
	}
	return false
}

// validGlobPrefix returns a concrete directory prefix a glob always covers, or
// ("", false) when the glob has no usable fixed prefix. A "/**" glob covers
// its cleaned prefix subtree; any other glob yields no fixed prefix.
func validGlobPrefix(pattern string) (string, bool) {
	pattern = strings.TrimSpace(pattern)
	prefix, ok := strings.CutSuffix(pattern, "/**")
	if !ok {
		return "", false
	}
	cleanPrefix := path.Clean(prefix)
	if cleanPrefix == "." || cleanPrefix == ".." || strings.HasPrefix(cleanPrefix, "../") {
		return "", false
	}
	return cleanPrefix, true
}

// globIsLiteral reports whether pattern contains no glob meta characters, so it
// denotes exactly one repository-relative path.
func globIsLiteral(pattern string) bool {
	return !strings.ContainsAny(pattern, "*?[")
}

// pathAndGlobOverlap reports whether a declared path prefix and a declared glob
// overlap. They overlap when the glob matches an anchor of the path, or the
// glob's fixed prefix is contained in the path prefix. When the glob has no
// fixed prefix and cannot be proven disjoint from the path, overlap is
// conservatively reported.
func pathAndGlobOverlap(p, g string) bool {
	cp := normalizeScopePath(p)
	if cp == "" {
		return false
	}

	g = strings.TrimSpace(g)

	// A literal glob is exactly one path: compare path-to-path.
	if globIsLiteral(g) {
		return pathsOverlap(p, g)
	}

	// If the glob directly matches the path prefix, they overlap.
	if globMatches(g, cp) {
		return true
	}

	// If the glob has a fixed prefix, containment decides deterministically.
	if prefix, ok := validGlobPrefix(g); ok {
		return containsWithin(prefix, cp) || containsWithin(cp, prefix)
	}

	// No fixed prefix and not matching the path text: disjointness cannot be
	// proven. Report overlap (conservative, rejecting direction).
	return true
}

// globsOverlap reports whether two declared globs overlap. Two globs overlap
// when either matches an anchor of the other's fixed prefix, when their fixed
// prefixes are nested, or when their disjointness cannot be proven. When two
// general globs cannot be soundly decided, overlap is conservatively reported so
// that an overlapping write scope is never admitted.
func globsOverlap(a, b string) bool {
	ca, cb := strings.TrimSpace(a), strings.TrimSpace(b)
	if ca == "" || cb == "" {
		return false
	}

	// A literal glob denotes exactly one path; compare path-to-path, or against
	// the other glob when only one is literal.
	la, lb := globIsLiteral(ca), globIsLiteral(cb)
	if la && lb {
		return pathsOverlap(ca, cb)
	}
	if la {
		return pathAndGlobOverlap(ca, cb)
	}
	if lb {
		return pathAndGlobOverlap(cb, ca)
	}

	pa, oka := validGlobPrefix(ca)
	pb, okb := validGlobPrefix(cb)
	if oka && okb {
		// Both are subtree globs: nested or equal prefixes overlap.
		return containsWithin(pa, pb) || containsWithin(pb, pa)
	}
	if oka {
		// cb has no fixed prefix; conservatively overlaps the subtree at pa.
		return globOverlapsSubtree(cb, pa)
	}
	if okb {
		return globOverlapsSubtree(ca, pb)
	}

	// Neither glob has a fixed prefix. Two general globs may or may not overlap;
	// disjointness cannot be proven, so report overlap (conservative, rejecting
	// direction). This ensures overlapping declarations such as
	// "internal/*/*.go" vs "internal/orchestration/*.go" are never admitted.
	return true
}

// globOverlapsSubtree reports whether glob can match at least one file inside
// the subtree rooted at prefix (or at the prefix itself). It probes a set of
// deterministic anchors and, when a match cannot be found, still reports overlap
// unless the glob's own fixed prefix proves disjointness, keeping the predicate
// conservative for admission.
func globOverlapsSubtree(glob, prefix string) bool {
	if globMatches(glob, prefix) {
		return true
	}
	if gp, ok := validGlobPrefix(glob); ok {
		return containsWithin(gp, prefix) || containsWithin(prefix, gp)
	}
	// Without a provable fixed prefix, conservatively report overlap: a glob with
	// meta characters may match files under any prefix.
	return true
}

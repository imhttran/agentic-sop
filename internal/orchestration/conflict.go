package orchestration

// This file implements ORCH-006: deterministic conflict detection and an
// explicit orchestration decision for every detected conflict.
//
// SINGLE-LIFECYCLE-AUTHORITY INVARIANT
//
// Nothing in this file mutates a domain.Task, invokes a provider, or holds a
// filesystem, git, or transport handle. Detection operates only on
// caller-supplied, provider-neutral orchestration values. A detected conflict
// is surfaced as an explicit ConflictDecision; a conflicting result is NEVER
// silently applied.
//
// PLACEHOLDER / UNSUPPORTED INPUT BOUNDARY
//
// The real inputs for several conflict classes do not yet exist in this
// repository. ORCH-006 therefore defines an explicit, caller-supplied input
// contract. When a required fact is absent the detector deterministically
// reports an explicit conflict (or an explicit "unknown") rather than guessing
// or silently passing. The real extractors are owned by other layers and remain
// OUT OF SCOPE for ORCH-006:
//
//   - File change extraction (repository-relative touched paths) belongs to
//     internal/git and internal/mergegate.
//   - Changed-symbol extraction (canonical, package-qualified symbol IDs)
//     belongs to internal/repoindex.
//   - Patch parsing/application (base identity, hunks, conflict markers) belongs
//     to internal/git and internal/mergegate.
//   - Repository identity / HEAD acquisition belongs to internal/git; callers
//     pass the current identity token on the request.
//
// ConflictDecision is a distinct vocabulary from Mode (how many workers run) and
// FailureCode (why one assignment failed). It expresses what orchestration does
// about a detected conflict and never means "apply anyway".

import (
	"sort"
	"strings"
)

// ConflictClass is a stable, machine-checkable identifier for one class of
// conflict between orchestration units. Its string values are stable and part of
// the ORCH-006 contract.
type ConflictClass string

const (
	// ConflictOverlappingFiles: two distinct assignments claim overlapping file
	// scopes (the same canonical repository-relative path).
	ConflictOverlappingFiles ConflictClass = "overlapping_files"
	// ConflictOverlappingSymbols: two distinct assignments claim overlapping
	// symbols (the same canonical symbol ID).
	ConflictOverlappingSymbols ConflictClass = "overlapping_symbols"
	// ConflictIncompatiblePatches: two patch descriptors target the same scope but
	// declare differing base identities, or a descriptor is explicitly declared
	// incompatible.
	ConflictIncompatiblePatches ConflictClass = "incompatible_patches"
	// ConflictStaleRepositoryIdentity: an assignment/result carries no repository
	// identity (unknown) or an identity unequal to the current repository identity.
	ConflictStaleRepositoryIdentity ConflictClass = "stale_repository_identity"
	// ConflictDependencyChanged: a result's dependency snapshot differs from the
	// current dependency set.
	ConflictDependencyChanged ConflictClass = "dependency_changed"
	// ConflictStaleWorkerResult: a result was computed against a repository
	// identity (HEAD) that differs from the current HEAD.
	ConflictStaleWorkerResult ConflictClass = "stale_worker_result"
)

// allConflictClasses is the canonical, ordered list of conflict classes. The
// order is significant only for deterministic iteration; it is not a priority
// order (priority is defined by DecisionForConflicts).
var allConflictClasses = []ConflictClass{
	ConflictOverlappingFiles,
	ConflictOverlappingSymbols,
	ConflictIncompatiblePatches,
	ConflictStaleRepositoryIdentity,
	ConflictDependencyChanged,
	ConflictStaleWorkerResult,
}

// ConflictUnit is the provider-neutral conflict-relevant view of one assignment
// (and, when present, its result). All scopes are canonical and are sorted
// internally by the detector; callers may supply them in any order.
type ConflictUnit struct {
	// AssignmentID identifies the assignment. It is the correlation key used in
	// findings; an empty ID is treated as an unknown unit and reported as a stale
	// identity conflict so it is never silently applied.
	AssignmentID string
	// Files is the caller-supplied set of repository-relative paths this unit
	// touches. Extraction belongs to internal/git and internal/mergegate.
	Files []string
	// Symbols is the caller-supplied set of canonical symbol IDs this unit changes.
	// Extraction belongs to internal/repoindex.
	Symbols []string
	// Patches describes the patches this unit proposes. Parsing belongs to
	// internal/git and internal/mergegate.
	Patches []PatchDescriptor
	// RepositoryIdentity is the opaque identity this unit was computed against. An
	// empty value means "unknown" and is a stale_repository_identity conflict.
	RepositoryIdentity RepositoryIdentity
	// Dependencies is this unit's dependency snapshot, as recorded when the unit
	// was computed. Comparison against the current set happens in the detector.
	Dependencies []string
	// ResultIdentity is the repository identity a worker result was computed
	// against. When the unit is a result-bearing unit and this differs from the
	// current identity, that is a stale_worker_result conflict. An empty value means
	// "unknown" and is likewise reported.
	ResultIdentity RepositoryIdentity
	// HasResult reports whether this unit carries a worker result whose identity
	// must be validated against HEAD.
	HasResult bool
}

// PatchDescriptor is a caller-supplied, provider-neutral description of a
// proposed patch. ORCH-006 never parses patch bytes; the descriptor's base
// identity and compatibility flag are computed by the owning layer
// (internal/git, internal/mergegate).
type PatchDescriptor struct {
	// PatchID identifies the patch within its unit.
	PatchID string
	// TargetScope is the canonical repository-relative path or symbol scope the
	// patch targets. Two descriptors with the same TargetScope are competing.
	TargetScope string
	// BaseIdentity is the opaque base revision the patch was computed against.
	// Two competing patches with differing BaseIdentity are incompatible.
	BaseIdentity string
	// Incompatible is an explicit, caller-declared incompatibility flag. A true
	// value independently yields an incompatible_patches conflict.
	Incompatible bool
}

// ConflictInput is the provider-neutral, caller-supplied input to
// DetectConflicts. Every field that requires an extractor the repository does not
// yet have is documented above as a caller-supplied placeholder.
type ConflictInput struct {
	// Units are the assignments (and optional results) under consideration.
	Units []ConflictUnit
	// CurrentIdentity is the repository identity considered current (for example
	// the current git HEAD). It belongs to the caller because HEAD acquisition is
	// owned by internal/git.
	CurrentIdentity RepositoryIdentity
	// CurrentDependencies is the current dependency set, keyed by assignment ID.
	// An assignment whose unit snapshot differs is a dependency_changed conflict.
	CurrentDependencies map[string][]string
}

// ConflictFinding pairs a detected conflict class with the involved assignment
// IDs and the deterministic decision taken for it. It carries no transition,
// approval, or commit field.
type ConflictFinding struct {
	// Class is the detected conflict class.
	Class ConflictClass
	// Assignments lists the assignment IDs involved, sorted and deduplicated.
	Assignments []string
	// Detail is a stable, human-readable explanation. It is informational; Class
	// is authoritative.
	Detail string
}

// Canonicalize returns a stable, order-independent representation of one finding.
func (f ConflictFinding) Canonicalize() string {
	ids := append([]string(nil), f.Assignments...)
	sort.Strings(ids)
	return string(f.Class) + "|" + strings.Join(ids, ",") + "|" + f.Detail
}

// canonicalizeFindings returns a stable, order-independent representation of a
// finding set.
func canonicalizeFindings(findings []ConflictFinding) string {
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = f.Canonicalize()
	}
	sort.Strings(lines)
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}

// sortedUnique returns the sorted, deduplicated, non-empty entries of in.
func sortedUnique(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

// DetectConflicts reports every conflict class observable from the input. It is
// pure and order-independent: identical inputs in any unit/scope order yield
// findings whose canonical form is identical. A missing or unknown required
// fact (repository identity, dependency snapshot, worker result identity) is
// reported as an explicit conflict, never a silent pass.
func DetectConflicts(input ConflictInput) []ConflictFinding {
	units := append([]ConflictUnit(nil), input.Units...)
	sort.Slice(units, func(i, j int) bool { return units[i].AssignmentID < units[j].AssignmentID })

	var findings []ConflictFinding
	findings = append(findings, detectStaleIdentity(units, input.CurrentIdentity)...)
	findings = append(findings, detectOverlaps(units, ConflictOverlappingFiles, func(u ConflictUnit) []string { return u.Files })...)
	findings = append(findings, detectOverlaps(units, ConflictOverlappingSymbols, func(u ConflictUnit) []string { return u.Symbols })...)
	findings = append(findings, detectIncompatiblePatches(units)...)
	findings = append(findings, detectDependencyChanges(units, input.CurrentDependencies)...)
	findings = append(findings, detectStaleResults(units, input.CurrentIdentity)...)

	sort.Slice(findings, func(i, j int) bool { return findings[i].Canonicalize() < findings[j].Canonicalize() })
	return findings
}

// detectStaleIdentity reports a stale_repository_identity finding for every unit
// whose identity is unknown or differs from the current identity.
func detectStaleIdentity(units []ConflictUnit, current RepositoryIdentity) []ConflictFinding {
	var findings []ConflictFinding
	for _, u := range units {
		if u.AssignmentID == "" {
			findings = append(findings, ConflictFinding{
				Class:       ConflictStaleRepositoryIdentity,
				Assignments: nil,
				Detail:      "unit has no assignment id; identity cannot be verified",
			})
			continue
		}
		if u.RepositoryIdentity.IsZero() {
			findings = append(findings, ConflictFinding{
				Class:       ConflictStaleRepositoryIdentity,
				Assignments: []string{u.AssignmentID},
				Detail:      "repository identity unknown",
			})
			continue
		}
		if current.IsZero() {
			findings = append(findings, ConflictFinding{
				Class:       ConflictStaleRepositoryIdentity,
				Assignments: []string{u.AssignmentID},
				Detail:      "current repository identity unknown; cannot verify freshness",
			})
			continue
		}
		if u.RepositoryIdentity.Revision != current.Revision {
			findings = append(findings, ConflictFinding{
				Class:       ConflictStaleRepositoryIdentity,
				Assignments: []string{u.AssignmentID},
				Detail:      "repository identity " + u.RepositoryIdentity.Revision + " does not match current " + current.Revision,
			})
		}
	}
	return findings
}

// detectStaleResults reports a stale_worker_result finding for every result-bearing
// unit whose result identity is unknown or differs from the current HEAD.
func detectStaleResults(units []ConflictUnit, current RepositoryIdentity) []ConflictFinding {
	var findings []ConflictFinding
	for _, u := range units {
		if !u.HasResult || u.AssignmentID == "" {
			continue
		}
		if u.ResultIdentity.IsZero() || current.IsZero() || u.ResultIdentity.Revision != current.Revision {
			detail := "worker result computed against unknown HEAD"
			if !u.ResultIdentity.IsZero() && !current.IsZero() {
				detail = "worker result computed against " + u.ResultIdentity.Revision + " but HEAD is " + current.Revision
			}
			findings = append(findings, ConflictFinding{
				Class:       ConflictStaleWorkerResult,
				Assignments: []string{u.AssignmentID},
				Detail:      detail,
			})
		}
	}
	return findings
}

// detectOverlaps reports a finding for every pair of distinct units sharing at
// least one canonical entry from the scope selector. The detail names the shared
// entry deterministically (the lexicographically smallest shared entry).
func detectOverlaps(units []ConflictUnit, class ConflictClass, scope func(ConflictUnit) []string) []ConflictFinding {
	var findings []ConflictFinding
	for i := 0; i < len(units); i++ {
		for j := i + 1; j < len(units); j++ {
			a, b := units[i], units[j]
			if a.AssignmentID == "" || b.AssignmentID == "" || a.AssignmentID == b.AssignmentID {
				continue
			}
			shared := intersection(scope(a), scope(b))
			if len(shared) == 0 {
				continue
			}
			findings = append(findings, ConflictFinding{
				Class:       class,
				Assignments: []string{a.AssignmentID, b.AssignmentID},
				Detail:      "shared " + shared[0],
			})
		}
	}
	return findings
}

// intersection returns the sorted, deduplicated intersection of a and b.
func intersection(a, b []string) []string {
	as := sortedUnique(a)
	bs := sortedUnique(b)
	set := make(map[string]bool, len(bs))
	for _, v := range bs {
		set[v] = true
	}
	var out []string
	for _, v := range as {
		if set[v] {
			out = append(out, v)
		}
	}
	return out
}

// detectIncompatiblePatches reports an incompatible_patches finding for every
// pair of units whose patch descriptors compete: same target scope with differing
// base identities. A descriptor explicitly declared Incompatible yields a
// finding on its own, regardless of competition.
func detectIncompatiblePatches(units []ConflictUnit) []ConflictFinding {
	var findings []ConflictFinding

	for _, u := range units {
		if u.AssignmentID == "" {
			continue
		}
		for _, p := range u.Patches {
			if p.Incompatible {
				findings = append(findings, ConflictFinding{
					Class:       ConflictIncompatiblePatches,
					Assignments: []string{u.AssignmentID},
					Detail:      "patch " + p.PatchID + " explicitly declared incompatible",
				})
			}
		}
	}

	for i := 0; i < len(units); i++ {
		for j := i + 1; j < len(units); j++ {
			a, b := units[i], units[j]
			if a.AssignmentID == "" || b.AssignmentID == "" || a.AssignmentID == b.AssignmentID {
				continue
			}
			for _, pa := range a.Patches {
				for _, pb := range b.Patches {
					if pa.TargetScope == "" || pa.TargetScope != pb.TargetScope {
						continue
					}
					if pa.BaseIdentity != pb.BaseIdentity {
						findings = append(findings, ConflictFinding{
							Class:       ConflictIncompatiblePatches,
							Assignments: []string{a.AssignmentID, b.AssignmentID},
							Detail:      "competing patches on " + pa.TargetScope + " with differing base identities",
						})
					}
				}
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool { return findings[i].Canonicalize() < findings[j].Canonicalize() })
	return findings
}

// detectDependencyChanges reports a dependency_changed finding for every unit
// whose dependency snapshot differs from the current dependency set (added,
// removed, or both), and for a result-bearing unit whose current dependency set
// is unknown while a snapshot exists.
func detectDependencyChanges(units []ConflictUnit, current map[string][]string) []ConflictFinding {
	var findings []ConflictFinding
	for _, u := range units {
		if u.AssignmentID == "" {
			continue
		}
		cur, ok := current[u.AssignmentID]
		if !ok {
			if len(u.Dependencies) > 0 {
				findings = append(findings, ConflictFinding{
					Class:       ConflictDependencyChanged,
					Assignments: []string{u.AssignmentID},
					Detail:      "current dependency set unknown for snapshot-bearing unit",
				})
			}
			continue
		}
		if !equalStringSets(u.Dependencies, cur) {
			findings = append(findings, ConflictFinding{
				Class:       ConflictDependencyChanged,
				Assignments: []string{u.AssignmentID},
				Detail:      "dependency set changed since the unit was computed",
			})
		}
	}
	return findings
}

// equalStringSets reports whether a and b contain the same set of non-empty
// entries, independent of order or duplicates.
func equalStringSets(a, b []string) bool {
	as := sortedUnique(a)
	bs := sortedUnique(b)
	if len(as) != len(bs) {
		return false
	}
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

// ConflictDecision is the explicit orchestration decision for a (possibly empty)
// set of conflict findings. It is a distinct vocabulary from Mode and
// FailureCode; a non-zero value when a conflict exists is guaranteed. It never
// means "apply anyway".
type ConflictDecision string

const (
	// DecisionNone is the zero decision: no conflict was detected.
	DecisionNone ConflictDecision = ""
	// DecisionReject rejects the conflicting assignments: none may be applied.
	DecisionReject ConflictDecision = "REJECT"
	// DecisionSequentialize serializes the overlapping assignments deterministically.
	DecisionSequentialize ConflictDecision = "SEQUENTIALIZE"
	// DecisionEscalate surfaces the conflict to a human gate: no automated path.
	DecisionEscalate ConflictDecision = "ESCALATE"
)

// conflictPriority defines the documented total ordering over conflict classes
// for decision aggregation. Higher precedence dominates.
var conflictPriority = map[ConflictClass]int{
	ConflictStaleRepositoryIdentity: 6,
	ConflictStaleWorkerResult:       6,
	ConflictIncompatiblePatches:     5,
	ConflictOverlappingSymbols:     4,
	ConflictOverlappingFiles:       3,
	ConflictDependencyChanged:      2,
}

// decisionForClass maps one conflict class to its canonical, total decision.
func decisionForClass(class ConflictClass) ConflictDecision {
	switch class {
	case ConflictStaleRepositoryIdentity, ConflictStaleWorkerResult, ConflictIncompatiblePatches:
		return DecisionReject
	case ConflictOverlappingSymbols, ConflictOverlappingFiles:
		return DecisionSequentialize
	case ConflictDependencyChanged:
		return DecisionEscalate
	default:
		return DecisionEscalate
	}
}

// ConflictResolution is the aggregate decision for a conflict finding set,
// together with the non-applicable assignment IDs a caller must not apply.
type ConflictResolution struct {
	// Decision is the aggregate decision. It is non-zero whenever any finding is
	// present.
	Decision ConflictDecision
	// Findings are the input findings in canonical order.
	Findings []ConflictFinding
	// NonApplicable lists, sorted and deduplicated, every assignment ID involved in
	// a conflict. A caller must not mark these applicable.
	NonApplicable []string
}

// Canonicalize returns a stable, order-independent representation of the
// resolution.
func (r ConflictResolution) Canonicalize() string {
	ids := append([]string(nil), r.NonApplicable...)
	sort.Strings(ids)
	return string(r.Decision) + "|" + canonicalizeFindings(r.Findings) + "|" + strings.Join(ids, ",")
}

// DecisionForConflicts returns the explicit orchestration decision for a set of
// findings. For any non-empty finding set the returned decision is non-zero: the
// highest-priority class's canonical decision dominates, and every involved
// assignment is recorded as non-applicable. An empty finding set yields
// DecisionNone.
func DecisionForConflicts(findings []ConflictFinding) ConflictResolution {
	if len(findings) == 0 {
		return ConflictResolution{Decision: DecisionNone}
	}

	ordered := append([]ConflictFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Canonicalize() < ordered[j].Canonicalize() })

	best := DecisionEscalate
	bestPriority := -1
	for _, f := range ordered {
		p, ok := conflictPriority[f.Class]
		if !ok {
			p = 1
		}
		if p > bestPriority {
			bestPriority = p
			best = decisionForClass(f.Class)
		}
	}

	var ids []string
	for _, f := range ordered {
		ids = append(ids, f.Assignments...)
	}
	ids = sortedUnique(ids)

	return ConflictResolution{
		Decision:      best,
		Findings:      ordered,
		NonApplicable: ids,
	}
}

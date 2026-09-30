// Package router is SOP's deterministic model-class router (Phase 3.5).
//
// It maps typed evidence — the SOP task's structure and, when present, typed JEV
// evidence — to one of the configured model classes (small, medium, large). It is
// pure and deterministic: the same inputs always yield the same class and the
// same reasons.
//
// The router follows the repository-wide rule
//
//	evidence -> structured classification -> deterministic SOP policy -> action
//
// and never prose -> string matching -> action. It reads no summary text and
// performs no keyword matching: only typed categories, severities, scope/level
// values, and counts participate. JEV provides evidence; SOP's router decides the
// class, and the model layer (internal/model) resolves the chosen class to a
// concrete provider and model.
//
// The router carries no authority. It is a pure function over data: it never
// transitions task state, mutates the repository, calls a provider, chooses a
// concrete model name, or persists anything. It also never routes on a bare
// confidence threshold (Phase 3.5 §9): a stated confidence is carried as evidence
// only and does not, by itself, select a class.
package router

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
)

// Level is a closed, three-value ordering used for risk and complexity. An
// unknown value is invalid so callers can fail closed rather than default.
type Level string

const (
	LevelLow    Level = "LOW"
	LevelMedium Level = "MEDIUM"
	LevelHigh   Level = "HIGH"
)

// Valid reports whether l is a known level.
func (l Level) Valid() bool {
	switch l {
	case LevelLow, LevelMedium, LevelHigh:
		return true
	default:
		return false
	}
}

// Scope is a closed enumeration of how broad a task's change is.
type Scope string

const (
	// ScopeSingle: the change is expected to stay localized.
	ScopeSingle Scope = "SINGLE_FILE"
	// ScopeMulti: the change is expected to touch several files.
	ScopeMulti Scope = "MULTI_FILE"
	// ScopeCross: the change is cross-cutting (it touches an unexpected area or
	// exceeds the task's scope).
	ScopeCross Scope = "CROSS_CUTTING"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	switch s {
	case ScopeSingle, ScopeMulti, ScopeCross:
		return true
	default:
		return false
	}
}

// Reason phrases. Like model.Reason*, these are a fixed, deterministic set —
// never model-generated prose — so a routing decision stays auditable and is
// never parsed back to drive a decision.
const (
	// ReasonRiskHigh: typed evidence indicates high risk.
	ReasonRiskHigh = "high-risk evidence"
	// ReasonComplexityHigh: typed evidence indicates high complexity.
	ReasonComplexityHigh = "high-complexity evidence"
	// ReasonCrossCutting: the change is cross-cutting.
	ReasonCrossCutting = "cross-cutting change"
	// ReasonMultiFile: the task or evidence implies a multi-file change.
	ReasonMultiFile = "multi-file scope"
	// ReasonSmallTask: a clear, isolated, low-risk task.
	ReasonSmallTask = "isolated low-risk task"
	// ReasonDefault: nothing escalated or qualified; the safe default applies.
	ReasonDefault = "moderate task; default class"
	// ReasonJEVUnavailable: no JEV evidence informed the decision.
	ReasonJEVUnavailable = "JEV evidence unavailable; defaulting to medium"
)

// Small-qualification bounds. They are deliberately conservative: a task only
// routes to the smallest class when it is affirmatively small on every signal.
const (
	// smallMaxCriteria is the largest acceptance-criteria count a SMALL task may
	// have.
	smallMaxCriteria = 3
	// smallMaxFiles is the largest file count a SMALL task may affect.
	smallMaxFiles = 1
)

// TaskSignals are the SOP-owned, structural task signals. They come from the
// task definition (taskfile.Spec), never from prose analysis.
type TaskSignals struct {
	// AcceptanceCriteria is the number of acceptance criteria the task states.
	AcceptanceCriteria int
	// Dependencies is the number of task dependencies the task states.
	Dependencies int
	// FilesAffected is the number of files the task is expected to touch, when
	// known. It is 0 when unknown.
	FilesAffected int
}

// Signals are the typed, deterministic inputs to routing. They are derived from
// two typed sources only — SOP task structure and typed JEV evidence — never from
// free-form prose.
type Signals struct {
	// JEVAvailable reports whether typed JEV evidence informed these signals. It is
	// false when no evidence was produced (the analyzer is disabled, absent, or
	// failed), in which case routing falls back to the safe default.
	JEVAvailable bool
	// Risk is the risk level derived from typed evidence.
	Risk Level
	// Complexity is the complexity level derived from typed evidence.
	Complexity Level
	// Scope is the expected breadth of the change.
	Scope Scope
	// CrossCutting reports a cross-cutting change (scope or unexpected-area
	// evidence, or a cross-cutting scope).
	CrossCutting bool
	// RequiresContext reports that additional context is required before the task
	// can be implemented (missing-context evidence).
	RequiresContext bool
	// Confidence is the analysis confidence, bounded to [0,1], when stated. It is
	// advisory evidence only and never selects a class by itself.
	Confidence float64

	// AcceptanceCriteria is the task's acceptance-criteria count.
	AcceptanceCriteria int
	// Dependencies is the task's dependency count.
	Dependencies int
	// FilesAffected is the number of files the task is expected to touch (0 when
	// unknown).
	FilesAffected int
}

// SignalsFrom derives typed routing signals from typed JEV evidence and the SOP
// task's structural signals. It performs no string matching: only typed
// categories, severities, and counts participate.
//
// ev may be nil (no evidence): the returned signals carry no JEV signal and the
// router resolves to the safe default. An evidence value whose status is not
// valid is treated the same way, so malformed evidence is never misread as a
// clean result.
func SignalsFrom(ev *jev.Evidence, task TaskSignals) Signals {
	s := Signals{
		Risk:               LevelLow,
		Complexity:         LevelLow,
		Scope:              ScopeSingle,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Dependencies:       task.Dependencies,
		FilesAffected:      task.FilesAffected,
	}
	if task.FilesAffected > 1 {
		s.Scope = ScopeMulti
	}
	if ev == nil || !ev.Status.Valid() {
		return s
	}

	s.JEVAvailable = true
	s.Confidence = ev.Confidence

	risk := 0
	complexity := 0
	for _, item := range ev.Items {
		sev := severityScore(item.Severity)

		// Risk: the severity, floored at HIGH for the security-family categories,
		// so a security/destructive/credential/approval concern never routes below
		// the largest class (Phase 3.5 §5: conservative escalation; a security risk
		// overrides the small-task optimization).
		itemRisk := sev
		if securityFamily(item.Category) && itemRisk < 3 {
			itemRisk = 3
		}
		if itemRisk > risk {
			risk = itemRisk
		}

		// Complexity: the severity, raised by the categories that imply the task is
		// harder to reason about.
		itemComplexity := sev
		switch item.Category {
		case jev.CategoryRequirementConflict, jev.CategoryDependencyConcern,
			jev.CategoryAmbiguity, jev.CategoryMissingContext:
			if itemComplexity < 3 {
				itemComplexity = 3
			}
		case jev.CategoryScope, jev.CategoryUnexpectedArea:
			if itemComplexity < 2 {
				itemComplexity = 2
			}
		}
		if itemComplexity > complexity {
			complexity = itemComplexity
		}

		switch item.Category {
		case jev.CategoryScope, jev.CategoryUnexpectedArea:
			s.Scope = ScopeCross
		case jev.CategoryMissingContext:
			s.RequiresContext = true
		}
	}

	s.Risk = levelForScore(risk)
	s.Complexity = levelForScore(complexity)
	if s.Scope == ScopeCross {
		s.CrossCutting = true
	}
	return s
}

// Merge combines two signal sets conservatively: the riskier, more complex,
// broader value wins, and the boolean flags are OR-ed. It is used to fold the
// evidence from more than one early checkpoint into a single routing input, and
// it can only ever escalate — never downgrade — a signal.
func (s Signals) Merge(o Signals) Signals {
	out := s
	out.JEVAvailable = s.JEVAvailable || o.JEVAvailable
	out.Risk = maxLevel(s.Risk, o.Risk)
	out.Complexity = maxLevel(s.Complexity, o.Complexity)
	out.Scope = maxScope(s.Scope, o.Scope)
	out.CrossCutting = s.CrossCutting || o.CrossCutting
	out.RequiresContext = s.RequiresContext || o.RequiresContext
	if o.Confidence > out.Confidence {
		out.Confidence = o.Confidence
	}
	out.AcceptanceCriteria = maxInt(s.AcceptanceCriteria, o.AcceptanceCriteria)
	out.Dependencies = maxInt(s.Dependencies, o.Dependencies)
	out.FilesAffected = maxInt(s.FilesAffected, o.FilesAffected)
	return out
}

// Decision is the deterministic routing outcome: the class SOP's policy selects
// and the fixed-phrase reasons for it. It carries no authority; the model layer
// resolves Class to a concrete provider and model, and SOP's lifecycle remains
// the sole owner of any action taken.
type Decision struct {
	// Class is the selected model class.
	Class model.Class
	// Reasons are the deterministic, ordered explanations for the class. They are
	// fixed phrases, never model-generated prose.
	Reasons []string
}

// Decide maps routing signals to a model class. It is pure and deterministic.
//
// # Precedence (Phase 3.5 §5)
//
//	risk escalation       -> LARGE (a high-risk signal overrides the small-task
//	                         optimization: low complexity + high risk is LARGE)
//	complexity escalation -> LARGE
//	cross-cutting scope   -> LARGE
//	multi-file scope      -> MEDIUM
//	no JEV evidence       -> MEDIUM (the safe default; a safety escalation above
//	                         still wins, because those checks run first)
//	SMALL qualification   -> SMALL
//	otherwise             -> MEDIUM
//
// A bare confidence threshold is never a rule: Confidence is not read here.
func Decide(s Signals) Decision {
	switch {
	case s.Risk == LevelHigh:
		return Decision{Class: model.ClassLarge, Reasons: []string{ReasonRiskHigh}}
	case s.Complexity == LevelHigh:
		return Decision{Class: model.ClassLarge, Reasons: []string{ReasonComplexityHigh}}
	case s.CrossCutting:
		return Decision{Class: model.ClassLarge, Reasons: []string{ReasonCrossCutting}}
	case s.Scope == ScopeMulti:
		return Decision{Class: model.ClassMedium, Reasons: []string{ReasonMultiFile}}
	case !s.JEVAvailable:
		return Decision{Class: model.ClassMedium, Reasons: []string{ReasonJEVUnavailable}}
	case s.qualifiesSmall():
		return Decision{Class: model.ClassSmall, Reasons: []string{ReasonSmallTask}}
	default:
		return Decision{Class: model.ClassMedium, Reasons: []string{ReasonDefault}}
	}
}

// qualifiesSmall reports whether a task is affirmatively small on every signal,
// so it is safe to route to the smallest class. It requires typed JEV evidence:
// without evidence the router never selects SMALL (Phase 3.5 §9).
func (s Signals) qualifiesSmall() bool {
	return s.JEVAvailable &&
		s.Risk == LevelLow &&
		s.Complexity == LevelLow &&
		s.Scope == ScopeSingle &&
		!s.CrossCutting &&
		!s.RequiresContext &&
		s.FilesAffected <= smallMaxFiles &&
		s.AcceptanceCriteria <= smallMaxCriteria &&
		s.Dependencies == 0
}

// levelForScore maps an internal 0..4 score to a level: 0-1 LOW, 2 MEDIUM, 3+
// HIGH. It is the single place the score thresholds live.
func levelForScore(score int) Level {
	switch {
	case score >= 3:
		return LevelHigh
	case score == 2:
		return LevelMedium
	default:
		return LevelLow
	}
}

// severityScore ranks a typed severity 0..4. An unknown severity scores 0 so it
// never escalates on its own.
func severityScore(s jev.Severity) int {
	switch s {
	case jev.SeverityCritical:
		return 4
	case jev.SeverityHigh:
		return 3
	case jev.SeverityMedium:
		return 2
	case jev.SeverityLow:
		return 1
	case jev.SeverityInfo:
		return 0
	default:
		return 0
	}
}

// securityFamily reports whether a typed category is security-sensitive, so it
// never routes to the smallest class.
func securityFamily(c jev.Category) bool {
	switch c {
	case jev.CategorySecurity, jev.CategoryDestructive,
		jev.CategoryCredentialSensitivity, jev.CategoryApprovalSensitive:
		return true
	default:
		return false
	}
}

// maxLevel returns the higher of two levels (HIGH > MEDIUM > LOW).
func maxLevel(a, b Level) Level {
	if levelRank(a) >= levelRank(b) {
		return a
	}
	return b
}

// levelRank orders levels. An unknown level ranks lowest.
func levelRank(l Level) int {
	switch l {
	case LevelHigh:
		return 2
	case LevelMedium:
		return 1
	case LevelLow:
		return 0
	default:
		return -1
	}
}

// maxScope returns the broader of two scopes (CROSS > MULTI > SINGLE).
func maxScope(a, b Scope) Scope {
	if scopeRank(a) >= scopeRank(b) {
		return a
	}
	return b
}

// scopeRank orders scopes. An unknown scope ranks lowest.
func scopeRank(s Scope) int {
	switch s {
	case ScopeCross:
		return 2
	case ScopeMulti:
		return 1
	case ScopeSingle:
		return 0
	default:
		return -1
	}
}

// maxInt returns the larger of two ints.
func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// ReasonsText joins the reasons into one deterministic sentence for a
// single-line display. It never alters the reason phrases.
func ReasonsText(reasons []string) string {
	return strings.Join(reasons, "; ")
}

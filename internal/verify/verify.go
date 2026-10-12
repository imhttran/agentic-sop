// Package verify is SOP's deterministic, model-free verification risk tier.
//
// It maps typed evidence to one of three tiers — LOW, MEDIUM, HIGH — that select
// how much verification a task requires: which configured checks run and whether
// JEV is required. It is pure: it performs no I/O, calls no model, and transitions
// no state. The tier never decides a gate verdict; the deterministic quality gate
// still owns PASS/FAIL/NEEDS_HUMAN.
//
// The tier is escalate-only and fail-closed: the conservative (higher) reading
// always wins, a high-risk/complexity/cross-cutting signal never derives below
// HIGH, and absent evidence never derives LOW unless the operator explicitly
// declared the task verify-first. A model cannot select or lower a tier.
package verify

import "github.com/imhttran/agentic-sop/internal/router"

// Tier is a closed, ordered verification risk tier.
type Tier string

const (
	// Low: an isolated, low-risk task (or an operator-declared verify-first task).
	// Verification may use a targeted command subset.
	Low Tier = "LOW"
	// Medium: the safe default. Verification runs the configured tests+build set.
	Medium Tier = "MEDIUM"
	// High: a high-risk, high-complexity, or cross-cutting change. Verification runs
	// the full configured set, requires executed acceptance checks, and requires JEV
	// when JEV is enabled.
	High Tier = "HIGH"
)

// Valid reports whether t is a known tier.
func (t Tier) Valid() bool {
	switch t {
	case Low, Medium, High:
		return true
	default:
		return false
	}
}

// rank orders tiers (HIGH > MEDIUM > LOW). An unknown tier ranks lowest.
func (t Tier) rank() int {
	switch t {
	case High:
		return 2
	case Medium:
		return 1
	case Low:
		return 0
	default:
		return -1
	}
}

// Max returns the higher of two tiers. It is the escalate-only merge: a tier can
// only ever be raised, never lowered.
func Max(a, b Tier) Tier {
	if a.rank() >= b.rank() {
		return a
	}
	return b
}

// RequiresJEV reports whether the tier requires JEV when JEV is enabled. JEV is
// optional at LOW/MEDIUM and required at HIGH; whether JEV is enabled at all is
// the caller's decision.
func (t Tier) RequiresJEV() bool {
	return t == High
}

// Reason phrases. Like the router's, these are a fixed, deterministic set — never
// model-generated prose — so a tier stays auditable.
const (
	// ReasonRiskHigh: typed evidence indicates high risk.
	ReasonRiskHigh = "high-risk evidence"
	// ReasonComplexityHigh: typed evidence indicates high complexity.
	ReasonComplexityHigh = "high-complexity evidence"
	// ReasonCrossCutting: the change is cross-cutting.
	ReasonCrossCutting = "cross-cutting change"
	// ReasonVerifyFirst: the operator declared the task verify-first.
	ReasonVerifyFirst = "operator-declared verify-first task"
	// ReasonLowRisk: typed evidence affirmatively establishes a low risk.
	ReasonLowRisk = "isolated low-risk task"
	// ReasonNoEvidence: no typed low-risk evidence was available; the safe default
	// applies.
	ReasonNoEvidence = "no typed low-risk evidence; default verification tier"
	// ReasonDefault: nothing escalated or qualified; the safe default applies.
	ReasonDefault = "moderate task; default verification tier"
)

// Small-qualification bounds for a LOW tier from typed evidence alone. They mirror
// the model router's deliberately conservative SMALL bounds: a task qualifies LOW
// only when it is affirmatively small on every signal.
const (
	smallMaxCriteria = 3
	smallMaxFiles    = 1
)

// Derive maps typed routing signals and the task's declared execution mode to a
// verification tier and the fixed-phrase reasons for it. It is pure and
// deterministic.
//
// Precedence (escalate-only):
//
//	high risk         -> HIGH (a high-risk signal overrides the low-risk shortcut)
//	high complexity   -> HIGH
//	cross-cutting     -> HIGH
//	verify-first      -> LOW  (an operator-declared, deterministic-validation task)
//	low-risk evidence -> LOW
//	otherwise         -> MEDIUM (the safe default)
//
// Risk signals are typed (router.Signals), never prose. Missing evidence never
// derives LOW: an unevidenced task defaults to MEDIUM unless it is explicitly
// declared verify-first.
func Derive(risk router.Signals, verifyFirst bool) (Tier, []string) {
	switch {
	case risk.Risk == router.LevelHigh:
		return High, []string{ReasonRiskHigh}
	case risk.Complexity == router.LevelHigh:
		return High, []string{ReasonComplexityHigh}
	case risk.CrossCutting:
		return High, []string{ReasonCrossCutting}
	}
	if verifyFirst {
		return Low, []string{ReasonVerifyFirst}
	}
	if risk.JEVAvailable &&
		risk.Risk == router.LevelLow &&
		risk.Complexity == router.LevelLow &&
		risk.Scope == router.ScopeSingle &&
		!risk.RequiresContext &&
		risk.FilesAffected <= smallMaxFiles &&
		risk.AcceptanceCriteria <= smallMaxCriteria &&
		risk.Dependencies == 0 {
		return Low, []string{ReasonLowRisk}
	}
	if !risk.JEVAvailable {
		return Medium, []string{ReasonNoEvidence}
	}
	return Medium, []string{ReasonDefault}
}

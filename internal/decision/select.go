package decision

import (
	"fmt"
	"sort"
)

// This file adds SOP's deterministic engineering-approach selection: given several
// candidate approaches, SOP selects the best one from evidence, correctness, risk,
// reversibility, and execution cost — without a human for routine choices — and
// escalates only on a genuine security, authorization, or scope boundary. It reuses
// the package's Choice/Target vocabulary (a winner maps to a model tier; only a
// boundary or a lack of evidence escalates to a human).
//
// internal/decision is an architecture-guarded leaf and may import no internal
// package, so the risk vocabulary is mirrored here and mapped from
// internal/autonomy.ApprovalRisk at the cli boundary.

// EvidenceLevel is how well an approach is supported by verified, deterministic
// evidence (its correctness support). A model's opinion is not evidence.
type EvidenceLevel string

const (
	// EvidenceNone: no deterministic support; the approach must not be selected.
	EvidenceNone EvidenceLevel = "NONE"
	// EvidencePartial: some support, not yet conclusive.
	EvidencePartial EvidenceLevel = "PARTIAL"
	// EvidenceStrong: independently verified / proven correct.
	EvidenceStrong EvidenceLevel = "STRONG"
)

// Risk mirrors autonomy.ApprovalRisk (NONE < LOW < MEDIUM < HIGH < IRREVERSIBLE).
type Risk string

const (
	RiskNone         Risk = "NONE"
	RiskLow          Risk = "LOW"
	RiskMedium       Risk = "MEDIUM"
	RiskHigh         Risk = "HIGH"
	RiskIrreversible Risk = "IRREVERSIBLE"
)

// Boundary marks an approach that crosses a genuine authority boundary and must
// never be selected automatically.
type Boundary string

const (
	BoundaryNone          Boundary = ""
	BoundarySecurity      Boundary = "SECURITY"
	BoundaryAuthorization Boundary = "AUTHORIZATION"
	BoundaryScope         Boundary = "SCOPE"
)

// Alternative is one candidate engineering approach, described with only
// provider-neutral, deterministic attributes.
type Alternative struct {
	// Name identifies the approach.
	Name string
	// Evidence is how well the approach is supported by deterministic evidence.
	Evidence EvidenceLevel
	// Risk is the consequence if the approach is wrong.
	Risk Risk
	// Reversible reports whether the approach can be safely undone.
	Reversible bool
	// Cost is the relative execution cost; lower is cheaper. It is a tiebreak only.
	Cost float64
	// Boundary marks a genuine security/authorization/scope boundary.
	Boundary Boundary
}

// Selection is the deterministic verdict: the choice, where it routes, the selected
// approach, the reason, and the ranked alternatives (best first) for the record.
type Selection struct {
	Choice       Choice
	Target       Target
	Approach     string
	Reason       string
	Alternatives []Alternative
}

// Select chooses the best approach. It never consults a model or a confidence
// score. Rules, in order:
//
//  1. An alternative that crosses a boundary is never auto-selected.
//  2. If every alternative crosses a boundary, or none is supported by evidence,
//     escalate to a human (Choice HUMAN / Target HUMAN).
//  3. Otherwise rank the safe alternatives by evidence (desc), reversibility
//     (reversible first), risk (asc), and cost (asc) — so when the leading
//     alternatives are close (same evidence), the safest reversible one wins.
//  4. A strong, low-risk, reversible winner routes to the small model (routine);
//     any other auto winner routes to the strong model.
func Select(alts []Alternative) Selection {
	if len(alts) == 0 {
		return escalate(nil, "no alternatives were provided")
	}
	safe := make([]Alternative, 0, len(alts))
	for _, a := range alts {
		if a.Boundary == BoundaryNone {
			safe = append(safe, a)
		}
	}
	if len(safe) == 0 {
		return escalate(alts, "every alternative crosses a security, authorization, or scope boundary")
	}
	ranked := append([]Alternative(nil), safe...)
	sort.SliceStable(ranked, func(i, j int) bool { return better(ranked[i], ranked[j]) })

	best := ranked[0]
	if best.Evidence == EvidenceNone {
		return escalate(alts, "no alternative is supported by deterministic evidence")
	}
	choice := Medium
	if best.Evidence == EvidenceStrong && riskRank(best.Risk) <= riskRank(RiskLow) && best.Reversible {
		choice = Low
	}
	return Selection{
		Choice:       choice,
		Target:       targetFor(choice),
		Approach:     best.Name,
		Reason:       fmt.Sprintf("selected %q: evidence %s, risk %s, reversible=%t, cost %.2f", best.Name, best.Evidence, best.Risk, best.Reversible, best.Cost),
		Alternatives: ranked,
	}
}

// better reports whether a ranks before b.
func better(a, b Alternative) bool {
	if a.Evidence != b.Evidence {
		return evidenceRank(a.Evidence) > evidenceRank(b.Evidence)
	}
	if a.Reversible != b.Reversible {
		return a.Reversible
	}
	if a.Risk != b.Risk {
		return riskRank(a.Risk) < riskRank(b.Risk)
	}
	return a.Cost < b.Cost
}

func evidenceRank(e EvidenceLevel) int {
	switch e {
	case EvidenceStrong:
		return 2
	case EvidencePartial:
		return 1
	default:
		return 0
	}
}

func riskRank(r Risk) int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskIrreversible:
		return 4
	default:
		return 0
	}
}

func targetFor(c Choice) Target {
	switch c {
	case Low:
		return SmallModel
	case Medium:
		return StrongModel
	default:
		return HumanTarget
	}
}

// escalate builds a fail-closed selection that requires a human.
func escalate(alts []Alternative, reason string) Selection {
	return Selection{Choice: Human, Target: HumanTarget, Reason: reason, Alternatives: alts}
}

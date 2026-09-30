package autonomy

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// Early JEV evidence policy (Phase 3).
//
// This is the deterministic bridge between early JEV evidence and a lifecycle
// disposition. It follows the repository-wide rule
// `JEV evidence -> structured classification -> deterministic SOP policy ->
// lifecycle action`, and never `JEV prose -> string matching -> lifecycle
// action` (PRD-Phase-3-OpenJEV FR-P3-7):
//
//   - the escalation trigger is the typed (category, severity) pair matched
//     against the configured fail_on severities, never free-form summary text;
//   - a typed category maps into the existing failure.Kind vocabulary;
//   - the existing autonomy.Decide produces the disposition, so evidence
//     interpretation and disposition stay in one SOP-owned place.
//
// JEV provides evidence; SOP decides. This function only classifies and applies
// policy: it never transitions task state, mutates persistence, or runs anything.

// DecideEarly maps structured early JEV evidence into a deterministic disposition.
// It is pure and deterministic: the same (evidence, failOn, policy) always yields
// the same Decision.
//
// # Decision-input contract
//
// The disposition is a function of typed, structured values only:
//
//   - inputs: jev.Evidence.Purpose and jev.Evidence.Status (context), each
//     jev.EvidenceItem.Severity and jev.EvidenceItem.Category (the decision
//     inputs), the failOn severity set, and the resolved autonomy Policy.
//   - outputs (every field of autonomy.Decision): Action, Level, Risk,
//     RequiresHuman, Classification (Kind/Disposition/Confidence/Reason), and
//     Reason.
//
// No prose field participates in the decision. jev.Evidence.Summary and
// jev.EvidenceItem.Detail / jev.EvidenceItem.Evidence are provenance only: this
// path performs no string matching on them, and an item's Detail is appended to
// the decision Reason as description only — after the action, risk, and human
// boundary have already been fully determined by the typed fields.
//
// # Disposition rules
//
// When no finding's severity is in failOn the task continues (the advisory
// default): a clear or low-severity task proceeds without human intervention.
// Severity values that are unknown or absent never match failOn, so a malformed
// severity is non-blocking and the task continues. When a blocking finding is
// present, its typed category selects a failure kind and the existing Decide
// produces the action — which, for the authority boundaries (ambiguity,
// security, destructive, approval), is a human boundary; an empty or unknown
// category fails closed to failure.BlockingFindings (also a human boundary). A
// destructive/security concern therefore reaches a human boundary only when the
// configured fail_on policy includes the finding's severity.
//
// # fail_on input contract
//
// failOn is caller-supplied and is the only policy input for severity gating.
// internal/autonomy does not read configuration or build this list itself:
// supplying the early fail_on severities (from configuration or the lifecycle
// caller) is owned outside this package. Entries are matched against the typed
// severity case-insensitively and whitespace-tolerantly.
//
// Confidence is deliberately not a decision input here: it is advisory metadata
// carried on the evidence, and a bare confidence threshold is not a rule.
func DecideEarly(ev jev.Evidence, failOn []string, p Policy) Decision {
	item, ok := earlyBlockingFinding(ev, failOn)
	if !ok {
		return Decision{
			Action: ActionAutoContinue,
			Level:  p.Level,
			Risk:   RiskLow,
			Reason: "early JEV evidence raises no blocking finding; the task continues under deterministic policy",
		}
	}
	c := failure.Classification{
		Kind:        kindForEarlyCategory(item.Category),
		Disposition: failure.NeedsHuman,
		Confidence:  failure.High,
		Reason:      earlyFindingReason(item),
	}
	return Decide(c, p)
}

// earlyBlockingFinding returns the highest-severity finding whose severity is
// named in failOn, and whether one exists. Severity matching is case-insensitive
// so the lowercase configuration vocabulary (critical, high) matches the
// uppercase evidence vocabulary. Only the typed severity is compared: no summary
// or detail text participates in the decision.
func earlyBlockingFinding(ev jev.Evidence, failOn []string) (jev.EvidenceItem, bool) {
	var best jev.EvidenceItem
	found := false
	for _, item := range ev.Items {
		if !severityBlocking(item.Severity, failOn) {
			continue
		}
		if !found || severityRankEarly(item.Severity) > severityRankEarly(best.Severity) {
			best, found = item, true
		}
	}
	return best, found
}

// severityBlocking reports whether s is one of the failOn severities. Only the
// typed severity participates: an unknown or empty severity never matches, so it
// is never treated as blocking.
func severityBlocking(s jev.Severity, failOn []string) bool {
	for _, f := range failOn {
		if strings.EqualFold(strings.TrimSpace(f), string(s)) {
			return true
		}
	}
	return false
}

// severityRankEarly orders the typed severities so the most serious blocking
// finding is chosen. An unknown severity ranks lowest and is never blocking,
// because it cannot match failOn.
func severityRankEarly(s jev.Severity) int {
	switch s {
	case jev.SeverityInfo:
		return 1
	case jev.SeverityLow:
		return 2
	case jev.SeverityMedium:
		return 3
	case jev.SeverityHigh:
		return 4
	case jev.SeverityCritical:
		return 5
	default:
		return 0
	}
}

// kindForEarlyCategory maps a typed early category to the existing failure.Kind
// vocabulary. An empty or unknown category fails closed to BlockingFindings (a
// human boundary) rather than a locally-fixable action, so an unrecognized
// category can never silently auto-advance.
func kindForEarlyCategory(c jev.Category) failure.Kind {
	switch c {
	case jev.CategoryAmbiguity, jev.CategoryMissingContext,
		jev.CategoryRequirementConflict, jev.CategoryDependencyConcern:
		return failure.AmbiguousContract
	case jev.CategorySecurity, jev.CategoryCredentialSensitivity:
		return failure.SecurityBoundary
	case jev.CategoryDestructive:
		return failure.DestructiveOperation
	case jev.CategoryApprovalSensitive:
		return failure.ApprovalRequired
	default:
		return failure.BlockingFindings
	}
}

// earlyFindingReason builds a deterministic reason from the typed finding fields.
// The category and severity drive the classification; the item's detail is
// appended as descriptive provenance only, so the reason explains WHY SOP
// stopped without the reason text influencing the decision.
func earlyFindingReason(item jev.EvidenceItem) string {
	category := string(item.Category)
	if category == "" {
		category = "unspecified"
	}
	reason := fmt.Sprintf("early JEV evidence (%s, %s) crosses an authority boundary that requires a human",
		category, strings.ToLower(string(item.Severity)))
	if d := strings.TrimSpace(item.Detail); d != "" {
		reason += ": " + d
	}
	return reason
}

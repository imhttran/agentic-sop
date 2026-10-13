package cli

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// AdvisoryOutcome is one advisory decision-layer evaluation, produced for tooling
// (the decision-model effectiveness evaluation). It records the advisory
// recommendation and the final governed action SEPARATELY.
type AdvisoryOutcome struct {
	BaselineAction string
	FinalAction    string
	Escalated      bool
	Consulted      bool
	OutcomeClass   string
	Choice         string
	Confidence     float64
	FailureLabel   string
	ServedProvider string
	FallbackReason string
	LatencyMS      int
}

// EvaluateAdvisoryDecision runs the SEAM-004 advisory checkpoint exactly once for
// a deterministic classification, reusing the production path
// (applyDecisionEvidence + the configured decision provider). It is diagnostic
// tooling: it never transitions task state, never touches the repository, and
// never changes governance. The advisory recommendation can only ADD attention;
// the final action is the governed action the lifecycle would take, expressed in
// the failure-disposition vocabulary (AUTO_FIX/CONTINUE/RETRY/REPLAN/NEEDS_HUMAN/
// BLOCK) so baseline and advisory are directly comparable.
func EvaluateAdvisoryDecision(ctx context.Context, cfg config.Config, cls failure.Classification, spec *taskfile.Spec) AdvisoryOutcome {
	base := string(cls.Disposition)
	out := AdvisoryOutcome{BaselineAction: base, FinalAction: base}

	d := deps{newDecisionProvider: decisionProviderFromConfig}
	res := lifeResult{classification: cls}
	var doc *decisionEvidenceDoc
	res = applyDecisionEvidence(ctx, cfg, d, spec, res, func(ed *decisionEvidenceDoc) { doc = ed })

	// applyDecisionEvidence returns the escalation in the autonomy vocabulary; map
	// it back to the disposition vocabulary for comparison. A non-escalating call
	// leaves res.decision empty, so the final action stays the deterministic one.
	if res.decision.Action != "" {
		out.FinalAction = dispositionForAction(res.decision.Action)
	}
	if doc != nil {
		out.Consulted = doc.Consulted
		out.OutcomeClass = doc.OutcomeClass
		out.Choice = doc.Choice
		out.Confidence = doc.Confidence
		out.FailureLabel = doc.FailureLabel
		out.ServedProvider = doc.ServedProvider
		out.FallbackReason = doc.FallbackReason
		out.LatencyMS = doc.LatencyMS
		out.Escalated = doc.Escalated
	}
	return out
}

// dispositionForAction maps an autonomy action to the failure-disposition
// vocabulary so an advisory final action is comparable to the deterministic
// classification and the frozen labels.
func dispositionForAction(a autonomy.Action) string {
	switch a {
	case autonomy.ActionAutoRetry:
		return string(failure.Retry)
	case autonomy.ActionAutoContinue:
		return string(failure.Continue)
	case autonomy.ActionAutoFix:
		return string(failure.AutoFix)
	case autonomy.ActionAutoReconcile:
		return string(failure.Replan)
	case autonomy.ActionTerminal:
		return string(failure.Block)
	case autonomy.ActionHumanApproval:
		return string(failure.NeedsHuman)
	default:
		return string(a)
	}
}

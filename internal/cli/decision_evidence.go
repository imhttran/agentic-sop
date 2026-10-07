package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// Optional external decision-evidence checkpoint (SEAM-004).
//
// This is the single place the lifecycle lets an optional, provider-neutral
// external decision provider add ATTENTION to a governed result. It follows the
// repository-wide rule "providers evaluate; SOP governs":
//
//   - the provider returns bounded evidence (a choice and a confidence), never an
//     action, and the SOP-owned validator (decision.Validate) must accept it;
//   - SOP interprets the evidence deterministically into a typed
//     failure.Classification and lets the existing autonomy.Decide produce the
//     verdict, so there is exactly one policy interpretation path;
//   - the evidence can only ADD attention — raise an automated result to a human
//     boundary. It can never reduce risk, authorize execution, approve work,
//     bypass validation/review/human approval, or choose a lifecycle transition;
//   - the capability is OFF by default, so an installation with no decision
//     configuration is a strict no-op exactly as before, and no model or process
//     is consulted.
//
// It is read-only with respect to SOP state: it reads a bounded request and
// returns an autonomy decision (a value the existing lifecycle acts on); it never
// transitions task state, mutates persistence, or touches the repository. It
// contains no provider-specific logic — no Clef/Nimble/oMLX/Ollama/SystemOne
// branch, and no branch on a provider, model, or executable name.

const (
	// decisionEvidenceTimeout bounds a single external decision-provider call, so a
	// hung or slow provider cannot stall the lifecycle. SOP owns the deadline; the
	// provider's own timeout is not trusted.
	decisionEvidenceTimeout = 60 * time.Second
	// decisionEvidenceKind is the provider-neutral decision kind SOP requests. It is
	// an opaque label: the contract does not prescribe a provider-specific taxonomy,
	// and SOP policy never branches on a provider or model name.
	decisionEvidenceKind = "change-risk"
	// decisionEvidenceSubjectLimit bounds the free-text subject sent to the
	// provider, so a large task or run cannot produce an unbounded request.
	decisionEvidenceSubjectLimit = 2048
)

// decisionEvidenceEnabled reports whether the optional external decision-evidence
// capability is active. It is OFF by default, so an installation with no decision
// configuration is never consulted and behaves exactly as before.
func decisionEvidenceEnabled(cfg config.Config) bool {
	return cfg.Decision.Enabled
}

// applyDecisionEvidence is the optional, disabled-by-default evidence checkpoint
// on a governed lifecycle result. When the capability is disabled or absent, or
// the result already withholds execution, or the provider contributes no adverse
// evidence, it returns res UNCHANGED (a strict no-op). Otherwise it replaces the
// result's decision with a human boundary produced by the single policy path
// (autonomy.Decide), so the provider can only add attention.
func applyDecisionEvidence(ctx context.Context, cfg config.Config, d deps, spec *taskfile.Spec, res lifeResult) lifeResult {
	if strings.TrimSpace(string(res.classification.Disposition)) == "" {
		// A passing run (or no classification) has no failure to attend to; the
		// checkpoint is a strict no-op.
		return res
	}
	base := res.decision
	if base.Action == "" {
		// Mirror the driver's own fallback, so the baseline the checkpoint reasons
		// about is the same one the lifecycle would act on.
		base = autonomy.Decide(res.classification, cfg.AutonomyPolicy())
	}
	if !attentionable(base) {
		// The governed baseline already withholds execution (a human boundary or a
		// terminal stop). Provider evidence may not change it: evidence can only add
		// attention to a result that would otherwise proceed automatically.
		return res
	}
	escalation, ok, failure := decisionEvidenceEscalation(ctx, cfg, d, spec, res.classification)
	if failure != nil {
		// A CONFIGURED provider was consulted and could not supply usable evidence.
		// Record it distinctly so "provider failed" is never silently identical to "no
		// provider configured" — but the governed baseline always stands, and a provider
		// failure is never a success, an approval, or an authorization.
		emitDecisionEvidenceUnavailable(ctx, failure)
		return res
	}
	if !ok {
		return res
	}
	res.decision = escalation
	return res
}

// attentionable reports whether a governed decision would proceed automatically
// and may therefore be escalated to a human boundary. A human boundary, a terminal
// stop, or an empty decision is NOT attentionable: the provider can only make SOP
// MORE conservative, never less.
func attentionable(d autonomy.Decision) bool {
	switch d.Action {
	case autonomy.ActionAutoRetry, autonomy.ActionAutoContinue, autonomy.ActionAutoFix, autonomy.ActionAutoReconcile:
		return true
	default:
		return false
	}
}

// decisionEvidenceEscalation consults the optional decision provider. It returns the
// escalation when a configured provider contributes adverse evidence (ok=true),
// and otherwise ok=false. When a CONFIGURED provider is consulted but fails — as
// opposed to being absent or disabled — it also returns a non-nil failure so the
// caller can RECORD it distinctly; that failure is never authority, and the governed
// baseline always stands. Absent/disabled is (zero, false, nil): a strict no-op
// indistinguishable from pre-SEAM behavior.
func decisionEvidenceEscalation(ctx context.Context, cfg config.Config, d deps, spec *taskfile.Spec, cls failure.Classification) (autonomy.Decision, bool, error) {
	if !decisionEvidenceEnabled(cfg) || d.newDecisionProvider == nil {
		return autonomy.Decision{}, false, nil
	}
	provider, err := d.newDecisionProvider(cfg)
	if err != nil {
		// A misconfigured provider is a failure, recorded distinctly; deterministic
		// policy owns the outcome exactly as before.
		return autonomy.Decision{}, false, err
	}
	if provider == nil {
		// No provider configured: a strict no-op, indistinguishable from disabled.
		return autonomy.Decision{}, false, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, decisionEvidenceTimeout)
	defer cancel()
	dec, err := provider.Decide(callCtx, decisionEvidenceRequest(spec, cls))
	if err != nil {
		// Unavailable, timed out, cancelled, unsupported, indeterminate, malformed, or
		// a process failure: never a success, never an approval. The governed baseline
		// stands.
		return autonomy.Decision{}, false, err
	}
	// SOP owns validation: the provider is never trusted to validate its own result.
	if err := decision.Validate(dec); err != nil {
		return autonomy.Decision{}, false, err
	}
	if !decisionEvidenceAdverse(dec) {
		// Benign or lower-attention evidence adds nothing: a provider can never make
		// SOP less conservative than the governed baseline. Confidence is evidence,
		// never authorization, so a high-confidence benign choice still adds nothing.
		return autonomy.Decision{}, false, nil
	}
	return autonomy.Decide(decisionEvidenceClassification(dec), cfg.AutonomyPolicy()), true, nil
}

// emitDecisionEvidenceUnavailable records on the activity stream that a configured
// external decision provider could not supply usable evidence. It is diagnostic
// only: it labels the failure by its SOP-owned sentinel and never copies the
// provider's own text, so no diagnostic or secret leaks into the stream, and it
// never changes the decision. It is a no-op when no recorder is present.
func emitDecisionEvidenceUnavailable(ctx context.Context, err error) {
	rec := activity.FromContext(ctx)
	if rec == nil {
		return
	}
	rec.Emit(activity.StageAutonomy, "decision-evidence-unavailable", decisionEvidenceFailureLabel(err))
}

// decisionEvidenceFailureLabel maps a provider/validation failure to a short,
// secret-free, provider-neutral label. It reads only SOP-owned sentinels, never the
// provider's error text.
func decisionEvidenceFailureLabel(err error) string {
	switch {
	case errors.Is(err, decision.ErrUnsupportedCapability):
		return "unsupported capability"
	case errors.Is(err, decision.ErrInvalidResult):
		return "invalid result"
	case errors.Is(err, decision.ErrIndeterminate):
		return "indeterminate result"
	case errors.Is(err, decision.ErrProviderResult):
		return "provider-reported error"
	default:
		return "provider unavailable"
	}
}

// decisionEvidenceAdverse reports whether validated provider evidence adds
// attention. Only a HIGH or HUMAN choice is adverse; it mirrors the existing,
// provider-neutral decision.Route rule (HUMAN or HIGH → a human). LOW and MEDIUM
// add nothing, so the provider can never make SOP less conservative.
func decisionEvidenceAdverse(d decision.Decision) bool {
	return d.Choice == decision.High || d.Choice == decision.Human
}

// decisionEvidenceClassification translates validated adverse provider evidence
// into a typed failure.Classification so the existing autonomy.Decide produces the
// verdict (SEAM-004 Option A). It reuses the existing "unresolved external finding"
// vocabulary: the disposition is forced to NEEDS_HUMAN, so Decide fails closed to a
// human boundary and the provider cannot select a policy outcome directly.
func decisionEvidenceClassification(d decision.Decision) failure.Classification {
	return failure.Classification{
		Kind:        failure.BlockingFindings,
		Disposition: failure.NeedsHuman,
		Confidence:  failure.High,
		Reason:      decisionEvidenceReason(d.Choice),
	}
}

// decisionEvidenceReason builds a deterministic, secret-free, provider-neutral
// reason from the validated choice. It never includes a provider/model/executable
// name or raw provider diagnostics, so provider identity cannot affect the policy
// result and advisory text is never read as policy.
func decisionEvidenceReason(c decision.Choice) string {
	return fmt.Sprintf("external decision evidence (%s) raises attention that requires a human; provider evidence can add attention but never authorize execution", c)
}

// decisionEvidenceRequest builds the bounded, provider-neutral request an external
// decision provider decides on. It carries only task-scoped context and the closed
// choice vocabulary: no lifecycle handle, no persistence handle, no approval,
// commit, merge, or policy directive.
func decisionEvidenceRequest(spec *taskfile.Spec, cls failure.Classification) decision.Request {
	req := decision.Request{
		UseCase: decisionEvidenceKind,
		Subject: decisionEvidenceSubject(spec, cls),
		Choices: []decision.Choice{decision.Low, decision.Medium, decision.High, decision.Human},
	}
	if spec != nil {
		req.Signals = map[string]float64{
			"criteria":     float64(len(spec.AcceptanceCriteria)),
			"deliverables": float64(len(spec.Deliverables)),
		}
	}
	return req
}

// decisionEvidenceSubject assembles the bounded free-text subject of the decision
// from task-scoped fields and the typed classification kind. It reads no run prose
// and no provider output, and it is truncated to a fixed bound.
func decisionEvidenceSubject(spec *taskfile.Spec, cls failure.Classification) string {
	parts := make([]string, 0, 3)
	if spec != nil {
		if t := strings.TrimSpace(spec.Title); t != "" {
			parts = append(parts, t)
		}
		if d := strings.TrimSpace(spec.Description); d != "" {
			parts = append(parts, d)
		}
	}
	if k := strings.TrimSpace(string(cls.Kind)); k != "" {
		parts = append(parts, k)
	}
	s := strings.Join(parts, "\n")
	if len(s) > decisionEvidenceSubjectLimit {
		s = s[:decisionEvidenceSubjectLimit]
	}
	return strings.TrimSpace(s)
}

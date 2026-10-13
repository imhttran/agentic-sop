package cli

// DME-005 EVALUATION-ONLY failure-recovery advisory path.
//
// The production SEAM-004 checkpoint asks a change-RISK question (LOW/MEDIUM/
// HIGH/HUMAN) over the "change-risk" kind. The DME-005 experiment asks whether a
// failure-RECOVERY question decides better. To measure that faithfully it routes
// the recovery question through the REAL advisory machinery — the commandprofile
// adapter connector, SOP's own validator, the adverse rule, the attention
// (`attentionable`) gate, and autonomy.Decide — changing only the question kind
// and its choice mapping.
//
// This file changes NOTHING in production: the change-risk kind, the production
// registry (decision_adapter_profile.go), the request builder
// (decisionEvidenceRequest), governance, routing, and escalation thresholds are
// all untouched. It reuses applyDecisionEvidence verbatim; only the decision
// provider is swapped for an eval-only one.
//
// Choice mapping (so the add-attention clamp is preserved exactly): AUTO_FIX and
// CONTINUE map to benign canonical choices (they add nothing); HUMAN maps to
// decision.Human, which is the one adverse choice. The provider can therefore only
// ADD attention, never lower it.

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
	"github.com/imhttran/agentic-sop/internal/decision/commandprofile"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// recoveryEvalKind is the evaluation-only question kind. It is NOT the production
// decisionEvidenceKind and is never used by the governed lifecycle.
const recoveryEvalKind = "failure-recovery"

// Recovery question choices (native, adapter-facing).
const (
	recoveryEvalAutoFix  = "AUTO_FIX"
	recoveryEvalContinue = "CONTINUE"
	recoveryEvalHuman    = "HUMAN"
)

// recoveryEvalRegistry builds an eval-only M3 registry that serves the
// failure-recovery kind for the selected adapter. The profile name must equal the
// adapter's reported provider, which the connector masks to the configured
// adapter provider.
func recoveryEvalRegistry(cfg config.Config) (*bridge.Registry, error) {
	name := strings.TrimSpace(cfg.Decision.AdapterProvider)
	if name == "" {
		name = "nimble"
	}
	return bridge.NewRegistry(&bridge.Profile{Name: name, Version: 1, Kinds: map[string]bridge.KindSpec{
		recoveryEvalKind: {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
			recoveryEvalAutoFix:  decision.Low,
			recoveryEvalContinue: decision.Medium,
			recoveryEvalHuman:    decision.Human,
		}},
	}}), nil
}

// newRecoveryEvalProvider builds the REAL adapter connector (commandprofile) with
// the eval-only recovery registry, wrapped so that the canonical change-risk
// request applyDecisionEvidence builds is rewritten to the failure-recovery
// question. It returns (nil, nil) when the capability is disabled or the adapter
// transport is not selected, mirroring decisionProviderFromConfig.
func newRecoveryEvalProvider(cfg config.Config) (decision.Provider, error) {
	if !cfg.Decision.DecisionEnabled() {
		return nil, nil
	}
	if strings.TrimSpace(cfg.Decision.Provider) != "adapter" {
		return nil, nil
	}
	reg, err := recoveryEvalRegistry(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := commandprofile.New(commandprofile.Options{
		Registry: reg,
		Argv:     cfg.Decision.AdapterCommand,
		Provider: strings.TrimSpace(cfg.Decision.AdapterProvider),
		Model:    strings.TrimSpace(cfg.Decision.AdapterModel),
	})
	if err != nil {
		return nil, err
	}
	return recoveryQuestionProvider{inner: conn}, nil
}

// recoveryQuestionProvider rewrites a canonical request to the evaluation-only
// failure-recovery kind and choice set, then delegates to the real connector.
type recoveryQuestionProvider struct{ inner decision.Provider }

func (p recoveryQuestionProvider) Name() string { return p.inner.Name() }

func (p recoveryQuestionProvider) Decide(ctx context.Context, req decision.Request) (decision.Decision, error) {
	req.UseCase = recoveryEvalKind
	req.Choices = []decision.Choice{decision.Low, decision.Medium, decision.Human}
	return p.inner.Decide(ctx, req)
}

// EvaluateAdvisoryRecovery runs the real SEAM-004 advisory checkpoint exactly once
// with the evaluation-only failure-recovery question. It reuses the production
// checkpoint (applyDecisionEvidence) end-to-end — the attention gate, the single
// provider call under SOP's timeout, SOP-owned validation, the adverse rule, and
// autonomy.Decide — so the only difference from the production advisory is the
// question kind and its choice mapping. The model's recommendation is the
// canonical Choice; callers map it back to the native recovery choice.
func EvaluateAdvisoryRecovery(ctx context.Context, cfg config.Config, cls failure.Classification, spec *taskfile.Spec) AdvisoryOutcome {
	base := string(cls.Disposition)
	out := AdvisoryOutcome{BaselineAction: base, FinalAction: base}

	d := deps{newDecisionProvider: newRecoveryEvalProvider}
	res := lifeResult{classification: cls}
	var doc *decisionEvidenceDoc
	res = applyDecisionEvidence(ctx, cfg, d, spec, res, func(ed *decisionEvidenceDoc) { doc = ed })

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

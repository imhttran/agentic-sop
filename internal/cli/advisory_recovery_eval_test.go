package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// stubProvider records the request it receives and returns a fixed decision.
type stubProvider struct {
	decide func(decision.Request) (decision.Decision, error)
}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) Decide(_ context.Context, req decision.Request) (decision.Decision, error) {
	return s.decide(req)
}

// stubAdapter writes a POSIX shell adapter that prints a fixed DecisionResult,
// standing in for the real adapter subprocess.
func stubAdapter(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub adapter uses a POSIX shell script")
	}
	p := filepath.Join(t.TempDir(), "adapter.sh")
	script := "#!/bin/sh\ncat <<'EOF'\n" + body + "\nEOF\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func recoveryTestConfig(adapter string) config.Config {
	cfg := config.Default()
	enabled := true
	cfg.Decision.Enabled = &enabled
	cfg.Decision.Provider = "adapter"
	cfg.Decision.AdapterProvider = "clef"
	cfg.Decision.AdapterCommand = []string{adapter}
	return cfg
}

func attentionableClassification() failure.Classification {
	return failure.Classification{Kind: failure.TestFailure, Disposition: failure.AutoFix, Confidence: failure.High}
}

// TestRecoveryEvalRegistryServesRecoveryKind proves the eval-only registry serves
// the failure-recovery kind (and not the production change-risk kind).
func TestRecoveryEvalRegistryServesRecoveryKind(t *testing.T) {
	reg, err := recoveryEvalRegistry(recoveryTestConfig("x"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := reg.Build(decision.Request{
		UseCase: recoveryEvalKind,
		Subject: "s",
		Choices: []decision.Choice{decision.Low, decision.Medium, decision.Human},
	})
	if err != nil {
		t.Fatalf("recovery kind not served: %v", err)
	}
	qs := built.Request.Questions
	if len(qs) != 1 || qs[0].Type != "choice" {
		t.Fatalf("question = %+v", qs)
	}
	want := map[string]bool{"AUTO_FIX": true, "CONTINUE": true, "HUMAN": true}
	if len(qs[0].Choices) != 3 {
		t.Fatalf("choices = %v, want 3 recovery choices", qs[0].Choices)
	}
	for _, c := range qs[0].Choices {
		if !want[c] {
			t.Errorf("unexpected choice %q", c)
		}
	}
	if _, err := reg.Build(decision.Request{UseCase: adapterDecisionKind}); err == nil {
		t.Error("eval registry must not serve the production change-risk kind")
	}
}

// TestRecoveryQuestionProviderRewrites proves the wrapper rewrites the canonical
// request to the recovery kind and choice set before delegating.
func TestRecoveryQuestionProviderRewrites(t *testing.T) {
	var got decision.Request
	p := recoveryQuestionProvider{inner: stubProvider{decide: func(req decision.Request) (decision.Decision, error) {
		got = req
		return decision.Decision{Choice: decision.Low, Confidence: 0.5}, nil
	}}}
	if _, err := p.Decide(context.Background(), decision.Request{
		UseCase: "change-risk",
		Choices: []decision.Choice{decision.Low, decision.Medium, decision.High, decision.Human},
	}); err != nil {
		t.Fatal(err)
	}
	if got.UseCase != recoveryEvalKind {
		t.Errorf("use-case = %q, want %q", got.UseCase, recoveryEvalKind)
	}
	if len(got.Choices) != 3 {
		t.Errorf("choices = %v, want the 3 recovery choices", got.Choices)
	}
}

// TestEvaluateAdvisoryRecoveryHumanEscalates proves a HUMAN recommendation flows
// through the real advisory path and escalates to NEEDS_HUMAN.
func TestEvaluateAdvisoryRecoveryHumanEscalates(t *testing.T) {
	adapter := stubAdapter(t, `{"provider":"clef","model":"stub","answers":{"failure-recovery":{"type":"choice","choice":"HUMAN","confidence":0.9}}}`)
	cfg := recoveryTestConfig(adapter)
	cls := attentionableClassification()
	if d := autonomy.Decide(cls, cfg.AutonomyPolicy()); !attentionable(d) {
		t.Fatalf("test setup: classification is not attentionable (%+v)", d)
	}
	out := EvaluateAdvisoryRecovery(context.Background(), cfg, cls, &taskfile.Spec{Title: "a benign failure"})
	if !out.Consulted {
		t.Fatalf("not consulted: %+v", out)
	}
	if out.Choice != string(decision.Human) {
		t.Errorf("choice = %q, want HUMAN", out.Choice)
	}
	if out.FinalAction != "NEEDS_HUMAN" || !out.Escalated {
		t.Errorf("final=%q escalated=%v, want NEEDS_HUMAN/true", out.FinalAction, out.Escalated)
	}
}

// TestEvaluateAdvisoryRecoveryAutoFixAddsNothing proves a benign AUTO_FIX
// recommendation is mapped to a benign canonical choice and adds no attention.
func TestEvaluateAdvisoryRecoveryAutoFixAddsNothing(t *testing.T) {
	adapter := stubAdapter(t, `{"provider":"clef","answers":{"failure-recovery":{"type":"choice","choice":"AUTO_FIX","confidence":0.8}}}`)
	cfg := recoveryTestConfig(adapter)
	cls := attentionableClassification()
	base := string(cls.Disposition)
	out := EvaluateAdvisoryRecovery(context.Background(), cfg, cls, &taskfile.Spec{Title: "a benign failure"})
	if !out.Consulted {
		t.Fatalf("not consulted: %+v", out)
	}
	if out.Choice != string(decision.Low) {
		t.Errorf("choice = %q, want LOW (AUTO_FIX maps to a benign choice)", out.Choice)
	}
	if out.FinalAction != base || out.Escalated {
		t.Errorf("final=%q escalated=%v, want %s/false", out.FinalAction, out.Escalated, base)
	}
}

// TestEvaluateAdvisoryRecoveryNotEligible proves the attention gate is preserved:
// a baseline that already withholds execution is never consulted, so the model
// can never lower attention.
func TestEvaluateAdvisoryRecoveryNotEligible(t *testing.T) {
	adapter := stubAdapter(t, `{"provider":"clef","answers":{"failure-recovery":{"type":"choice","choice":"HUMAN","confidence":0.9}}}`)
	cfg := recoveryTestConfig(adapter)
	cls := failure.Classification{Kind: failure.ApprovalRequired, Disposition: failure.NeedsHuman, Confidence: failure.High}
	out := EvaluateAdvisoryRecovery(context.Background(), cfg, cls, &taskfile.Spec{Title: "an approval boundary"})
	if out.Consulted {
		t.Errorf("a non-attentionable baseline must not be consulted: %+v", out)
	}
	if out.FinalAction != "NEEDS_HUMAN" {
		t.Errorf("final = %q, want the unchanged baseline NEEDS_HUMAN", out.FinalAction)
	}
}

// TestEvaluateAdvisoryRecoveryRejectsMalformed proves a non-conforming adapter
// response fails closed (no usable choice, no escalation) through the real path.
func TestEvaluateAdvisoryRecoveryRejectsMalformed(t *testing.T) {
	adapter := stubAdapter(t, `{"provider":"clef","answers":{"failure-recovery":{"type":"choice","choice":"SIDEWAYS","confidence":0.9}}}`)
	cfg := recoveryTestConfig(adapter)
	out := EvaluateAdvisoryRecovery(context.Background(), cfg, attentionableClassification(), &taskfile.Spec{Title: "x"})
	if out.FinalAction != string(failure.AutoFix) {
		t.Errorf("final = %q, want the unchanged baseline AUTO_FIX", out.FinalAction)
	}
	if out.Escalated {
		t.Error("a rejected response must never escalate")
	}
}

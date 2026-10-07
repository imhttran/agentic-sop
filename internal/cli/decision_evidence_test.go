package cli

import (
	"bytes"
	"context"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// fakeDecisionProvider is a test-only, provider-neutral decision provider. It
// returns a canned result and records the request it received, so a test can prove
// what crossed the boundary without a real model or process.
type fakeDecisionProvider struct {
	name  string
	dec   decision.Decision
	err   error
	calls int
	req   decision.Request
}

func (p *fakeDecisionProvider) Name() string { return p.name }

func (p *fakeDecisionProvider) Decide(_ context.Context, req decision.Request) (decision.Decision, error) {
	p.calls++
	p.req = req
	return p.dec, p.err
}

// decisionEvidenceDeps injects a provider factory that ignores configuration, so a
// test controls exactly what the seam receives.
func decisionEvidenceDeps(p decision.Provider, err error) deps {
	return deps{newDecisionProvider: func(config.Config) (decision.Provider, error) { return p, err }}
}

// decisionEvidenceConfig is a configuration with the decision layer explicitly
// enabled or disabled. The disabled form is the default an existing installation
// has.
func decisionEvidenceConfig(enabled bool) config.Config {
	cfg := config.Default()
	cfg.Decision.Enabled = enabled
	cfg.Decision.Provider = "deterministic"
	return cfg
}

// permitsExecution reports whether a decision lets the lifecycle proceed
// automatically. It is the monotonicity lens: provider evidence must never make a
// non-permitting (governed) result permitting.
func permitsExecution(d autonomy.Decision) bool {
	switch d.Action {
	case autonomy.ActionAutoRetry, autonomy.ActionAutoContinue, autonomy.ActionAutoFix, autonomy.ActionAutoReconcile:
		return true
	default:
		return false
	}
}

// decisionEvidenceResult builds a governed result for a baseline classification,
// computing the baseline decision exactly as the lifecycle would.
func decisionEvidenceResult(cls failure.Classification) lifeResult {
	return lifeResult{classification: cls, decision: autonomy.Decide(cls, autonomy.PolicyFor(autonomy.Balanced))}
}

func autoFixClassification() failure.Classification {
	return failure.Classification{Kind: failure.CompilerError, Disposition: failure.AutoFix, Confidence: failure.High, Reason: "build failed"}
}

func benignSpec() *taskfile.Spec {
	return &taskfile.Spec{ID: "T001", Title: "Add widget", Description: "Add the widget."}
}

// TestDecisionEvidenceDisabledIsStrictNoOp proves the strongest backward
// compatibility requirement: with the capability disabled, the checkpoint never
// consults a provider and returns the governed result byte-for-byte unchanged, even
// when a provider would have been available.
func TestDecisionEvidenceDisabledIsStrictNoOp(t *testing.T) {
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.99}}
	in := decisionEvidenceResult(autoFixClassification())

	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(false), decisionEvidenceDeps(p, nil), benignSpec(), in)

	if p.calls != 0 {
		t.Fatalf("a disabled capability must not consult the provider (calls=%d)", p.calls)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("disabled checkpoint changed the result:\n got %+v\nwant %+v", out, in)
	}
}

// TestDecisionEvidenceAbsentProviderIsNoOp proves an enabled capability with no
// constructible provider is a strict no-op: the factory returns nothing, so the
// governed baseline stands.
func TestDecisionEvidenceAbsentProviderIsNoOp(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(nil, nil), benignSpec(), in)
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("an absent provider must be a strict no-op:\n got %+v\nwant %+v", out, in)
	}
	if out.decision.RequiresHuman {
		t.Fatalf("an absent provider must not require a human: %+v", out.decision)
	}
}

// TestDecisionEvidenceAdverseAddsAttention proves the enabled path only ADDS
// attention: an adverse HIGH choice raises an automated result to a human boundary
// produced by autonomy.Decide, while the classification itself is unchanged.
func TestDecisionEvidenceAdverseAddsAttention(t *testing.T) {
	cls := autoFixClassification()
	in := decisionEvidenceResult(cls)
	if permitsExecution(in.decision) == false {
		t.Fatalf("fixture must start from an automated result: %+v", in.decision)
	}
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}

	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)

	if !out.decision.RequiresHuman || out.decision.Action != autonomy.ActionHumanApproval {
		t.Fatalf("adverse evidence must add a human boundary, got %+v", out.decision)
	}
	if permitsExecution(out.decision) {
		t.Fatalf("the escalated result must not permit execution: %+v", out.decision)
	}
	if out.classification != cls {
		t.Fatalf("the checkpoint must not rewrite the classification: %+v", out.classification)
	}
	if p.calls != 1 {
		t.Fatalf("the provider must be consulted exactly once, got %d", p.calls)
	}
	// The request is provider-neutral and carries no governance or lifecycle handle.
	if p.req.UseCase != decisionEvidenceKind {
		t.Errorf("request kind = %q, want the neutral %q", p.req.UseCase, decisionEvidenceKind)
	}
	if want := []decision.Choice{decision.Low, decision.Medium, decision.High, decision.Human}; !reflect.DeepEqual(p.req.Choices, want) {
		t.Errorf("request choices = %v, want the closed set %v", p.req.Choices, want)
	}
}

// TestDecisionEvidenceBenignAddsNothing proves LOW/MEDIUM evidence adds nothing: a
// provider can never make SOP less conservative than the governed baseline. A
// high-confidence benign choice is still evidence, not a permission.
func TestDecisionEvidenceBenignAddsNothing(t *testing.T) {
	for _, choice := range []decision.Choice{decision.Low, decision.Medium} {
		in := decisionEvidenceResult(autoFixClassification())
		p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: choice, Confidence: 1.0}}
		out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
		if !reflect.DeepEqual(out, in) {
			t.Errorf("benign choice %s must add nothing:\n got %+v\nwant %+v", choice, out, in)
		}
	}
}

// TestDecisionEvidenceGovernedBaselinePreserved proves the checkpoint cannot touch
// a baseline that already withholds execution: a human boundary and a terminal stop
// are preserved, and the provider is never even consulted.
func TestDecisionEvidenceGovernedBaselinePreserved(t *testing.T) {
	for _, tc := range []struct {
		name string
		cls  failure.Classification
	}{
		{"human boundary", failure.Classification{Kind: failure.AmbiguousContract, Disposition: failure.NeedsHuman, Reason: "ambiguous"}},
		{"terminal stop", failure.Classification{Kind: failure.NoProgress, Disposition: failure.Retry, Reason: "no progress"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := decisionEvidenceResult(tc.cls)
			p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.Human, Confidence: 1.0}}
			out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
			if !reflect.DeepEqual(out, in) {
				t.Fatalf("a governed baseline must be preserved:\n got %+v\nwant %+v", out, in)
			}
			if p.calls != 0 {
				t.Fatalf("the provider must not be consulted for a governed baseline (calls=%d)", p.calls)
			}
		})
	}
}

// TestDecisionEvidencePassingRunIsNoOp proves a passing run (no classification) is
// never touched and never consults a provider.
func TestDecisionEvidencePassingRunIsNoOp(t *testing.T) {
	in := lifeResult{}
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("a passing run must be a strict no-op: %+v", out)
	}
	if p.calls != 0 {
		t.Fatalf("a passing run must not consult the provider (calls=%d)", p.calls)
	}
}

// TestDecisionEvidenceFailsClosed proves every unusable provider result fails
// closed to the governed baseline: a transport error, an unknown choice, an invalid
// confidence, an indeterminate/absent result, and a provider-reported error all
// leave the automated baseline intact and never become a success or an approval.
func TestDecisionEvidenceFailsClosed(t *testing.T) {
	nan := math.NaN()
	for _, tc := range []struct {
		name string
		dec  decision.Decision
		err  error
	}{
		{"transport failure", decision.Decision{}, decision.ErrProviderFailure},
		{"provider error", decision.Decision{}, decision.ErrProviderResult},
		{"unsupported capability", decision.Decision{}, decision.ErrUnsupportedCapability},
		{"indeterminate", decision.Decision{}, decision.ErrIndeterminate},
		{"empty result", decision.Decision{}, nil},
		{"unknown choice", decision.Decision{Choice: decision.Choice("SKYNET"), Confidence: 0.9}, nil},
		{"confidence above range", decision.Decision{Choice: decision.Low, Confidence: 1.5}, nil},
		{"confidence below range", decision.Decision{Choice: decision.Low, Confidence: -0.1}, nil},
		{"confidence NaN", decision.Decision{Choice: decision.Low, Confidence: nan}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := decisionEvidenceResult(autoFixClassification())
			p := &fakeDecisionProvider{name: "fake", dec: tc.dec, err: tc.err}
			out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
			if !reflect.DeepEqual(out, in) {
				t.Fatalf("an unusable result must fail closed:\n got %+v\nwant %+v", out, in)
			}
			if out.decision.RequiresHuman {
				t.Fatalf("a provider failure must never become a human approval: %+v", out.decision)
			}
		})
	}
}

// TestDecisionEvidenceMonotonic is the property test for the governing lattice:
// across every baseline and every provider choice, external evidence can never make
// an existing governed result more permissive. The result is either the governed
// baseline or a human boundary.
func TestDecisionEvidenceMonotonic(t *testing.T) {
	baselines := map[string]failure.Classification{
		"auto fix":      {Kind: failure.CompilerError, Disposition: failure.AutoFix},
		"auto continue": {Kind: failure.IncompleteImplementation, Disposition: failure.Continue},
		"auto retry":    {Kind: failure.TransientProvider, Disposition: failure.Retry},
		"replan":        {Kind: failure.ReplanRequired, Disposition: failure.Replan},
		"human":         {Kind: failure.AmbiguousContract, Disposition: failure.NeedsHuman},
		"terminal":      {Kind: failure.NoProgress, Disposition: failure.Retry},
	}
	choices := []decision.Choice{"", decision.Low, decision.Medium, decision.High, decision.Human}
	for bname, cls := range baselines {
		for _, choice := range choices {
			in := decisionEvidenceResult(cls)
			p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: choice, Confidence: 0.9}}
			out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)

			if permitsExecution(out.decision) && !permitsExecution(in.decision) {
				t.Fatalf("[%s/%q] evidence made a governed result permitting: base=%+v out=%+v", bname, choice, in.decision, out.decision)
			}
			escalated := out.decision.Action == autonomy.ActionHumanApproval && out.decision.RequiresHuman
			if !escalated && !reflect.DeepEqual(out.decision, in.decision) {
				t.Fatalf("[%s/%q] a non-escalating result must equal the baseline: base=%+v out=%+v", bname, choice, in.decision, out.decision)
			}
			if out.classification != cls {
				t.Fatalf("[%s/%q] classification rewritten: %+v", bname, choice, out.classification)
			}
		}
	}
}

// TestDecisionEvidenceProviderIdentityIsIrrelevant proves provider identity never
// affects the policy result: two providers with different names returning identical
// evidence yield byte-for-byte identical decisions.
func TestDecisionEvidenceProviderIdentityIsIrrelevant(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())
	ev := decision.Decision{Choice: decision.High, Confidence: 0.9}

	a := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(&fakeDecisionProvider{name: "provider-a", dec: ev}, nil), benignSpec(), in)
	b := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(&fakeDecisionProvider{name: "provider-b", dec: ev}, nil), benignSpec(), in)

	if !reflect.DeepEqual(a.decision, b.decision) {
		t.Fatalf("provider identity must not affect policy:\n a=%+v\n b=%+v", a.decision, b.decision)
	}
	// The reason is provider-neutral: it names the choice, never the provider.
	for _, s := range []string{a.decision.Reason, b.decision.Reason} {
		if strings.Contains(s, "provider-a") || strings.Contains(s, "provider-b") {
			t.Errorf("the decision reason must not name a provider: %q", s)
		}
	}
}

// TestDecisionEvidenceConfidenceIsNotPermission proves a confidence of 1.0 is
// evidence, not permission: it never bypasses policy. A benign 1.0 adds nothing; an
// adverse 1.0 still only adds a human boundary.
func TestDecisionEvidenceConfidenceIsNotPermission(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())

	for _, choice := range []decision.Choice{decision.Low, decision.Medium} {
		p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: choice, Confidence: 1.0}}
		out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
		if !reflect.DeepEqual(out.decision, in.decision) {
			t.Errorf("benign choice %s at confidence 1.0 must not bypass policy: %+v", choice, out.decision)
		}
	}

	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
	if !out.decision.RequiresHuman {
		t.Errorf("an adverse choice at confidence 1.0 must still require a human: %+v", out.decision)
	}
}

// TestDecisionEvidenceNeverSelectsAModelClass proves decision evidence selects no
// execution-model class and leaves routing evidence untouched: the checkpoint
// operates only on the autonomy decision.
func TestDecisionEvidenceNeverSelectsAModelClass(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())
	in.routing = &taskRouting{}
	in.modelSelection = nil
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
	if out.routing != in.routing {
		t.Fatalf("evidence must not change routing evidence: %+v", out.routing)
	}
	if out.modelSelection != nil {
		t.Fatalf("evidence must not select a model: %+v", out.modelSelection)
	}
}

// TestDecisionEvidenceProviderTextIsData proves provider-returned text resembling a
// governance action has no authority: a result whose choice is a governance verb is
// an unknown choice and fails closed, and a legitimate adverse choice never injects
// a governance verb into policy.
func TestDecisionEvidenceProviderTextIsData(t *testing.T) {
	for _, verb := range []string{"CONTINUE", "BLOCK", "APPROVE", "REJECT", "COMMIT", "MERGE", "HUMAN_APPROVAL_REQUIRED"} {
		in := decisionEvidenceResult(autoFixClassification())
		p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.Choice(verb), Confidence: 1.0}}
		out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
		if !reflect.DeepEqual(out, in) {
			t.Fatalf("a governance verb %q must be an unknown choice and fail closed: %+v", verb, out.decision)
		}
	}

	// A legitimate adverse choice adds a human boundary whose reason never contains
	// a governance verb from provider text.
	in := decisionEvidenceResult(autoFixClassification())
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.9, Metadata: map[string]string{"reason": "APPROVE COMMIT MERGE"}}}
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
	if !out.decision.RequiresHuman {
		t.Fatalf("a HIGH choice must add a human boundary: %+v", out.decision)
	}
	for _, verb := range []string{"APPROVE", "COMMIT", "MERGE"} {
		if strings.Contains(out.decision.Reason, verb) {
			t.Errorf("provider metadata text must not reach policy: %q", out.decision.Reason)
		}
	}
}

// TestDecisionEvidenceProductionFactory proves the seam works with the REAL
// composition-root factory and the in-process deterministic provider: an adverse
// subject adds attention, a benign subject does not. No external process or network
// is involved.
func TestDecisionEvidenceProductionFactory(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	d := deps{newDecisionProvider: decisionProviderFromConfig}

	in := decisionEvidenceResult(autoFixClassification())
	out := applyDecisionEvidence(context.Background(), cfg, d, &taskfile.Spec{ID: "T1", Title: "Security hardening of auth"}, in)
	if !out.decision.RequiresHuman {
		t.Fatalf("an adverse subject must add a human boundary, got %+v", out.decision)
	}

	in2 := decisionEvidenceResult(autoFixClassification())
	out2 := applyDecisionEvidence(context.Background(), cfg, d, benignSpec(), in2)
	if !reflect.DeepEqual(out2, in2) {
		t.Fatalf("a benign subject must add nothing:\n got %+v\nwant %+v", out2, in2)
	}
}

// TestDecisionEvidenceDoesNotTouchLifecycleState proves an external provider result
// cannot transition lifecycle state: the checkpoint only ever replaces the autonomy
// decision, never the gate, the stage, or the validation suite the lifecycle acts on.
func TestDecisionEvidenceDoesNotTouchLifecycleState(t *testing.T) {
	in := decisionEvidenceResult(autoFixClassification())
	in.gate = quality.Result{Decision: quality.Fail, Reasons: []string{"build failed"}}
	in.stage = runpkg.Failed
	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}

	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)

	if !reflect.DeepEqual(out.gate, in.gate) {
		t.Fatalf("the gate must be unchanged: %+v", out.gate)
	}
	if out.stage != in.stage {
		t.Fatalf("the stage must be unchanged: %s", out.stage)
	}
	if !reflect.DeepEqual(out.suite, in.suite) {
		t.Fatalf("the validation suite must be unchanged: %+v", out.suite)
	}
	if out.classification != in.classification {
		t.Fatalf("the classification must be unchanged: %+v", out.classification)
	}
	// It may ONLY have added attention to the decision.
	if !out.decision.RequiresHuman {
		t.Fatalf("the escalation must be limited to the decision: %+v", out.decision)
	}
}

// TestDecisionEvidenceIsIndependentOfModelContext proves decision-provider policy is
// independent of execution-model routing: the same classification and the same
// evidence produce the same decision regardless of the routing/escalation context.
func TestDecisionEvidenceIsIndependentOfModelContext(t *testing.T) {
	ev := decision.Decision{Choice: decision.High, Confidence: 0.9}

	plain := deps{newDecisionProvider: func(config.Config) (decision.Provider, error) {
		return &fakeDecisionProvider{name: "fake", dec: ev}, nil
	}}
	modeled := deps{
		newDecisionProvider: func(config.Config) (decision.Provider, error) {
			return &fakeDecisionProvider{name: "fake", dec: ev}, nil
		},
		modelClass:     "large",
		routingEnabled: true,
		routing:        model.Result{Active: true, Selection: model.Selection{Class: model.ClassLarge}},
	}

	a := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), plain, benignSpec(), decisionEvidenceResult(autoFixClassification()))
	b := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), modeled, benignSpec(), decisionEvidenceResult(autoFixClassification()))

	if !reflect.DeepEqual(a.decision, b.decision) {
		t.Fatalf("decision-provider policy must not depend on execution-model context:\n plain=%+v\n modeled=%+v", a.decision, b.decision)
	}
}

// runInjectedCLIWithDecision runs a command with an injected diff, agent, and
// decision-provider factory.
func runInjectedCLIWithDecision(t *testing.T, dir, diff string, a agent.Agent, newProvider func(config.Config) (decision.Provider, error), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:               func() (string, error) { return dir, nil },
		newAgent:            func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:            func(context.Context, string) (string, error) { return diff, nil },
		commit:              func(context.Context, string, string) error { return nil },
		newGitHub:           func(string) github.Client { return &fakeGitHub{} },
		newDecisionProvider: newProvider,
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// TestDecisionEvidenceLivePathEscalates is the live-path integration test: through
// the real `run --task` lifecycle, an enabled adverse provider turns an otherwise
// automatic continuation into a human boundary, while a benign provider and a
// disabled capability leave the governed result unchanged.
func TestDecisionEvidenceLivePathEscalates(t *testing.T) {
	const cfgEnabled = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\ndecision:\n  enabled: true\n  provider: deterministic\n"
	const cfgDisabled = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\n"

	// A productive no-change outcome is an automatic continuation under high
	// autonomy: the baseline that provider evidence may escalate.
	agentFor := func() agent.Agent {
		return outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
	}

	for _, tc := range []struct {
		name     string
		config   string
		provider decision.Provider
		wantAuto bool
	}{
		{"disabled capability is unchanged", cfgDisabled, nil, true},
		{"benign evidence is unchanged", cfgEnabled, &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.Low, Confidence: 0.9}}, true},
		{"adverse evidence adds a human boundary", cfgEnabled, &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "TASK.md", runTaskFile)
			writeConfig(t, dir, tc.config)

			var factory func(config.Config) (decision.Provider, error)
			if tc.provider != nil {
				factory = func(config.Config) (decision.Provider, error) { return tc.provider, nil }
			} else {
				factory = decisionProviderFromConfig
			}

			code, stdout, _ := runInjectedCLIWithDecision(t, dir, "diff\n", agentFor(), factory, "run", "--task", "TASK.md")
			if code != exitError {
				t.Fatalf("code=%d, want a non-pass result; stdout=%s", code, stdout)
			}
			gotAuto := strings.Contains(stdout, "decision=AUTO_CONTINUE")
			if gotAuto != tc.wantAuto {
				t.Fatalf("decision=AUTO_CONTINUE is %v, want %v:\n%s", gotAuto, tc.wantAuto, stdout)
			}
			if !tc.wantAuto && !strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
				t.Fatalf("adverse evidence must record a human-approval decision:\n%s", stdout)
			}

			// The classification artifact records the escalation, and the original
			// classification kind is preserved (evidence adds attention, it does not
			// rewrite why the lifecycle stopped).
			artifact := readClassificationArtifact(t, dir, "T001")
			if artifact.Autonomy == nil {
				t.Fatalf("missing autonomy decision in the artifact")
			}
			if !tc.wantAuto {
				if artifact.Autonomy.Action != autonomy.ActionHumanApproval || !artifact.Autonomy.RequiresHuman {
					t.Fatalf("artifact autonomy = %+v, want a human approval", artifact.Autonomy)
				}
			}
		})
	}
}

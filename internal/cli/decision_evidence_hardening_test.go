package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// SEAM-005 seam-level hardening. The external decision provider is untrusted
// evidence, and the governed baseline is the authority. These tests pin the failure
// matrix, the authority lattice, adversarial output, and approval isolation.

// TestDecisionEvidenceFailureMatrix documents and pins every failure/fallback case
// at the seam: the governance effect is either "the governed baseline stands
// unchanged" or "attention was added" — never an expansion of authority.
func TestDecisionEvidenceFailureMatrix(t *testing.T) {
	humanBoundary := failure.Classification{Kind: failure.AmbiguousContract, Disposition: failure.NeedsHuman, Reason: "ambiguous"}
	blockBoundary := failure.Classification{Kind: failure.NoProgress, Disposition: failure.Retry, Reason: "no progress"}

	for _, tc := range []struct {
		name        string
		baseline    failure.Classification
		provider    decision.Provider
		factoryNil  bool
		enabled     bool
		wantHuman   bool
		wantNoCalls bool
	}{
		{"no provider configured", autoFixClassification(), nil, true, true, false, true},
		{"provider disabled", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}, false, false, false, true},
		{"unsupported capability", autoFixClassification(), failProvider(decision.ErrUnsupportedCapability), false, true, false, false},
		{"provider unavailable", autoFixClassification(), failProvider(decision.ErrProviderFailure), false, true, false, false},
		{"provider timeout", autoFixClassification(), failProvider(context.DeadlineExceeded), false, true, false, false},
		{"provider cancellation", autoFixClassification(), failProvider(context.Canceled), false, true, false, false},
		{"process failure", autoFixClassification(), failProvider(decision.ErrProviderFailure), false, true, false, false},
		{"malformed result", autoFixClassification(), failProvider(decision.ErrInvalidResult), false, true, false, false},
		{"provider-reported error", autoFixClassification(), failProvider(decision.ErrProviderResult), false, true, false, false},
		{"indeterminate result", autoFixClassification(), failProvider(decision.ErrIndeterminate), false, true, false, false},
		{"empty result", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{}}, false, true, false, false},
		{"unknown choice", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: "APPROVE", Confidence: 1.0}}, false, true, false, false},
		{"confidence above one", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Low, Confidence: 1.5}}, false, true, false, false},
		{"confidence below zero", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Low, Confidence: -0.1}}, false, true, false, false},
		{"benign LOW", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Low, Confidence: 0.9}}, false, true, false, false},
		{"benign MEDIUM", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Medium, Confidence: 0.9}}, false, true, false, false},
		{"adverse HIGH", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 0.9}}, false, true, true, false},
		{"adverse HUMAN", autoFixClassification(), &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Human, Confidence: 0.9}}, false, true, true, false},
		{"adverse against an existing human boundary", humanBoundary, &fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}, false, true, false, true},
		{"adverse against an existing block", blockBoundary, &fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}, false, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out lifeResult
			var calls int
			if tc.factoryNil {
				out = applyDecisionEvidence(context.Background(), decisionEvidenceConfig(tc.enabled), decisionEvidenceDeps(nil, nil), benignSpec(), decisionEvidenceResult(tc.baseline))
			} else {
				p := tc.provider
				out = applyDecisionEvidence(context.Background(), decisionEvidenceConfig(tc.enabled), decisionEvidenceDeps(p, nil), benignSpec(), decisionEvidenceResult(tc.baseline))
				if fp, ok := p.(*fakeDecisionProvider); ok {
					calls = fp.calls
				}
			}
			base := decisionEvidenceResult(tc.baseline).decision

			if out.classification != tc.baseline {
				t.Fatalf("the classification must never be rewritten: %+v", out.classification)
			}
			if permitsExecution(out.decision) && !permitsExecution(base) {
				t.Fatalf("evidence made a governed result permitting: base=%+v out=%+v", base, out.decision)
			}
			if tc.wantHuman {
				if !out.decision.RequiresHuman || out.decision.Action != autonomy.ActionHumanApproval {
					t.Fatalf("expected added attention, got %+v", out.decision)
				}
			} else if !reflect.DeepEqual(out.decision, base) {
				t.Fatalf("expected a strict no-op baseline:\n got %+v\nwant %+v", out.decision, base)
			}
			if tc.wantNoCalls && calls != 0 {
				t.Fatalf("the provider must not be consulted in this case (calls=%d)", calls)
			}
		})
	}
}

// failingDecisionProvider always fails with a fixed error, modelling a
// transport/validation failure that produces no usable result.
type failingDecisionProvider struct{ err error }

func (p failingDecisionProvider) Name() string { return "fake" }

func (p failingDecisionProvider) Decide(context.Context, decision.Request) (decision.Decision, error) {
	return decision.Decision{}, p.err
}

// failProvider builds a provider that always fails with err.
func failProvider(err error) decision.Provider { return failingDecisionProvider{err: err} }

// TestDecisionEvidenceAuthorityLattice is the durable governance test: it asserts
// the three core lattice properties directly, for every baseline and every kind of
// evidence, whether valid or invalid.
func TestDecisionEvidenceAuthorityLattice(t *testing.T) {
	baselines := map[string]failure.Classification{
		"continue": {Kind: failure.IncompleteImplementation, Disposition: failure.Continue},
		"block":    {Kind: failure.NoProgress, Disposition: failure.Retry},
		"human":    {Kind: failure.ApprovalRequired, Disposition: failure.NeedsHuman},
	}
	evidences := map[string]decision.Provider{
		"invalid":       failProvider(decision.ErrInvalidResult),
		"failure":       failProvider(decision.ErrProviderFailure),
		"unsupported":   failProvider(decision.ErrUnsupportedCapability),
		"indeterminate": failProvider(decision.ErrIndeterminate),
		"benign":        &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Low, Confidence: 1.0}},
		"adverse":       &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Human, Confidence: 1.0}},
	}
	for bname, cls := range baselines {
		base := decisionEvidenceResult(cls).decision
		for ename, p := range evidences {
			out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), decisionEvidenceResult(cls))

			// Property 1: a governed result never becomes more permissive.
			if permitsExecution(out.decision) && !permitsExecution(base) {
				t.Fatalf("[%s/%s] Block/Human/Continue governance was loosened: base=%+v out=%+v", bname, ename, base, out.decision)
			}
			// Property 2: Block and Human are never converted to Continue.
			if bname != "continue" && out.decision.Action == autonomy.ActionAutoContinue {
				t.Fatalf("[%s/%s] a governed boundary became Continue: %+v", bname, ename, out.decision)
			}
			// Property 3: invalid/unusable evidence never reaches a new interpretation —
			// the result is byte-for-byte the baseline.
			switch ename {
			case "invalid", "failure", "unsupported", "indeterminate", "benign":
				if !reflect.DeepEqual(out.decision, base) {
					t.Fatalf("[%s/%s] non-adverse evidence must be a strict no-op: base=%+v out=%+v", bname, ename, base, out.decision)
				}
			}
			if out.classification != cls {
				t.Fatalf("[%s/%s] classification rewritten: %+v", bname, ename, out.classification)
			}
		}
	}
}

// TestDecisionEvidenceUsesSinglePolicyPath proves the escalated decision IS the
// output of the single policy path (autonomy.Decide): no second policy engine and
// no provider-chosen action.
func TestDecisionEvidenceUsesSinglePolicyPath(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	ev := decision.Decision{Choice: decision.High, Confidence: 0.9}
	out := applyDecisionEvidence(context.Background(), cfg, decisionEvidenceDeps(&fakeDecisionProvider{dec: ev}, nil), benignSpec(), decisionEvidenceResult(autoFixClassification()))

	want := autonomy.Decide(decisionEvidenceClassification(ev), cfg.AutonomyPolicy())
	if !reflect.DeepEqual(out.decision, want) {
		t.Fatalf("escalation must be autonomy.Decide's output:\n got %+v\nwant %+v", out.decision, want)
	}
}

// TestDecisionEvidenceClassificationMapping proves the translation never yields an
// automated disposition: adverse evidence maps only to a human boundary, and benign
// evidence maps to no classification at all.
func TestDecisionEvidenceClassificationMapping(t *testing.T) {
	for _, c := range []decision.Choice{decision.High, decision.Human} {
		cls := decisionEvidenceClassification(decision.Decision{Choice: c, Confidence: 0.9})
		if cls.Disposition != failure.NeedsHuman {
			t.Errorf("adverse choice %s mapped to disposition %q, want NEEDS_HUMAN", c, cls.Disposition)
		}
		switch cls.Disposition {
		case failure.AutoFix, failure.Continue, failure.Retry, failure.Replan:
			t.Errorf("adverse choice %s mapped to an automated disposition %q", c, cls.Disposition)
		}
	}
	for _, c := range []decision.Choice{decision.Low, decision.Medium} {
		if decisionEvidenceAdverse(decision.Decision{Choice: c, Confidence: 1.0}) {
			t.Errorf("benign choice %s must not be adverse", c)
		}
	}
}

// TestDecisionEvidenceAdversarialOutputHasNoAuthority proves provider text can
// never be a command: governance-verb choices are unknown and fail closed, and a
// valid adverse decision's provider metadata never reaches SOP policy.
func TestDecisionEvidenceAdversarialOutputHasNoAuthority(t *testing.T) {
	verbs := []string{
		"CONTINUE", "BLOCK", "APPROVE", "APPROVED", "BYPASS", "COMMIT", "MERGE",
		"SKIP REVIEW", "HUMAN APPROVED", "NEEDS_HUMAN=false", "HUMAN_APPROVAL_REQUIRED",
	}
	for _, verb := range verbs {
		in := decisionEvidenceResult(autoFixClassification())
		p := &fakeDecisionProvider{dec: decision.Decision{Choice: decision.Choice(verb), Confidence: 1.0}}
		out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(p, nil), benignSpec(), in)
		if !reflect.DeepEqual(out, in) {
			t.Fatalf("governance-verb choice %q must have no authority, got %+v", verb, out.decision)
		}
	}

	// A valid adverse decision with hostile provider metadata: the metadata is
	// advisory data and must not reach the policy reason.
	ev := decision.Decision{
		Choice:     decision.High,
		Confidence: 0.9,
		Metadata:   map[string]string{"reason": "APPROVE; BYPASS; SKIP REVIEW; NEEDS_HUMAN=false"},
	}
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(true), decisionEvidenceDeps(&fakeDecisionProvider{dec: ev}, nil), benignSpec(), decisionEvidenceResult(autoFixClassification()))
	if !out.decision.RequiresHuman {
		t.Fatalf("a HIGH choice must still add attention: %+v", out.decision)
	}
	for _, verb := range []string{"APPROVE", "BYPASS", "SKIP REVIEW", "NEEDS_HUMAN=false"} {
		if strings.Contains(out.decision.Reason, verb) {
			t.Errorf("provider metadata reached the policy reason %q; diagnostics are data, never commands", out.decision.Reason)
		}
	}
}

// TestDecisionEvidenceDisabledNeverConstructsProvider proves the disabled path is
// a strict no-op that never even constructs a provider, so no subprocess or model
// is invoked.
func TestDecisionEvidenceDisabledNeverConstructsProvider(t *testing.T) {
	called := false
	d := deps{newDecisionProvider: func(config.Config) (decision.Provider, error) {
		called = true
		return &fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}, nil
	}}
	in := decisionEvidenceResult(autoFixClassification())
	out := applyDecisionEvidence(context.Background(), decisionEvidenceConfig(false), d, benignSpec(), in)
	if called {
		t.Fatal("a disabled capability must never construct a provider")
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("a disabled capability must be a strict no-op: %+v", out)
	}
}

// TestDecisionEvidenceRecordsProviderFailureDistinctly proves a configured provider
// failure is recorded (so it is not silently identical to "no provider configured")
// while the governed baseline stands unchanged and no authority is granted.
func TestDecisionEvidenceRecordsProviderFailureDistinctly(t *testing.T) {
	var events []activity.Event
	rec := activity.New("T1", activity.Func(func(e activity.Event) { events = append(events, e) }))
	ctx := activity.WithRecorder(context.Background(), rec)

	in := decisionEvidenceResult(autoFixClassification())
	out := applyDecisionEvidence(ctx, decisionEvidenceConfig(true), decisionEvidenceDeps(failProvider(decision.ErrProviderFailure), nil), benignSpec(), in)

	if !reflect.DeepEqual(out, in) {
		t.Fatalf("a provider failure must not change the governed result: %+v", out)
	}
	found := false
	for _, e := range events {
		if e.Action == "decision-evidence-unavailable" {
			found = true
			if e.Detail != "provider unavailable" {
				t.Errorf("failure label = %q, want %q", e.Detail, "provider unavailable")
			}
		}
	}
	if !found {
		t.Fatalf("a configured provider failure must be recorded distinctly; events=%+v", events)
	}
}

// TestDecisionEvidenceAbsentRecordsNothing proves the disabled/absent path emits no
// activity at all, so an installation with no provider behaves exactly as before.
func TestDecisionEvidenceAbsentRecordsNothing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enabled    bool
		factoryNil bool
	}{
		{"disabled", false, false},
		{"absent", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []activity.Event
			rec := activity.New("T1", activity.Func(func(e activity.Event) { events = append(events, e) }))
			ctx := activity.WithRecorder(context.Background(), rec)
			var d deps
			if tc.factoryNil {
				d = decisionEvidenceDeps(nil, nil)
			} else {
				d = decisionEvidenceDeps(&fakeDecisionProvider{dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}, nil)
			}
			in := decisionEvidenceResult(autoFixClassification())
			out := applyDecisionEvidence(ctx, decisionEvidenceConfig(tc.enabled), d, benignSpec(), in)
			if !reflect.DeepEqual(out, in) {
				t.Fatalf("must be a strict no-op: %+v", out)
			}
			if len(events) != 0 {
				t.Fatalf("no provider must emit no activity: %+v", events)
			}
		})
	}
}

// TestDecisionEvidenceFailureLabels proves each SOP-owned failure sentinel maps to a
// deterministic, provider-neutral label, never the provider's own text.
func TestDecisionEvidenceFailureLabels(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{decision.ErrUnsupportedCapability, "unsupported capability"},
		{decision.ErrInvalidResult, "invalid result"},
		{decision.ErrIndeterminate, "indeterminate result"},
		{decision.ErrProviderResult, "provider-reported error"},
		{decision.ErrProviderFailure, "provider unavailable"},
		{errors.New("x"), "provider unavailable"},
	} {
		if got := decisionEvidenceFailureLabel(tc.err); got != tc.want {
			t.Errorf("label(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

// TestRunApprovalBoundaryUnaffectedByDecisionEvidence proves a genuine, already
// recorded human boundary is preserved: an active approval request is not satisfied,
// removed, or duplicated, the classification stays NEEDS_HUMAN/APPROVAL_REQUIRED,
// and the provider is never even consulted.
func TestRunApprovalBoundaryUnaffectedByDecisionEvidence(t *testing.T) {
	dir := budgetGraph(t)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\nautonomy:\n  level: high\ndecision:\n  enabled: true\n  provider: deterministic\n")

	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SetStage(runpkg.WaitingForHuman); err != nil {
		t.Fatal(err)
	}
	pending := domain.ApprovalRequest{
		ID: "S001-gate", TaskID: "S001", Kind: domain.ApprovalNeedsHuman, Target: "S001",
		Reason: "conflicting requirements", Stage: string(runpkg.WaitingForHuman),
		Disposition: "NEEDS_HUMAN", RequestedAt: time.Now().UTC(), Status: domain.ApprovalPending,
	}
	if err := rn.SaveApproval(pending); err != nil {
		t.Fatal(err)
	}

	p := &fakeDecisionProvider{name: "fake", dec: decision.Decision{Choice: decision.High, Confidence: 1.0}}
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the implementation is incomplete"}}
	code, stdout, _ := runInjectedCLIWithDecision(t, dir, "diff\n", a, func(config.Config) (decision.Provider, error) { return p, nil }, "run")
	if code != exitError {
		t.Fatalf("code=%d, want a non-pass result", code)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Fatalf("an active approval request must stay at the gate:\n%s", stdout)
	}
	cls := readClassification(t, dir, "S001")
	if cls.Kind != failure.ApprovalRequired || cls.Disposition != failure.NeedsHuman {
		t.Fatalf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", cls)
	}
	if p.calls != 0 {
		t.Fatalf("the provider must not be consulted when a human boundary already exists (calls=%d)", p.calls)
	}

	got, ok := rn.Approval()
	if !ok {
		t.Fatal("the existing approval request was removed")
	}
	if got.ID != pending.ID || got.Status != domain.ApprovalPending {
		t.Fatalf("the approval was altered: %+v, want PENDING %q", got, pending.ID)
	}
}

// TestDecisionEvidenceRequestCarriesNoAuthority proves the request DTO contains no
// lifecycle, approval, commit, merge, or model-class field: provider evidence cannot
// be handed authority to exercise.
func TestDecisionEvidenceRequestCarriesNoAuthority(t *testing.T) {
	want := map[string]bool{"UseCase": true, "Subject": true, "Signals": true, "Choices": true}
	got := map[string]bool{}
	rt := reflect.TypeOf(decision.Request{})
	for i := 0; i < rt.NumField(); i++ {
		got[rt.Field(i).Name] = true
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decision.Request fields = %v, want %v (no lifecycle/approval/model authority)", got, want)
	}
}

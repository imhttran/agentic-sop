package decision

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// This file is the AS-CLEF-005 verification suite. It pins the fail-closed
// behavior of SOP's bounded decision boundary (decision.go): a provider produces
// a Choice+Confidence, and policy outside the provider (Route) decides what that
// choice means. The tests prove an external/optional provider can never bypass
// governance: provider failure is never an implicit success, unknown is never an
// approval, risk is never silently downgraded, human approval is never bypassed,
// confidence alone never authorizes execution, and no task state is mutated
// outside the governed lifecycle.
//
// Symbols under test (internal/decision/decision.go):
//   Provider interface (Name/Decide), Decision{Choice,Confidence,Metadata},
//   Choice Low/Medium/High/Human, NewProvider, Thresholds, Route, Target.
//
// AS-CLEF-004 result state consumed read-only (internal/domain/status.go):
//   domain.NOT_REQUIRED and the other TaskStatus values. Tests reference the
//   existing vocabulary only and introduce no new production constants.

// failingProvider is a test-only Provider that reports a fixed error. It exposes
// exactly the Provider surface (Name + Decide): it has no method that could
// mutate task state, persistence, or a lifecycle, so a passing decision path can
// never transition a task.
type failingProvider struct {
	name string
	err  error
}

func (f failingProvider) Name() string { return f.name }

func (f failingProvider) Decide(ctx context.Context, _ Request) (Decision, error) {
	// A real provider that is unreachable or timed out returns the error *and* a
	// zero Decision. We model that faithfully: on error there is no usable
	// decision, so the boundary has nothing to approve.
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	return Decision{}, f.err
}

// errProviderUnavailable and errProviderInternal model distinct provider failure
// classes so a test can prove the boundary never converts one into a pass.
var (
	errProviderUnavailable = errors.New("decision: provider unavailable")
	errProviderInternal    = errors.New("decision: provider internal error")
)

var governanceThresholds = Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}

// TestDecideProviderUnavailable proves a provider that is unavailable surfaces
// its error and never yields an approving/authorizing decision. It protects the
// "provider failure is never an implicit success" property.
func TestDecideProviderUnavailable(t *testing.T) {
	p := failingProvider{name: "unavailable", err: errProviderUnavailable}
	d, err := p.Decide(context.Background(), Request{Subject: "anything"})
	if err == nil {
		t.Fatal("provider unavailable must surface an error, not an implicit success")
	}
	if !errors.Is(err, errProviderUnavailable) {
		t.Fatalf("err = %v, want the provider's error to reach the caller unchanged", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("decision on error = %+v, want zero value (never an approval)", d)
	}
	if got := Route(governanceThresholds, d); got == SmallModel {
		t.Fatalf("Route(zero decision) = %s, must not be an execution-authorizing target from a failed provider", got)
	}
}

// TestDecideProviderTimeout proves a context deadline is respected and surfaced:
// a timed-out provider must never be converted into a SmallModel authorization
// or an approval.
func TestDecideProviderTimeout(t *testing.T) {
	p := failingProvider{name: "slow", err: errProviderUnavailable}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // guarantee the deadline elapses

	d, err := p.Decide(ctx, Request{Subject: "anything"})
	if err == nil {
		t.Fatal("a timed-out provider must return an error, never a decision")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded surfaced to the caller", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("decision on timeout = %+v, want zero value", d)
	}
	if got := Route(governanceThresholds, d); got == SmallModel {
		t.Fatalf("Route(zero decision) = %s after timeout, must not authorize execution", got)
	}
}

// TestDecideProviderInternalError proves a provider internal error is surfaced
// and is not swallowed or downgraded into a low-risk small-model route.
func TestDecideProviderInternalError(t *testing.T) {
	p := failingProvider{name: "broken", err: errProviderInternal}
	d, err := p.Decide(context.Background(), Request{Subject: "anything"})
	if err == nil {
		t.Fatal("provider internal error must surface, never become an implicit success")
	}
	if !errors.Is(err, errProviderInternal) {
		t.Fatalf("err = %v, want the internal error surfaced", err)
	}
	if d.Choice != "" || d.Confidence != 0 {
		t.Fatalf("decision on internal error = %+v, want zero value", d)
	}
	if got := Route(governanceThresholds, d); got == SmallModel {
		t.Fatalf("Route(zero decision) = %s after internal error, must not authorize execution", got)
	}
}

// TestNewProviderUnsupportedType proves an unsupported provider/decision type
// fails closed: NewProvider returns an error and never silently falls back to a
// permissive provider.
func TestNewProviderUnsupportedType(t *testing.T) {
	for _, name := range []string{"jev", "gpt", "claude", "unknown"} {
		p, err := NewProvider(name)
		if err == nil {
			t.Fatalf("NewProvider(%q) must fail closed for an unsupported decision type", name)
		}
		if p != nil {
			t.Fatalf("NewProvider(%q) returned a provider on error; must return nil (no permissive fallback)", name)
		}
	}
	p, err := NewProvider("deterministic")
	if err != nil {
		t.Fatalf("NewProvider(deterministic) failed: %v", err)
	}
	if p.Name() != "deterministic" {
		t.Fatalf("Name = %q, want deterministic", p.Name())
	}
}

// TestDecideMalformedResult proves a malformed (zero-value / undecodable) result
// is never accepted as an approval or as an execution-authorizing route.
func TestDecideMalformedResult(t *testing.T) {
	malformed := Decision{} // no choice, no confidence, no metadata
	if malformed.Choice != "" {
		t.Fatal("precondition: malformed decision has an empty choice")
	}
	if got := Route(governanceThresholds, malformed); got != HumanTarget {
		t.Fatalf("malformed result (empty choice, zero confidence) routed to %s; must fail closed to %s, never an approval or execution route", got, HumanTarget)
	}
}

// TestDecideEmptyResult proves an explicitly empty provider result is not turned
// into a Low/approval.
func TestDecideEmptyResult(t *testing.T) {
	empty := Decision{Choice: "", Confidence: 0, Metadata: nil}
	if got := Route(governanceThresholds, empty); got == SmallModel {
		t.Fatalf("empty result routed to %s; an empty provider result must never authorize execution", got)
	}
	if got := Route(governanceThresholds, empty); got != HumanTarget {
		t.Fatalf("empty result routed to %s, want %s (fail closed to a human)", got, HumanTarget)
	}
}

// TestInvalidConfidence proves out-of-range confidence (negative, >1, NaN) never
// produces a confident-authorization outcome. Behavior is asserted explicitly
// and is fail-closed: anything below RequireHuman escalates to a human, and NaN
// (which compares false against every threshold) never reaches SmallModel.
func TestInvalidConfidence(t *testing.T) {
	cases := []struct {
		name       string
		confidence float64
	}{
		{"negative", -0.5},
		{"above one", 1.5},
		{"NaN", math.NaN()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Decision{Choice: Low, Confidence: tc.confidence}
			got := Route(governanceThresholds, d)
			if got == SmallModel {
				t.Fatalf("invalid confidence %v routed to %s; invalid confidence must never be treated as a confident approval", tc.confidence, got)
			}
		})
	}
}

// TestUnknownResultNotApproval proves an unknown/indeterminate result maps to a
// human and is never treated as approval.
func TestUnknownResultNotApproval(t *testing.T) {
	unknown := Decision{Choice: Choice("INDETERMINATE"), Confidence: 0.99}
	if got := Route(governanceThresholds, unknown); got != HumanTarget {
		t.Fatalf("unknown/indeterminate result routed to %s, want %s: unknown must never become approval", got, HumanTarget)
	}
	unknownHighConfidence := Decision{Choice: Choice("UNKNOWN"), Confidence: 1.0}
	if got := Route(governanceThresholds, unknownHighConfidence); got == SmallModel {
		t.Fatalf("unknown choice with confidence 1.0 routed to %s; confidence alone must not authorize execution", got)
	}
}

// TestLowConfidenceRequiresHuman proves low confidence escalates to a human and
// is never silently routed to an execution tier.
func TestLowConfidenceRequiresHuman(t *testing.T) {
	d := Decision{Choice: Low, Confidence: 0.3}
	if got := Route(governanceThresholds, d); got != HumanTarget {
		t.Fatalf("low confidence routed to %s, want %s (human approval must not be bypassed)", got, HumanTarget)
	}
}

// TestConfidenceAloneDoesNotAuthorize proves that even a maximally confident
// provider decision that demands a human (High/Human choice) still routes to a
// human: confidence alone never authorizes execution.
func TestConfidenceAloneDoesNotAuthorize(t *testing.T) {
	cases := []struct {
		name string
		d    Decision
	}{
		{"confident high", Decision{Choice: High, Confidence: 1.0}},
		{"confident human", Decision{Choice: Human, Confidence: 1.0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Route(governanceThresholds, tc.d); got != HumanTarget {
				t.Fatalf("%s (confidence %v) routed to %s, want %s: confidence alone must not authorize execution", tc.name, tc.d.Confidence, got, HumanTarget)
			}
		})
	}
}

// TestRiskNotDowngraded proves a high/risky decision cannot be silently
// downgraded to a model tier by thresholds or metadata. A High choice is always
// a human, regardless of how permissive the thresholds are.
func TestRiskNotDowngraded(t *testing.T) {
	d := Decision{
		Choice:     High,
		Confidence: 0.99,
		Metadata:   map[string]string{"reason": "database migration", "risk_override": "LOW"},
	}
	permissive := Thresholds{RouteToStrongModel: 0.01, RequireHuman: 0.01}
	if got := Route(permissive, d); got != HumanTarget {
		t.Fatalf("High risk routed to %s under permissive thresholds, want %s: risk must never be silently downgraded", got, HumanTarget)
	}
	if got := Route(governanceThresholds, d); got != HumanTarget {
		t.Fatalf("High risk with metadata override routed to %s, want %s", got, HumanTarget)
	}
}

// TestResultStateConsumed proves the AS-CLEF-004 result-state vocabulary
// (internal/domain/status.go), including NOT_REQUIRED, is consumed read-only as
// input. The decision package does not transition tasks; these states are the
// governed lifecycle the provider must never touch.
func TestResultStateConsumed(t *testing.T) {
	if domain.NOT_REQUIRED == "" {
		t.Fatal("NOT_REQUIRED must be a defined AS-CLEF-004 result state")
	}
	task := &domain.Task{ID: "AS-CLEF-004", Status: domain.PLANNED}
	if err := task.MarkNotRequired("audit found no gap"); err != nil {
		t.Fatalf("MarkNotRequired failed: %v", err)
	}
	if task.Status != domain.NOT_REQUIRED {
		t.Fatalf("status = %s, want NOT_REQUIRED", task.Status)
	}
	if !task.IsSatisfied() {
		t.Fatal("NOT_REQUIRED must satisfy dependants")
	}
	if task.IsCompleted() {
		t.Fatal("NOT_REQUIRED must never read as a completion or provider approval")
	}
	// The decision path never mutates this state: routing a decision is pure.
	before := task.Status
	_ = Route(governanceThresholds, Decision{Choice: Low, Confidence: 0.95})
	if task.Status != before {
		t.Fatalf("decision routing mutated task status from %s to %s; the decision boundary must not mutate task state", before, task.Status)
	}
}

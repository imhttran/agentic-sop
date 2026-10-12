package bridge

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decision"
)

func fptr(f float64) *float64 { return &f }

// testRegistry is a provider-neutral demo profile. It names no provider: profile
// identity is structural, and the name is only a correlation hint.
func testRegistry() *Registry {
	rng := [2]float64{0, 10}
	return NewRegistry(&Profile{
		Name:    "demo",
		Version: 3,
		Kinds: map[string]KindSpec{
			"complexity": {
				Type: TypeChoice,
				ChoiceMap: map[string]decision.Choice{
					"low": decision.Low, "med": decision.Medium,
					"high": decision.High, "human": decision.Human,
				},
			},
			"approve": {
				Type:             TypeBoolean,
				BooleanThreshold: fptr(0.5),
				TrueChoice:       decision.High,
				FalseChoice:      decision.Low,
			},
			"magnitude": {
				Type: TypeScore,
				ScoreBands: []ScoreBand{
					{Max: 3, Choice: decision.Low},
					{Max: 7, Choice: decision.Medium},
					{Max: 10, Choice: decision.High},
				},
				ScoreRange: &rng,
			},
		},
	})
}

func mustBuild(t *testing.T, r *Registry, req decision.Request) Built {
	t.Helper()
	b, err := r.Build(req)
	if err != nil {
		t.Fatalf("Build(%+v): unexpected error %v", req, err)
	}
	return b
}

func TestBuildDerivesDeterministicQuestionID(t *testing.T) {
	r := testRegistry()
	b1 := mustBuild(t, r, decision.Request{UseCase: "complexity", Subject: "does x hold?"})
	b2 := mustBuild(t, r, decision.Request{UseCase: "complexity", Subject: "does x hold?"})

	q1 := b1.Request.Questions[0]
	if q1.ID != "complexity" {
		t.Fatalf("question ID = %q, want deterministic %q", q1.ID, "complexity")
	}
	if b2.Request.Questions[0].ID != q1.ID {
		t.Fatalf("question ID is not deterministic: %q vs %q", q1.ID, b2.Request.Questions[0].ID)
	}
	if q1.Type != TypeChoice {
		t.Fatalf("question type = %q, want %q", q1.Type, TypeChoice)
	}
	if b1.Request.State == "" {
		t.Fatal("state must be non-empty")
	}
	want := []string{"high", "human", "low", "med"}
	if len(q1.Choices) != len(want) {
		t.Fatalf("choices = %v, want %v", q1.Choices, want)
	}
	for i := range want {
		if q1.Choices[i] != want[i] {
			t.Fatalf("choices = %v, want sorted %v", q1.Choices, want)
		}
	}
}

func TestBuildStateNotEmptyWithoutSubject(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	if b.Request.State == "" {
		t.Fatal("state must be non-empty even with no subject and no signals")
	}
}

func TestBuildRejectsEmptyKind(t *testing.T) {
	_, err := testRegistry().Build(decision.Request{UseCase: "  "})
	if !errors.Is(err, decision.ErrUnsupportedCapability) {
		t.Fatalf("empty kind: err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestBuildRejectsUnknownKind(t *testing.T) {
	_, err := testRegistry().Build(decision.Request{UseCase: "nope"})
	if !errors.Is(err, decision.ErrUnsupportedCapability) {
		t.Fatalf("unknown kind: err = %v, want ErrUnsupportedCapability", err)
	}
}

func TestBuildRejectsAmbiguousKind(t *testing.T) {
	r := NewRegistry(
		&Profile{Name: "a", Kinds: map[string]KindSpec{"k": {Type: TypeChoice, ChoiceMap: map[string]decision.Choice{"x": decision.Low}}}},
		&Profile{Name: "b", Kinds: map[string]KindSpec{"k": {Type: TypeChoice, ChoiceMap: map[string]decision.Choice{"y": decision.High}}}},
	)
	_, err := r.Build(decision.Request{UseCase: "k"})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("ambiguous kind: err = %v, want ErrInvalidResult", err)
	}
}

func TestBuildRejectsMisconfiguredProfile(t *testing.T) {
	cases := map[string]KindSpec{
		"empty choice map":     {Type: TypeChoice},
		"boolean no threshold": {Type: TypeBoolean, TrueChoice: decision.High, FalseChoice: decision.Low},
		"score no range":       {Type: TypeScore, ScoreBands: []ScoreBand{{Max: 1, Choice: decision.Low}}},
	}
	for name, spec := range cases {
		r := NewRegistry(&Profile{Name: "p", Kinds: map[string]KindSpec{"k": spec}})
		if _, err := r.Build(decision.Request{UseCase: "k"}); !errors.Is(err, decision.ErrInvalidResult) {
			t.Errorf("%s: err = %v, want ErrInvalidResult", name, err)
		}
	}
}

func TestInterpretChoiceProducesValidatedEvidence(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity", Subject: "s"})
	out, err := b.Interpret(Result{
		Provider: "demo",
		Model:    "demo-model",
		Answers:  map[string]Answer{"complexity": {Type: TypeChoice, Choice: "high", Confidence: fptr(0.9)}},
	})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if out.Status != decision.StatusOK || out.Choice != decision.High || out.Kind != "complexity" {
		t.Fatalf("out = %+v", out)
	}
	if out.Confidence == nil || *out.Confidence != 0.9 {
		t.Fatalf("confidence = %v, want 0.9", out.Confidence)
	}
	// The evidence must pass the SOP-owned validator.
	probe := decision.Request{UseCase: "complexity"}
	if err := probe.ValidateResult(out); err != nil {
		t.Fatalf("produced evidence fails SOP validation: %v", err)
	}
}

func TestInterpretRoutesDeterministically(t *testing.T) {
	th := decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
	cases := []struct {
		native string
		want   decision.Target
	}{
		{"low", decision.SmallModel},
		{"med", decision.StrongModel},
		{"high", decision.HumanTarget},
		{"human", decision.HumanTarget},
	}
	for _, c := range cases {
		b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
		out, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
			"complexity": {Type: TypeChoice, Choice: c.native, Confidence: fptr(0.9)},
		}})
		if err != nil {
			t.Fatalf("%s: Interpret: %v", c.native, err)
		}
		got := decision.Route(th, decision.Decision{Choice: out.Choice, Confidence: *out.Confidence})
		if got != c.want {
			t.Errorf("choice %q routes to %v, want %v", c.native, got, c.want)
		}
	}
}

func TestInterpretNoAnswersFailsClosed(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	if _, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{}}); !errors.Is(err, decision.ErrIndeterminate) {
		t.Fatalf("no answers: err = %v, want ErrIndeterminate", err)
	}
}

func TestInterpretUnexpectedAnswerFailsClosed(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"some-other-id": {Type: TypeChoice, Choice: "low", Confidence: fptr(0.9)},
	}})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("unexpected answer: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretAnswerTypeMismatchFailsClosed(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"complexity": {Type: TypeBoolean, Probability: 0.9},
	}})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("type mismatch: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretUnmappedChoiceFailsClosed(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"complexity": {Type: TypeChoice, Choice: "invented", Confidence: fptr(0.9)},
	}})
	if !errors.Is(err, decision.ErrIndeterminate) {
		t.Fatalf("unmapped choice: err = %v, want ErrIndeterminate", err)
	}
}

func TestInterpretProviderRecognition(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	ans := map[string]Answer{"complexity": {Type: TypeChoice, Choice: "low", Confidence: fptr(0.9)}}
	if _, err := b.Interpret(Result{Provider: "", Answers: ans}); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("empty provider: err = %v, want ErrInvalidResult", err)
	}
	if _, err := b.Interpret(Result{Provider: "other", Answers: ans}); !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("unknown provider: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretConfidenceOutOfRangeFailsClosed(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"complexity": {Type: TypeChoice, Choice: "low", Confidence: fptr(1.5)},
	}})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("out-of-range confidence: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretProbabilityOfOtherChoiceIsNotConfidence(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	// Probabilities present, no explicit confidence, and the capability does not
	// opt in: the result is indeterminate, never a confident authorization.
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"complexity": {Type: TypeChoice, Choice: "med", Probabilities: map[string]float64{"med": 0.6}},
	}})
	if !errors.Is(err, decision.ErrIndeterminate) {
		t.Fatalf("implicit confidence: err = %v, want ErrIndeterminate", err)
	}
}

func TestInterpretChosenProbabilityAsConfidenceWhenOptedIn(t *testing.T) {
	r := NewRegistry(&Profile{Name: "p", Version: 1, Kinds: map[string]KindSpec{
		"k": {Type: TypeChoice, ChoiceMap: map[string]decision.Choice{"yes": decision.High}, ChosenProbAsConfidence: true},
	}})
	b := mustBuild(t, r, decision.Request{UseCase: "k"})
	out, err := b.Interpret(Result{Provider: "p", Answers: map[string]Answer{
		"k": {Type: TypeChoice, Choice: "yes", Probabilities: map[string]float64{"yes": 0.77}},
	}})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if out.Confidence == nil || *out.Confidence != 0.77 {
		t.Fatalf("confidence = %v, want 0.77", out.Confidence)
	}
}

func TestInterpretBooleanThresholdMapsChoiceAndConfidence(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "approve"})
	yes, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"approve": {Type: TypeBoolean, Probability: 0.8},
	}})
	if err != nil {
		t.Fatalf("yes: %v", err)
	}
	if yes.Choice != decision.High || yes.Confidence == nil || *yes.Confidence != 0.8 {
		t.Fatalf("yes = %+v", yes)
	}
	no, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"approve": {Type: TypeBoolean, Probability: 0.2},
	}})
	if err != nil {
		t.Fatalf("no: %v", err)
	}
	if no.Choice != decision.Low || no.Confidence == nil || *no.Confidence != 0.8 {
		t.Fatalf("no = %+v", no)
	}
}

func TestInterpretScoreNormalizesAndBands(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "magnitude"})
	cases := []struct {
		score  float64
		choice decision.Choice
		conf   float64
	}{
		{0, decision.Low, 0},
		{5, decision.Medium, 0.5},
		{10, decision.High, 1},
	}
	for _, c := range cases {
		out, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
			"magnitude": {Type: TypeScore, Score: c.score},
		}})
		if err != nil {
			t.Fatalf("score %v: %v", c.score, err)
		}
		if out.Choice != c.choice || out.Confidence == nil || *out.Confidence != c.conf {
			t.Errorf("score %v = {%v %v}, want {%v %v}", c.score, out.Choice, out.Confidence, c.choice, c.conf)
		}
	}
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"magnitude": {Type: TypeScore, Score: 11},
	}})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("out-of-range score: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretAllowedChoiceMembershipEnforced(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity", Choices: []decision.Choice{decision.Low}})
	_, err := b.Interpret(Result{Provider: "demo", Answers: map[string]Answer{
		"complexity": {Type: TypeChoice, Choice: "high", Confidence: fptr(0.9)},
	}})
	if !errors.Is(err, decision.ErrInvalidResult) {
		t.Fatalf("choice outside allowed set: err = %v, want ErrInvalidResult", err)
	}
}

func TestInterpretDeprecatedProfileIsRecorded(t *testing.T) {
	r := NewRegistry(&Profile{Name: "p", Version: 2, Deprecated: true, Kinds: map[string]KindSpec{
		"k": {Type: TypeChoice, ChoiceMap: map[string]decision.Choice{"x": decision.Low}},
	}})
	b := mustBuild(t, r, decision.Request{UseCase: "k"})
	out, err := b.Interpret(Result{Provider: "p", Answers: map[string]Answer{
		"k": {Type: TypeChoice, Choice: "x", Confidence: fptr(0.5)},
	}})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	if out.Diagnostics["deprecated"] != "true" {
		t.Fatalf("deprecation diagnostic missing: %v", out.Diagnostics)
	}
}

func TestInterpretDiagnosticsAreBoundedAndSecretFree(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity"})
	out, err := b.Interpret(Result{
		Provider: "demo",
		Model:    "demo-model",
		Answers:  map[string]Answer{"complexity": {Type: TypeChoice, Choice: "low", Confidence: fptr(0.9)}},
	})
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	allowed := map[string]bool{"profile": true, "profile_version": true, "model": true, "deprecated": true}
	for k := range out.Diagnostics {
		if !allowed[k] {
			t.Errorf("unexpected diagnostic key %q", k)
		}
	}
	if out.Diagnostics["profile"] != "demo" || out.Diagnostics["profile_version"] != "3" {
		t.Fatalf("diagnostics = %v", out.Diagnostics)
	}
}

func TestMapErrorClassifiesKinds(t *testing.T) {
	cases := map[string]error{
		"unavailable":            decision.ErrProviderFailure,
		"provider_failure":       decision.ErrProviderFailure,
		"invalid_request":        decision.ErrInvalidResult,
		"malformed_response":     decision.ErrInvalidResult,
		"unsupported":            decision.ErrUnsupportedCapability,
		"unsupported_capability": decision.ErrUnsupportedCapability,
		"something_else":         decision.ErrIndeterminate,
	}
	for kind, want := range cases {
		if got := MapError(kind); !errors.Is(got, want) {
			t.Errorf("MapError(%q) = %v, want %v", kind, got, want)
		}
	}
}

func TestRequestJSONShape(t *testing.T) {
	b := mustBuild(t, testRegistry(), decision.Request{UseCase: "complexity", Subject: "s", Signals: map[string]float64{"files": 3}})
	data, err := b.RequestJSON()
	if err != nil {
		t.Fatalf("RequestJSON: %v", err)
	}
	var got Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.State == "" || len(got.Questions) != 1 || got.Questions[0].ID != "complexity" {
		t.Fatalf("request JSON = %s", data)
	}
}

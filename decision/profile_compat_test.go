package decision

import (
	"errors"
	"math"
	"testing"
)

// This file pins the profile invariants the SOP-side bridge depends on. The
// bridge lives in agentic-sop (internal/decision/bridge); it translates the
// canonical one-decision-per-call seam to this batched profile and back. These
// tests assert the adapter-side guarantees the bridge relies on, and document the
// one guarantee the adapter deliberately does not provide (per-question answer
// completeness is a bridge obligation, not an adapter one).

func TestProfileRequestValidationInvariants(t *testing.T) {
	wellFormed := DecisionRequest{
		State: "s",
		Questions: []Question{
			{ID: "q1", Type: QuestionChoice, Criteria: "c", Choices: []string{"a", "b"}},
			{ID: "q2", Type: QuestionBoolean},
			{ID: "q3", Type: QuestionScore},
		},
	}
	if err := wellFormed.Validate(); err != nil {
		t.Fatalf("well-formed batched request rejected: %v", err)
	}

	cases := map[string]func(*DecisionRequest){
		"state required":        func(r *DecisionRequest) { r.State = "   " },
		"at least one question": func(r *DecisionRequest) { r.Questions = nil },
		"duplicate question id": func(r *DecisionRequest) {
			r.Questions = append(r.Questions, Question{ID: "q1", Type: QuestionBoolean})
		},
		"choice needs criteria": func(r *DecisionRequest) { r.Questions[0].Criteria = "" },
		"choice needs choices":  func(r *DecisionRequest) { r.Questions[0].Choices = nil },
		"duplicate choice":      func(r *DecisionRequest) { r.Questions[0].Choices = []string{"a", "a"} },
		"empty choice":          func(r *DecisionRequest) { r.Questions[0].Choices = []string{"a", "  "} },
		"unknown question type": func(r *DecisionRequest) { r.Questions[1].Type = "nonsense" },
	}
	for name, mutate := range cases {
		r := DecisionRequest{
			State: "s",
			Questions: []Question{
				{ID: "q1", Type: QuestionChoice, Criteria: "c", Choices: []string{"a", "b"}},
				{ID: "q2", Type: QuestionBoolean},
			},
		}
		mutate(&r)
		if err := r.Validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: err = %v, want ErrInvalidRequest", name, err)
		}
	}
}

func TestProfileAnswerRangeValidation(t *testing.T) {
	bad := map[string]Answer{
		"choice confidence > 1":   {Type: AnswerChoice, Choice: "a", Confidence: ptr(1.5)},
		"choice confidence < 0":   {Type: AnswerChoice, Choice: "a", Confidence: ptr(-0.1)},
		"choice confidence NaN":   {Type: AnswerChoice, Choice: "a", Confidence: ptr(math.NaN())},
		"choice probability > 1":  {Type: AnswerChoice, Choice: "a", Probabilities: map[string]float64{"a": 2}},
		"boolean probability > 1": {Type: AnswerBoolean, Probability: 1.2},
		"boolean probability NaN": {Type: AnswerBoolean, Probability: math.NaN()},
		"choice answer empty":     {Type: AnswerChoice},
		"unknown answer type":     {Type: "nonsense"},
	}
	for name, ans := range bad {
		if err := ans.Validate(); !errors.Is(err, ErrMalformedResponse) {
			t.Errorf("%s: err = %v, want ErrMalformedResponse", name, err)
		}
	}

	good := map[string]Answer{
		"choice with confidence":    {Type: AnswerChoice, Choice: "a", Confidence: ptr(0.5)},
		"choice with probabilities": {Type: AnswerChoice, Choice: "a", Probabilities: map[string]float64{"a": 0.5, "b": 0.5}},
		"boolean":                   {Type: AnswerBoolean, Probability: 0.5},
		"score":                     {Type: AnswerScore, Score: 3.5},
	}
	for name, ans := range good {
		if err := ans.Validate(); err != nil {
			t.Errorf("%s: well-formed answer rejected: %v", name, err)
		}
	}
}

// TestResultValidationDoesNotEnforceAnswerCompleteness documents the boundary the
// SOP bridge must cover. DecisionResult.Validate checks only that the answer map
// is non-empty and each answer is internally valid; it cannot assert that every
// requested question is answered (it has no request). That completeness guarantee
// is therefore a *bridge* obligation: the bridge rejects a result whose answer set
// does not exactly match the questions it sent, failing closed.
func TestResultValidationDoesNotEnforceAnswerCompleteness(t *testing.T) {
	partial := DecisionResult{
		Provider: "demo",
		Answers:  map[string]Answer{"q1": {Type: AnswerChoice, Choice: "a"}},
	}
	if err := partial.Validate(); err != nil {
		t.Fatalf("Validate rejected a partial answer set (%v); the bridge must enforce completeness, so this test should be updated deliberately if that changes", err)
	}
	empty := DecisionResult{Provider: "demo"}
	if err := empty.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("empty answer set: err = %v, want ErrMalformedResponse", err)
	}
}

func TestProviderErrorKindClassification(t *testing.T) {
	cases := []struct {
		kind ErrorKind
		want error
	}{
		{KindInvalidRequest, ErrInvalidRequest},
		{KindUnavailable, ErrUnavailable},
		{KindMalformedResponse, ErrMalformedResponse},
		{KindProviderFailure, ErrProviderFailure},
	}
	for _, c := range cases {
		err := &ProviderError{Provider: "demo", Kind: c.kind}
		if !errors.Is(err, c.want) {
			t.Errorf("kind %q: errors.Is(%v) = false, want true", c.kind, c.want)
		}
		for _, other := range cases {
			if other.want == c.want {
				continue
			}
			if errors.Is(err, other.want) {
				t.Errorf("kind %q wrongly matches %v", c.kind, other.want)
			}
		}
	}
}

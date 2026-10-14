package decision

import (
	"errors"
	"math"
	"os"
	"testing"
)

func mustRead(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func ptr(v float64) *float64 { return &v }

func TestDecisionResultValidateMultiAnswer(t *testing.T) {
	res := DecisionResult{
		Provider: "nimble",
		Model:    "nimble",
		Answers: map[string]Answer{
			"risk": {
				Type:          AnswerChoice,
				Choice:        "HIGH",
				Probabilities: map[string]float64{"LOW": 0.0013, "MEDIUM": 0.041, "HIGH": 0.9576},
				Confidence:    ptr(0.8349),
			},
			"approval_required": {
				Type:        AnswerBoolean,
				Probability: 0.9974,
			},
			"execution_model": {
				Type:          AnswerChoice,
				Choice:        "LARGE",
				Probabilities: map[string]float64{"SMALL": 0.0016, "MEDIUM": 0.037, "LARGE": 0.9613},
				Confidence:    ptr(0.8447),
			},
		},
		Usage: Usage{InputTokens: 983, OutputTokens: 4},
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("expected valid result, got %v", err)
	}
}

func TestDecisionResultValidateRejectsNoAnswers(t *testing.T) {
	res := DecisionResult{Provider: "nimble"}
	if err := res.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestAnswerValidateChoice(t *testing.T) {
	valid := Answer{Type: AnswerChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 1}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid choice answer, got %v", err)
	}

	empty := Answer{Type: AnswerChoice}
	if err := empty.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestAnswerValidateProbabilityRange(t *testing.T) {
	bad := Answer{Type: AnswerChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 1.5}}
	if err := bad.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestAnswerValidateConfidenceRange(t *testing.T) {
	bad := Answer{Type: AnswerChoice, Choice: "HIGH", Confidence: ptr(1.5)}
	if err := bad.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestAnswerValidateBoolean(t *testing.T) {
	valid := Answer{Type: AnswerBoolean, Probability: 0.997}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid boolean answer, got %v", err)
	}
	bad := Answer{Type: AnswerBoolean, Probability: -0.1}
	if err := bad.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestAnswerValidateScore(t *testing.T) {
	answer := Answer{Type: AnswerScore, Score: 7.5}
	if err := answer.Validate(); err != nil {
		t.Fatalf("expected valid score answer, got %v", err)
	}
}

func TestAnswerValidateUnknownType(t *testing.T) {
	answer := Answer{Type: AnswerType("ranking")}
	if err := answer.Validate(); !errors.Is(err, ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

// TestAnswerValidateRejectsNonFiniteValues is a regression test for the NaN/Inf
// rejection added to inUnitRangeFloat: NaN, +Inf, and -Inf must be rejected with
// ErrMalformedResponse for every range-checked field.
func TestAnswerValidateRejectsNonFiniteValues(t *testing.T) {
	nonFinite := []struct {
		name  string
		value float64
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
	}

	cases := []struct {
		name   string
		answer Answer
	}{
		{
			name:   "choice_probabilities",
			answer: Answer{Type: AnswerChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 0}},
		},
		{
			name:   "boolean_probability",
			answer: Answer{Type: AnswerBoolean},
		},
		{
			name:   "confidence",
			answer: Answer{Type: AnswerChoice, Choice: "HIGH", Confidence: ptr(0)},
		},
	}

	for _, tc := range cases {
		for _, nf := range nonFinite {
			answer := tc.answer
			switch tc.name {
			case "choice_probabilities":
				answer.Probabilities = map[string]float64{"HIGH": nf.value}
			case "boolean_probability":
				answer.Probability = nf.value
			case "confidence":
				answer.Confidence = ptr(nf.value)
			}
			t.Run(tc.name+"/"+nf.name, func(t *testing.T) {
				if err := answer.Validate(); !errors.Is(err, ErrMalformedResponse) {
					t.Fatalf("expected ErrMalformedResponse for %v, got %v", nf.value, err)
				}
			})
		}
	}
}

func TestProviderErrorClassifiesAvailabilityAndMalformedResponses(t *testing.T) {
	unavailable := &ProviderError{Provider: "nimble", Kind: KindUnavailable, Err: errors.New("boom")}
	if !errors.Is(unavailable, ErrUnavailable) {
		t.Fatal("expected ErrUnavailable")
	}
	if errors.Is(unavailable, ErrMalformedResponse) {
		t.Fatal("did not expect ErrMalformedResponse")
	}

	malformed := &ProviderError{Provider: "nimble", Kind: KindMalformedResponse}
	if !errors.Is(malformed, ErrMalformedResponse) {
		t.Fatal("expected ErrMalformedResponse")
	}

	invalid := &ProviderError{Provider: "nimble", Kind: KindInvalidRequest}
	if !errors.Is(invalid, ErrInvalidRequest) {
		t.Fatal("expected ErrInvalidRequest")
	}

	failure := &ProviderError{Provider: "nimble", Kind: KindProviderFailure}
	if !errors.Is(failure, ErrProviderFailure) {
		t.Fatal("expected ErrProviderFailure")
	}
}

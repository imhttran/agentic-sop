package clef

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// fptr is a small helper for building *float64 wire fields.
func fptr(v float64) *float64 { return &v }

// choiceRequest is a single well-formed CHOICE question request used across the
// request-translation tests.
func choiceRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:       "risk",
			Type:     decision.QuestionChoice,
			Criteria: "assess the operational risk",
			Choices:  []string{"LOW", "HIGH"},
		}},
	}
}

// TestBuildSystemOneRequestStateAndModel covers the request-level fields the
// translation must carry through: State and the model argument.
func TestBuildSystemOneRequestStateAndModel(t *testing.T) {
	req := choiceRequest()
	sys, err := BuildSystemOneRequest("clef-flash", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	if sys.Model != "clef-flash" {
		t.Errorf("Model = %q, want clef-flash", sys.Model)
	}
	if sys.State != req.State {
		t.Errorf("State = %q, want %q", sys.State, req.State)
	}
}

// TestBuildSystemOneRequestChoice locks in the SystemOne contract: a CHOICE
// question's criteria MUST be an object keyed by exactly the allowed choices
// (values null); a plain string is rejected.
func TestBuildSystemOneRequestChoice(t *testing.T) {
	sys, err := BuildSystemOneRequest("clef-flash", choiceRequest())
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	q, ok := sys.Questions["risk"]
	if !ok {
		t.Fatal("missing risk question")
	}
	if q.Type != systemOneTypeChoice {
		t.Errorf("Type = %q, want %q", q.Type, systemOneTypeChoice)
	}
	if strings.TrimSpace(q.Instructions) == "" {
		t.Errorf("Instructions = %q, want non-empty", q.Instructions)
	}
	criteria, ok := q.Criteria.(map[string]any)
	if !ok {
		t.Fatalf("Criteria = %#v (%T), want map[string]any", q.Criteria, q.Criteria)
	}
	if len(criteria) != 2 {
		t.Fatalf("len(Criteria) = %d, want 2", len(criteria))
	}
	for _, choice := range []string{"LOW", "HIGH"} {
		v, ok := criteria[choice]
		if !ok {
			t.Errorf("Criteria missing key %q", choice)
			continue
		}
		if v != nil {
			t.Errorf("Criteria[%q] = %#v, want nil", choice, v)
		}
	}
	if _, ok := criteria["MEDIUM"]; ok {
		t.Errorf("Criteria unexpectedly contains a choice that is not allowed: %#v", criteria)
	}
	if len(q.Choices) != 2 || q.Choices[0] != "LOW" || q.Choices[1] != "HIGH" {
		t.Errorf("Choices = %v", q.Choices)
	}
}

// TestBuildSystemOneRequestBooleanMapsToNoul covers the BOOLEAN mapping: the
// SystemOne "noul" type with nil Criteria.
func TestBuildSystemOneRequestBooleanMapsToNoul(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:   "approval_required",
			Type: decision.QuestionBoolean,
		}},
	}
	sys, err := BuildSystemOneRequest("clef-flash", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	q := sys.Questions["approval_required"]
	if q.Type != systemOneTypeBoolean {
		t.Errorf("Type = %q, want %q", q.Type, systemOneTypeBoolean)
	}
	if systemOneTypeBoolean != "noul" {
		t.Errorf("systemOneTypeBoolean = %q, want noul", systemOneTypeBoolean)
	}
	if q.Criteria != nil {
		t.Errorf("Criteria = %#v, want nil for noul", q.Criteria)
	}
	if strings.TrimSpace(q.Instructions) == "" {
		t.Errorf("Instructions = %q, want non-empty", q.Instructions)
	}
}

// TestBuildSystemOneRequestScore covers the SCORE mapping: the SystemOne "score"
// type with Criteria as a []string equal to Choices.
func TestBuildSystemOneRequestScore(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:       "confidence_score",
			Type:     decision.QuestionScore,
			Criteria: "rate the confidence",
			Choices:  []string{"low", "medium", "high"},
		}},
	}
	sys, err := BuildSystemOneRequest("clef-flash", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	q := sys.Questions["confidence_score"]
	if q.Type != systemOneTypeScore {
		t.Errorf("Type = %q, want %q", q.Type, systemOneTypeScore)
	}
	criteria, ok := q.Criteria.([]string)
	if !ok {
		t.Fatalf("Criteria = %#v (%T), want []string", q.Criteria, q.Criteria)
	}
	want := []string{"low", "medium", "high"}
	if len(criteria) != len(want) {
		t.Fatalf("len(Criteria) = %d, want %d (%v)", len(criteria), len(want), criteria)
	}
	for i := range want {
		if criteria[i] != want[i] {
			t.Errorf("Criteria[%d] = %q, want %q", i, criteria[i], want[i])
		}
	}
}

// TestBuildSystemOneRequestScoreRequiresCandidates asserts that the adapter
// refuses to emit an invalid SCORE request: SystemOne requires 2-26 candidate
// descriptions, so fewer than two Choices wraps decision.ErrInvalidRequest.
func TestBuildSystemOneRequestScoreRequiresCandidates(t *testing.T) {
	cases := map[string][]string{
		"no candidates":    nil,
		"single candidate": {"only"},
	}
	for name, choices := range cases {
		t.Run(name, func(t *testing.T) {
			req := decision.DecisionRequest{
				State: "deploying a schema migration during business hours",
				Questions: []decision.Question{{
					ID:      "confidence_score",
					Type:    decision.QuestionScore,
					Choices: choices,
				}},
			}
			_, err := BuildSystemOneRequest("clef-flash", req)
			if !errors.Is(err, decision.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

// TestBuildSystemOneRequestFallbackInstructions covers the instruction fallback
// chain for every question type: explicit instructions win, then criteria, then
// a synthesized value containing the question ID, and the result is never empty.
func TestBuildSystemOneRequestFallbackInstructions(t *testing.T) {
	tests := map[string]struct {
		question     decision.Question
		wantExact    string
		wantContains string
	}{
		"boolean with no instructions or criteria": {
			question:     decision.Question{ID: "approval_required", Type: decision.QuestionBoolean},
			wantContains: "approval_required",
		},
		"choice with only criteria": {
			question:  decision.Question{ID: "risk", Type: decision.QuestionChoice, Criteria: "assess the risk", Choices: []string{"LOW", "HIGH"}},
			wantExact: "assess the risk",
		},
		"score with only criteria": {
			question:  decision.Question{ID: "confidence_score", Type: decision.QuestionScore, Criteria: "rate it", Choices: []string{"low", "high"}},
			wantExact: "rate it",
		},
		"score with no instructions or criteria": {
			question:     decision.Question{ID: "confidence_score", Type: decision.QuestionScore, Choices: []string{"low", "high"}},
			wantContains: "confidence_score",
		},
		"explicit instructions preserved": {
			question:  decision.Question{ID: "risk", Type: decision.QuestionChoice, Criteria: "assess", Instructions: "use the rubric", Choices: []string{"LOW", "HIGH"}},
			wantExact: "use the rubric",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := decision.DecisionRequest{
				State:     "deploying a schema migration during business hours",
				Questions: []decision.Question{tt.question},
			}
			sys, err := BuildSystemOneRequest("clef-flash", req)
			if err != nil {
				t.Fatalf("BuildSystemOneRequest() error = %v", err)
			}
			got := sys.Questions[tt.question.ID].Instructions
			if strings.TrimSpace(got) == "" {
				t.Fatalf("Instructions = %q, want non-empty", got)
			}
			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("Instructions = %q, want %q", got, tt.wantExact)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("Instructions = %q, want it to contain %q", got, tt.wantContains)
			}
		})
	}
}

// TestBuildSystemOneRequestMetadataNotTransmitted proves the request is accepted
// when Metadata is set but the wire request carries no Metadata field.
func TestBuildSystemOneRequestMetadataNotTransmitted(t *testing.T) {
	req := choiceRequest()
	req.Metadata = map[string]string{"trace": "abc"}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	sys, err := BuildSystemOneRequest("clef-flash", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	// systemOneRequest has no Metadata field: the only assertable property is
	// that translation succeeded while the wire shape stayed provider-neutral.
	if sys.Model != "clef-flash" || sys.State != req.State {
		t.Errorf("unexpected request = %+v", sys)
	}
}

// TestBuildSystemOneRequestMultipleQuestions asserts every question is keyed by
// its ID.
func TestBuildSystemOneRequestMultipleQuestions(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{
			{ID: "risk", Type: decision.QuestionChoice, Criteria: "assess", Choices: []string{"LOW", "HIGH"}},
			{ID: "approval_required", Type: decision.QuestionBoolean},
			{ID: "confidence_score", Type: decision.QuestionScore, Choices: []string{"low", "high"}},
		},
	}
	sys, err := BuildSystemOneRequest("clef-flash", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	if len(sys.Questions) != 3 {
		t.Fatalf("len(Questions) = %d, want 3", len(sys.Questions))
	}
	for _, id := range []string{"risk", "approval_required", "confidence_score"} {
		if _, ok := sys.Questions[id]; !ok {
			t.Errorf("missing question %q", id)
		}
	}
}

// TestBuildSystemOneRequestRejectsMalformed covers malformed requests: Validate
// failure, unknown type, CHOICE without criteria, and missing questions.
func TestBuildSystemOneRequestRejectsMalformed(t *testing.T) {
	cases := map[string]decision.DecisionRequest{
		"empty state":             {State: "   ", Questions: []decision.Question{{ID: "x", Type: decision.QuestionBoolean}}},
		"no questions":            {State: "deploying a schema migration during business hours"},
		"unknown type":            {State: "deploying a schema migration during business hours", Questions: []decision.Question{{ID: "x", Type: "ranking"}}},
		"choice without criteria": {State: "deploying a schema migration during business hours", Questions: []decision.Question{{ID: "x", Type: decision.QuestionChoice, Choices: []string{"A"}}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := BuildSystemOneRequest("clef-flash", req)
			if !errors.Is(err, decision.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

// multiRequest is the response-normalization fixture: one CHOICE, one BOOLEAN,
// and one SCORE question.
func multiRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{
			{ID: "risk", Type: decision.QuestionChoice, Criteria: "assess", Choices: []string{"LOW", "MEDIUM", "HIGH"}},
			{ID: "approval_required", Type: decision.QuestionBoolean},
			{ID: "confidence_score", Type: decision.QuestionScore, Choices: []string{"low", "high"}},
		},
	}
}

// baseAnswers returns a valid answer set for multiRequest that individual tests
// mutate to isolate a single failure.
func baseAnswers() map[string]systemOneAnswer {
	return map[string]systemOneAnswer{
		"risk": {
			Type:          systemOneTypeChoice,
			Choice:        "HIGH",
			Probabilities: map[string]float64{"LOW": 0.0013, "MEDIUM": 0.041, "HIGH": 0.9576},
			Confidence:    fptr(0.8349),
		},
		"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9974)},
		"confidence_score":  {Type: systemOneTypeScore, Score: fptr(0.75)},
	}
}

// TestNormalizeSystemOneResponseSuccess covers the choice, boolean, and score
// success paths, carrying confidence and probabilities through unchanged.
func TestNormalizeSystemOneResponseSuccess(t *testing.T) {
	resp := systemOneResponse{
		Model:   "clef-flash",
		Answers: baseAnswers(),
		Usage:   systemOneUsage{InputTokens: 983, OutputTokens: 4},
	}

	got, err := NormalizeSystemOneResponse(resp, multiRequest())
	if err != nil {
		t.Fatalf("NormalizeSystemOneResponse() error = %v", err)
	}
	if got.Provider != ProviderName || got.Model != "clef-flash" {
		t.Errorf("Provider/Model = %q/%q", got.Provider, got.Model)
	}
	if got.Usage.InputTokens != 983 || got.Usage.OutputTokens != 4 {
		t.Errorf("Usage = %+v", got.Usage)
	}

	risk := got.Answers["risk"]
	if risk.Type != decision.AnswerChoice || risk.Choice != "HIGH" {
		t.Errorf("risk = %+v", risk)
	}
	if risk.Confidence == nil || *risk.Confidence != 0.8349 {
		t.Errorf("risk confidence = %v", risk.Confidence)
	}
	if p := risk.Probabilities["HIGH"]; p != 0.9576 {
		t.Errorf("risk probability HIGH = %v, want 0.9576", p)
	}

	appr := got.Answers["approval_required"]
	if appr.Type != decision.AnswerBoolean || appr.Probability != 0.9974 {
		t.Errorf("approval_required = %+v", appr)
	}

	score := got.Answers["confidence_score"]
	if score.Type != decision.AnswerScore || score.Score != 0.75 {
		t.Errorf("confidence_score = %+v", score)
	}
}

// TestNormalizeSystemOneResponseChoiceWithoutOptionalFields proves a CHOICE
// answer without probabilities or confidence still normalizes.
func TestNormalizeSystemOneResponseChoiceWithoutOptionalFields(t *testing.T) {
	answers := baseAnswers()
	answers["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "LOW"}
	resp := systemOneResponse{Model: "clef-flash", Answers: answers}
	got, err := NormalizeSystemOneResponse(resp, multiRequest())
	if err != nil {
		t.Fatalf("NormalizeSystemOneResponse() error = %v", err)
	}
	if got.Answers["risk"].Choice != "LOW" {
		t.Errorf("risk choice = %q, want LOW", got.Answers["risk"].Choice)
	}
}

// TestNormalizeSystemOneResponseIndeterminate asserts that null-like forms stay
// indeterminate: missing answer, nil Noul, and nil Score are rejected with
// ErrMalformedResponse and never returned as success-shaped answers.
func TestNormalizeSystemOneResponseIndeterminate(t *testing.T) {
	t.Run("missing answer", func(t *testing.T) {
		answers := baseAnswers()
		delete(answers, "risk")
		_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
	})

	t.Run("nil noul", func(t *testing.T) {
		answers := baseAnswers()
		answers["approval_required"] = systemOneAnswer{Type: systemOneTypeBoolean}
		got, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
		if got.Answers != nil {
			t.Errorf("Answers = %+v, want nil on indeterminate outcome", got.Answers)
		}
	})

	t.Run("nil score", func(t *testing.T) {
		answers := baseAnswers()
		answers["confidence_score"] = systemOneAnswer{Type: systemOneTypeScore}
		got, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
		if got.Answers != nil {
			t.Errorf("Answers = %+v, want nil on indeterminate outcome", got.Answers)
		}
	})
}

// TestNormalizeSystemOneResponseRejectsOutOfSetChoice asserts a choice outside
// the request's allowed set is rejected, not returned, including unknown
// probability keys.
func TestNormalizeSystemOneResponseRejectsOutOfSetChoice(t *testing.T) {
	t.Run("choice outside allowed", func(t *testing.T) {
		answers := baseAnswers()
		answers["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "CRITICAL"}
		got, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
		if _, ok := got.Answers["risk"]; ok {
			t.Errorf("out-of-set choice was returned: %+v", got.Answers["risk"])
		}
	})

	t.Run("empty choice", func(t *testing.T) {
		answers := baseAnswers()
		answers["risk"] = systemOneAnswer{Type: systemOneTypeChoice}
		_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
	})

	t.Run("unknown probability key", func(t *testing.T) {
		answers := baseAnswers()
		answers["risk"] = systemOneAnswer{
			Type:          systemOneTypeChoice,
			Choice:        "HIGH",
			Probabilities: map[string]float64{"LOW": 0.1, "MEDIUM": 0.2, "HIGH": 0.6, "BANANA": 0.1},
		}
		_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
		if !errors.Is(err, decision.ErrMalformedResponse) {
			t.Fatalf("error = %v, want ErrMalformedResponse", err)
		}
	})
}

// TestNormalizeSystemOneResponseBounds asserts probabilities, confidence, and
// boolean probability are bounded to [0,1].
func TestNormalizeSystemOneResponseBounds(t *testing.T) {
	tests := map[string]func(map[string]systemOneAnswer){
		"probability above one": func(a map[string]systemOneAnswer) {
			a["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 1.5}}
		},
		"probability below zero": func(a map[string]systemOneAnswer) {
			a["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": -0.1}}
		},
		"confidence above one": func(a map[string]systemOneAnswer) {
			a["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Confidence: fptr(1.0001)}
		},
		"confidence below zero": func(a map[string]systemOneAnswer) {
			a["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Confidence: fptr(-0.5)}
		},
		"noul above one": func(a map[string]systemOneAnswer) {
			a["approval_required"] = systemOneAnswer{Type: systemOneTypeBoolean, Noul: fptr(1.5)}
		},
		"noul below zero": func(a map[string]systemOneAnswer) {
			a["approval_required"] = systemOneAnswer{Type: systemOneTypeBoolean, Noul: fptr(-0.5)}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			answers := baseAnswers()
			mutate(answers)
			_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

// TestNormalizeSystemOneResponseNaNInf asserts NaN and +Inf/-Inf are rejected for
// every numeric field: probability, confidence, boolean probability, and score.
func TestNormalizeSystemOneResponseNaNInf(t *testing.T) {
	values := map[string]float64{
		"nan":  math.NaN(),
		"+inf": math.Inf(1),
		"-inf": math.Inf(-1),
	}
	for valueName, v := range values {
		v := v
		t.Run("probability "+valueName, func(t *testing.T) {
			answers := baseAnswers()
			answers["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": v}}
			_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
		t.Run("confidence "+valueName, func(t *testing.T) {
			answers := baseAnswers()
			answers["risk"] = systemOneAnswer{Type: systemOneTypeChoice, Choice: "HIGH", Confidence: fptr(v)}
			_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
		t.Run("boolean "+valueName, func(t *testing.T) {
			answers := baseAnswers()
			answers["approval_required"] = systemOneAnswer{Type: systemOneTypeBoolean, Noul: fptr(v)}
			_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
		t.Run("score "+valueName, func(t *testing.T) {
			answers := baseAnswers()
			answers["confidence_score"] = systemOneAnswer{Type: systemOneTypeScore, Score: fptr(v)}
			_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

// TestNormalizeSystemOneResponseIncompatibleTypes asserts an answer whose type
// does not match the question type is rejected.
func TestNormalizeSystemOneResponseIncompatibleTypes(t *testing.T) {
	answers := baseAnswers()
	answers["risk"] = systemOneAnswer{Type: systemOneTypeBoolean, Noul: fptr(0.9)}
	_, err := NormalizeSystemOneResponse(systemOneResponse{Answers: answers}, multiRequest())
	if !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

// TestInUnitRange covers the range helper directly, including NaN/Inf.
func TestInUnitRange(t *testing.T) {
	if err := inUnitRange("p", 0); err != nil {
		t.Errorf("inUnitRange(0) error = %v, want nil", err)
	}
	if err := inUnitRange("p", 1); err != nil {
		t.Errorf("inUnitRange(1) error = %v, want nil", err)
	}
	for _, bad := range []float64{-0.0001, 1.0001, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if err := inUnitRange("p", bad); err == nil {
			t.Errorf("inUnitRange(%v) = nil, want error", bad)
		}
	}
}

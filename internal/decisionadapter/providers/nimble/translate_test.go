package nimble

import (
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// exampleRequest mirrors the SOP example: one state evaluated against three
// provider-neutral questions.
func exampleRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "A schema migration will execute against production during business hours.",
		Questions: []decision.Question{
			{
				ID:       "risk",
				Type:     decision.QuestionChoice,
				Criteria: "Assess the operational risk of the described change.",
				Choices:  []string{"LOW", "MEDIUM", "HIGH"},
			},
			{
				ID:   "approval_required",
				Type: decision.QuestionBoolean,
			},
			{
				ID:       "execution_model",
				Type:     decision.QuestionChoice,
				Criteria: "Select the execution model.",
				Choices:  []string{"SMALL", "MEDIUM", "LARGE"},
			},
		},
	}
}

// TestBuildSystemOneRequestChoice locks in the verified SystemOne contract: a
// CHOICE question's criteria MUST be an object keyed by exactly the allowed
// choices (values may be null); a plain string is rejected with HTTP 400.
func TestBuildSystemOneRequestChoice(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:       "risk",
			Type:     decision.QuestionChoice,
			Criteria: "assess",
			Choices:  []string{"LOW", "HIGH"},
		}},
	}
	sys, err := BuildSystemOneRequest("nimble", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	if sys.Model != "nimble" {
		t.Errorf("Model = %q, want nimble", sys.Model)
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

func TestBuildSystemOneRequestBooleanMapsToNoul(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:   "approval_required",
			Type: decision.QuestionBoolean,
		}},
	}
	sys, err := BuildSystemOneRequest("nimble", req)
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
}

// TestBuildSystemOneRequestScore locks in the verified SystemOne contract: a
// SCORE question's criteria MUST be an array of candidate descriptions, which
// the provider-neutral question carries in Choices.
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
	sys, err := BuildSystemOneRequest("nimble", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	q := sys.Questions["confidence_score"]
	if q.Type != systemOneTypeScore {
		t.Errorf("Type = %q, want %q", q.Type, systemOneTypeScore)
	}
	if strings.TrimSpace(q.Instructions) == "" {
		t.Errorf("Instructions = %q, want non-empty", q.Instructions)
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
// descriptions, so fewer than two Choices is an invalid provider-neutral
// request rather than a silently malformed HTTP call.
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
			_, err := BuildSystemOneRequest("nimble", req)
			if !errors.Is(err, decision.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, "2-26") {
				t.Errorf("error = %q, want it to name the 2-26 candidate requirement", msg)
			}
			if !strings.Contains(msg, "Choices") {
				t.Errorf("error = %q, want it to name the Choices field", msg)
			}
		})
	}
}

// TestBuildSystemOneRequestBareBooleanHasInstructions locks in the fix for the
// live failure: a bare BOOLEAN question (no Instructions, no Criteria) is a
// valid provider-neutral request, but SystemOne requires non-empty
// `instructions`, so the adapter must synthesize one.
func TestBuildSystemOneRequestBareBooleanHasInstructions(t *testing.T) {
	req := decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{{
			ID:   "approval_required",
			Type: decision.QuestionBoolean,
		}},
	}
	sys, err := BuildSystemOneRequest("nimble", req)
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	q := sys.Questions["approval_required"]
	if q.Type != systemOneTypeBoolean {
		t.Errorf("Type = %q, want %q", q.Type, systemOneTypeBoolean)
	}
	if strings.TrimSpace(q.Instructions) == "" {
		t.Errorf("Instructions = %q, want a non-empty synthesized fallback", q.Instructions)
	}
}

// TestBuildSystemOneRequestFallbackInstructions covers the fallback chain for
// every question type: explicit instructions win, then criteria, then a
// synthesized value, and the result is never empty.
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
			sys, err := BuildSystemOneRequest("nimble", req)
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

func TestBuildSystemOneRequestMultipleQuestions(t *testing.T) {
	sys, err := BuildSystemOneRequest("nimble", exampleRequest())
	if err != nil {
		t.Fatalf("BuildSystemOneRequest() error = %v", err)
	}
	if len(sys.Questions) != 3 {
		t.Fatalf("len(Questions) = %d, want 3", len(sys.Questions))
	}
	for _, id := range []string{"risk", "approval_required", "execution_model"} {
		if _, ok := sys.Questions[id]; !ok {
			t.Errorf("missing question %q", id)
		}
	}
}

func TestBuildSystemOneRequestRejectsMalformed(t *testing.T) {
	// Every case carries a valid non-empty State so the rejection is attributable
	// to the malformed question (or the absence of questions), not the state
	// requirement.
	cases := map[string]decision.DecisionRequest{
		"no questions":            {State: "deploying a schema migration during business hours"},
		"unknown type":            {State: "deploying a schema migration during business hours", Questions: []decision.Question{{ID: "x", Type: "ranking"}}},
		"choice without criteria": {State: "deploying a schema migration during business hours", Questions: []decision.Question{{ID: "x", Type: decision.QuestionChoice, Choices: []string{"A"}}}},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := BuildSystemOneRequest("nimble", req)
			if !errors.Is(err, decision.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestNormalizeSystemOneResponseMultiAnswer(t *testing.T) {
	resp := systemOneResponse{
		Model: "nimble",
		Answers: map[string]systemOneAnswer{
			"risk": {
				Type:          systemOneTypeChoice,
				Choice:        "HIGH",
				Probabilities: map[string]float64{"LOW": 0.0013, "MEDIUM": 0.041, "HIGH": 0.9576},
				Confidence:    fptr(0.8349),
			},
			"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9974)},
			"execution_model": {
				Type:          systemOneTypeChoice,
				Choice:        "LARGE",
				Probabilities: map[string]float64{"SMALL": 0.0016, "MEDIUM": 0.037, "LARGE": 0.9613},
				Confidence:    fptr(0.8447),
			},
		},
		Usage: systemOneUsage{InputTokens: 983, OutputTokens: 4},
	}

	got, err := NormalizeSystemOneResponse(resp, exampleRequest())
	if err != nil {
		t.Fatalf("NormalizeSystemOneResponse() error = %v", err)
	}
	if got.Provider != ProviderName || got.Model != "nimble" {
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

	appr := got.Answers["approval_required"]
	if appr.Type != decision.AnswerBoolean || appr.Probability != 0.9974 {
		t.Errorf("approval_required = %+v", appr)
	}

	exec := got.Answers["execution_model"]
	if exec.Type != decision.AnswerChoice || exec.Choice != "LARGE" {
		t.Errorf("execution_model = %+v", exec)
	}
}

func TestNormalizeSystemOneResponseRejections(t *testing.T) {
	req := exampleRequest()
	tests := map[string]systemOneResponse{
		"missing answer": {
			Answers: map[string]systemOneAnswer{
				"risk":            {Type: systemOneTypeChoice, Choice: "HIGH"},
				"execution_model": {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"unknown answer type": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: "ranking"},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"choice outside allowed": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeChoice, Choice: "CRITICAL"},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"probability above one": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 1.5}},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"probability below zero": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeChoice, Choice: "HIGH", Probabilities: map[string]float64{"HIGH": -0.1}},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"unknown probability key": {
			Answers: map[string]systemOneAnswer{
				"risk": {
					Type:          systemOneTypeChoice,
					Choice:        "HIGH",
					Probabilities: map[string]float64{"LOW": 0.1, "MEDIUM": 0.2, "HIGH": 0.6, "BANANA": 0.1},
				},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"confidence out of range": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeChoice, Choice: "HIGH", Confidence: fptr(2)},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"noul out of range": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeChoice, Choice: "HIGH"},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(1.5)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
		"incompatible answer type": {
			Answers: map[string]systemOneAnswer{
				"risk":              {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"approval_required": {Type: systemOneTypeBoolean, Noul: fptr(0.9)},
				"execution_model":   {Type: systemOneTypeChoice, Choice: "LARGE"},
			},
		},
	}

	for name, resp := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeSystemOneResponse(resp, req)
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

func fptr(v float64) *float64 { return &v }

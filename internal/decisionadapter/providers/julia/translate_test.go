package julia

import (
	"errors"
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

// fptr returns a pointer to v, for constructing optional answer fields.
func fptr(v float64) *float64 { return &v }

// exampleRequest is the shared risk-evaluation fixture used across the Julia
// tests: a choice question (LOW/MEDIUM/HIGH), a choice question
// (SMALL/MEDIUM/LARGE), and a boolean question.
func exampleRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "deploying a schema migration during business hours",
		Questions: []decision.Question{
			{
				ID:           "risk",
				Type:         decision.QuestionChoice,
				Criteria:     "Overall risk of the change",
				Choices:      []string{"LOW", "MEDIUM", "HIGH"},
				Instructions: "Assess deployment risk.",
			},
			{
				ID:       "execution_model",
				Type:     decision.QuestionChoice,
				Criteria: "Preferred execution model",
				Choices:  []string{"SMALL", "MEDIUM", "LARGE"},
			},
			{
				ID:           "approval_required",
				Type:         decision.QuestionBoolean,
				Instructions: "Is human approval required?",
			},
		},
	}
}

// okOutputs is a valid raw Outputs for exampleRequest.
func okOutputs() Outputs {
	return Outputs{
		Model: "julia",
		Answers: map[string]QuestionOutput{
			"risk": {
				Choice:        "HIGH",
				Confidence:    fptr(0.83),
				Probabilities: map[string]float64{"HIGH": 0.95, "MEDIUM": 0.04, "LOW": 0.01},
			},
			"execution_model": {
				Choice:     "LARGE",
				Confidence: fptr(0.84),
			},
			"approval_required": {
				Probability: fptr(0.997),
			},
		},
	}
}

func TestBuildInputs(t *testing.T) {
	tests := map[string]struct {
		req      decision.DecisionRequest
		wantText map[string]string
	}{
		"instructions preferred": {
			req: decision.DecisionRequest{
				State: "s",
				Questions: []decision.Question{{
					ID:           "risk",
					Type:         decision.QuestionChoice,
					Criteria:     "criteria text",
					Instructions: "instructions text",
					Choices:      []string{"LOW", "HIGH"},
				}},
			},
			wantText: map[string]string{"risk": "instructions text"},
		},
		"criteria fallback": {
			req: decision.DecisionRequest{
				State: "s",
				Questions: []decision.Question{{
					ID:       "risk",
					Type:     decision.QuestionChoice,
					Criteria: "criteria text",
					Choices:  []string{"LOW", "HIGH"},
				}},
			},
			wantText: map[string]string{"risk": "criteria text"},
		},
		"id fallback": {
			req: decision.DecisionRequest{
				State: "s",
				Questions: []decision.Question{{
					ID:   "approval_required",
					Type: decision.QuestionBoolean,
				}},
			},
			wantText: map[string]string{"approval_required": "Decide approval_required."},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := BuildInputs(tt.req)
			if err != nil {
				t.Fatalf("BuildInputs() error = %v", err)
			}
			if got.State != tt.req.State {
				t.Errorf("State = %q, want %q", got.State, tt.req.State)
			}
			if len(got.Questions) != len(tt.req.Questions) {
				t.Fatalf("len(Questions) = %d, want %d", len(got.Questions), len(tt.req.Questions))
			}
			for _, qi := range got.Questions {
				want, ok := tt.wantText[qi.ID]
				if !ok {
					continue
				}
				if qi.Text != want {
					t.Errorf("question %q Text = %q, want %q", qi.ID, qi.Text, want)
				}
			}
		})
	}
}

func TestBuildInputsOptionsAndQType(t *testing.T) {
	got, err := BuildInputs(exampleRequest())
	if err != nil {
		t.Fatalf("BuildInputs() error = %v", err)
	}
	byID := map[string]QuestionInput{}
	for _, qi := range got.Questions {
		byID[qi.ID] = qi
	}

	risk := byID["risk"]
	if risk.Type != decision.QuestionChoice {
		t.Errorf("risk Type = %q, want choice", risk.Type)
	}
	if risk.QType != juliaQTypeChoice {
		t.Errorf("risk QType = %d, want %d", risk.QType, juliaQTypeChoice)
	}
	if want := []string{"LOW", "MEDIUM", "HIGH"}; !reflect.DeepEqual(risk.Options, want) {
		t.Errorf("risk Options = %v, want %v", risk.Options, want)
	}

	approval := byID["approval_required"]
	if approval.QType != juliaQTypeBoolean {
		t.Errorf("approval_required QType = %d, want %d", approval.QType, juliaQTypeBoolean)
	}
	if want := []string{"false", "true"}; !reflect.DeepEqual(approval.Options, want) {
		t.Errorf("approval_required Options = %v, want %v", approval.Options, want)
	}
}

func TestBuildInputsCopiesOptions(t *testing.T) {
	// BuildInputs must not alias the caller's Choices slice.
	req := exampleRequest()
	got, err := BuildInputs(req)
	if err != nil {
		t.Fatalf("BuildInputs() error = %v", err)
	}
	for i, qi := range got.Questions {
		if qi.ID != "risk" {
			continue
		}
		got.Questions[i].Options[0] = "MUTATED"
	}
	if req.Questions[0].Choices[0] != "LOW" {
		t.Errorf("BuildInputs aliased caller Choices: %v", req.Questions[0].Choices)
	}
}

func TestBuildInputsOptionLimits(t *testing.T) {
	choices := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = string(rune('A' + i))
		}
		return out
	}

	tests := map[string]struct {
		qtype decision.QuestionType
		n     int
		ok    bool
	}{
		"choice 1 option rejected":   {qtype: decision.QuestionChoice, n: 1, ok: false},
		"choice 2 options accepted":  {qtype: decision.QuestionChoice, n: 2, ok: true},
		"choice 20 options accepted": {qtype: decision.QuestionChoice, n: 20, ok: true},
		"choice 21 options rejected": {qtype: decision.QuestionChoice, n: 21, ok: false},
		"score 1 option rejected":    {qtype: decision.QuestionScore, n: 1, ok: false},
		"score 2 options accepted":   {qtype: decision.QuestionScore, n: 2, ok: true},
		"score 20 options accepted":  {qtype: decision.QuestionScore, n: 20, ok: true},
		"score 21 options rejected":  {qtype: decision.QuestionScore, n: 21, ok: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := decision.DecisionRequest{
				State: "s",
				Questions: []decision.Question{{
					ID:       "q",
					Type:     tt.qtype,
					Criteria: "criteria",
					Choices:  choices(tt.n),
				}},
			}
			_, err := BuildInputs(req)
			if tt.ok {
				if err != nil {
					t.Fatalf("BuildInputs() error = %v, want success", err)
				}
				return
			}
			if !errors.Is(err, decision.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestBuildInputsBooleanNeedsNoOptions(t *testing.T) {
	req := decision.DecisionRequest{
		State: "s",
		Questions: []decision.Question{{
			ID:   "approval_required",
			Type: decision.QuestionBoolean,
		}},
	}
	got, err := BuildInputs(req)
	if err != nil {
		t.Fatalf("BuildInputs() error = %v", err)
	}
	if want := []string{"false", "true"}; !reflect.DeepEqual(got.Questions[0].Options, want) {
		t.Errorf("Options = %v, want %v", got.Questions[0].Options, want)
	}
}

func TestBuildInputsInvalidRequest(t *testing.T) {
	if _, err := BuildInputs(decision.DecisionRequest{}); !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestNormalizeOutputsHappyPath(t *testing.T) {
	got, err := NormalizeOutputs(okOutputs(), exampleRequest())
	if err != nil {
		t.Fatalf("NormalizeOutputs() error = %v", err)
	}
	if got.Provider != ProviderName {
		t.Errorf("Provider = %q, want %q", got.Provider, ProviderName)
	}
	if got.Model != "julia" {
		t.Errorf("Model = %q, want julia", got.Model)
	}
	if len(got.Answers) != 3 {
		t.Fatalf("len(Answers) = %d, want 3", len(got.Answers))
	}
	if got.Answers["risk"].Choice != "HIGH" {
		t.Errorf("risk = %+v", got.Answers["risk"])
	}
	if got.Answers["execution_model"].Choice != "LARGE" {
		t.Errorf("execution_model = %+v", got.Answers["execution_model"])
	}
	if got.Answers["approval_required"].Probability != 0.997 {
		t.Errorf("approval_required = %+v", got.Answers["approval_required"])
	}
	if err := got.Validate(); err != nil {
		t.Errorf("result.Validate() = %v", err)
	}
}

func TestNormalizeOutputsRejections(t *testing.T) {
	base := exampleRequest()

	tests := map[string]struct {
		mutate func(*Outputs)
	}{
		"missing answer": {
			mutate: func(o *Outputs) { delete(o.Answers, "risk") },
		},
		"choice outside allowed": {
			mutate: func(o *Outputs) { o.Answers["risk"] = QuestionOutput{Choice: "CRITICAL"} },
		},
		"incompatible type (choice answer without choice)": {
			mutate: func(o *Outputs) {
				o.Answers["risk"] = QuestionOutput{Probability: fptr(0.5)}
			},
		},
		"unknown probability key": {
			mutate: func(o *Outputs) {
				o.Answers["risk"] = QuestionOutput{Choice: "HIGH", Probabilities: map[string]float64{"CRITICAL": 0.5}}
			},
		},
		"probability below zero": {
			mutate: func(o *Outputs) {
				o.Answers["risk"] = QuestionOutput{Choice: "HIGH", Probabilities: map[string]float64{"HIGH": -0.1}}
			},
		},
		"probability above one": {
			mutate: func(o *Outputs) {
				o.Answers["risk"] = QuestionOutput{Choice: "HIGH", Probabilities: map[string]float64{"HIGH": 1.1}}
			},
		},
		"confidence out of range": {
			mutate: func(o *Outputs) { o.Answers["risk"] = QuestionOutput{Choice: "HIGH", Confidence: fptr(1.5)} },
		},
		"boolean with no probability": {
			mutate: func(o *Outputs) { o.Answers["approval_required"] = QuestionOutput{} },
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out := okOutputs()
			tt.mutate(&out)
			_, err := NormalizeOutputs(out, base)
			if !errors.Is(err, decision.ErrMalformedResponse) {
				t.Fatalf("error = %v, want ErrMalformedResponse", err)
			}
		})
	}
}

func TestNormalizeOutputsScoreQuestion(t *testing.T) {
	req := decision.DecisionRequest{
		State: "s",
		Questions: []decision.Question{{
			ID:       "confidence_score",
			Type:     decision.QuestionScore,
			Criteria: "Score the change",
		}},
	}
	out := Outputs{
		Model: "julia",
		Answers: map[string]QuestionOutput{
			"confidence_score": {Score: fptr(7.5), Confidence: fptr(0.9)},
		},
	}
	got, err := NormalizeOutputs(out, req)
	if err != nil {
		t.Fatalf("NormalizeOutputs() error = %v", err)
	}
	if got.Answers["confidence_score"].Score != 7.5 {
		t.Errorf("score = %+v", got.Answers["confidence_score"])
	}
}

// TestNormalizeOutputsScoreMissingValue is a genuine SCORE malformed-output test:
// a SCORE question whose answer carries no score must be rejected. (The previous
// "score with no value" case mutated the risk CHOICE answer, so it never
// exercised normalizeScoreAnswer.)
func TestNormalizeOutputsScoreMissingValue(t *testing.T) {
	req := decision.DecisionRequest{
		State: "s",
		Questions: []decision.Question{{
			ID:       "confidence_score",
			Type:     decision.QuestionScore,
			Criteria: "Score the change",
			Choices:  []string{"LOW", "MEDIUM", "HIGH"},
		}},
	}
	out := Outputs{
		Model:   "julia",
		Answers: map[string]QuestionOutput{"confidence_score": {}},
	}
	_, err := NormalizeOutputs(out, req)
	if !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

func TestNormalizeOutputsBooleanOutOfRange(t *testing.T) {
	out := okOutputs()
	out.Answers["approval_required"] = QuestionOutput{Probability: fptr(1.2)}
	if _, err := NormalizeOutputs(out, exampleRequest()); !errors.Is(err, decision.ErrMalformedResponse) {
		t.Fatalf("error = %v, want ErrMalformedResponse", err)
	}
}

func TestNormalizeOutputsInvalidRequest(t *testing.T) {
	if _, err := NormalizeOutputs(Outputs{}, decision.DecisionRequest{}); !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("error = %v, want ErrInvalidRequest", err)
	}
}

func TestNormalizeOutputsDoesNotRequireSum(t *testing.T) {
	// Probabilities that do not sum to 1.0 must still be accepted.
	out := okOutputs()
	out.Answers["risk"] = QuestionOutput{
		Choice:        "HIGH",
		Probabilities: map[string]float64{"HIGH": 0.5, "LOW": 0.1},
	}
	if _, err := NormalizeOutputs(out, exampleRequest()); err != nil {
		t.Fatalf("NormalizeOutputs() error = %v, want success for non-normalized probabilities", err)
	}
}

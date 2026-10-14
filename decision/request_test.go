package decision

import (
	"errors"
	"strings"
	"testing"
)

func exampleQuestions() []Question {
	return []Question{
		{
			ID:       "risk",
			Type:     QuestionChoice,
			Criteria: "Assess the operational risk of the described change.",
			Choices:  []string{"LOW", "MEDIUM", "HIGH"},
		},
		{
			ID:   "approval_required",
			Type: QuestionBoolean,
		},
		{
			ID:       "execution_model",
			Type:     QuestionChoice,
			Criteria: "Select the execution model.",
			Choices:  []string{"SMALL", "MEDIUM", "LARGE"},
		},
	}
}

func TestDecisionRequestValidateMultiQuestion(t *testing.T) {
	req := DecisionRequest{
		State:     "A schema migration will execute against production during business hours.",
		Questions: exampleQuestions(),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}
	if _, ok := req.Question("approval_required"); !ok {
		t.Fatal("expected to find approval_required")
	}
	if _, ok := req.Question("missing"); ok {
		t.Fatal("did not expect to find missing question")
	}
}

// TestDecisionRequestValidateRequiresState locks in the required state check:
// a request with an empty or whitespace-only State is rejected with
// ErrInvalidRequest, while a non-empty State passes.
func TestDecisionRequestValidateRequiresState(t *testing.T) {
	tests := map[string]struct {
		state   string
		wantErr bool
	}{
		"empty state":           {state: "", wantErr: true},
		"whitespace-only state": {state: "   ", wantErr: true},
		"valid state":           {state: "deploying during business hours", wantErr: false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			req := DecisionRequest{State: tt.state, Questions: exampleQuestions()}
			err := req.Validate()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRequest", err)
			}
			if !strings.Contains(err.Error(), "state is required") {
				t.Errorf("error = %q, want it to contain %q", err.Error(), "state is required")
			}
		})
	}
}

func TestDecisionRequestValidateRejectsEmptyQuestions(t *testing.T) {
	req := DecisionRequest{State: "x"}
	if err := req.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestDecisionRequestValidateRejectsDuplicateIDs(t *testing.T) {
	req := DecisionRequest{
		State: "x",
		Questions: []Question{
			{ID: "a", Type: QuestionBoolean},
			{ID: "a", Type: QuestionBoolean},
		},
	}
	if err := req.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestDecisionRequestValidatePropagatesQuestionErrors(t *testing.T) {
	req := DecisionRequest{
		State:     "x",
		Questions: []Question{{ID: "bad", Type: "unknown"}},
	}
	if err := req.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestQuestionValidateChoice(t *testing.T) {
	valid := Question{ID: "risk", Type: QuestionChoice, Criteria: "assess", Choices: []string{"LOW", "HIGH"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid choice question, got %v", err)
	}

	cases := map[string]Question{
		"missing criteria": {ID: "risk", Type: QuestionChoice, Choices: []string{"LOW"}},
		"no choices":       {ID: "risk", Type: QuestionChoice, Criteria: "assess"},
		"empty choice":     {ID: "risk", Type: QuestionChoice, Criteria: "assess", Choices: []string{""}},
		"duplicate choice": {ID: "risk", Type: QuestionChoice, Criteria: "assess", Choices: []string{"A", "A"}},
	}
	for name, q := range cases {
		if err := q.Validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: expected ErrInvalidRequest, got %v", name, err)
		}
	}
}

func TestQuestionValidateBooleanNeedsNoProviderFields(t *testing.T) {
	q := Question{ID: "approval_required", Type: QuestionBoolean}
	if err := q.Validate(); err != nil {
		t.Fatalf("expected valid boolean question, got %v", err)
	}
}

func TestQuestionValidateScore(t *testing.T) {
	q := Question{ID: "confidence_score", Type: QuestionScore}
	if err := q.Validate(); err != nil {
		t.Fatalf("expected valid score question, got %v", err)
	}
}

func TestQuestionValidateUnknownType(t *testing.T) {
	q := Question{ID: "x", Type: QuestionType("ranking")}
	if err := q.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestQuestionValidateRequiresID(t *testing.T) {
	q := Question{Type: QuestionBoolean}
	if err := q.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestQuestionAllows(t *testing.T) {
	q := Question{ID: "risk", Type: QuestionChoice, Criteria: "c", Choices: []string{"LOW", "HIGH"}}
	if !q.Allows("HIGH") {
		t.Fatal("expected HIGH to be allowed")
	}
	if q.Allows("MEDIUM") {
		t.Fatal("did not expect MEDIUM to be allowed")
	}
}

func TestNoProviderSpecificTermsInPublicAPI(t *testing.T) {
	// Guard against leaking transport terminology into the public contract.
	for name, source := range map[string]string{
		"request.go":  mustRead(t, "request.go"),
		"result.go":   mustRead(t, "result.go"),
		"errors.go":   mustRead(t, "errors.go"),
		"provider.go": mustRead(t, "provider.go"),
	} {
		if strings.Contains(strings.ToLower(source), "noul") {
			t.Errorf("%s must not mention the transport-specific term", name)
		}
	}
}

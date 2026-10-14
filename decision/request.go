package decision

import (
	"fmt"
	"strings"
)

// QuestionType enumerates the provider-neutral kinds of decisions a question can
// ask for. The values are deliberately provider-agnostic: no Nimble, Ollama, or
// SystemOne terminology appears here.
type QuestionType string

const (
	// QuestionChoice asks the model to select exactly one of the question's
	// allowed Choices.
	QuestionChoice QuestionType = "choice"
	// QuestionBoolean asks the model for a yes/no decision. The provider-neutral
	// representation intentionally carries no transport-specific field.
	QuestionBoolean QuestionType = "boolean"
	// QuestionScore asks the model for a numeric score.
	QuestionScore QuestionType = "score"
)

// Question is a provider-neutral description of one decision to evaluate.
//
// Only fields appropriate to the question type are required:
//
//   - CHOICE requires Criteria to be non-empty and Choices to contain at least
//     one distinct, non-empty entry.
//   - BOOLEAN requires no provider-specific representation.
//   - SCORE uses a provider-neutral representation (Criteria may describe the
//     scale); the answer carries a numeric score.
type Question struct {
	// ID identifies the question and keys the corresponding answer. It must be
	// non-empty and unique within a request.
	ID string `json:"id"`

	// Type is the provider-neutral question type.
	Type QuestionType `json:"type"`

	// Instructions is optional free-form guidance for the model.
	Instructions string `json:"instructions,omitempty"`

	// Criteria describes what the question is evaluating. Required for CHOICE.
	Criteria string `json:"criteria,omitempty"`

	// Choices are the allowed outcomes for a CHOICE question.
	Choices []string `json:"choices,omitempty"`
}

// Validate reports whether the question is well-formed. The returned error
// wraps ErrInvalidRequest.
func (q Question) Validate() error {
	if strings.TrimSpace(q.ID) == "" {
		return fmt.Errorf("%w: question id is required", ErrInvalidRequest)
	}

	switch q.Type {
	case QuestionChoice:
		if strings.TrimSpace(q.Criteria) == "" {
			return fmt.Errorf("%w: question %q: criteria is required for a choice question", ErrInvalidRequest, q.ID)
		}
		if len(q.Choices) == 0 {
			return fmt.Errorf("%w: question %q: at least one choice is required", ErrInvalidRequest, q.ID)
		}
		seen := make(map[string]struct{}, len(q.Choices))
		for i, choice := range q.Choices {
			if strings.TrimSpace(choice) == "" {
				return fmt.Errorf("%w: question %q: choice %d is empty", ErrInvalidRequest, q.ID, i)
			}
			if _, dup := seen[choice]; dup {
				return fmt.Errorf("%w: question %q: duplicate choice %q", ErrInvalidRequest, q.ID, choice)
			}
			seen[choice] = struct{}{}
		}
	case QuestionBoolean:
		// A boolean question needs no provider-specific representation: no
		// choices, criteria, or transport fields are required.
	case QuestionScore:
		// A score question is represented provider-neutrally; Criteria may
		// describe the scale but is not required.
	default:
		return fmt.Errorf("%w: question %q: unknown question type %q", ErrInvalidRequest, q.ID, q.Type)
	}

	return nil
}

// Allows reports whether choice is one of the question's allowed choices. It is
// only meaningful for CHOICE questions.
func (q Question) Allows(choice string) bool {
	for _, c := range q.Choices {
		if c == choice {
			return true
		}
	}
	return false
}

// DecisionRequest is a provider-neutral description of one or more decisions to
// evaluate against a single state.
//
// It deliberately contains no SOP policy: approval rules, risk thresholds,
// execution constraints, and safety gates belong to agentic-sop, not here.
type DecisionRequest struct {
	// State is free-form context describing the situation to decide on. It is
	// required: it must be non-empty (ignoring surrounding whitespace).
	State string `json:"state"`

	// Questions are the decisions to evaluate against State. At least one is
	// required and duplicate IDs are rejected.
	Questions []Question `json:"questions"`

	// Metadata holds optional, provider-neutral annotations. Values must not
	// encode SOP policy.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Validate reports whether the request is well-formed. The returned error wraps
// ErrInvalidRequest.
func (r DecisionRequest) Validate() error {
	if strings.TrimSpace(r.State) == "" {
		return fmt.Errorf("%w: state is required", ErrInvalidRequest)
	}
	if len(r.Questions) == 0 {
		return fmt.Errorf("%w: at least one question is required", ErrInvalidRequest)
	}
	seen := make(map[string]struct{}, len(r.Questions))
	for _, q := range r.Questions {
		if err := q.Validate(); err != nil {
			return err
		}
		if _, dup := seen[q.ID]; dup {
			return fmt.Errorf("%w: duplicate question id %q", ErrInvalidRequest, q.ID)
		}
		seen[q.ID] = struct{}{}
	}
	return nil
}

// Question returns the question with the given ID and whether it was found.
func (r DecisionRequest) Question(id string) (Question, bool) {
	for _, q := range r.Questions {
		if q.ID == id {
			return q, true
		}
	}
	return Question{}, false
}

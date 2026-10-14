package decision

import (
	"fmt"
	"math"
)

// AnswerType enumerates the provider-neutral kinds of answers a provider can
// return. The values mirror QuestionType so answers preserve the semantic type
// of the question they answer.
type AnswerType string

const (
	// AnswerChoice is a selected choice, optionally with per-choice
	// probabilities and a confidence.
	AnswerChoice AnswerType = "choice"
	// AnswerBoolean represents a yes/no decision by its underlying probability.
	// No transport-specific field is exposed.
	AnswerBoolean AnswerType = "boolean"
	// AnswerScore represents a numeric score for future use.
	AnswerScore AnswerType = "score"
)

// Answer is the normalized answer to a single question. Only fields appropriate
// to the answer's Type are populated:
//
//   - CHOICE: Choice, Probabilities, and optionally Confidence.
//   - BOOLEAN: Probability (the provider's yes-probability).
//   - SCORE: Score, and optionally Confidence.
type Answer struct {
	// Type is the provider-neutral answer type, matching the question type.
	Type AnswerType `json:"type"`

	// Choice is the selected choice for a CHOICE answer.
	Choice string `json:"choice,omitempty"`

	// Probabilities maps each allowed choice to a finite probability in [0, 1]
	// for CHOICE answers. It is optional but preserved when supplied.
	Probabilities map[string]float64 `json:"probabilities,omitempty"`

	// Confidence is the provider's confidence in the answer, a finite value in
	// [0, 1]. It is optional and preserved only when supplied.
	Confidence *float64 `json:"confidence,omitempty"`

	// Probability is the provider's yes-probability for a BOOLEAN answer, a
	// finite value in [0, 1]. It is provider output only: whether it is
	// sufficient to trigger an approval gate is decided by agentic-sop, not by
	// this adapter.
	Probability float64 `json:"probability,omitempty"`

	// Score is the numeric value for a SCORE answer.
	Score float64 `json:"score,omitempty"`
}

// Validate reports whether the answer is internally consistent. The returned
// error wraps ErrMalformedResponse.
func (a Answer) Validate() error {
	if err := inUnitRange("confidence", a.Confidence); err != nil {
		return err
	}

	switch a.Type {
	case AnswerChoice:
		if a.Choice == "" {
			return fmt.Errorf("%w: choice answer is empty", ErrMalformedResponse)
		}
		for choice, p := range a.Probabilities {
			if !inUnitRangeFloat(p) {
				return fmt.Errorf("%w: probability %v for choice %q out of range [0,1]", ErrMalformedResponse, p, choice)
			}
		}
	case AnswerBoolean:
		if !inUnitRangeFloat(a.Probability) {
			return fmt.Errorf("%w: boolean probability %v out of range [0,1]", ErrMalformedResponse, a.Probability)
		}
	case AnswerScore:
		// A score carries an arbitrary numeric value; no range is imposed here.
	default:
		return fmt.Errorf("%w: unknown answer type %q", ErrMalformedResponse, a.Type)
	}

	return nil
}

func inUnitRange(name string, v *float64) error {
	if v == nil {
		return nil
	}
	if !inUnitRangeFloat(*v) {
		return fmt.Errorf("%w: %s %v out of range [0,1]", ErrMalformedResponse, name, *v)
	}
	return nil
}

// inUnitRangeFloat reports whether v is a finite number within [0, 1]. NaN and
// Inf are rejected explicitly: a direct "< 0 || > 1" comparison is false for
// NaN, which would otherwise let a NaN probability pass as valid.
func inUnitRangeFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1
}

// Usage reports provider-neutral token accounting for a decision.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// DecisionResult is the normalized outcome of a provider evaluation.
//
// Every adapter must translate its native response into this type so callers
// never depend on provider-specific shapes.
type DecisionResult struct {
	// Provider is the adapter that produced the result (for example "nimble").
	Provider string `json:"provider"`

	// Model is the underlying model identifier, when the provider reports one.
	Model string `json:"model,omitempty"`

	// Answers maps each question ID to its normalized answer.
	Answers map[string]Answer `json:"answers"`

	// Usage carries provider-neutral token accounting when reported.
	Usage Usage `json:"usage"`

	// Diagnostics carries optional, non-secret, advisory metadata (for example
	// which backend actually served a failover chain). It is never policy: SOP
	// policy MUST NOT read it, and it never carries credentials or provider prose.
	Diagnostics map[string]string `json:"diagnostics,omitempty"`
}

// Validate reports whether the result is normalized and internally consistent.
// The returned error wraps ErrMalformedResponse.
func (r DecisionResult) Validate() error {
	if len(r.Answers) == 0 {
		return fmt.Errorf("%w: result has no answers", ErrMalformedResponse)
	}
	for id, answer := range r.Answers {
		if err := answer.Validate(); err != nil {
			return fmt.Errorf("answer %q: %w", id, err)
		}
	}
	return nil
}

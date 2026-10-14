package clef

import (
	"fmt"
	"math"
	"strings"

	"github.com/imhttran/agentic-sop/decision"
)

// ProviderName is the stable identifier reported by this adapter.
const ProviderName = "clef"

// SystemOne question/answer type strings. These are transport representations
// and stay confined to this package.
const (
	systemOneTypeChoice  = "choice"
	systemOneTypeScore   = "score"
	systemOneTypeBoolean = "noul"
)

// BuildSystemOneRequest translates a provider-neutral DecisionRequest into a
// SystemOne request for the CLEF-002-selected transport.
//
// The mapping is:
//
//	decision.CHOICE  -> SystemOne "choice" (criteria as an options map, plus choices)
//	decision.BOOLEAN -> SystemOne "noul"
//	decision.SCORE   -> SystemOne "score" (criteria as an array of candidates)
//
// Operations the Clef adapter cannot express return an UNSUPPORTED error (a
// decision.ErrInvalidRequest wrapping of the adapter's ErrUnsupported) rather
// than a success-shaped answer. Malformed questions are rejected before any
// HTTP call.
func BuildSystemOneRequest(model string, req decision.DecisionRequest) (systemOneRequest, error) {
	if err := req.Validate(); err != nil {
		return systemOneRequest{}, err
	}

	questions := make(map[string]systemOneQuestion, len(req.Questions))
	for _, q := range req.Questions {
		translated, err := translateQuestion(q)
		if err != nil {
			return systemOneRequest{}, err
		}
		questions[q.ID] = translated
	}

	return systemOneRequest{
		Model:     model,
		State:     req.State,
		Questions: questions,
	}, nil
}

// translateQuestion maps one provider-neutral question to its SystemOne shape.
func translateQuestion(q decision.Question) (systemOneQuestion, error) {
	instructions := instructionsFor(q)

	switch q.Type {
	case decision.QuestionChoice:
		// SystemOne rejects a CHOICE question whose criteria is a plain string:
		// it requires an object mapping each allowed option key to a description
		// or null.
		criteria := make(map[string]any, len(q.Choices))
		for _, choice := range q.Choices {
			criteria[choice] = nil
		}
		return systemOneQuestion{
			Type:         systemOneTypeChoice,
			Criteria:     criteria,
			Choices:      append([]string(nil), q.Choices...),
			Instructions: instructions,
		}, nil
	case decision.QuestionBoolean:
		return systemOneQuestion{
			Type:         systemOneTypeBoolean,
			Instructions: instructions,
		}, nil
	case decision.QuestionScore:
		// SystemOne rejects a SCORE question whose criteria is a plain string:
		// it requires an array of 2-26 candidate descriptions.
		if len(q.Choices) < 2 {
			return systemOneQuestion{}, fmt.Errorf("%w: %w: question %q: SystemOne score questions require 2-26 candidate descriptions, which a provider-neutral score question carries in Choices (got %d)", decision.ErrInvalidRequest, ErrUnsupported, q.ID, len(q.Choices))
		}
		return systemOneQuestion{
			Type:         systemOneTypeScore,
			Criteria:     append([]string(nil), q.Choices...),
			Instructions: instructions,
		}, nil
	default:
		return systemOneQuestion{}, fmt.Errorf("%w: %w: question %q: unsupported question type %q", decision.ErrInvalidRequest, ErrUnsupported, q.ID, q.Type)
	}
}

// instructionsFor returns a non-empty instruction string for a valid
// provider-neutral question, preferring the caller's explicit instructions and
// falling back to criteria and then to a synthesized, provider-neutral
// instruction.
func instructionsFor(q decision.Question) string {
	if s := strings.TrimSpace(q.Instructions); s != "" {
		return q.Instructions
	}
	if s := strings.TrimSpace(q.Criteria); s != "" {
		return q.Criteria
	}
	return fmt.Sprintf("Decide %s.", q.ID)
}

// NormalizeSystemOneResponse converts a raw SystemOne response into a
// provider-neutral DecisionResult, enforcing the response contract:
//
//   - every requested question must have an answer
//   - answer types must be known and compatible with the question type
//   - CHOICE answers must select one of the allowed choices
//   - CHOICE probability keys must each be one of the allowed choices
//   - probabilities and confidence must be within [0, 1]
//   - NaN/Inf are rejected
//
// It returns errors wrapping decision.ErrMalformedResponse. Indeterminate
// responses (a missing answer, or a value the provider left undefined) remain
// indeterminate: they are never converted into a success-shaped answer.
func NormalizeSystemOneResponse(resp systemOneResponse, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}

	answers := make(map[string]decision.Answer, len(req.Questions))
	for _, q := range req.Questions {
		raw, ok := resp.Answers[q.ID]
		if !ok {
			return decision.DecisionResult{}, fmt.Errorf("%w: missing answer for question %q", decision.ErrMalformedResponse, q.ID)
		}
		answer, err := normalizeAnswer(q, raw)
		if err != nil {
			return decision.DecisionResult{}, err
		}
		answers[q.ID] = answer
	}

	result := decision.DecisionResult{
		Provider: ProviderName,
		Model:    resp.Model,
		Answers:  answers,
		Usage: decision.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}
	if err := result.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	return result, nil
}

// normalizeAnswer converts one SystemOne answer for the given question into a
// provider-neutral answer.
func normalizeAnswer(q decision.Question, raw systemOneAnswer) (decision.Answer, error) {
	switch q.Type {
	case decision.QuestionChoice:
		return normalizeChoiceAnswer(q, raw)
	case decision.QuestionBoolean:
		return normalizeBooleanAnswer(q, raw)
	case decision.QuestionScore:
		return normalizeScoreAnswer(q, raw)
	default:
		return decision.Answer{}, fmt.Errorf("%w: question %q: unsupported question type %q", decision.ErrMalformedResponse, q.ID, q.Type)
	}
}

func normalizeChoiceAnswer(q decision.Question, raw systemOneAnswer) (decision.Answer, error) {
	if raw.Type != systemOneTypeChoice {
		return decision.Answer{}, fmt.Errorf("%w: question %q: incompatible answer type %q", decision.ErrMalformedResponse, q.ID, raw.Type)
	}
	if raw.Choice == "" {
		return decision.Answer{}, fmt.Errorf("%w: question %q: choice is empty", decision.ErrMalformedResponse, q.ID)
	}
	// The returned Choice must belong to the request's allowed set.
	if !q.Allows(raw.Choice) {
		return decision.Answer{}, fmt.Errorf("%w: question %q: choice %q is not one of %v", decision.ErrMalformedResponse, q.ID, raw.Choice, q.Choices)
	}
	for choice, p := range raw.Probabilities {
		if !q.Allows(choice) {
			return decision.Answer{}, fmt.Errorf("%w: question %q: probability returned for unknown choice %q", decision.ErrMalformedResponse, q.ID, choice)
		}
		if err := inUnitRange("probability", p); err != nil {
			return decision.Answer{}, fmt.Errorf("%w: question %q: %s", decision.ErrMalformedResponse, q.ID, err.Error())
		}
	}
	if raw.Confidence != nil {
		if err := inUnitRange("confidence", *raw.Confidence); err != nil {
			return decision.Answer{}, fmt.Errorf("%w: question %q: %s", decision.ErrMalformedResponse, q.ID, err.Error())
		}
	}
	answer := decision.Answer{
		Type:          decision.AnswerChoice,
		Choice:        raw.Choice,
		Probabilities: raw.Probabilities,
		Confidence:    raw.Confidence,
	}
	if err := answer.Validate(); err != nil {
		return decision.Answer{}, err
	}
	return answer, nil
}

func normalizeBooleanAnswer(q decision.Question, raw systemOneAnswer) (decision.Answer, error) {
	if raw.Type != systemOneTypeBoolean {
		return decision.Answer{}, fmt.Errorf("%w: question %q: incompatible answer type %q", decision.ErrMalformedResponse, q.ID, raw.Type)
	}
	if raw.Noul == nil {
		// An absent probability is indeterminate, not a negative answer.
		return decision.Answer{}, fmt.Errorf("%w: question %q: boolean answer has no probability (indeterminate)", decision.ErrMalformedResponse, q.ID)
	}
	if err := inUnitRange("boolean probability", *raw.Noul); err != nil {
		return decision.Answer{}, fmt.Errorf("%w: question %q: %s", decision.ErrMalformedResponse, q.ID, err.Error())
	}
	return decision.Answer{
		Type:        decision.AnswerBoolean,
		Probability: *raw.Noul,
	}, nil
}

func normalizeScoreAnswer(q decision.Question, raw systemOneAnswer) (decision.Answer, error) {
	if raw.Type != systemOneTypeScore {
		return decision.Answer{}, fmt.Errorf("%w: question %q: incompatible answer type %q", decision.ErrMalformedResponse, q.ID, raw.Type)
	}
	if raw.Score == nil {
		// An absent score is indeterminate, not a zero score.
		return decision.Answer{}, fmt.Errorf("%w: question %q: score answer has no value (indeterminate)", decision.ErrMalformedResponse, q.ID)
	}
	if math.IsNaN(*raw.Score) || math.IsInf(*raw.Score, 0) {
		return decision.Answer{}, fmt.Errorf("%w: question %q: score is not a finite number", decision.ErrMalformedResponse, q.ID)
	}
	return decision.Answer{
		Type:  decision.AnswerScore,
		Score: *raw.Score,
	}, nil
}

// inUnitRange reports whether p is a finite value within [0, 1]. It rejects
// NaN and Inf explicitly: NaN comparisons would otherwise slip through the
// range check.
func inUnitRange(name string, p float64) error {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return fmt.Errorf("%s %v is not a finite number", name, p)
	}
	if p < 0 || p > 1 {
		return fmt.Errorf("%s %v out of range [0,1]", name, p)
	}
	return nil
}

package nimble

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/decision"
)

// ProviderName is the stable identifier reported by this adapter.
const ProviderName = "nimble"

// SystemOne question/answer type strings. These are transport representations
// and stay confined to this package.
const (
	systemOneTypeChoice  = "choice"
	systemOneTypeScore   = "score"
	systemOneTypeBoolean = "noul"
)

// BuildSystemOneRequest translates a provider-neutral DecisionRequest into a
// SystemOne request.
//
// The mapping is:
//
//	decision.CHOICE  -> SystemOne "choice" (criteria as an options map, plus choices)
//	decision.BOOLEAN -> SystemOne "noul"
//	decision.SCORE   -> SystemOne "score" (criteria as an array of candidates)
//
// Malformed questions are rejected with decision.ErrInvalidRequest before any
// HTTP call is attempted.
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
	// SystemOne's POST /v1/systemone endpoint requires `instructions` to be a
	// non-empty string, object, or array; it rejects the whole request with HTTP
	// 400 otherwise. A provider-neutral question need not carry instructions or
	// criteria (a bare BOOLEAN is a legitimate caller request per the public
	// decision contract), so the adapter must always synthesize a non-empty,
	// provider-neutral fallback here rather than emitting an empty field.
	instructions := instructionsFor(q)

	switch q.Type {
	case decision.QuestionChoice:
		// SystemOne rejects a CHOICE question whose criteria is a plain string:
		// the verified contract requires an object mapping each allowed option
		// key to a description or null. Emit one nil-valued key per allowed
		// choice, which serializes as e.g. {"LOW":null,"HIGH":null}.
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
		// The provider-neutral BOOLEAN maps onto SystemOne's "noul" type. The
		// semantic interpretation (the probability is provider output, not an
		// SOP approval policy) is documented in nimble.go.
		return systemOneQuestion{
			Type:         systemOneTypeBoolean,
			Instructions: instructions,
		}, nil
	case decision.QuestionScore:
		// SystemOne rejects a SCORE question whose criteria is a plain string:
		// the verified contract requires an array of 2-26 candidate
		// descriptions. The provider-neutral contract has no first-class
		// score-scale field yet, so candidates are carried in Choices; without at
		// least two candidates there is no valid request to emit.
		if len(q.Choices) < 2 {
			return systemOneQuestion{}, fmt.Errorf("%w: question %q: SystemOne score questions require 2-26 candidate descriptions, which a provider-neutral score question carries in Choices (got %d)", decision.ErrInvalidRequest, q.ID, len(q.Choices))
		}
		return systemOneQuestion{
			Type:         systemOneTypeScore,
			Criteria:     append([]string(nil), q.Choices...),
			Instructions: instructions,
		}, nil
	default:
		return systemOneQuestion{}, fmt.Errorf("%w: question %q: unsupported question type %q", decision.ErrInvalidRequest, q.ID, q.Type)
	}
}

// instructionsFor returns a non-empty instruction string for a valid
// provider-neutral question, preferring the caller's explicit instructions and
// falling back to criteria and then to a synthesized, provider-neutral
// instruction derived from the question ID. SystemOne requires non-empty
// instructions, so this guarantees no valid question produces an invalid
// request.
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
//
// It returns errors wrapping decision.ErrMalformedResponse.
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
	if !q.Allows(raw.Choice) {
		return decision.Answer{}, fmt.Errorf("%w: question %q: choice %q is not one of %v", decision.ErrMalformedResponse, q.ID, raw.Choice, q.Choices)
	}
	for choice, p := range raw.Probabilities {
		if !q.Allows(choice) {
			return decision.Answer{}, fmt.Errorf("%w: question %q: probability returned for unknown choice %q", decision.ErrMalformedResponse, q.ID, choice)
		}
		if p < 0 || p > 1 {
			return decision.Answer{}, fmt.Errorf("%w: question %q: probability %v for choice %q out of range [0,1]", decision.ErrMalformedResponse, q.ID, p, choice)
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
		return decision.Answer{}, fmt.Errorf("%w: question %q: boolean answer has no probability", decision.ErrMalformedResponse, q.ID)
	}
	if *raw.Noul < 0 || *raw.Noul > 1 {
		return decision.Answer{}, fmt.Errorf("%w: question %q: probability %v out of range [0,1]", decision.ErrMalformedResponse, q.ID, *raw.Noul)
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
		return decision.Answer{}, fmt.Errorf("%w: question %q: score answer has no value", decision.ErrMalformedResponse, q.ID)
	}
	return decision.Answer{
		Type:  decision.AnswerScore,
		Score: *raw.Score,
	}, nil
}

package julia

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/decision"
)

// ProviderName is the stable identifier reported by this adapter.
const ProviderName = "julia"

// BuildInputs translates a provider-neutral DecisionRequest into the Julia
// adapter/runtime request handed to a Runner.
//
// The mapping is one QuestionInput per question:
//
//   - QType is Julia's native question type (choice 0, score 1, noul/boolean 2).
//   - Text uses the same fallback chain as the Nimble adapter:
//     Instructions -> Criteria -> "Decide <id>."
//   - Options carries the ordered Julia options: the question's Choices for
//     CHOICE and SCORE, and Julia's fixed false/true options for BOOLEAN.
//
// Julia-specific constraints (the 2..20 option limit) are enforced here, so a
// request that is valid for the provider-neutral contract but not for Julia is
// rejected with decision.ErrInvalidRequest before any inference is attempted.
func BuildInputs(req decision.DecisionRequest) (Inputs, error) {
	if err := req.Validate(); err != nil {
		return Inputs{}, err
	}

	questions := make([]QuestionInput, 0, len(req.Questions))
	for _, q := range req.Questions {
		options, err := juliaOptions(q)
		if err != nil {
			return Inputs{}, err
		}
		qtype, err := juliaQType(q.Type)
		if err != nil {
			return Inputs{}, err
		}
		questions = append(questions, QuestionInput{
			ID:      q.ID,
			Type:    q.Type,
			QType:   qtype,
			Text:    instructionsFor(q),
			Options: options,
		})
	}

	return Inputs{
		State:     req.State,
		Questions: questions,
	}, nil
}

// instructionsFor returns a non-empty instruction string for a valid
// provider-neutral question, preferring the caller's explicit instructions and
// falling back to criteria and then to a synthesized, provider-neutral
// instruction derived from the question ID.
func instructionsFor(q decision.Question) string {
	if s := strings.TrimSpace(q.Instructions); s != "" {
		return q.Instructions
	}
	if s := strings.TrimSpace(q.Criteria); s != "" {
		return q.Criteria
	}
	return fmt.Sprintf("Decide %s.", q.ID)
}

// NormalizeOutputs converts a raw Runner Outputs into a provider-neutral
// DecisionResult, enforcing the response contract:
//
//   - every requested question must have an answer
//   - CHOICE answers must select one of the allowed labels, every probability
//     key must be an allowed label, and every probability must be within [0, 1]
//   - BOOLEAN answers must carry a probability within [0, 1]
//   - SCORE answers must carry a score
//   - confidence, when present, must be within [0, 1]
//
// The response-shape rules above are the same for every provider; the Julia
// request-side option-count limits are enforced by BuildInputs (and therefore by
// Decide) on the way in, not here.
//
// Probabilities are not required to sum to 1.0. It returns errors wrapping
// decision.ErrMalformedResponse.
func NormalizeOutputs(out Outputs, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}

	answers := make(map[string]decision.Answer, len(req.Questions))
	for _, q := range req.Questions {
		raw, ok := out.Answers[q.ID]
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
		Model:    out.Model,
		Answers:  answers,
	}
	if err := result.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	return result, nil
}

// normalizeAnswer converts one raw question output for the given question into
// a provider-neutral answer.
func normalizeAnswer(q decision.Question, raw QuestionOutput) (decision.Answer, error) {
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

func normalizeChoiceAnswer(q decision.Question, raw QuestionOutput) (decision.Answer, error) {
	if raw.Choice == "" {
		return decision.Answer{}, fmt.Errorf("%w: question %q: choice answer is empty", decision.ErrMalformedResponse, q.ID)
	}
	if !q.Allows(raw.Choice) {
		return decision.Answer{}, fmt.Errorf("%w: question %q: choice %q is not one of %v", decision.ErrMalformedResponse, q.ID, raw.Choice, q.Choices)
	}
	for label, p := range raw.Probabilities {
		if !q.Allows(label) {
			return decision.Answer{}, fmt.Errorf("%w: question %q: probability returned for unknown choice %q", decision.ErrMalformedResponse, q.ID, label)
		}
		if p < 0 || p > 1 {
			return decision.Answer{}, fmt.Errorf("%w: question %q: probability %v for choice %q out of range [0,1]", decision.ErrMalformedResponse, q.ID, p, label)
		}
	}
	if err := validateConfidence(q.ID, raw.Confidence); err != nil {
		return decision.Answer{}, err
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

func normalizeBooleanAnswer(q decision.Question, raw QuestionOutput) (decision.Answer, error) {
	if raw.Probability == nil {
		return decision.Answer{}, fmt.Errorf("%w: question %q: boolean answer has no probability", decision.ErrMalformedResponse, q.ID)
	}
	if *raw.Probability < 0 || *raw.Probability > 1 {
		return decision.Answer{}, fmt.Errorf("%w: question %q: probability %v out of range [0,1]", decision.ErrMalformedResponse, q.ID, *raw.Probability)
	}
	if err := validateConfidence(q.ID, raw.Confidence); err != nil {
		return decision.Answer{}, err
	}
	return decision.Answer{
		Type:        decision.AnswerBoolean,
		Probability: *raw.Probability,
		Confidence:  raw.Confidence,
	}, nil
}

func normalizeScoreAnswer(q decision.Question, raw QuestionOutput) (decision.Answer, error) {
	if raw.Score == nil {
		return decision.Answer{}, fmt.Errorf("%w: question %q: score answer has no value", decision.ErrMalformedResponse, q.ID)
	}
	if err := validateConfidence(q.ID, raw.Confidence); err != nil {
		return decision.Answer{}, err
	}
	return decision.Answer{
		Type:       decision.AnswerScore,
		Score:      *raw.Score,
		Confidence: raw.Confidence,
	}, nil
}

// validateConfidence rejects a confidence outside [0, 1].
func validateConfidence(id string, confidence *float64) error {
	if confidence == nil {
		return nil
	}
	if *confidence < 0 || *confidence > 1 {
		return fmt.Errorf("%w: question %q: confidence %v out of range [0,1]", decision.ErrMalformedResponse, id, *confidence)
	}
	return nil
}

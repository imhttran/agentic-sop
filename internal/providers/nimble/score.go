package nimble

// Standalone Nimble /score transport (additive).
//
// This transport targets the standalone Bespoke-Nimble-9B scoring service that
// exposes ParallelScorer.score(context, schema) over POST /score. It is a SEPARATE
// path from the Ollama /v1/systemone transport, which is preserved unchanged and
// remains the default. The adapter only translates and normalizes; it defines no
// SOP policy.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/imhttran/sop-decision-adapters/decision"
)

// scoreField is one field of the /score request schema. The service supports
// "enum" (a list of choices) and "boolean".
type scoreField struct {
	Type        string   "json:\"type\""
	Description string   "json:\"description\""
	Choices     []string "json:\"choices,omitempty\""
}

// scoreRequest is the body of POST /score.
type scoreRequest struct {
	Context string                "json:\"context\""
	Schema  map[string]scoreField "json:\"schema\""
}

// scoreFieldResult is one field of the /score response: the chosen value and the
// per-choice softmax scores.
type scoreFieldResult struct {
	Value  any                "json:\"value\""
	Scores map[string]float64 "json:\"scores,omitempty\""
}

// scoreResponse is the body of a successful /score response.
type scoreResponse struct {
	Model    string                      "json:\"model\""
	Revision string                      "json:\"revision,omitempty\""
	Backend  string                      "json:\"backend,omitempty\""
	Output   map[string]any              "json:\"output\""
	Fields   map[string]scoreFieldResult "json:\"fields\""
}

// Scorer performs a standalone /score call and an availability probe. It is an
// interface so the provider can be tested without a running service.
type Scorer interface {
	Score(ctx context.Context, req scoreRequest) (scoreResponse, error)
	Health(ctx context.Context) error
}

// ScoreTransport is the production Scorer backed by the standalone /score
// service.
type ScoreTransport struct {
	scoreURL  string
	healthURL string
	client    *http.Client
}

// NewScoreTransport builds a ScoreTransport for scoreURL. A non-positive timeout
// falls back to DefaultTimeout. The health URL is derived from the score URL
// (its sibling /health).
func NewScoreTransport(scoreURL string, timeout time.Duration) *ScoreTransport {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	u := strings.TrimRight(strings.TrimSpace(scoreURL), "/")
	if u == "" {
		u = DefaultScoreURL
	}
	health := u
	if strings.HasSuffix(health, "/score") {
		health = strings.TrimSuffix(health, "/score") + "/health"
	}
	return &ScoreTransport{scoreURL: u, healthURL: health, client: &http.Client{Timeout: timeout}}
}

// Score performs POST {scoreURL} and returns the raw response.
func (t *ScoreTransport) Score(ctx context.Context, req scoreRequest) (scoreResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return scoreResponse{}, fmt.Errorf("nimble: encode score request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.scoreURL, bytes.NewReader(body))
	if err != nil {
		return scoreResponse{}, fmt.Errorf("nimble: build score request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return scoreResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return scoreResponse{}, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	var out scoreResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return scoreResponse{}, &malformedResponseError{Err: fmt.Errorf("decode score response: %w", err)}
	}
	return out, nil
}

// Health performs GET {healthURL}; a non-200 is an error.
func (t *ScoreTransport) Health(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.healthURL, nil)
	if err != nil {
		return fmt.Errorf("nimble: build health request: %w", err)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	return nil
}

// BuildScoreRequest maps a provider-neutral CHOICE question to the standalone
// /score schema (an "enum" field named by the question ID). A BOOLEAN or SCORE
// question is not representable for this decision kind through
// ParallelScorer.score and is rejected as invalid.
func BuildScoreRequest(req decision.DecisionRequest) (scoreRequest, error) {
	if err := req.Validate(); err != nil {
		return scoreRequest{}, err
	}
	schema := make(map[string]scoreField, len(req.Questions))
	for _, q := range req.Questions {
		if q.Type != decision.QuestionChoice {
			return scoreRequest{}, fmt.Errorf("%w: question %q: the standalone /score transport supports choice questions only", decision.ErrInvalidRequest, q.ID)
		}
		if len(q.Choices) == 0 {
			return scoreRequest{}, fmt.Errorf("%w: question %q: a choice question requires choices", decision.ErrInvalidRequest, q.ID)
		}
		schema[q.ID] = scoreField{
			Type:        "enum",
			Description: scoreDescription(q),
			Choices:     append([]string(nil), q.Choices...),
		}
	}
	return scoreRequest{Context: req.State, Schema: schema}, nil
}

// scoreDescription returns the nonempty description the /score schema requires.
func scoreDescription(q decision.Question) string {
	if s := strings.TrimSpace(q.Instructions); s != "" {
		return s
	}
	if s := strings.TrimSpace(q.Criteria); s != "" {
		return s
	}
	return fmt.Sprintf("Decide %s.", q.ID)
}

// NormalizeScoreResponse converts a /score response into a provider-neutral
// DecisionResult. The chosen choice is the field value; the confidence is the
// maximum per-choice score (the service's softmax probability).
func NormalizeScoreResponse(resp scoreResponse, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	answers := make(map[string]decision.Answer, len(req.Questions))
	for _, q := range req.Questions {
		f, ok := resp.Fields[q.ID]
		if !ok {
			return decision.DecisionResult{}, fmt.Errorf("%w: missing field %q in score response", decision.ErrMalformedResponse, q.ID)
		}
		choice, ok := f.Value.(string)
		if !ok || strings.TrimSpace(choice) == "" {
			return decision.DecisionResult{}, fmt.Errorf("%w: field %q: choice value is missing", decision.ErrMalformedResponse, q.ID)
		}
		if !q.Allows(choice) {
			return decision.DecisionResult{}, fmt.Errorf("%w: field %q: choice %q is not one of %v", decision.ErrMalformedResponse, q.ID, choice, q.Choices)
		}
		probs := make(map[string]float64, len(f.Scores))
		var conf float64
		for k, p := range f.Scores {
			if !q.Allows(k) {
				return decision.DecisionResult{}, fmt.Errorf("%w: field %q: score for unknown choice %q", decision.ErrMalformedResponse, q.ID, k)
			}
			if p < 0 || p > 1 {
				return decision.DecisionResult{}, fmt.Errorf("%w: field %q: score %v for %q out of range [0,1]", decision.ErrMalformedResponse, q.ID, p, k)
			}
			probs[k] = p
			if p > conf {
				conf = p
			}
		}
		confidence := conf
		answers[q.ID] = decision.Answer{
			Type:          decision.AnswerChoice,
			Choice:        choice,
			Probabilities: probs,
			Confidence:    &confidence,
		}
	}
	result := decision.DecisionResult{Provider: ProviderName, Model: resp.Model, Answers: answers}
	if err := result.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	return result, nil
}

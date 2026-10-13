package clef

// OpenAI-compatible chat-completions transport (additive).
//
// This transport targets an OpenAI-style /v1/chat/completions backend such as
// oMLX serving clef-4bit. It is a SEPARATE path from the Ollama /v1/systemone
// transport, which is preserved unchanged and remains the default. The adapter
// only translates and normalizes; it defines no SOP policy.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/imhttran/sop-decision-adapters/decision"
)

// DefaultChatBaseURL is the conventional local oMLX endpoint.
const DefaultChatBaseURL = "http://127.0.0.1:8000"

type chatMessage struct {
	Role    string "json:\"role\""
	Content string "json:\"content\""
}

type chatRequest struct {
	Model       string        "json:\"model\""
	Messages    []chatMessage "json:\"messages\""
	Temperature float64       "json:\"temperature\""
	MaxTokens   int           "json:\"max_tokens,omitempty\""
}

type chatResponse struct {
	Model   string "json:\"model\""
	Choices []struct {
		Message      chatMessage "json:\"message\""
		FinishReason string      "json:\"finish_reason\""
	} "json:\"choices\""
	Usage struct {
		PromptTokens     int "json:\"prompt_tokens\""
		CompletionTokens int "json:\"completion_tokens\""
	} "json:\"usage\""
}

// chatTransport performs a chat completion and a model listing for availability.
type chatTransport interface {
	Chat(ctx context.Context, req chatRequest) (chatResponse, error)
	Models(ctx context.Context) ([]string, error)
}

type httpChatTransport struct {
	baseURL string
	client  *http.Client
}

func newHTTPChatTransport(baseURL string, timeout time.Duration) *httpChatTransport {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		u = DefaultChatBaseURL
	}
	return &httpChatTransport{baseURL: u, client: &http.Client{Timeout: timeout}}
}

func (t *httpChatTransport) Chat(ctx context.Context, req chatRequest) (chatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return chatResponse{}, fmt.Errorf("clef: encode chat request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatResponse{}, fmt.Errorf("clef: build chat request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return chatResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return chatResponse{}, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return chatResponse{}, &malformedResponseError{Err: fmt.Errorf("decode chat response: %w", err)}
	}
	return out, nil
}

// Models performs GET {base}/v1/models (the OpenAI listing) for availability.
func (t *httpChatTransport) Models(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, t.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("clef: build model request: %w", err)
	}
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(msg))}
	}
	var list struct {
		Data []struct {
			ID string "json:\"id\""
		} "json:\"data\""
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, &malformedResponseError{Err: fmt.Errorf("decode model list: %w", err)}
	}
	names := make([]string, 0, len(list.Data))
	for _, m := range list.Data {
		if id := strings.TrimSpace(m.ID); id != "" {
			names = append(names, id)
		}
	}
	return names, nil
}

// chatSystemPrompt instructs the model to answer with the decision JSON only.
const chatSystemPrompt = "You are a decision scorer. For each question choose exactly one of its allowed choices and give a confidence in [0,1]. Reply ONLY with compact JSON: {\"answers\":{\"<question id>\":{\"choice\":\"<allowed choice>\",\"confidence\":<number>}}}."

// BuildChatRequest translates a provider-neutral CHOICE question into a chat
// completion request. BOOLEAN and SCORE questions are not representable through
// this transport and are rejected as invalid.
func BuildChatRequest(model string, req decision.DecisionRequest) (chatRequest, error) {
	if err := req.Validate(); err != nil {
		return chatRequest{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Context: %s\n", req.State)
	for _, q := range req.Questions {
		if q.Type != decision.QuestionChoice {
			return chatRequest{}, fmt.Errorf("%w: question %q: the chat-completions transport supports choice questions only", decision.ErrInvalidRequest, q.ID)
		}
		if len(q.Choices) == 0 {
			return chatRequest{}, fmt.Errorf("%w: question %q: a choice question requires choices", decision.ErrInvalidRequest, q.ID)
		}
		fmt.Fprintf(&b, "Question %s: %s Choose one of [%s].\n", q.ID, instructionsFor(q), strings.Join(q.Choices, ", "))
	}
	b.WriteString("Reply with the JSON described in the system message.")
	return chatRequest{
		Model:       model,
		Messages:    []chatMessage{{Role: "system", Content: chatSystemPrompt}, {Role: "user", Content: b.String()}},
		Temperature: 0,
	}, nil
}

// choiceAndConfidence extracts the chosen choice and the raw confidence from one
// answer object, accepting either {"choice":..,"confidence":..} or the
// single-key {"<allowed choice>": <confidence>}.
func choiceAndConfidence(m map[string]json.RawMessage) (string, json.RawMessage, error) {
	if raw, ok := m["choice"]; ok {
		var choice string
		if err := json.Unmarshal(raw, &choice); err != nil {
			return "", nil, fmt.Errorf("choice is not a string")
		}
		return strings.TrimSpace(choice), m["confidence"], nil
	}
	if len(m) == 1 {
		for k, v := range m {
			return strings.TrimSpace(k), v, nil
		}
	}
	return "", nil, fmt.Errorf("answer must carry a choice")
}

// NormalizeChatResponse converts a chat-completions response into a provider-
// neutral DecisionResult. The model content must be the decision JSON. The
// adapter tolerates the wrapper the model may omit: answers may be nested under
// "answers" or be the object itself, and each answer may be
// {"choice":..,"confidence":..} or the single-key {"<allowed choice>":
// <confidence>}. The chosen choice must be an allowed choice with a finite
// [0,1] confidence; anything else is malformed and never a decision.
func NormalizeChatResponse(resp chatResponse, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	if len(resp.Choices) == 0 {
		return decision.DecisionResult{}, fmt.Errorf("%w: chat response carried no choices", decision.ErrMalformedResponse)
	}
	content := resp.Choices[0].Message.Content
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return decision.DecisionResult{}, fmt.Errorf("%w: chat response content is not JSON", decision.ErrMalformedResponse)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content[start:end+1]), &top); err != nil {
		return decision.DecisionResult{}, fmt.Errorf("%w: decode chat answers: %v", decision.ErrMalformedResponse, err)
	}
	answersRaw := top
	if nested, ok := top["answers"]; ok {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(nested, &m); err == nil {
			answersRaw = m
		}
	}
	answers := make(map[string]decision.Answer, len(req.Questions))
	for _, q := range req.Questions {
		rawQ, ok := answersRaw[q.ID]
		if !ok {
			return decision.DecisionResult{}, fmt.Errorf("%w: chat response omitted question %q", decision.ErrMalformedResponse, q.ID)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(rawQ, &m); err != nil {
			return decision.DecisionResult{}, fmt.Errorf("%w: question %q: answer is not an object", decision.ErrMalformedResponse, q.ID)
		}
		choice, rawConf, err := choiceAndConfidence(m)
		if err != nil {
			return decision.DecisionResult{}, fmt.Errorf("%w: question %q: %v", decision.ErrMalformedResponse, q.ID, err)
		}
		if choice == "" || !q.Allows(choice) {
			return decision.DecisionResult{}, fmt.Errorf("%w: question %q: choice %q is not one of %v", decision.ErrMalformedResponse, q.ID, choice, q.Choices)
		}
		var conf float64
		if rawConf == nil || json.Unmarshal(rawConf, &conf) != nil ||
			math.IsNaN(conf) || math.IsInf(conf, 0) || conf < 0 || conf > 1 {
			return decision.DecisionResult{}, fmt.Errorf("%w: question %q: confidence must be a finite value in [0,1]", decision.ErrMalformedResponse, q.ID)
		}
		confidence := conf
		answers[q.ID] = decision.Answer{Type: decision.AnswerChoice, Choice: choice, Confidence: &confidence}
	}
	result := decision.DecisionResult{
		Provider: ProviderName,
		Model:    resp.Model,
		Answers:  answers,
		Usage:    decision.Usage{InputTokens: resp.Usage.PromptTokens, OutputTokens: resp.Usage.CompletionTokens},
	}
	if err := result.Validate(); err != nil {
		return decision.DecisionResult{}, err
	}
	return result, nil
}

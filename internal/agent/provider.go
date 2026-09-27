package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// EnvAgentProvider selects which provider the composition root builds. It is
// optional: when unset the historical subprocess command agent is used, so
// existing configurations keep working unchanged.
const EnvAgentProvider = "SOP_AGENT_PROVIDER"

// Provider names accepted by EnvAgentProvider.
const (
	ProviderCommand  = "command"
	ProviderOllama   = "ollama"
	ProviderLlamaCpp = "llamacpp"
)

const (
	// defaultProviderTimeout bounds a single model call when none is configured.
	defaultProviderTimeout = 120 * time.Second
	// maxResponseBytes caps how much of a provider response is read, so a
	// misbehaving server cannot exhaust memory.
	maxResponseBytes = 10 << 20 // 10 MiB
	// maxErrorDetail caps how much of an error body is echoed back.
	maxErrorDetail = 2048
)

// FromEnv builds the Agent named by EnvAgentProvider, or the default command
// agent when it is unset.
func FromEnv() (Agent, error) {
	return FromConfig("")
}

// FromConfig builds the Agent for a configured provider name, with
// EnvAgentProvider overriding the configured value when it is set. A blank
// provider means "let the environment decide", which keeps the historical
// command agent as the default. An unsupported name is an error rather than a
// silent fallback.
func FromConfig(provider string) (Agent, error) {
	name, _ := EffectiveProvider(provider)
	switch name {
	case ProviderCommand:
		return NewCommandAgentFromEnv()
	case ProviderOllama:
		return NewOllamaFromEnv()
	case ProviderLlamaCpp:
		return NewLlamaCppFromEnv()
	default:
		return nil, fmt.Errorf("unknown agent provider %q: want %s, %s or %s",
			name, ProviderCommand, ProviderOllama, ProviderLlamaCpp)
	}
}

// ProviderSource records where the effective provider name came from.
type ProviderSource string

const (
	// SourceEnvironment: the SOP_AGENT_PROVIDER environment variable.
	SourceEnvironment ProviderSource = "environment"
	// SourceConfiguration: agent.provider in .agent-sdlc/config.yaml.
	SourceConfiguration ProviderSource = "configuration"
	// SourceDefault: neither was set, so the historical command agent is used.
	SourceDefault ProviderSource = "default"
)

// EffectiveProvider returns the provider name FromConfig would use for a
// configured provider, and where it came from: SOP_AGENT_PROVIDER (environment)
// wins over the configured value, which wins over the default command provider.
func EffectiveProvider(configured string) (string, ProviderSource) {
	if env := strings.TrimSpace(os.Getenv(EnvAgentProvider)); env != "" {
		return env, SourceEnvironment
	}
	if cfg := strings.TrimSpace(configured); cfg != "" {
		return cfg, SourceConfiguration
	}
	return ProviderCommand, SourceDefault
}

// renderPrompt renders a Request into the single user message that local text
// providers send. It keeps the provider-agnostic Request shape as the input
// instead of inventing provider-specific concepts.
func renderPrompt(r Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Capability: %s\n\n", r.Capability)
	fmt.Fprintf(&b, "Task:\n%s\n", strings.TrimSpace(r.Task))
	if input := strings.TrimSpace(r.Input); input != "" {
		fmt.Fprintf(&b, "\nInput:\n%s\n", input)
	}
	if reqs := strings.TrimSpace(r.OutputRequirements); reqs != "" {
		fmt.Fprintf(&b, "\nOutput requirements:\n%s\n", reqs)
	}
	return b.String()
}

// chatMessage is the minimal chat turn shape shared by local providers.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the minimal chat completion request shared by local providers.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// timeoutFromEnv parses a duration environment variable, falling back to the
// default when unset. A present but invalid value is an error, not a silent
// fallback, so a typo cannot quietly disable a configured timeout.
func timeoutFromEnv(key string) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultProviderTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s: duration must be positive, got %q", key, raw)
	}
	return d, nil
}

// postJSON marshals in as JSON, POSTs it to url, and decodes a 2xx body into
// out. Non-2xx responses become errors that include the status and a bounded
// body snippet; context cancellation stays distinguishable via errors.Is.
func postJSON(ctx context.Context, client *http.Client, url, apiKey string, in, out any) error {
	payload, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("request canceled: %w", ctxErr)
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(data))
		if len(detail) > maxErrorDetail {
			detail = detail[:maxErrorDetail]
		}
		if detail != "" {
			return fmt.Errorf("http %d: %s", resp.StatusCode, detail)
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// rawError renders an optional provider error field that may be a string, an
// object, or absent, so a 200-with-error body is reported instead of silently
// treated as empty output.
func rawError(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(string(raw))
}

// requireNonEmpty returns an error naming the provider when content is blank,
// matching the command agent's contract that a model call must return output.
func requireNonEmpty(provider string, cap Capability, content string) (Response, error) {
	if strings.TrimSpace(content) == "" {
		return Response{}, fmt.Errorf("%s %s returned empty output", provider, cap)
	}
	return Response{Content: content}, nil
}

// normalizeBaseURL trims whitespace and any trailing slashes from a base URL.
func normalizeBaseURL(baseURL string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// errMissingEnv is the shared "set this variable" error for provider settings.
func errMissingEnv(provider, key string) error {
	return fmt.Errorf("%s: set %s", provider, key)
}

var errBaseURLRequired = errors.New("base URL is required")

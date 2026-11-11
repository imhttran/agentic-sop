package agent

import (
	"context"
	"os"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// Redacted wraps an Agent so the caller-supplied text a model receives (task, input,
// and output requirements) passes model-input redaction before any transport renders
// it. Every chat transport here builds its prompt from a Request, so this one wrapper
// covers the OpenAI-compatible, llama.cpp, MLX, and Ollama paths. The C1 rules run
// only when policy is enabled; the baseline known-format and environment-value pass
// always runs.
type Redacted struct {
	inner  Agent
	policy redact.Policy
}

// NewRedacted wraps a, redacting each request under policy.
func NewRedacted(a Agent, policy redact.Policy) *Redacted {
	return &Redacted{inner: a, policy: policy}
}

// Capabilities reports the wrapped agent's capabilities, so NewChecked still
// routes by what the real provider serves.
func (r *Redacted) Capabilities() Capabilities { return CapabilitiesOf(r.inner) }

// Generate redacts the request's model-bound text and delegates.
func (r *Redacted) Generate(ctx context.Context, request Request) (Response, error) {
	env := os.Environ()
	scrub := func(s string) string { return redact.ModelInputWithPolicy(s, env, r.policy) }
	request.Task = scrub(request.Task)
	request.Input = scrub(request.Input)
	request.OutputRequirements = scrub(request.OutputRequirements)
	return r.inner.Generate(ctx, request)
}

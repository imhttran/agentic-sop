// Package provider defines SOP's provider/runtime abstraction: the layer that
// reports which model server is configured, whether it is reachable, which
// models it serves, and what those models can do.
//
// It sits underneath SOP's model-class routing (internal/model, internal/router)
// and deliberately does not participate in it. Phase 3.5 remains authoritative
// for choosing the small/medium/large class; a provider only inspects the runtime
// that serves the already-selected model. Providers MUST NOT choose a model
// class, mutate repository or SOP state, or control any lifecycle transition —
// the Provider interface below has no method that could.
package provider

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
)

// ID is a closed provider identifier. Provider names MUST be validated through
// ParseID rather than compared as raw strings, so an unknown provider fails with
// one actionable error instead of silently matching nothing.
type ID string

const (
	// Ollama: a local or cloud Ollama server (text-only, one model per call).
	Ollama ID = "ollama"
	// LlamaCPP: an OpenAI-compatible llama.cpp llama-server endpoint.
	LlamaCPP ID = "llamacpp"
	// Command: an external subprocess that is already a full agent; it exposes
	// no model-list or health API of its own.
	Command ID = "command"
	// MLX: an Apple-Silicon MLX / oMLX runtime behind an OpenAI-compatible HTTP
	// boundary.
	MLX ID = "mlx"
	// OpenaiCompatible: a generic OpenAI-compatible inference endpoint (for
	// example vLLM, LM Studio, LocalAI, Hugging Face or NVIDIA NIM inference
	// servers). It speaks the same /v1/models and chat-completions vocabulary as
	// the specialized mlx and llamacpp adapters, but is not tied to a specific
	// runtime. It is a distinct identity from mlx and llamacpp, which keep their
	// own identities and default endpoints.
	OpenaiCompatible ID = "openai_compatible"
)

// knownIDs is the canonical, stable set of provider identifiers. Identity lists
// in the model, config, agent, and CLI layers derive from or are checked against
// this set; see internal/model/provider_identity_test.go for the drift guard.
var knownIDs = []ID{Ollama, LlamaCPP, Command, MLX, OpenaiCompatible}

// ParseID validates a provider name. An empty string is rejected; callers that
// treat "unset" specially must check for it first.
func ParseID(s string) (ID, error) {
	id := ID(strings.TrimSpace(s))
	if id.Valid() {
		return id, nil
	}
	return "", fmt.Errorf("%w %q (want %s)", ErrUnknownProvider, s, joinIDs(knownIDs))
}

// Valid reports whether the identifier is one of the known providers.
func (id ID) Valid() bool {
	for _, k := range knownIDs {
		if id == k {
			return true
		}
	}
	return false
}

// String renders the identifier.
func (id ID) String() string { return string(id) }

// KnownIDs returns the known provider identifiers in canonical order.
func KnownIDs() []ID {
	out := make([]ID, len(knownIDs))
	copy(out, knownIDs)
	return out
}

// ModelInfo describes one model a provider serves. Locality is left empty when
// the runtime cannot determine it reliably (for example Ollama's model list does
// not report local vs cloud); the authoritative locality for a run is the model
// routing selection's, not a provider's guess.
type ModelInfo struct {
	Name         string
	Provider     ID
	Locality     model.Locality
	Capabilities Capabilities

	// Optional metadata. Each field is populated only when the runtime reports it
	// reliably; a zero value means "not determined", never "absent". Like
	// Capabilities it is evidence only and MUST NOT influence routing or any
	// lifecycle state. Ollama fills these from /api/tags; the OpenAI-compatible
	// adapters leave them unset, because /v1/models reports no such detail.
	Family        string
	ParameterSize string
	Quantization  string
	ContextWindow int
	SizeBytes     int64
}

// Metadata renders the optional metadata as a deterministic "key=value" list,
// omitting fields the provider did not determine so the output shows only what was
// actually observed. It returns "" when nothing is known.
func (m ModelInfo) Metadata() string {
	var parts []string
	if m.Family != "" {
		parts = append(parts, "family="+m.Family)
	}
	if m.ParameterSize != "" {
		parts = append(parts, "params="+m.ParameterSize)
	}
	if m.Quantization != "" {
		parts = append(parts, "quant="+m.Quantization)
	}
	if m.ContextWindow > 0 {
		parts = append(parts, "ctx="+strconv.Itoa(m.ContextWindow))
	}
	if m.SizeBytes > 0 {
		parts = append(parts, "size="+humanBytes(m.SizeBytes))
	}
	return strings.Join(parts, " ")
}

// humanBytes renders a byte count in binary units with one decimal place.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	labels := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	i := -1
	for value >= unit && i < len(labels)-1 {
		value /= unit
		i++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + labels[i]
}

// Provider inspects one model runtime. Every method is a read-only observation:
// none transitions SOP state, executes a task, changes the repository, or selects
// a model class. The interface is intentionally narrow — the harness layer
// (internal/agent, internal/ollamaagent), not the provider, owns agent execution.
type Provider interface {
	// ID returns the provider's identity.
	ID() ID
	// Health reports reachability. It never returns an error: a provider that
	// cannot be reached reports HealthUnavailable rather than failing.
	Health(ctx context.Context) HealthResult
	// Models lists the models the provider serves. A provider that cannot
	// enumerate models returns ErrDiscoveryUnsupported rather than an empty list,
	// so a caller never mistakes "unknown" for "absent".
	Models(ctx context.Context) ([]ModelInfo, error)
	// Capabilities reports what a named model can do. A capability the provider
	// cannot determine is reported CapUnknown, never fabricated.
	Capabilities(ctx context.Context, model string) (Capabilities, error)
}

// joinIDs renders a provider list for error messages.
func joinIDs(ids []ID) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = string(id)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

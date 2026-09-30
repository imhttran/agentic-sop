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
)

// knownIDs is the canonical, stable set of provider identifiers.
var knownIDs = []ID{Ollama, LlamaCPP, Command, MLX}

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

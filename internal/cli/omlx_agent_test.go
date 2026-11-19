package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestNewAgentSelectsGovernedOMLXOnlyWhenEnabled proves the composition root
// advertises IMPLEMENT for oMLX only on the opt-in governed path. The default
// text-only MLX path is unchanged, and a malformed flag fails clearly.
func TestNewAgentSelectsGovernedOMLXOnlyWhenEnabled(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(agent.EnvAgentProvider, agent.ProviderMLX)
	t.Setenv(agent.EnvAgentModel, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	t.Setenv(agent.EnvMLXModel, "Qwen3-4B-4bit")
	t.Setenv(agent.EnvMLXBaseURL, "http://127.0.0.1:8000")
	d := defaultDeps()

	t.Setenv(agent.EnvMLXToolLoop, "true")
	governed, err := d.newAgent(agent.HarnessTool, agent.ProviderMLX, "configured", "")
	if err != nil {
		t.Fatalf("governed oMLX agent: %v", err)
	}
	declarer, ok := governed.(agent.Declarer)
	if !ok {
		t.Fatal("governed oMLX agent does not declare capabilities")
	}
	if caps := declarer.Capabilities(); !caps.Supports(agent.Implement) || !caps.Supports(agent.Fix) {
		t.Errorf("capabilities = %s, want IMPLEMENT and FIX", caps)
	}

	t.Setenv(agent.EnvMLXToolLoop, "false")
	textOnly, err := d.newAgent(agent.HarnessTool, agent.ProviderMLX, "configured", "")
	if err != nil {
		t.Fatalf("text-only MLX agent: %v", err)
	}
	declarer, ok = textOnly.(agent.Declarer)
	if !ok {
		t.Fatal("text-only MLX agent does not declare capabilities")
	}
	if caps := declarer.Capabilities(); caps.Supports(agent.Implement) || caps.Supports(agent.Fix) {
		t.Errorf("capabilities = %s, text-only MLX must not advertise IMPLEMENT/FIX", caps)
	}

	t.Setenv(agent.EnvMLXToolLoop, "maybe")
	if _, err := d.newAgent(agent.HarnessTool, agent.ProviderMLX, "configured", ""); err == nil {
		t.Fatal("a malformed SOP_MLX_TOOL_LOOP must fail")
	}
}

// TestNewAgentTextOnlyMLXFallbackStillWorks proves the flag-off fallback is a
// working text transport, not just a capability set: a PLAN call still reaches
// the OpenAI-compatible endpoint and returns its document.
func TestNewAgentTextOnlyMLXFallbackStillWorks(t *testing.T) {
	t.Chdir(t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "the plan"},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	t.Setenv(agent.EnvAgentProvider, agent.ProviderMLX)
	t.Setenv(agent.EnvAgentModel, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	t.Setenv(agent.EnvMLXModel, "Qwen3-4B-4bit")
	t.Setenv(agent.EnvMLXBaseURL, srv.URL)
	t.Setenv(agent.EnvMLXToolLoop, "false")

	d := defaultDeps()
	a, err := d.newAgent(agent.HarnessTool, agent.ProviderMLX, "configured", "")
	if err != nil {
		t.Fatalf("text-only MLX agent: %v", err)
	}
	resp, err := a.Generate(context.Background(), agent.Request{
		Capability:         agent.Plan,
		Task:               "plan this",
		OutputRequirements: "json",
	})
	if err != nil {
		t.Fatalf("text-only MLX PLAN: %v", err)
	}
	if resp.Content != "the plan" {
		t.Errorf("content = %q, want %q", resp.Content, "the plan")
	}
}

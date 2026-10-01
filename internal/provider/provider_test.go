package provider_test

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
)

func TestParseIDKnown(t *testing.T) {
	for _, want := range []provider.ID{provider.Ollama, provider.LlamaCPP, provider.Command, provider.MLX, provider.OpenaiCompatible} {
		got, err := provider.ParseID(string(want))
		if err != nil {
			t.Fatalf("ParseID(%q): %v", want, err)
		}
		if got != want {
			t.Fatalf("ParseID(%q) = %q", want, got)
		}
	}
}

func TestParseIDUnknown(t *testing.T) {
	for _, in := range []string{"", "  ", "vllm", "OpenAI"} {
		_, err := provider.ParseID(in)
		if !errors.Is(err, provider.ErrUnknownProvider) {
			t.Fatalf("ParseID(%q) err = %v, want ErrUnknownProvider", in, err)
		}
	}
}

func TestParseIDTrimsWhitespace(t *testing.T) {
	if id, err := provider.ParseID("  ollama\n"); err != nil || id != provider.Ollama {
		t.Fatalf("ParseID trimmed = %q, %v", id, err)
	}
}

func TestKnownIDsCopy(t *testing.T) {
	ids := provider.KnownIDs()
	if len(ids) != 5 {
		t.Fatalf("KnownIDs len = %d", len(ids))
	}
	ids[0] = "mutated"
	if provider.KnownIDs()[0] == "mutated" {
		t.Fatal("KnownIDs must return a copy")
	}
}

// TestModelInfoMetadata pins the deterministic metadata rendering: only fields
// the provider determined are shown, and an unknown set renders as empty.
func TestModelInfoMetadata(t *testing.T) {
	if got := (provider.ModelInfo{}).Metadata(); got != "" {
		t.Fatalf("empty metadata = %q, want empty", got)
	}
	m := provider.ModelInfo{Family: "llama", ParameterSize: "8B", Quantization: "Q5", ContextWindow: 8192, SizeBytes: 512}
	if got, want := m.Metadata(), "family=llama params=8B quant=Q5 ctx=8192 size=512B"; got != want {
		t.Fatalf("Metadata() = %q, want %q", got, want)
	}
	if got := (provider.ModelInfo{SizeBytes: 1024}).Metadata(); got != "size=1.0KiB" {
		t.Fatalf("size rendering = %q", got)
	}
}

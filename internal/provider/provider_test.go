package provider_test

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
)

func TestParseIDKnown(t *testing.T) {
	for _, want := range []provider.ID{provider.Ollama, provider.LlamaCPP, provider.Command, provider.MLX} {
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
	if len(ids) != 4 {
		t.Fatalf("KnownIDs len = %d", len(ids))
	}
	ids[0] = "mutated"
	if provider.KnownIDs()[0] == "mutated" {
		t.Fatal("KnownIDs must return a copy")
	}
}

package llamacpp_test

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
)

func TestLlamaCppIdentity(t *testing.T) {
	p := llamacpp.New(llamacpp.DefaultBaseURL, 0)
	if p.ID() != provider.LlamaCPP {
		t.Fatalf("ID = %q", p.ID())
	}
	if llamacpp.EnvBaseURL != "SOP_LLAMACPP_BASE_URL" {
		t.Fatalf("EnvBaseURL = %q", llamacpp.EnvBaseURL)
	}
}

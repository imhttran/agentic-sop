package cli

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

func TestValidateRAGConfig(t *testing.T) {
	if err := validateRAGConfig(config.Config{}); err != nil {
		t.Fatalf("disabled RAG must be valid: %v", err)
	}
	t.Setenv(config.EnvRAGEnabled, "true")
	t.Setenv(config.EnvRAGExecutable, "/nonexistent/rag-producer")
	t.Setenv(config.EnvRAGCorpus, "kb")
	t.Setenv(config.EnvRAGAllowedCorpora, "kb")
	if err := validateRAGConfig(config.Config{}); err == nil {
		t.Fatal("enabled RAG with a missing producer must fail")
	}
}

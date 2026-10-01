package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
)

// writeOpenAICompatibleConfig writes a minimal project config naming the generic
// provider's endpoint.
func writeOpenAICompatibleConfig(t *testing.T, dir, endpoint string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(config.Path(dir)), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "version: 1\nproject:\n  name: demo\nagent:\n  harness: tool\n  provider: ollama\n  model: m\n" +
		"providers:\n  openai_compatible:\n    endpoint: " + endpoint + "\n"
	if err := os.WriteFile(config.Path(dir), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// TestOpenAICompatibleExecutionEndpointPrecedence proves the CLI composition root
// resolves the generic provider's execution endpoint with the documented
// precedence: environment > configuration > default.
func TestOpenAICompatibleExecutionEndpointPrecedence(t *testing.T) {
	for _, k := range []string{agent.EnvOpenAICompatibleBaseURL} {
		t.Setenv(k, "")
	}

	t.Run("default when neither is set", func(t *testing.T) {
		dir := t.TempDir()
		if got, want := openAICompatibleExecutionEndpoint(dir), config.DefaultOpenAICompatibleBaseURL; got != want {
			t.Fatalf("endpoint = %q, want default %q", got, want)
		}
	})

	t.Run("configuration used when environment is unset", func(t *testing.T) {
		dir := t.TempDir()
		writeOpenAICompatibleConfig(t, dir, "http://yaml.example:9100")
		if got, want := openAICompatibleExecutionEndpoint(dir), "http://yaml.example:9100"; got != want {
			t.Fatalf("endpoint = %q, want config %q", got, want)
		}
	})

	t.Run("environment overrides configuration", func(t *testing.T) {
		dir := t.TempDir()
		writeOpenAICompatibleConfig(t, dir, "http://yaml.example:9100")
		t.Setenv(agent.EnvOpenAICompatibleBaseURL, "http://127.0.0.1:9999")
		if got, want := openAICompatibleExecutionEndpoint(dir), "http://127.0.0.1:9999"; got != want {
			t.Fatalf("endpoint = %q, want env %q", got, want)
		}
	})
}

package openai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/openai"
)

// bareModelsServer serves one fixed /v1/models body and nothing else.
func bareModelsServer(t *testing.T, payload string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, payload)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOpenAICompatibleDiscoveredModelsUndetermined covers a localhost
// endpoint listing models: every listed model, including a Claude name and a
// ":cloud"-suffixed name, reports undetermined locality. The endpoint host and
// the names are not evidence.
func TestOpenAICompatibleDiscoveredModelsUndetermined(t *testing.T) {
	p := openai.New(provider.OpenaiCompatible,
		bareModelsServer(t, `{"data":[{"id":"llama3.1"},{"id":"claude-3-5-sonnet:cloud"},{"id":"gpt-5"}]}`).URL,
		2*time.Second)
	infos, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("Models returned %d infos, want 3", len(infos))
	}
	for _, in := range infos {
		if in.Locality != "" {
			t.Errorf("model %q: Locality = %q, want undetermined", in.Name, in.Locality)
		}
	}
}

// TestOpenAICompatibleConfiguredAndRemoteUndetermined covers a remote,
// unreachable endpoint with an operator-configured model: the fallback reports
// the identity without claiming a locality, so a Claude name with a ":cloud"
// suffix stays undetermined.
func TestOpenAICompatibleConfiguredAndRemoteUndetermined(t *testing.T) {
	p := openai.New(provider.OpenaiCompatible, "http://203.0.113.5:9999", 250*time.Millisecond).
		WithConfiguredModel("claude-3-5-sonnet:cloud")
	infos, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(infos) != 1 || infos[0].Name != "claude-3-5-sonnet:cloud" {
		t.Fatalf("Models = %+v, want the configured model", infos)
	}
	if infos[0].Locality != "" {
		t.Errorf("configured model %q: Locality = %q, want undetermined", infos[0].Name, infos[0].Locality)
	}
}

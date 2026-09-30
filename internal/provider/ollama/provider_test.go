package ollama_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

func TestOllamaID(t *testing.T) {
	if got := ollama.New(ollama.DefaultBaseURL, 0).ID(); got != provider.Ollama {
		t.Fatalf("ID = %q", got)
	}
}

func TestOllamaHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"version":"0.1"}`))
	}))
	defer srv.Close()

	h := ollama.New(srv.URL, 0).Health(context.Background())
	if h.Status != provider.HealthHealthy {
		t.Fatalf("health = %+v", h)
	}
}

func TestOllamaHealthDegradedAndUnavailable(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer bad.Close()
	if h := ollama.New(bad.URL, 0).Health(context.Background()); h.Status != provider.HealthDegraded {
		t.Fatalf("health = %+v, want degraded", h)
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := closed.URL
	closed.Close()
	if h := ollama.New(url, 0).Health(context.Background()); h.Status != provider.HealthUnavailable {
		t.Fatalf("health = %+v, want unavailable", h)
	}
}

func TestOllamaModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:4b","model":"qwen3:4b","size":2147483648,"details":{"family":"qwen3","parameter_size":"4.0B","quantization_level":"Q4_K_M","context_length":40960}},{"model":"glm-5.3-flash:cloud"}]}`))
	}))
	defer srv.Close()

	infos, err := ollama.New(srv.URL, 0).Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("models = %+v", infos)
	}
	if infos[0].Name != "qwen3:4b" || infos[1].Name != "glm-5.3-flash:cloud" {
		t.Fatalf("names = %+v", infos)
	}
	// Locality must not be guessed from the model name.
	if infos[1].Locality != "" {
		t.Fatalf("locality = %q, want unset (Ollama does not report it)", infos[1].Locality)
	}
	// Metadata comes from the /api/tags details and size, and stays zero when the
	// server does not report it — never fabricated.
	if infos[0].Family != "qwen3" || infos[0].ParameterSize != "4.0B" || infos[0].Quantization != "Q4_K_M" ||
		infos[0].ContextWindow != 40960 || infos[0].SizeBytes != 2147483648 {
		t.Fatalf("metadata = %+v", infos[0])
	}
	if meta := infos[0].Metadata(); !strings.Contains(meta, "family=qwen3") || !strings.Contains(meta, "size=2.0GiB") {
		t.Fatalf("Metadata() = %q", meta)
	}
	if got := infos[1].Metadata(); got != "" {
		t.Fatalf("unreported metadata = %q, want empty", got)
	}
}

func TestOllamaModelsUnsupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()
	_, err := ollama.New(srv.URL, 0).Models(context.Background())
	if !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("err = %v, want ErrDiscoveryUnsupported", err)
	}
}

func TestOllamaCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"capabilities":["completion","tools"]}`))
	}))
	defer srv.Close()

	caps, err := ollama.New(srv.URL, 0).Capabilities(context.Background(), "qwen3:4b")
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !caps.Chat.Present() || !caps.Tools.Present() || !caps.Streaming.Present() {
		t.Fatalf("caps = %+v", caps)
	}
	if caps.Images.Known() {
		t.Fatalf("images should be unknown, got %v", caps.Images)
	}
}

func TestOllamaCapabilitiesUnknownWhenUnreported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	caps, err := ollama.New(srv.URL, 0).Capabilities(context.Background(), "qwen3:4b")
	if err != nil {
		t.Fatalf("an unreported capability must not be an error: %v", err)
	}
	if caps.Chat.Known() {
		t.Fatalf("chat should be unknown, got %v", caps.Chat)
	}
	if !caps.Streaming.Present() {
		t.Fatalf("streaming should be yes, got %v", caps.Streaming)
	}
}

func TestOllamaCapabilitiesEmptyModel(t *testing.T) {
	if _, err := ollama.New(ollama.DefaultBaseURL, 0).Capabilities(context.Background(), "  "); err == nil {
		t.Fatal("an empty model must be rejected")
	}
}

func TestOllamaNoEndpointIsUnavailable(t *testing.T) {
	if h := ollama.New("", 0).Health(context.Background()); h.Status != provider.HealthUnavailable {
		t.Fatalf("health = %+v", h)
	}
}

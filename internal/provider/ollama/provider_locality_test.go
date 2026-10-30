package ollama_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// remoteHostTagsServer serves an /api/tags body in which one entry carries a
// remote_host (a cloud-served model) and one does not (a locally served model).
func remoteHostTagsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"models":[`+
			`{"name":"gpt-oss:120b","remote_host":"https://ollama.com","size":64000000000,"details":{"family":"gptoss","parameter_size":"120b","quantization_level":"Q4_K_M","context_length":131072}},`+
			`{"name":"qwen3:8b","size":5200000000,"details":{"family":"qwen3","parameter_size":"8b","quantization_level":"Q4_K_M","context_length":40960}},`+
			`{"name":"qwen3-coder-30b:cloud","size":33000000000}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOllamaTagsRemoteHostCloudOrLocal(t *testing.T) {
	p := ollama.New(remoteHostTagsServer(t).URL, 2*time.Second)
	infos, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	// The ":cloud"-suffixed entry carries no remote_host: the runtime's
	// remote_host field is the only locality evidence, so the name suffix never
	// flips the entry to cloud.
	want := map[string]model.Locality{
		"gpt-oss:120b":          model.LocalityCloud,
		"qwen3:8b":              model.LocalityLocal,
		"qwen3-coder-30b:cloud": model.LocalityLocal,
	}
	if len(infos) != len(want) {
		t.Fatalf("Models returned %d infos, want %d", len(infos), len(want))
	}
	for _, in := range infos {
		loc, ok := want[in.Name]
		if !ok {
			t.Fatalf("unexpected model %q", in.Name)
		}
		if in.Locality != loc {
			t.Errorf("model %q: Locality = %q, want %q", in.Name, in.Locality, loc)
		}
	}
}

// TestOllamaShowLocalityCloudOrUndetermined exercises the per-model /api/show
// locality path: a non-empty remote_host is cloud, a missing or empty field is
// undetermined regardless of the model name, and a failed request or an absent
// model is undetermined. The endpoint host (a localhost test server) is never
// evidence, and the only request sent is the read-only /api/show probe.
func TestOllamaShowLocalityCloudOrUndetermined(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if r.Method != http.MethodPost || r.URL.Path != "/api/show" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Model {
		case "gpt-oss:120b":
			io.WriteString(w, `{"remote_host":"https://ollama.com","capabilities":["completion"]}`)
		case "boom":
			http.Error(w, "nope", http.StatusInternalServerError)
		default:
			// A locally served model: /api/show reports no remote_host, and a
			// name ending in ":cloud" must not change that.
			io.WriteString(w, `{"capabilities":["completion"]}`)
		}
	}))
	defer srv.Close()
	p := ollama.New(srv.URL, 2*time.Second)
	ctx := context.Background()

	if got := p.Locality(ctx, "gpt-oss:120b"); got != model.LocalityCloud {
		t.Errorf("Locality(gpt-oss:120b) = %q, want %q", got, model.LocalityCloud)
	}
	for _, name := range []string{"made-up:cloud", "claude-3-5-sonnet:cloud", "qwen3:8b"} {
		if got := p.Locality(ctx, name); got != "" {
			t.Errorf("Locality(%q) = %q, want undetermined from an empty remote_host", name, got)
		}
	}
	if got := p.Locality(ctx, "boom"); got != "" {
		t.Errorf("Locality on a failed request = %q, want undetermined", got)
	}
	if got := p.Locality(ctx, "  "); got != "" {
		t.Errorf("Locality of an empty model name = %q, want undetermined", got)
	}
	if got := p.Locality(ctx, "  gpt-oss:120b  "); got != model.LocalityCloud {
		t.Errorf("Locality with surrounding spaces = %q, want %q", got, model.LocalityCloud)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, path := range paths {
		if !strings.HasPrefix(path, "POST /api/show") {
			t.Errorf("locality discovery sent %q: it must only read /api/show, and never a generation request", path)
		}
	}
}

// TestOllamaModelsErrDiscoveryUnsupported keeps the honest-absence behavior:
// a server that does not answer the list is unsupported discovery, and the
// adapter reports no locality for it.
func TestOllamaModelsErrDiscoveryUnsupported(t *testing.T) {
	p := ollama.New("", time.Second)
	infos, err := p.Models(context.Background())
	if err == nil || infos != nil {
		t.Fatalf("Models with no endpoint = (%v, %v), want ErrDiscoveryUnsupported", infos, err)
	}
	if err != provider.ErrDiscoveryUnsupported {
		t.Errorf("err = %v, want %v", err, provider.ErrDiscoveryUnsupported)
	}
	if got := p.Locality(context.Background(), "gpt-oss:120b"); got != "" {
		t.Errorf("Locality with no endpoint = %q, want undetermined", got)
	}
}

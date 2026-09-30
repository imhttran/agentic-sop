package llamacpp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
)

func TestLlamaCppIdentity(t *testing.T) {
	p := llamacpp.New(llamacpp.DefaultBaseURL, "", 0)
	if p.ID() != provider.LlamaCPP {
		t.Fatalf("ID = %q", p.ID())
	}
	if llamacpp.EnvBaseURL != "SOP_LLAMACPP_BASE_URL" {
		t.Fatalf("EnvBaseURL = %q", llamacpp.EnvBaseURL)
	}
}

// TestLlamaCppConfiguredIdentity proves a single-model llama-server still reports
// a known model when it cannot enumerate one: the configured identity is reported,
// never invented, and a server that does answer still wins.
func TestLlamaCppConfiguredIdentity(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"served-model"}]}`))
	}))
	defer ok.Close()
	infos, err := llamacpp.New(ok.URL, "configured-model", 0).Models(context.Background())
	if err != nil || len(infos) != 1 || infos[0].Name != "served-model" {
		t.Fatalf("discovery should win: infos=%+v err=%v", infos, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer bad.Close()
	infos, err = llamacpp.New(bad.URL, "configured-model", 0).Models(context.Background())
	if err != nil || len(infos) != 1 || infos[0].Name != "configured-model" {
		t.Fatalf("configured identity should be reported: infos=%+v err=%v", infos, err)
	}

	// No configured model and no discovery: absence stays unknown, never asserted.
	if _, err := llamacpp.New(bad.URL, "", 0).Models(context.Background()); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("err = %v, want ErrDiscoveryUnsupported", err)
	}
}

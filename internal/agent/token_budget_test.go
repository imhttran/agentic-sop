package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/perf"
)

// LLM10: once the run's token budget is used, the next request is refused
// before it reaches the provider.
func TestTokenBudgetRefusesBeforeDispatch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":60,"completion_tokens":50}}`))
	}))
	defer srv.Close()
	a, err := NewLlamaCpp(srv.URL, "m", "", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(perf.EnvMaxTotalTokens, "100")
	ctx := perf.WithUsage(context.Background(), &perf.Usage{})

	if _, err := a.Generate(ctx, validRequest(Plan)); err != nil {
		t.Fatalf("first request under budget: %v", err)
	}
	if _, err := a.Generate(ctx, validRequest(Plan)); !errors.Is(err, perf.ErrTokenBudgetExceeded) {
		t.Fatalf("second request after 110 of 100 tokens: err = %v, want ErrTokenBudgetExceeded", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("provider received %d requests, want 1", n)
	}
}

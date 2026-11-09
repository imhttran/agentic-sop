package ollamaagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestOllamaClientSendsOperatorMaxOutputTokens proves the in-process tool loop
// sends the operator-owned output-token bound as options.num_predict.
func TestOllamaClientSendsOperatorMaxOutputTokens(t *testing.T) {
	t.Setenv(agent.EnvAgentMaxOutputTokens, "123")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxOutputTokens != 123 {
		t.Fatalf("MaxOutputTokens = %d, want 123", cfg.MaxOutputTokens)
	}

	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"},"prompt_eval_count":1,"eval_count":1}`))
	}))
	defer srv.Close()

	cfg.BaseURL = srv.URL
	cfg.Model = "m"
	cfg.Timeout = time.Second
	c := newOllamaClient(cfg)
	if _, _, err := c.chat(context.Background(), []chatMessage{{Role: "user", Content: "hi"}}); err != nil {
		t.Fatal(err)
	}

	opts, _ := body["options"].(map[string]any)
	if opts == nil || opts["num_predict"] != float64(123) {
		t.Errorf("options = %+v, want num_predict 123", body["options"])
	}
}

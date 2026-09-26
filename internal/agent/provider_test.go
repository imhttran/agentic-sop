package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRenderPromptIncludesSections(t *testing.T) {
	got := renderPrompt(Request{
		Capability:         Plan,
		Task:               "  plan the work  ",
		Input:              "the prd",
		OutputRequirements: "json",
	})
	for _, want := range []string{"Capability: PLAN", "plan the work", "the prd", "Output requirements", "json"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderPrompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderPromptOmitsEmptySections(t *testing.T) {
	got := renderPrompt(validRequest(Implement))
	if strings.Contains(got, "Input:") {
		t.Errorf("renderPrompt should omit an empty input section:\n%s", got)
	}
	if strings.Contains(got, "Output requirements:") {
		t.Errorf("renderPrompt should omit empty output requirements:\n%s", got)
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	cases := map[string]string{
		"  http://x:1/  ": "http://x:1",
		"http://x:1///":   "http://x:1",
		"http://x:1":      "http://x:1",
		"":                "",
	}
	for in, want := range cases {
		if got := normalizeBaseURL(in); got != want {
			t.Errorf("normalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimeoutFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv(EnvOllamaTimeout, "")
		got, err := timeoutFromEnv(EnvOllamaTimeout)
		if err != nil || got != defaultProviderTimeout {
			t.Errorf("got (%v, %v), want (%v, nil)", got, err, defaultProviderTimeout)
		}
	})
	t.Run("valid", func(t *testing.T) {
		t.Setenv(EnvOllamaTimeout, "90s")
		got, err := timeoutFromEnv(EnvOllamaTimeout)
		if err != nil || got != 90*time.Second {
			t.Errorf("got (%v, %v), want (90s, nil)", got, err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		t.Setenv(EnvOllamaTimeout, "soon")
		if _, err := timeoutFromEnv(EnvOllamaTimeout); err == nil {
			t.Error("expected error for an invalid duration")
		}
	})
	t.Run("non-positive", func(t *testing.T) {
		t.Setenv(EnvOllamaTimeout, "0s")
		if _, err := timeoutFromEnv(EnvOllamaTimeout); err == nil {
			t.Error("expected error for a zero duration")
		}
	})
}

func TestFromEnvSelection(t *testing.T) {
	t.Run("default command", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, "")
		t.Setenv(EnvAgentCommand, "printf '%s' 'ok'")
		a, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv failed: %v", err)
		}
		if _, ok := a.(*CommandAgent); !ok {
			t.Errorf("got %T, want *CommandAgent", a)
		}
	})
	t.Run("ollama", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, ProviderOllama)
		t.Setenv(EnvOllamaModel, "qwen3:8b")
		a, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv failed: %v", err)
		}
		if _, ok := a.(*Ollama); !ok {
			t.Errorf("got %T, want *Ollama", a)
		}
	})
	t.Run("llamacpp", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, ProviderLlamaCpp)
		a, err := FromEnv()
		if err != nil {
			t.Fatalf("FromEnv failed: %v", err)
		}
		if _, ok := a.(*LlamaCpp); !ok {
			t.Errorf("got %T, want *LlamaCpp", a)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, "wat")
		if _, err := FromEnv(); err == nil {
			t.Error("expected error for an unknown provider")
		}
	})
}

func TestNewOllamaValidation(t *testing.T) {
	if _, err := NewOllama("", "m", 0); err == nil {
		t.Error("expected error for an empty base URL")
	}
	if _, err := NewOllama("http://x", "  ", 0); err == nil {
		t.Error("expected error for an empty model")
	}
}

func TestNewLlamaCppValidation(t *testing.T) {
	if _, err := NewLlamaCpp("", "m", "", 0); err == nil {
		t.Error("expected error for an empty base URL")
	}
	if _, err := NewLlamaCpp("http://x", "  ", "", 0); err == nil {
		t.Error("expected error for an empty model")
	}
}

func TestOllamaGenerate(t *testing.T) {
	var gotPath string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"planned"}}`))
	}))
	defer srv.Close()

	o, err := NewOllama(srv.URL+"/", "qwen3:8b", 5*time.Second)
	if err != nil {
		t.Fatalf("NewOllama failed: %v", err)
	}
	resp, err := o.Generate(context.Background(), Request{Capability: Plan, Task: "plan it", Input: "prd"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content != "planned" {
		t.Errorf("Content = %q, want planned", resp.Content)
	}
	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if gotBody.Model != "qwen3:8b" {
		t.Errorf("model = %q, want qwen3:8b", gotBody.Model)
	}
	if gotBody.Stream {
		t.Error("stream = true, want false")
	}
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want one user message", gotBody.Messages)
	}
	if !strings.Contains(gotBody.Messages[0].Content, "plan it") {
		t.Errorf("prompt missing the task:\n%s", gotBody.Messages[0].Content)
	}
}

func TestOllamaGenerateErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{"http error", http.StatusInternalServerError, "boom", "http 500"},
		{"body error field", http.StatusOK, `{"error":"model not found"}`, "model not found"},
		{"empty output", http.StatusOK, `{"message":{"role":"assistant","content":"   "}}`, "empty output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			o, _ := NewOllama(srv.URL, "m", time.Second)
			_, err := o.Generate(context.Background(), validRequest(Plan))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

func TestOllamaGenerateRejectsInvalidRequestBeforeCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server was called for an invalid request")
	}))
	defer srv.Close()

	o, _ := NewOllama(srv.URL, "m", time.Second)
	if _, err := o.Generate(context.Background(), Request{Capability: Implement, Task: "  "}); err == nil {
		t.Fatal("expected a request validation error")
	}
}

func TestOllamaGenerateRespectsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o, _ := NewOllama(srv.URL, "m", time.Second)
	_, err := o.Generate(ctx, validRequest(Plan))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestLlamaCppGenerate(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"implemented"}}]}`))
	}))
	defer srv.Close()

	c, err := NewLlamaCpp(srv.URL, "local", "secret", 5*time.Second)
	if err != nil {
		t.Fatalf("NewLlamaCpp failed: %v", err)
	}
	resp, err := c.Generate(context.Background(), validRequest(Implement))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content != "implemented" {
		t.Errorf("Content = %q, want implemented", resp.Content)
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want Bearer secret", gotAuth)
	}
	if gotBody.Model != "local" || gotBody.Stream {
		t.Errorf("body = %+v, want model local and stream false", gotBody)
	}
}

func TestLlamaCppGenerateErrors(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{"http error", http.StatusBadGateway, "nope", "http 502"},
		{"body error field", http.StatusOK, `{"error":"bad model"}`, "bad model"},
		{"no choices", http.StatusOK, `{"choices":[]}`, "no choices"},
		{"empty output", http.StatusOK, `{"choices":[{"message":{"content":"  "}}]}`, "empty output"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, _ := NewLlamaCpp(srv.URL, "m", "", time.Second)
			_, err := c.Generate(context.Background(), validRequest(Review))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
		})
	}
}

func TestLlamaCppFromEnvDefaults(t *testing.T) {
	t.Setenv(EnvLlamaCppBaseURL, "")
	t.Setenv(EnvLlamaCppModel, "")
	t.Setenv(EnvLlamaCppAPIKey, "")
	a, err := NewLlamaCppFromEnv()
	if err != nil {
		t.Fatalf("NewLlamaCppFromEnv failed: %v", err)
	}
	c, ok := a.(*LlamaCpp)
	if !ok {
		t.Fatalf("got %T, want *LlamaCpp", a)
	}
	if c.baseURL != defaultLlamaCppBaseURL || c.model != defaultLlamaCppModel {
		t.Errorf("defaults = (%q, %q), want (%q, %q)", c.baseURL, c.model, defaultLlamaCppBaseURL, defaultLlamaCppModel)
	}
}

func TestFromConfigProvider(t *testing.T) {
	t.Run("configured provider", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, "")
		t.Setenv(EnvOllamaModel, "qwen3:8b")
		a, err := FromConfig(ProviderOllama)
		if err != nil {
			t.Fatalf("FromConfig failed: %v", err)
		}
		if _, ok := a.(*Ollama); !ok {
			t.Errorf("got %T, want *Ollama", a)
		}
	})
	t.Run("env overrides config", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, ProviderLlamaCpp)
		a, err := FromConfig(ProviderOllama)
		if err != nil {
			t.Fatalf("FromConfig failed: %v", err)
		}
		if _, ok := a.(*LlamaCpp); !ok {
			t.Errorf("got %T, want *LlamaCpp", a)
		}
	})
	t.Run("unknown configured provider", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, "")
		if _, err := FromConfig("skynet"); err == nil {
			t.Error("expected error for an unknown configured provider")
		}
	})
	t.Run("blank provider falls back to command", func(t *testing.T) {
		t.Setenv(EnvAgentProvider, "")
		t.Setenv(EnvAgentCommand, "printf '%s' 'ok'")
		a, err := FromConfig("")
		if err != nil {
			t.Fatalf("FromConfig failed: %v", err)
		}
		if _, ok := a.(*CommandAgent); !ok {
			t.Errorf("got %T, want *CommandAgent", a)
		}
	})
}

func TestNewOllamaFromEnvRequiresModel(t *testing.T) {
	t.Setenv(EnvOllamaModel, "")
	if _, err := NewOllamaFromEnv(); err == nil {
		t.Error("expected error when the Ollama model is unset")
	}
}

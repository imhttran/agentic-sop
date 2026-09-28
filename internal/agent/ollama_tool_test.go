package agent

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// mockAgent is a mock Agent provider for testing.
type mockAgent struct {
	generateFunc func(ctx context.Context, req Request) (Response, error)
}

func (m *mockAgent) Generate(ctx context.Context, req Request) (Response, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return Response{Content: "ok"}, nil
}

func (m *mockAgent) Capabilities() Capabilities {
	return ollamaCapabilities
}

func TestNewOllamaToolHarnessValidatesModel(t *testing.T) {
	t.Run("rejects non-deepseek model", func(t *testing.T) {
		provider, _ := NewOllama("http://localhost:11434", "mistral", 30*time.Second)
		_, err := NewOllamaToolHarness(provider, "mistral")
		if err == nil || !strings.Contains(err.Error(), "unsupported model") {
			t.Errorf("expected unsupported model error, got: %v", err)
		}
	})

	t.Run("accepts deepseek model variants", func(t *testing.T) {
		provider, _ := NewOllama("http://localhost:11434", "deepseek-v4.1-flash:cloud", 30*time.Second)
		harness, err := NewOllamaToolHarness(provider, "deepseek-v4.1-flash:cloud")
		if err != nil {
			t.Errorf("expected no error for deepseek model, got: %v", err)
		}
		if harness == nil {
			t.Error("expected harness, got nil")
		}
	})

	t.Run("accepts deepseek lowercase prefix", func(t *testing.T) {
		provider, _ := NewOllama("http://localhost:11434", "deepseek-v4", 30*time.Second)
		harness, err := NewOllamaToolHarness(provider, "deepseek-v4")
		if err != nil {
			t.Errorf("expected no error for deepseek-v4, got: %v", err)
		}
		if harness == nil {
			t.Error("expected harness, got nil")
		}
	})

	t.Run("rejects nil provider", func(t *testing.T) {
		_, err := NewOllamaToolHarness(nil, "deepseek-v4.1-flash:cloud")
		if err == nil || !strings.Contains(err.Error(), "required") {
			t.Errorf("expected provider required error, got: %v", err)
		}
	})
}

func TestOllamaToolHarnessExecuteClassifiesErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("connection refused error", func(t *testing.T) {
		mockProv := &mockAgent{
			generateFunc: func(_ context.Context, req Request) (Response, error) {
				return Response{}, &net.OpError{
					Op:  "dial",
					Err: errors.New("connection refused"),
				}
			},
		}
		h := &OllamaToolHarness{provider: mockProv, model: "deepseek-v4.1-flash:cloud"}
		_, err := h.Execute(ctx, validRequest(Plan))
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Errorf("expected unavailable error, got: %v", err)
		}
	})

	t.Run("model not found error", func(t *testing.T) {
		mockProv := &mockAgent{
			generateFunc: func(_ context.Context, req Request) (Response, error) {
				return Response{}, errors.New("ollama PLAN: model not found")
			},
		}
		h := &OllamaToolHarness{provider: mockProv, model: "deepseek-v4.1-flash:cloud"}
		_, err := h.Execute(ctx, validRequest(Plan))
		if err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Errorf("expected model unavailable error, got: %v", err)
		}
	})

	t.Run("empty response error", func(t *testing.T) {
		mockProv := &mockAgent{
			generateFunc: func(_ context.Context, req Request) (Response, error) {
				return Response{}, errors.New("ollama PLAN returned empty output")
			},
		}
		h := &OllamaToolHarness{provider: mockProv, model: "deepseek-v4.1-flash:cloud"}
		_, err := h.Execute(ctx, validRequest(Plan))
		if err == nil || !strings.Contains(err.Error(), "empty response") {
			t.Errorf("expected empty response error, got: %v", err)
		}
	})

	t.Run("timeout error", func(t *testing.T) {
		mockProv := &mockAgent{
			generateFunc: func(ctx context.Context, req Request) (Response, error) {
				return Response{}, context.DeadlineExceeded
			},
		}
		h := &OllamaToolHarness{provider: mockProv, model: "deepseek-v4.1-flash:cloud"}
		_, err := h.Execute(ctx, validRequest(Plan))
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})

	t.Run("successful response passes through", func(t *testing.T) {
		mockProv := &mockAgent{
			generateFunc: func(_ context.Context, req Request) (Response, error) {
				return Response{Content: "ok"}, nil
			},
		}
		h := &OllamaToolHarness{provider: mockProv, model: "deepseek-v4.1-flash:cloud"}
		resp, err := h.Execute(ctx, validRequest(Plan))
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if resp.Content != "ok" {
			t.Errorf("expected content 'ok', got: %q", resp.Content)
		}
	})
}

func TestIsDeepSeekModel(t *testing.T) {
	cases := map[string]bool{
		"deepseek-v4.1-flash:cloud": true,
		"deepseek-v4.1-flash":       true,
		"deepseek-v4":               true,
		"deepseek":                  true,
		"DEEPSEEK":                  true, // case-insensitive prefix
		"  deepseek  ":              true, // whitespace-trimmed
		"mistral":                   false,
		"llama2":                    false,
		"":                          false,
		"deepseek-v3":               true, // any deepseek variant
	}
	for input, want := range cases {
		if got := isDeepSeekModel(input); got != want {
			t.Errorf("isDeepSeekModel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestIsConnectionError(t *testing.T) {
	cases := map[string]bool{
		"connection refused":  true,
		"dial tcp":            true,
		"no such host":        true,
		"network unreachable": true,
		"connection reset":    true,
		"broken pipe":         true,
		"request failed":      true,
		"encode request":      false,
		"decode response":     false,
		"model error":         false,
		"":                    false,
	}
	for msg, want := range cases {
		err := errors.New(msg)
		if got := isConnectionError(err); got != want {
			t.Errorf("isConnectionError(%q) = %v, want %v", msg, got, want)
		}
	}

	// Test net.OpError
	netErr := &net.OpError{
		Op:  "dial",
		Err: errors.New("connection refused"),
	}
	if !isConnectionError(netErr) {
		t.Error("isConnectionError should recognize net.OpError")
	}

	// Test context.Canceled should not be treated as connection error
	if isConnectionError(context.Canceled) {
		t.Error("isConnectionError should not treat context.Canceled as connection error")
	}
}

func TestNewOllamaToolHarnessFromEnvValidatesDeepSeek(t *testing.T) {
	t.Run("rejects non-deepseek model", func(t *testing.T) {
		_, err := NewOllamaToolHarnessFromEnv("http://localhost:11434", "mistral", 30*time.Second)
		if err == nil || !strings.Contains(err.Error(), "unsupported model") {
			t.Errorf("expected unsupported model error, got: %v", err)
		}
	})

	t.Run("rejects empty model", func(t *testing.T) {
		_, err := NewOllamaToolHarnessFromEnv("http://localhost:11434", "", 30*time.Second)
		if err == nil || !strings.Contains(err.Error(), "no model configured") {
			t.Errorf("expected no model error, got: %v", err)
		}
	})

	t.Run("accepts deepseek model", func(t *testing.T) {
		harness, err := NewOllamaToolHarnessFromEnv("http://localhost:11434", "deepseek-v4.1-flash:cloud", 30*time.Second)
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
		if harness == nil {
			t.Error("expected harness, got nil")
		}
	})

	t.Run("normalizes base URL", func(t *testing.T) {
		harness, err := NewOllamaToolHarnessFromEnv("  http://localhost:11434/  ", "deepseek-v4.1-flash:cloud", 30*time.Second)
		if err != nil {
			t.Errorf("expected no error for whitespace base URL, got: %v", err)
		}
		if harness == nil {
			t.Error("expected harness, got nil")
		}
	})

	t.Run("uses default base URL when empty", func(t *testing.T) {
		harness, err := NewOllamaToolHarnessFromEnv("", "deepseek-v4.1-flash:cloud", 30*time.Second)
		if err != nil {
			t.Errorf("expected no error with empty base URL, got: %v", err)
		}
		if harness == nil {
			t.Error("expected harness, got nil")
		}
	})
}

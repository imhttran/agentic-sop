package config

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
)

func clearOpenAICompatibleEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		EnvOpenAICompatibleBaseURL, EnvOpenAICompatibleAPIKey,
		model.ClassEnvKey(model.ClassSmall, "BASE_URL"),
		model.ClassEnvKey(model.ClassMedium, "BASE_URL"),
		model.ClassEnvKey(model.ClassLarge, "BASE_URL"),
		model.ClassEnvKey(model.ClassSmall, "API_KEY"),
		model.ClassEnvKey(model.ClassMedium, "API_KEY"),
		model.ClassEnvKey(model.ClassLarge, "API_KEY"),
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

// TestResolveOpenAICompatibleRouteScopesCredentials proves the shared key is sent
// only to the shared origin, a class override to another origin sends none, and a
// class API key is used for that class's endpoint.
func TestResolveOpenAICompatibleRouteScopesCredentials(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	t.Setenv(EnvOpenAICompatibleBaseURL, "https://openrouter.ai/api")
	t.Setenv(EnvOpenAICompatibleAPIKey, "shared-openrouter-key")
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "BASE_URL"), "http://127.0.0.1:8000")

	cfg := Default()

	endpoint, key := cfg.ResolveOpenAICompatibleRoute(model.ClassSmall)
	if want := "http://127.0.0.1:8000"; endpoint != want {
		t.Errorf("small endpoint = %q, want %q", endpoint, want)
	}
	if key != "" {
		t.Errorf("small key = %q, want empty (shared key must not cross origins)", key)
	}

	for _, class := range []model.Class{model.ClassMedium, model.ClassLarge} {
		endpoint, key = cfg.ResolveOpenAICompatibleRoute(class)
		if want := "https://openrouter.ai/api"; endpoint != want {
			t.Errorf("%s endpoint = %q, want %q", class, endpoint, want)
		}
		if want := "shared-openrouter-key"; key != want {
			t.Errorf("%s key = %q, want shared key", class, key)
		}
	}

	// A class credential for its own endpoint is used instead of the shared key.
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "API_KEY"), "local-key")
	if _, key = cfg.ResolveOpenAICompatibleRoute(model.ClassSmall); key != "local-key" {
		t.Errorf("small key = %q, want its own class key", key)
	}
}

// TestResolveOpenAICompatibleRouteSameOrigin proves a class override that keeps
// the shared origin (including an explicit default port) still receives the
// shared key, while a lookalike host does not.
func TestResolveOpenAICompatibleRouteSameOrigin(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	t.Setenv(EnvOpenAICompatibleBaseURL, "https://openrouter.ai/api")
	t.Setenv(EnvOpenAICompatibleAPIKey, "shared-openrouter-key")
	cfg := Default()

	t.Setenv(model.ClassEnvKey(model.ClassMedium, "BASE_URL"), "https://openrouter.ai:443/other")
	if _, key := cfg.ResolveOpenAICompatibleRoute(model.ClassMedium); key != "shared-openrouter-key" {
		t.Errorf("same-origin override key = %q, want shared key", key)
	}

	t.Setenv(model.ClassEnvKey(model.ClassLarge, "BASE_URL"), "https://openrouter.ai.example.com/api")
	if _, key := cfg.ResolveOpenAICompatibleRoute(model.ClassLarge); key != "" {
		t.Errorf("lookalike host key = %q, want empty", key)
	}
}

// TestResolveOpenAICompatibleEndpointForClassPrecedence proves the class override
// takes precedence over the shared endpoint, and an absent override preserves the
// existing environment > configuration > default resolution.
func TestResolveOpenAICompatibleEndpointForClassPrecedence(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	cfg := Default()

	if got, want := cfg.ResolveOpenAICompatibleEndpointForClass(model.ClassSmall), DefaultOpenAICompatibleBaseURL; got != want {
		t.Errorf("default endpoint = %q, want %q", got, want)
	}
	t.Setenv(EnvOpenAICompatibleBaseURL, "https://shared.example/api")
	if got, want := cfg.ResolveOpenAICompatibleEndpointForClass(model.ClassMedium), "https://shared.example/api"; got != want {
		t.Errorf("shared endpoint = %q, want %q", got, want)
	}
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "BASE_URL"), "http://127.0.0.1:8000")
	if got, want := cfg.ResolveOpenAICompatibleEndpointForClass(model.ClassSmall), "http://127.0.0.1:8000"; got != want {
		t.Errorf("class override = %q, want %q", got, want)
	}
	if got, want := cfg.ResolveOpenAICompatibleEndpointForClass(model.ClassMedium), "https://shared.example/api"; got != want {
		t.Errorf("other class must keep the shared endpoint, got %q want %q", got, want)
	}
}

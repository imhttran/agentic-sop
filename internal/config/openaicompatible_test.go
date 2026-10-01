package config_test

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestResolveOpenAICompatibleEndpointPrecedence locks the Phase 1 endpoint
// precedence: environment (SOP_OPENAI_COMPATIBLE_BASE_URL) > configuration
// (providers.openai_compatible.endpoint) > the built-in default. Resolution is
// pure string resolution; it never touches a network or a provider runtime.
func TestResolveOpenAICompatibleEndpointPrecedence(t *testing.T) {
	const (
		envEndpoint  = "http://env.example:9000"
		yamlEndpoint = "http://yaml.example:9100"
	)

	t.Run("neither set uses default", func(t *testing.T) {
		t.Setenv(config.EnvOpenAICompatibleBaseURL, "")
		c := config.Config{}
		if got, want := c.ResolveOpenAICompatibleEndpoint(), config.DefaultOpenAICompatibleBaseURL; got != want {
			t.Fatalf("endpoint = %q, want default %q", got, want)
		}
	})

	t.Run("yaml used when env unset", func(t *testing.T) {
		t.Setenv(config.EnvOpenAICompatibleBaseURL, "")
		c := config.Config{Providers: config.Providers{
			OpenaiCompatible: config.ProviderEndpoint{Endpoint: yamlEndpoint},
		}}
		if got, want := c.ResolveOpenAICompatibleEndpoint(), yamlEndpoint; got != want {
			t.Fatalf("endpoint = %q, want yaml %q", got, want)
		}
	})

	t.Run("env overrides yaml", func(t *testing.T) {
		t.Setenv(config.EnvOpenAICompatibleBaseURL, envEndpoint)
		c := config.Config{Providers: config.Providers{
			OpenaiCompatible: config.ProviderEndpoint{Endpoint: yamlEndpoint},
		}}
		if got, want := c.ResolveOpenAICompatibleEndpoint(), envEndpoint; got != want {
			t.Fatalf("endpoint = %q, want env %q", got, want)
		}
	})

	t.Run("env only", func(t *testing.T) {
		t.Setenv(config.EnvOpenAICompatibleBaseURL, envEndpoint)
		c := config.Config{}
		if got, want := c.ResolveOpenAICompatibleEndpoint(), envEndpoint; got != want {
			t.Fatalf("endpoint = %q, want env %q", got, want)
		}
	})

	t.Run("blank env falls through to yaml", func(t *testing.T) {
		t.Setenv(config.EnvOpenAICompatibleBaseURL, "   ")
		c := config.Config{Providers: config.Providers{
			OpenaiCompatible: config.ProviderEndpoint{Endpoint: yamlEndpoint},
		}}
		if got, want := c.ResolveOpenAICompatibleEndpoint(), yamlEndpoint; got != want {
			t.Fatalf("endpoint = %q, want yaml %q", got, want)
		}
	})
}

// TestOpenAICompatibleEndpointKeyAndAccessor pins that the documented Phase 1
// key exists and the accessor trims a configured endpoint (empty means
// "not configured", so configuration can never invent an endpoint).
func TestOpenAICompatibleEndpointKeyAndAccessor(t *testing.T) {
	if config.OpenAICompatibleEndpointKey != "providers.openai_compatible.endpoint" {
		t.Fatalf("key = %q", config.OpenAICompatibleEndpointKey)
	}
	if config.DefaultOpenAICompatibleBaseURL != "http://127.0.0.1:8000" {
		t.Fatalf("default endpoint = %q", config.DefaultOpenAICompatibleBaseURL)
	}
	p := config.Providers{OpenaiCompatible: config.ProviderEndpoint{Endpoint: "  http://cfg:1  "}}
	if got, want := p.OpenAICompatibleEndpoint(), "http://cfg:1"; got != want {
		t.Fatalf("accessor = %q, want %q", got, want)
	}
	if got := (config.Providers{}).OpenAICompatibleEndpoint(); got != "" {
		t.Fatalf("unset accessor = %q, want empty", got)
	}
}

// TestOpenAICompatibleConfigKeyRoundTrips proves the YAML key is accepted by the
// strict loader: providers.openai_compatible.endpoint parses and reaches the
// accessor used by the resolver.
func TestOpenAICompatibleConfigKeyRoundTrips(t *testing.T) {
	c, err := config.Parse([]byte("" +
		"version: 1\n" +
		"project:\n  name: demo\n" +
		"agent:\n  harness: tool\n  provider: ollama\n  model: m\n" +
		"providers:\n  openai_compatible:\n    endpoint: http://yaml.example:9100\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got, want := c.Providers.OpenAICompatibleEndpoint(), "http://yaml.example:9100"; got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

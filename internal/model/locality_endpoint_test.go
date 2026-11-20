package model

import "testing"

func TestOriginNormalization(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"https://openrouter.ai/api/v1", "https://openrouter.ai", true},
		{"https://OpenRouter.AI:443/x", "https://openrouter.ai", true},
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000", true},
		{"http://127.0.0.1:80", "http://127.0.0.1", true},
		{"https://openrouter.ai:8443", "https://openrouter.ai:8443", true},
		{"", "", false},
		{"openrouter.ai/api", "", false},
		{"://bad", "", false},
	}
	for _, tc := range cases {
		got, ok := Origin(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Origin(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://openrouter.ai/api", "https://openrouter.ai:443/other", true},
		{"https://openrouter.ai/api", "http://openrouter.ai/api", false},
		{"http://127.0.0.1:8000", "http://127.0.0.1:8001", false},
		{"http://127.0.0.1:8000", "http://127.0.0.1:8000/v1", true},
		{"https://openrouter.ai", "https://openrouter.ai.example.com", false},
		{"", "https://openrouter.ai", false},
	}
	for _, tc := range cases {
		if got := SameOrigin(tc.a, tc.b); got != tc.want {
			t.Errorf("SameOrigin(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestInferEndpointLocality proves only the exact OpenRouter HTTPS origin is
// cloud evidence. An arbitrary name, a loopback URL, and lookalike hosts are not.
func TestInferEndpointLocality(t *testing.T) {
	cases := []struct {
		provider string
		endpoint string
		want     Locality
		ok       bool
	}{
		{"openai_compatible", "https://openrouter.ai/api", LocalityCloud, true},
		{"openai_compatible", "https://openrouter.ai/api/v1", LocalityCloud, true},
		{"openai_compatible", "https://openrouter.ai:443", LocalityCloud, true},
		{"openai_compatible", "http://openrouter.ai/api", "", false},
		{"openai_compatible", "https://openrouter.ai.example.com/api", "", false},
		{"openai_compatible", "https://notopenrouter.ai/api", "", false},
		{"openai_compatible", "https://openrouter.ai:8443/api", "", false},
		{"openai_compatible", "http://127.0.0.1:8000", "", false},
		{"openai_compatible", "", "", false},
		{"mlx", "https://openrouter.ai/api", "", false},
		{"ollama", "https://openrouter.ai/api", "", false},
	}
	for _, tc := range cases {
		got, ok := InferEndpointLocality(tc.provider, tc.endpoint)
		if got != tc.want || ok != tc.ok {
			t.Errorf("InferEndpointLocality(%q, %q) = (%q, %v), want (%q, %v)", tc.provider, tc.endpoint, got, ok, tc.want, tc.ok)
		}
	}
}

// TestResolveLocalityFromEndpoint proves per-class endpoint inference: the same
// provider resolves local for a loopback override with an explicit local value
// and cloud for an OpenRouter endpoint with no explicit locality.
func TestResolveLocalityFromEndpoint(t *testing.T) {
	var cfg Route
	cfg.setClassConfig(ClassSmall, ClassConfig{
		Provider: "openai_compatible",
		Name:     "Qwen3-4B-4bit",
		Locality: LocalityLocal,
	})
	cfg.setClassConfig(ClassMedium, ClassConfig{
		Provider: "openai_compatible",
		Name:     "openrouter-medium-model-id",
	})
	endpointFor := func(_ string, c Class) string {
		if c == ClassSmall {
			return "http://127.0.0.1:8000"
		}
		return "https://openrouter.ai/api"
	}
	lookup := func(string) string { return "" }

	small, err := Resolve(Inputs{Config: cfg, Lookup: lookup, CLIClass: "small", EndpointFor: endpointFor})
	if err != nil {
		t.Fatalf("resolve small: %v", err)
	}
	if got, want := small.Selection.Locality, LocalityLocal; got != want {
		t.Errorf("small locality = %q, want %q", got, want)
	}
	if got, want := small.Selection.LocalitySource, LocalitySourceExplicit; got != want {
		t.Errorf("small locality source = %q, want %q", got, want)
	}
	if got, want := small.Selection.BaseURL, "http://127.0.0.1:8000"; got != want {
		t.Errorf("small base URL = %q, want %q", got, want)
	}

	medium, err := Resolve(Inputs{Config: cfg, Lookup: lookup, CLIClass: "medium", EndpointFor: endpointFor})
	if err != nil {
		t.Fatalf("resolve medium: %v", err)
	}
	if got, want := medium.Selection.Locality, LocalityCloud; got != want {
		t.Errorf("medium locality = %q, want %q", got, want)
	}
	if got, want := medium.Selection.LocalitySource, LocalitySourceInferred; got != want {
		t.Errorf("medium locality source = %q, want %q", got, want)
	}
	if got, want := medium.Selection.BaseURL, "https://openrouter.ai/api"; got != want {
		t.Errorf("medium base URL = %q, want %q", got, want)
	}
}

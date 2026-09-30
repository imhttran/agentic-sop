package config

import "testing"

const providersYAML = `version: 1
project:
  name: providers
providers:
  validate: true
  ollama:
    endpoint: http://127.0.0.1:11434
  llamacpp:
    endpoint: http://127.0.0.1:8080
  mlx:
    endpoint: http://127.0.0.1:8000
`

func TestParseProvidersBlock(t *testing.T) {
	c, err := Parse([]byte(providersYAML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.Providers.ValidateSelections() {
		t.Fatal("providers.validate: true must enable validation")
	}
	if c.Providers.Ollama.Endpoint != "http://127.0.0.1:11434" {
		t.Fatalf("ollama endpoint = %q", c.Providers.Ollama.Endpoint)
	}
	if c.Providers.LlamaCpp.Endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("llamacpp endpoint = %q", c.Providers.LlamaCpp.Endpoint)
	}
	if c.Providers.MLX.Endpoint != "http://127.0.0.1:8000" {
		t.Fatalf("mlx endpoint = %q", c.Providers.MLX.Endpoint)
	}
}

func TestProvidersValidateDefaultsOff(t *testing.T) {
	c, err := Parse([]byte("version: 1\nproject:\n  name: x\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Providers.ValidateSelections() {
		t.Fatal("validation must default OFF")
	}
}

func TestParseRejectsUnknownProvidersKey(t *testing.T) {
	_, err := Parse([]byte("version: 1\nproject:\n  name: x\nproviders:\n  vllm:\n    endpoint: http://x\n"))
	if err == nil {
		t.Fatal("an unknown providers key must fail")
	}
}

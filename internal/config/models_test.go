package config

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
)

const modelsYAML = `version: 1
project:
  name: routed
models:
  default_class: large
  fallback_class: medium
  allow_cloud_fallback_for_local: true
  routing_enabled: true
  small:
    provider: ollama
    name: qwen3:4b
    locality: local
  medium:
    provider: ollama
    name: glm-5.3-flash:cloud
    locality: cloud
  large:
    provider: ollama
    name: deepseek-v4.1-flash:cloud
    locality: cloud
`

func TestParseModelsBlock(t *testing.T) {
	c, err := Parse([]byte(modelsYAML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !c.Models.Configured() {
		t.Fatal("models block must be reported as configured")
	}
	if c.Models.DefaultClass != model.ClassLarge {
		t.Errorf("default_class = %q, want large", c.Models.DefaultClass)
	}
	if c.Models.FallbackClass != model.ClassMedium {
		t.Errorf("fallback_class = %q, want medium", c.Models.FallbackClass)
	}
	if c.Models.AllowCloudFallbackForLocal == nil || !*c.Models.AllowCloudFallbackForLocal {
		t.Errorf("allow_cloud_fallback_for_local = %v, want true", c.Models.AllowCloudFallbackForLocal)
	}
	if c.Models.RoutingEnabled == nil || !*c.Models.RoutingEnabled {
		t.Errorf("routing_enabled = %v, want true", c.Models.RoutingEnabled)
	}
	if c.Models.Small.Locality != model.LocalityLocal || c.Models.Small.Name != "qwen3:4b" {
		t.Errorf("small = %+v", c.Models.Small)
	}
}

func TestParseModelsRejectsUnknownValues(t *testing.T) {
	wrap := func(body string) []byte {
		return []byte("version: 1\nproject:\n  name: x\nmodels:\n" + body)
	}
	cases := []struct {
		name string
		body string
		want string
	}{
		{"locality", "  small:\n    locality: orbit\n", "locality"},
		{"provider", "  small:\n    provider: gpt\n", "provider"},
		{"default class", "  default_class: huge\n", "default_class"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(wrap(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse = %v, want an error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsUnknownModelsKey(t *testing.T) {
	_, err := Parse([]byte("version: 1\nproject:\n  name: x\nmodels:\n  gigantic:\n    provider: ollama\n"))
	if err == nil {
		t.Fatal("expected an unknown key under models: to be rejected")
	}
}

func TestConfigWithoutModelsIsUnconfigured(t *testing.T) {
	c, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Models.Configured() {
		t.Fatal("models must be unconfigured when the block is omitted")
	}
}

// TestParseEscalationBlock pins the Phase 5 escalation configuration: the fields
// parse, and enabling escalation alone does not make the models block Configured()
// (so it never activates model routing by itself).
func TestParseEscalationBlock(t *testing.T) {
	c, err := Parse([]byte("version: 1\nproject:\n  name: escalated\nmodels:\n  escalation_enabled: true\n  max_escalations: 4\n"))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if c.Models.EscalationEnabled == nil || !*c.Models.EscalationEnabled {
		t.Errorf("escalation_enabled = %v, want true", c.Models.EscalationEnabled)
	}
	if c.Models.MaxEscalations == nil || *c.Models.MaxEscalations != 4 {
		t.Errorf("max_escalations = %v, want 4", c.Models.MaxEscalations)
	}
	if c.Models.Configured() {
		t.Error("escalation settings alone must not mark the models block configured")
	}
}

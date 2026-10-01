package jev

import (
	"strings"
	"testing"
)

// TestPromptRunesBound proves the analyzer's prompt bound can be raised or lowered by
// environment and that an unset, blank, non-numeric, or non-positive value keeps the
// built-in default, so behavior is unchanged when it is not set.
func TestPromptRunesBound(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want int
	}{
		{"unset", "", maxPromptRunes},
		{"blank", "   ", maxPromptRunes},
		{"valid", "12345", 12345},
		{"larger", "1048576", 1048576},
		{"non-numeric", "lots", maxPromptRunes},
		{"zero", "0", maxPromptRunes},
		{"negative", "-5", maxPromptRunes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(maxPromptRunesEnv, tc.val)
			if got := promptRunesBound(); got != tc.want {
				t.Errorf("promptRunesBound() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestBuildPromptHonoursRunesBound proves the bound is applied, not merely computed: a
// lowered SOP_JEV_MAX_PROMPT_RUNES truncates the assembled prompt, and a raised one
// leaves a prompt of the same size intact.
func TestBuildPromptHonoursRunesBound(t *testing.T) {
	a := &OllamaAnalyzer{}
	req := Request{Task: strings.Repeat("x", 4000)}

	t.Setenv(maxPromptRunesEnv, "500")
	if got := a.buildPrompt(req); !strings.Contains(got, "...[truncated]") {
		t.Errorf("a lowered bound must truncate the prompt; got %d runes", len([]rune(got)))
	}

	t.Setenv(maxPromptRunesEnv, "100000")
	if got := a.buildPrompt(req); strings.Contains(got, "...[truncated]") {
		t.Errorf("a raised bound must not truncate this prompt; got %d runes", len([]rune(got)))
	}
}

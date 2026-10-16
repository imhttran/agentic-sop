package config

import "testing"

// TestOutputResponsesDefaultAndOverride proves the output.responses policy: an
// omitted block defaults to the concise shape (so behavior is unchanged), an
// explicit detailed value selects the fuller shape, and an unknown value fails at
// load time.
func TestOutputResponsesDefaultAndOverride(t *testing.T) {
	t.Run("omitted defaults to concise", func(t *testing.T) {
		c, err := Parse([]byte("version: 1\nproject:\n  name: demo\n"))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if c.Output.Responses != ResponseConcise {
			t.Errorf("output.responses = %q, want %q", c.Output.Responses, ResponseConcise)
		}
		if c.ResponsesDetailed() {
			t.Error("an omitted block must resolve to the concise default")
		}
	})

	t.Run("explicit detailed", func(t *testing.T) {
		c, err := Parse([]byte("version: 1\nproject:\n  name: demo\noutput:\n  responses: detailed\n"))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !c.ResponsesDetailed() {
			t.Errorf("output.responses = %q, want detailed", c.Output.Responses)
		}
	})

	t.Run("unknown value rejected", func(t *testing.T) {
		if _, err := Parse([]byte("version: 1\nproject:\n  name: demo\noutput:\n  responses: loud\n")); err == nil {
			t.Error("an unknown output.responses value must fail at load time")
		}
	})

	t.Run("default config is concise", func(t *testing.T) {
		if Default().ResponsesDetailed() {
			t.Error("the built-in default must be the concise shape")
		}
	})
}

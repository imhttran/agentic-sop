package cli

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/handoff"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// redactDiagnosticText removes known environment secret values and
// credential-shaped text from a diagnostic before it is sent to a model or
// persisted as evidence. It reuses the shared detector so a shape one surface
// learns is removed here too; the marker stays the established [redacted] text.
//
// It is deliberately scoped to diagnostics and command output. It does not
// rewrite the repository diff or implementation report: those are evidence, and
// altering them could hide what actually happened.
func redactDiagnosticText(s string) string {
	if s == "" {
		return s
	}
	s = handoff.Redact(s, os.Environ())
	return redact.ShapesValues(s, redact.Fixed("[redacted]"))
}

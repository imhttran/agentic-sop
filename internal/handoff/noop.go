package handoff

import (
	"context"
	"strings"
)

// NoOpCompressor is the deterministic no-op compressor: it performs no
// compression and returns artifacts verbatim, so SOP works normally with
// compression disabled and tests need no external service.
type NoOpCompressor struct{}

// Compress concatenates the bundled artifacts in order. It is an identity
// transform: nothing is dropped and no references are produced.
func (NoOpCompressor) Compress(_ context.Context, input ContextBundle) (CompressedContext, error) {
	var b strings.Builder
	for i, artifact := range input.Artifacts {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(string(artifact.Kind))
		if name := strings.TrimSpace(artifact.Name); name != "" {
			b.WriteString(" ")
			b.WriteString(name)
		}
		b.WriteString("\n")
		b.WriteString(artifact.Content)
	}
	return CompressedContext{Content: b.String()}, nil
}

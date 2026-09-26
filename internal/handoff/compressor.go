package handoff

import "context"

// ArtifactKind identifies a kind of bulky supporting context.
type ArtifactKind string

const (
	ArtifactDiff       ArtifactKind = "DIFF"
	ArtifactTestLog    ArtifactKind = "TEST_LOG"
	ArtifactCILog      ArtifactKind = "CI_LOG"
	ArtifactAgentOut   ArtifactKind = "AGENT_OUTPUT"
	ArtifactReview     ArtifactKind = "REVIEW_OUTPUT"
	ArtifactDiagnostic ArtifactKind = "DIAGNOSTIC"
)

// Artifact is a bulky piece of supporting context that may be compressed.
type Artifact struct {
	Kind    ArtifactKind `json:"kind"`
	Name    string       `json:"name,omitempty"`
	Content string       `json:"content"`
}

// ContextBundle is the input to a Compressor: the semantic capsule plus the
// bulky artifacts that may need compression.
type ContextBundle struct {
	Capsule   Capsule    `json:"capsule"`
	Artifacts []Artifact `json:"artifacts"`
}

// Reference points back to an original artifact, when a compressor can retain
// one. Recoverability is explicit: not every compressor supports it.
type Reference struct {
	Kind    ArtifactKind `json:"kind"`
	Locator string       `json:"locator"`
}

// CompressedContext is the result of compressing a bundle.
type CompressedContext struct {
	Content    string      `json:"content"`
	References []Reference `json:"references,omitempty"`
}

// Compressor reduces bulky supporting context. Implementations are adapters
// (a no-op, or an optional provider); core orchestration must not depend on any
// specific one.
type Compressor interface {
	Compress(ctx context.Context, input ContextBundle) (CompressedContext, error)
}

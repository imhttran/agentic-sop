package run

import (
	"encoding/json"
	"fmt"
)

// Model-selection evidence (Phase 3.5), versioned.
//
// The resolved (non-secret) model-routing selection for one run is persisted as
// model-selection.json beside the other run artifacts, so a developer can
// determine after the process exits which model the run selected and why.
//
// This artifact is WRITE-ONLY diagnostic evidence, exactly like routing.json:
//
//   - it is never read back to drive a decision, and no stage re-derives the
//     selection from it;
//   - it is never a second source of workflow state; and
//   - it carries no credential (the class, provider, model, locality, source, and
//     fixed-phrase reason are all non-secret).
//
// It carries a schema_version so a consumer can reject an unknown version rather
// than guess. A historical artifact written before versioning has no
// schema_version; a consumer treats a missing version as ModelSelectionVersion.

// ModelSelectionVersion is the schema version of the model-selection artifact.
const ModelSelectionVersion = 1

// modelSelectionFileName is the artifact holding the model-selection evidence.
const modelSelectionFileName = "model-selection.json"

// ModelSelectionArtifact is the persisted, versioned model-selection evidence for
// one run. It mirrors the non-secret fields of model.Selection plus a schema
// version, so the persisted shape stays byte-compatible with historical records
// (which simply lack schema_version).
type ModelSelectionArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	Class         string `json:"class"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Locality      string `json:"locality"`
	// LocalitySource records how the locality was resolved (explicit, inferred,
	// runtime, or default). It is omitted when empty so a historical artifact
	// without it decodes and re-encodes unchanged.
	LocalitySource string `json:"locality_source,omitempty"`
	Source         string `json:"source"`
	Fallback       bool   `json:"fallback"`
	Reason         string `json:"reason,omitempty"`
}

// Validate rejects an unsupported artifact version, failing closed rather than
// persisting an artifact a consumer could not interpret.
func (a ModelSelectionArtifact) Validate() error {
	if a.SchemaVersion != ModelSelectionVersion {
		return fmt.Errorf("run: unsupported model-selection artifact version %d", a.SchemaVersion)
	}
	return nil
}

// Marshal renders the artifact as indented JSON with a trailing newline.
func (a ModelSelectionArtifact) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

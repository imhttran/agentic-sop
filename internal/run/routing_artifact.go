package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Model-routing decision artifact (Phase 3.5).
//
// A per-task routing decision — the class SOP selected, the deterministic reasons
// for it, the resolved provider/model, and the typed evidence the router used — is
// persisted beside the other run artifacts, so a developer can determine after the
// process exits why a task ran on the model it did.
//
// This artifact is WRITE-ONLY diagnostic evidence:
//
//   - It is never read back to drive a decision, and no read/decision path
//     consumes it. The sole source of truth for the model choice remains the
//     in-process router.Decide output (the CLI's taskRouting, derived in memory).
//   - It is not a second source of truth: it never replaces state.json or the
//     state database, and no stage may re-drive or override the decision from it.
//   - It carries no credential: model.Selection and the routing reasons are
//     non-secret, and the typed signals are counts and closed enumerations.
//
// The writer fails closed: an artifact with an unknown version or an unknown
// source is rejected and not persisted, rather than silently written or defaulted.

// RoutingArtifactVersion is the schema version of the routing artifact contract.
const RoutingArtifactVersion = 1

// RoutingSource is the closed enumeration of what selected the model class.
type RoutingSource string

const (
	// RoutingSourcePolicy: SOP's deterministic router selected the class.
	RoutingSourcePolicy RoutingSource = "policy"
	// RoutingSourceManual: an operator's --model-class override selected the class.
	RoutingSourceManual RoutingSource = "manual_override"
	// RoutingSourceDefault: no routing layer applied; the resolved class stands.
	RoutingSourceDefault RoutingSource = "default"
)

// routingSourceRank is the closed set of known routing sources.
var routingSourceRank = map[RoutingSource]bool{
	RoutingSourcePolicy:  true,
	RoutingSourceManual:  true,
	RoutingSourceDefault: true,
}

// Valid reports whether s is a known routing source.
func (s RoutingSource) Valid() bool { return routingSourceRank[s] }

// RoutingArtifact is the persisted, versioned routing decision for one task run.
type RoutingArtifact struct {
	// Version is the schema version; unknown versions fail closed.
	Version int `json:"version"`
	// Task is the task identifier the decision pertains to.
	Task string `json:"task"`
	// Class is the selected model class (small, medium, or large).
	Class string `json:"class"`
	// Source records what selected the class.
	Source RoutingSource `json:"source"`
	// Reasons are the deterministic explanations for the class. They are fixed
	// phrases, never model-generated prose, and are never parsed back to drive a
	// decision.
	Reasons []string `json:"reasons"`
	// Provider, Model, and Locality are the resolved (non-secret) selection.
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Locality string `json:"locality,omitempty"`
	// Checkpoints names the early-JEV checkpoints whose evidence informed the
	// decision (task_triage, pre_execution). Empty when no evidence was available.
	Checkpoints []string `json:"checkpoints,omitempty"`
	// Signals is the typed routing-signal summary the router consumed. It is
	// diagnostic and contains no prose.
	Signals *RoutingSignals `json:"signals,omitempty"`
	// Timestamp records when the decision was made (RFC 3339, UTC).
	Timestamp string `json:"timestamp"`
}

// RoutingSignals is the typed, diagnostic summary of the signals the router used.
type RoutingSignals struct {
	JEVAvailable       bool    `json:"jev_available"`
	Risk               string  `json:"risk"`
	Complexity         string  `json:"complexity"`
	Scope              string  `json:"scope"`
	CrossCutting       bool    `json:"cross_cutting"`
	RequiresContext    bool    `json:"requires_context"`
	Confidence         float64 `json:"confidence"`
	AcceptanceCriteria int     `json:"acceptance_criteria"`
	Dependencies       int     `json:"dependencies"`
	FilesAffected      int     `json:"files_affected"`
}

// Validate rejects a malformed routing artifact, failing closed on unknown
// versions and sources rather than defaulting. It is structural only.
func (a RoutingArtifact) Validate() error {
	if a.Version != RoutingArtifactVersion {
		return fmt.Errorf("run: unsupported routing artifact version %d", a.Version)
	}
	if a.Task == "" {
		return fmt.Errorf("run: routing artifact task is required")
	}
	if !a.Source.Valid() {
		return fmt.Errorf("run: unknown routing source %q", a.Source)
	}
	if a.Class == "" {
		return fmt.Errorf("run: routing artifact class is required")
	}
	return nil
}

// routingArtifactFileName is the artifact holding the routing decision for a run.
const routingArtifactFileName = "routing.json"

// WriteRoutingArtifact records the routing decision in the run directory.
//
// It validates the artifact before writing and fails closed: an artifact with an
// unknown version or unknown source is rejected and NO file is written, and the
// validation error is returned to the caller. This keeps the persisted artifact a
// faithful, single copy of the in-process decision.
//
// The write is purely additive diagnostic evidence: it never reads the artifact
// back, never consults it to drive a decision, and never touches state.json or
// state.db or transitions any state.
func (r *Run) WriteRoutingArtifact(a RoutingArtifact) error {
	// Validate first and refuse to write on a contract violation, so an unknown
	// version or source can never be persisted.
	if err := a.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode routing artifact: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(r.dir, routingArtifactFileName), data, 0o644); err != nil {
		return fmt.Errorf("run: write %s: %w", routingArtifactFileName, err)
	}
	return nil
}

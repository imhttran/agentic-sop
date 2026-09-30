package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/imhttran/agentic-sop/internal/jev"
)

// Early JEV analysis artifact persistence (P3-004).
//
// Early-checkpoint JEV analysis results (triage and pre-execution) are persisted
// through the same run-artifact mechanism used for the quality seam
// (WriteJEVArtifact), under .agent-sdlc/runs/<TASK>/, alongside jev.json and
// jev-history.jsonl. They reuse that mechanism rather than introducing a second
// store, package, or database.
//
// These artifacts are diagnostic evidence only. They are:
//
//   - written best-effort: a write failure is returned to the caller for
//     logging but never fails, blocks, or alters the run;
//   - never read back to drive a decision: no reader/parse-back API exists and
//     none is introduced here, so nothing loads an early artifact to influence
//     policy;
//   - not a second source of truth: they never replace state.json or the state
//     database, and persisting them performs no state transition, no migration,
//     and no deletion.

// EarlyArtifactVersion is the schema version of the early JEV artifact contract.
// Unknown versions fail closed.
const EarlyArtifactVersion = 1

// Checkpoint is the closed enumeration of the early-analysis checkpoint an
// artifact was produced at. It distinguishes TRIAGE from PRE_EXECUTION evidence
// so the two are never conflated. An unknown checkpoint fails closed rather than
// defaulting.
type Checkpoint string

const (
	// CheckpointTaskTriage: early JEV analysis run at task triage.
	CheckpointTaskTriage Checkpoint = "task_triage"
	// CheckpointPreExecution: early JEV analysis run before execution.
	CheckpointPreExecution Checkpoint = "pre_execution"
)

// checkpointRank is the closed set of known checkpoints. Membership in the map
// is the single definition of validity.
var checkpointRank = map[Checkpoint]bool{
	CheckpointTaskTriage:   true,
	CheckpointPreExecution: true,
}

// Valid reports whether c is a known checkpoint. Unknown values are invalid so
// callers can fail closed rather than default.
func (c Checkpoint) Valid() bool { return checkpointRank[c] }

// EarlyArtifact is the structured, versioned shape of an early JEV analysis
// artifact. It is plain diagnostic data: the record layer stores it verbatim and
// interprets none of its fields. PolicyDecision and PolicyReason are opaque,
// caller-supplied strings the record layer never derives meaning from.
//
// The artifact reuses jev.Evidence (P3-002) rather than minting a parallel
// evidence type, so early and quality evidence share one contract.
type EarlyArtifact struct {
	// Version is the schema version; unknown versions fail closed.
	Version int `json:"version"`
	// Checkpoint identifies where the early analysis ran (task_triage or
	// pre_execution).
	Checkpoint Checkpoint `json:"checkpoint"`
	// Task is the task identifier the analysis pertains to.
	Task string `json:"task"`
	// Evidence is the structured analysis evidence (P3-002), reused directly.
	Evidence jev.Evidence `json:"evidence"`
	// Provider identifies the analysis provider that produced the evidence.
	Provider string `json:"provider"`
	// Timestamp records when the artifact was produced (RFC 3339, UTC).
	Timestamp string `json:"timestamp"`
	// ProviderFailed is true when the analyzer failed to run. It is persisted
	// distinctly from Evidence so an infrastructure/provider failure is never
	// represented as a finding or a PASS (mirrors P3-013). When true, Evidence
	// MUST be zero-valued: a provider failure carries no evidence payload.
	ProviderFailed bool `json:"provider_failed"`
	// PolicyDecision is an opaque policy outcome recorded verbatim; the record
	// layer does not interpret it.
	PolicyDecision string `json:"policy_decision"`
	// PolicyReason is an opaque policy reason recorded verbatim; the record layer
	// does not interpret it.
	PolicyReason string `json:"policy_reason"`
}

// Validate rejects malformed early artifacts, failing closed on unknown
// versions and checkpoints rather than defaulting. It is structural only: it
// never derives meaning from PolicyDecision, PolicyReason, or evidence summary
// text.
//
// A provider failure (ProviderFailed) is not an analysis result and MUST carry
// no evidence payload: a non-zero Evidence alongside ProviderFailed is rejected,
// so a provider failure can never persist a contradictory value that a later
// consumer could read as a PASS. When no provider failure occurred the evidence
// must satisfy the jev.Evidence fail-closed contract.
func (a EarlyArtifact) Validate() error {
	if a.Version != EarlyArtifactVersion {
		return fmt.Errorf("run: unsupported early artifact version %d", a.Version)
	}
	if !a.Checkpoint.Valid() {
		return fmt.Errorf("run: unknown early artifact checkpoint %q", a.Checkpoint)
	}
	if a.Task == "" {
		return fmt.Errorf("run: early artifact task is required")
	}
	if a.ProviderFailed {
		// A provider failure is not an analysis result: it must not carry a
		// fabricated evidence payload. Rejecting a non-zero evidence value here
		// keeps the fail-closed contract: a provider failure can never be
		// mistaken for a PASS by any later consumer.
		// jev.Evidence contains a slice, so it is not comparable with ==; compare
		// structurally with reflect.DeepEqual against the zero value.
		if !reflect.DeepEqual(a.Evidence, jev.Evidence{}) {
			return fmt.Errorf("run: early artifact provider failure must not carry an evidence payload")
		}
		return nil
	}
	if err := a.Evidence.Validate(); err != nil {
		return fmt.Errorf("run: early artifact evidence invalid: %w", err)
	}
	return nil
}

// earlyJEVArtifactFileName is the artifact holding the latest early JEV result
// for a run. It is deliberately distinct from jev.json so early evidence never
// overwrites quality-seam evidence and vice versa.
const earlyJEVArtifactFileName = "early-jev.json"

// earlyJEVHistoryFileName is the append-only artifact preserving every prior
// early JEV result, so repeated runs or fix attempts never destroy earlier
// evidence.
const earlyJEVHistoryFileName = "early-jev-history.jsonl"

// WriteEarlyJEVArtifact records an early JEV diagnostic artifact in the run
// directory, best-effort.
//
// The latest result is written to early-jev.json and every result is appended to
// early-jev-history.jsonl, preserving earlier evidence instead of clobbering it.
// It writes plain data only: it never reads an artifact back to drive a
// decision, never touches state.json or state.db, and never deletes or migrates
// existing persistence. The returned error is for the caller to log; callers
// must not treat an artifact-write failure as a lifecycle action.
func (r *Run) WriteEarlyJEVArtifact(a EarlyArtifact) error {
	if err := a.Validate(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("run: encode early jev artifact: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(r.dir, earlyJEVArtifactFileName), data, 0o644); err != nil {
		return fmt.Errorf("run: write %s: %w", earlyJEVArtifactFileName, err)
	}

	// Append-only history: one compact JSON object per result, so the order of
	// results is preserved and no prior line is ever rewritten.
	line, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("run: encode early jev history: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(r.dir, earlyJEVHistoryFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("run: append %s: %w", earlyJEVHistoryFileName, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("run: append %s: %w", earlyJEVHistoryFileName, err)
	}
	return nil
}

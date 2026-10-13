package shadow

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// EvidenceSchemaVersion is the version of the persisted, offline shadow-evidence
// artifact. A consumer must reject an unknown version rather than guess.
const EvidenceSchemaVersion = 1

// Evidence is the versioned, offline recording of a shadow evaluation: one
// ObservationRecord per (case, source) observation, carrying provenance, the
// reference and observed decisions, comparability, the classification outcome,
// the recorded confidence, and the recorded latency. It is diagnostic only: it is
// never read by the primary decision path and never feeds SOP policy.
type Evidence struct {
	SchemaVersion int                 "json:\"schema_version\""
	Cases         []ObservationRecord "json:\"cases\""
}

// ObservationRecord is one raw shadow observation.
type ObservationRecord struct {
	// CaseID identifies the primary case under observation.
	CaseID string "json:\"case_id\""
	// Source is the comparison source (provenance), for example "clef" or "nimble".
	Source string "json:\"source\""
	// Comparable reports whether the source considered the case semantically
	// comparable. A non-comparable case is recorded but excluded from agreement.
	Comparable bool "json:\"comparable\""
	// Outcome is the recording label (shadow_agreement, shadow_disagreement,
	// shadow_timeout, shadow_error, shadow_malformed, shadow_invalid_choice,
	// shadow_indeterminate, skipped_not_comparable).
	Outcome string "json:\"outcome\""
	// Primary is the primary decision under observation, when recorded.
	Primary string "json:\"primary,omitempty\""
	// Shadow is the source's observed decision, when recorded.
	Shadow string "json:\"shadow,omitempty\""
	// Reference is the reference outcome the observation was compared against.
	Reference string "json:\"reference,omitempty\""
	// Confidence is the recorded confidence in [0,1], when supplied.
	Confidence *float64 "json:\"confidence,omitempty\""
	// LatencyMS is the recorded observation latency in milliseconds.
	LatencyMS int64 "json:\"latency_ms\""
}

// Evidence returns the deterministic evidence snapshot of the recorder's
// recordings. Records are sorted by (case id, source, outcome) so two producers
// over identical inputs emit byte-identical evidence.
func (r *Recorder) Evidence() Evidence {
	recs := append([]CaseRecord(nil), r.records...)
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].CaseID != recs[j].CaseID {
			return recs[i].CaseID < recs[j].CaseID
		}
		if recs[i].Source != recs[j].Source {
			return recs[i].Source < recs[j].Source
		}
		return recs[i].Label < recs[j].Label
	})
	out := make([]ObservationRecord, 0, len(recs))
	for _, rec := range recs {
		out = append(out, ObservationRecord{
			CaseID:     rec.CaseID,
			Source:     rec.Source,
			Comparable: rec.Label != LabelSkippedNotComparable,
			Outcome:    string(rec.Label),
			Primary:    rec.Primary,
			Shadow:     rec.Shadow,
			Reference:  rec.Reference,
			Confidence: rec.Confidence,
			LatencyMS:  rec.Latency.Milliseconds(),
		})
	}
	return Evidence{SchemaVersion: EvidenceSchemaVersion, Cases: out}
}

// Validate reports whether evidence is well-formed and at a known schema version.
func (e Evidence) Validate() error {
	if e.SchemaVersion != EvidenceSchemaVersion {
		return fmt.Errorf("shadow: unsupported evidence schema version %d", e.SchemaVersion)
	}
	for i, rec := range e.Cases {
		if rec.CaseID == "" {
			return fmt.Errorf("shadow: evidence case %d: case_id is required", i)
		}
		if rec.Source == "" {
			return fmt.Errorf("shadow: evidence case %d: source (provenance) is required", i)
		}
	}
	return nil
}

// MarshalEvidence encodes evidence as deterministic, indented JSON with a
// trailing newline.
func MarshalEvidence(e Evidence) ([]byte, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// WriteEvidence writes the evidence artifact to path.
func WriteEvidence(path string, e Evidence) error {
	data, err := MarshalEvidence(e)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadEvidence parses and validates an evidence artifact.
func ReadEvidence(data []byte) (Evidence, error) {
	var e Evidence
	if err := json.Unmarshal(data, &e); err != nil {
		return Evidence{}, fmt.Errorf("shadow: decode evidence: %w", err)
	}
	if err := e.Validate(); err != nil {
		return Evidence{}, err
	}
	return e, nil
}

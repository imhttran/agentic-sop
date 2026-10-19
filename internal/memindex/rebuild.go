package memindex

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/outcomefile"
)

// RebuildReport summarizes a rebuild-from-outcome-files operation.
type RebuildReport struct {
	Indexed  int               `json:"indexed"`
	Skipped  int               `json:"skipped"`
	Excluded int               `json:"excluded"`
	Rejected map[string]string `json:"rejected,omitempty"`
}

// Snapshot reads the current authoritative outcome records from dir and returns them
// keyed by stable record ID, plus a per-record exclusion map for ineligible records.
// The map is suitable for LocalRetriever.Current and DetectDrift.
func Snapshot(dir string) (map[string]outcomefile.OutcomeRecord, error) {
	records, _, err := outcomefile.ReadAll(dir)
	if err != nil {
		return nil, err
	}
	out := make(map[string]outcomefile.OutcomeRecord, len(records))
	for _, r := range records {
		out[r.RecordID] = r
	}
	return out, nil
}

// RebuildFromDir rebuilds the index from the authoritative outcome files in dir. Records
// that fail eligibility (incomplete, superseded, unauthorized) are counted once as
// excluded and not indexed. Documents rejected by the reader carry an explicit reason.
// The result is deterministic for the same frozen source records.
func (ix *Index) RebuildFromDir(ctx context.Context, dir string) (RebuildReport, error) {
	records, rejected, err := outcomefile.ReadAll(dir)
	if err != nil {
		return RebuildReport{}, err
	}
	rep := RebuildReport{Rejected: rejected}
	// Count each source-side exclusion exactly once.
	rep.Excluded = len(outcomefile.Gate(records))
	var in []Record
	for _, r := range records {
		if ok, _ := r.Eligible(); ok {
			in = append(in, Record{
				RecordID:    r.RecordID,
				ContentHash: r.ContentHash,
				Corpus:      r.Corpus,
				Repository:  r.Repository,
				TaskID:      r.TaskID,
				Status:      string(r.Status),
				Lifecycle:   r.Lifecycle,
				Eligible:    true,
				Citation:    r.Citation,
				Text:        r.Text,
			})
		}
	}
	indexed, skipped, _, err := ix.Rebuild(ctx, in)
	if err != nil {
		return RebuildReport{}, err
	}
	rep.Indexed = indexed
	// skipped from Rebuild counts empty/invalid records already excluded by the gate, so
	// do not add it to Excluded. It stays zero for gate-passed input.
	rep.Skipped = skipped
	return rep, nil
}

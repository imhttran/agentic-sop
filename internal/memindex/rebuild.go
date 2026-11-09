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
	return SnapshotContext(context.Background(), dir)
}

// SnapshotContext reads source records within the caller's deadline.
func SnapshotContext(ctx context.Context, dir string) (map[string]outcomefile.OutcomeRecord, error) {
	records, _, err := outcomefile.ReadAllContext(ctx, dir)
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
// excluded and not indexed. A record whose source artifact is missing or whose content no
// longer matches the digest recorded at ingest time is excluded and counted, not indexed.
// A success written before provenance digests existed is indexed as partial advisory
// history, never as a verified success. Documents rejected by the reader carry an explicit
// reason. The result is deterministic for the same frozen source records.
func (ix *Index) RebuildFromDir(ctx context.Context, dir string) (RebuildReport, error) {
	records, rejected, err := outcomefile.ReadAllContext(ctx, dir)
	if err != nil {
		return RebuildReport{}, err
	}
	rep := RebuildReport{Rejected: rejected}
	// Count each source-side exclusion exactly once.
	excluded := outcomefile.GateWithSourcesAndDigests(records, outcomefile.FileSourceExists, outcomefile.FileSourceDigest)
	rep.Excluded = len(excluded)
	excludedIDs := make(map[string]bool, len(excluded))
	for _, e := range excluded {
		excludedIDs[e.RecordID] = true
	}
	var in []Record
	for _, r := range records {
		if excludedIDs[r.RecordID] {
			continue
		}
		if ok, _ := r.Eligible(); ok {
			status := r.Status
			if status == outcomefile.StatusSuccess && len(r.SourceDigests) == 0 {
				// A success written before provenance digests cannot prove it was
				// corroborated at ingest. Keep it as advisory history, but never as a
				// verified success.
				status = outcomefile.StatusPartial
			}
			in = append(in, Record{
				RecordID:    r.RecordID,
				ContentHash: r.ContentHash,
				Corpus:      r.Corpus,
				Repository:  r.Repository,
				TaskID:      r.TaskID,
				Status:      string(status),
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

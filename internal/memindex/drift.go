package memindex

import (
	"context"
	"sort"

	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// DriftReason classifies why an index row diverges from the authoritative source.
type DriftReason string

const (
	DriftDeleted      DriftReason = "deleted"
	DriftSuperseded   DriftReason = "superseded"
	DriftModified     DriftReason = "modified"
	DriftUnauthorized DriftReason = "unauthorized"
	DriftIncomplete   DriftReason = "incomplete"
	DriftMissing      DriftReason = "missing"
)

// DriftEntry is one drifting record with its reason.
type DriftEntry struct {
	RecordID string      `json:"record_id"`
	Reason   DriftReason `json:"reason"`
}

// DriftReport is the aggregate drift report.
type DriftReport struct {
	Entries []DriftEntry   `json:"entries"`
	Counts  map[string]int `json:"counts"`
	Total   int            `json:"total"`
}

// DriftReasonFor classifies one indexed row against the current authoritative record.
// It applies the same eligibility rules as outcomefile.Eligible, so a source that became
// incomplete, superseded, unauthorized, or ineligible for any other reason is never
// treated as a healthy row. It reports false when the row matches its source.
func DriftReasonFor(src outcomefile.OutcomeRecord, hasSource bool, indexedHash string) (DriftReason, bool) {
	if !hasSource {
		return DriftDeleted, true
	}
	if ok, reason := src.Eligible(); !ok {
		switch reason {
		case "superseded":
			return DriftSuperseded, true
		case "unauthorized":
			return DriftUnauthorized, true
		default:
			return DriftIncomplete, true
		}
	}
	if src.ContentHash != indexedHash {
		return DriftModified, true
	}
	return "", false
}

// DetectDrift compares every index row against the current outcome records and reports
// rows whose source is deleted, superseded, changed (hash), or no longer authorized. It
// also reports eligible source records that are missing from the index (DriftMissing), so
// status can reveal an index that has not yet indexed new evidence.
func (ix *Index) DetectDrift(ctx context.Context, current map[string]outcomefile.OutcomeRecord) (DriftReport, error) {
	hashes, err := ix.HashRows(ctx)
	if err != nil {
		return DriftReport{}, err
	}
	rep := DriftReport{Counts: map[string]int{}}
	ids := make([]string, 0, len(hashes))
	for id := range hashes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		src, ok := current[id]
		if reason, drifted := DriftReasonFor(src, ok, hashes[id]); drifted {
			rep.add(id, reason)
		}
	}
	// Source records present and eligible but absent from the index are missing.
	srcIDs := make([]string, 0, len(current))
	for id := range current {
		srcIDs = append(srcIDs, id)
	}
	sort.Strings(srcIDs)
	for _, id := range srcIDs {
		if _, ok := hashes[id]; ok {
			continue
		}
		src := current[id]
		if src.Superseded || !src.Authorized || !src.Complete {
			continue
		}
		rep.add(id, DriftMissing)
	}
	sort.Slice(rep.Entries, func(i, j int) bool { return rep.Entries[i].RecordID < rep.Entries[j].RecordID })
	rep.Total = len(rep.Entries)
	return rep, nil
}

func (r *DriftReport) add(id string, reason DriftReason) {
	r.Entries = append(r.Entries, DriftEntry{RecordID: id, Reason: reason})
	r.Counts[string(reason)]++
}

// DriftedSet returns the set of record ids currently drifting, used by the read path to
// exclude stale evidence even before a rebuild.
func (r DriftReport) DriftedSet() map[string]bool {
	out := make(map[string]bool, len(r.Entries))
	for _, e := range r.Entries {
		out[e.RecordID] = true
	}
	return out
}

// LocalRetriever is the local SQLite retrieval implementation of the shared bounded
// retrieval interface. It excludes drifting rows on the read path so a stale index can
// never return invalid evidence.
type LocalRetriever struct {
	Index *Index
	// Current returns the current authoritative outcome records keyed by stable record ID.
	// It is required: a nil Current fails closed rather than silently disabling drift
	// exclusion and returning stale evidence.
	Current func(ctx context.Context) (map[string]outcomefile.OutcomeRecord, error)
}

// Retrieve searches the index and drops any evidence row that drifts from the current
// source, then reapplies the bounds so the result count and token budget stay honored.
//
// Only the fetched candidate rows are compared against the current source, so the read
// path does not scan the whole index or the whole source set per query. It fetches up to
// the ceiling so exclusion never starves a valid result that ranks lower, then applies
// MaxResults and the token budget once.
func (lr *LocalRetriever) Retrieve(ctx context.Context, q retrievalboundary.ScopedQuery) (retrievalboundary.Result, error) {
	if lr.Index == nil {
		return retrievalboundary.Result{}, &retrievalboundary.InvalidQueryError{Reason: "nil local index"}
	}
	if lr.Current == nil {
		return retrievalboundary.Result{}, &retrievalboundary.InvalidQueryError{Reason: "nil current source: drift exclusion would be disabled"}
	}
	lim, err := retrievalboundary.Validate(q)
	if err != nil {
		return retrievalboundary.Result{}, err
	}

	current, err := lr.Current(ctx)
	if err != nil {
		return retrievalboundary.Result{}, err
	}

	// Fetch up to the ceiling so exclusion never starves a valid result that ranks lower.
	fetch := q
	fetch.Limits.MaxResults = retrievalboundary.MaxResultsCeiling
	fetch.Limits.TokenBudget = retrievalboundary.MaxTokenBudgetCeiling
	raw, err := lr.Index.Search(ctx, fetch)
	if err != nil {
		return retrievalboundary.Result{}, err
	}
	kept := make([]retrievalboundary.Evidence, 0, len(raw.Evidence))
	for _, e := range raw.Evidence {
		src, ok := current[e.RecordID]
		if _, drifted := DriftReasonFor(src, ok, e.ContentHash); drifted {
			continue
		}
		kept = append(kept, e)
	}
	res := retrievalboundary.ApplyBounds(kept, lim)
	res.Compatibility = "local sqlite fts5 with drift exclusion"
	return res, nil
}

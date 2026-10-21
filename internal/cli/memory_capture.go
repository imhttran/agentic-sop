package cli

import (
	"context"
	"path/filepath"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// captureOutcome records a completed run in the authoritative outcome store and rebuilds
// the local advisory index. It runs only when advisory memory is enabled for the project.
//
// Capture is idempotent: the outcome document is keyed by stable record ID and content
// hash, and the index rebuild is derived from the same source, so repeating capture for
// the same run produces the same result. A disabled or misconfigured memory layer is a
// no-op/error for the caller, never a change to the task verdict.
func captureOutcome(dir string, cfg config.Config, taskID string) error {
	resolved, err := cfg.AdvisoryRetrieval.Resolved(nil)
	if err != nil {
		return err
	}
	if !resolved.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(resolved.CaptureTimeoutMS)*time.Millisecond)
	defer cancel()
	rec, err := outcomefile.IngestRunFor(filepath.Join(dir, config.DirName, "runs"), taskID, cfg.Project.Name)
	if err != nil {
		return err
	}
	// Failed and blocked outcomes are failure history, not remedies.
	if rec.Status == outcomefile.StatusFailed || rec.Status == outcomefile.StatusBlocked {
		rec.Corpus = retrievalboundary.CorpusFailure
	}
	if _, _, err := outcomefile.Write(resolved.OutcomesDir, rec); err != nil {
		return err
	}
	ix, err := memindex.OpenContext(ctx, resolved.IndexPath)
	if err != nil {
		return err
	}
	defer ix.Close()
	_, err = ix.RebuildFromDir(ctx, resolved.OutcomesDir)
	return err
}

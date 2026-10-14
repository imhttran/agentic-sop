package sopclient

import (
	"path/filepath"
	"strings"
)

// Checkpoint reads (CTRL010). checkpoint.json is optional; structured activity is
// the fallback. Neither present means an absent checkpoint, never a made-up one.

// checkpointArtifact is optional; SOP writes it only for bounded-progress work.
const checkpointArtifact = "checkpoint.json"

// checkpointDoc counters are pointers so a reported zero differs from absent.
type checkpointDoc struct {
	Label    string `json:"label"`
	Source   string `json:"source"`
	Analyzed *int   `json:"analyzed"`
	Total    *int   `json:"total"`
	Covered  *int   `json:"covered"`
	Missing  *int   `json:"missing"`
	Next     string `json:"next"`
}

// Checkpoint prefers checkpoint.json, else the latest activity entry carrying a
// bounded-progress line, else an absent record.
func (s *Store) Checkpoint(taskID string) CheckpointProgress {
	path := filepath.Join(s.runDir(taskID), checkpointArtifact)
	var doc checkpointDoc
	if readJSON(path, &doc) {
		return doc.progress("")
	}
	if cp, ok := s.checkpointFromActivity(taskID); ok {
		return cp
	}
	return CheckpointProgress{}
}

// progress marks a counter present only when SOP recorded it. fallbackSource
// labels provenance when the record carries no "source".
func (d checkpointDoc) progress(fallbackSource string) CheckpointProgress {
	source := strings.TrimSpace(d.Source)
	if source == "" {
		source = strings.TrimSpace(fallbackSource)
	}
	cp := CheckpointProgress{Label: d.Label, Source: source, Next: d.Next}
	cp.Analyzed, cp.HasAnalyzed = deref(d.Analyzed)
	cp.Total, cp.HasTotal = deref(d.Total)
	cp.Covered, cp.HasCovered = deref(d.Covered)
	cp.Missing, cp.HasMissing = deref(d.Missing)
	return cp.sanitized()
}

// checkpointFromActivity returns the newest activity entry that states a
// bounded-progress line.
func (s *Store) checkpointFromActivity(taskID string) (CheckpointProgress, bool) {
	events := s.activity(taskID)
	for i := len(events) - 1; i >= 0; i-- {
		if cp, ok := checkpointFromDetail(taskID, events[i]); ok {
			return cp, true
		}
	}
	return CheckpointProgress{}, false
}

// deref returns the pointed-to int and whether SOP reported it.
func deref(p *int) (int, bool) {
	if p == nil {
		return 0, false
	}
	return *p, true
}

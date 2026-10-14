package sopclient

import (
	"regexp"
	"strconv"
	"strings"
)

// checkpointFromDetail extracts a bounded-progress line from one activity Detail
// (already sanitized), e.g.:
//
// 	JEV012, 11/15 cases analyzed, covered: 11, missing: 4, next: add deterministic coverage
//
// A counter is reported only when stated; none is derived or zero-filled.

var analyzedTotalRe = regexp.MustCompile(`\b(\d+)\s*/\s*(\d+)\b`)

// Labelled counters, anchored on SOP's label so a stray number never matches.
var (
	coveredRe = regexp.MustCompile(`(?i)\bcovered\s*[:=]?\s*(\d+)\b`)
	missingRe = regexp.MustCompile(`(?i)\bmissing\s*[:=]?\s*(\d+)\b`)
	nextRe    = regexp.MustCompile(`(?i)\bnext\s*[:=]\s*(.+)$`)
	labelRe   = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*[0-9][A-Za-z0-9_-]*)\s*[,:]`)
)

// analyzedTotalKeywords: a bare "11/15" counts only when the unit is named.
var analyzedTotalKeywords = []string{"analyzed", "cases", "checks", "criteria", "items", "files", "test"}

// checkpointFromDetail returns ok=false when the detail has no progress evidence.
func checkpointFromDetail(taskID string, e ActivityEvent) (CheckpointProgress, bool) {
	detail := strings.TrimSpace(e.Detail)
	if detail == "" {
		return CheckpointProgress{}, false
	}

	label := ""
	if m := labelRe.FindStringSubmatch(detail); m != nil {
		label = m[1]
	}

	var (
		analyzed, total, covered, missing             int
		hasAnalyzed, hasTotal, hasCovered, hasMissing bool
	)

	lower := strings.ToLower(detail)
	namesUnit := false
	for _, kw := range analyzedTotalKeywords {
		if strings.Contains(lower, kw) {
			namesUnit = true
			break
		}
	}
	if namesUnit {
		if m := analyzedTotalRe.FindStringSubmatch(detail); m != nil {
			if v, err := strconv.Atoi(m[1]); err == nil {
				analyzed, hasAnalyzed = v, true
			}
			if v, err := strconv.Atoi(m[2]); err == nil {
				total, hasTotal = v, true
			}
		}
	}
	if m := coveredRe.FindStringSubmatch(detail); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			covered, hasCovered = v, true
		}
	}
	if m := missingRe.FindStringSubmatch(detail); m != nil {
		if v, err := strconv.Atoi(m[1]); err == nil {
			missing, hasMissing = v, true
		}
	}

	// A label alone (e.g. "JEV012, starting") is context, not progress.
	if !hasAnalyzed && !hasTotal && !hasCovered && !hasMissing {
		return CheckpointProgress{}, false
	}

	next := ""
	if m := nextRe.FindStringSubmatch(detail); m != nil {
		next = strings.TrimSpace(m[1])
	}

	// Fall back to the stage as label when the detail names no scope id.
	if label == "" {
		label = strings.TrimSpace(e.Stage)
	}

	return CheckpointProgress{
		Label: label, Source: "activity", Next: next,
		Analyzed: analyzed, HasAnalyzed: hasAnalyzed,
		Total: total, HasTotal: hasTotal,
		Covered: covered, HasCovered: hasCovered,
		Missing: missing, HasMissing: hasMissing,
	}.sanitized(), true
}

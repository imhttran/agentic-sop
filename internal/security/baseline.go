package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Baseline is a reviewed set of suppressions. An entry requires a stable
// fingerprint, a reason, an owner, and an expiry. Suppression never removes a
// finding from the report; it only marks it and excludes it from blocking.
type Baseline struct {
	Version int             `json:"version"`
	Entries []BaselineEntry `json:"entries"`
}

// BaselineEntry is one reviewed suppression. Path and RuleID are optional
// additional constraints; the fingerprint is the primary identity.
type BaselineEntry struct {
	Fingerprint string    `json:"fingerprint"`
	RuleID      string    `json:"rule_id,omitempty"`
	Path        string    `json:"path,omitempty"`
	Reason      string    `json:"reason"`
	Owner       string    `json:"owner"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// LoadBaseline reads a reviewed baseline. An absent file is an empty baseline,
// not an error. A malformed or oversized file is an error: a baseline that
// cannot be understood must not silently suppress anything.
func LoadBaseline(root, relativePath string) (Baseline, []string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return Baseline{}, nil, nil
	}
	path := relativePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(relativePath))
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Baseline{}, nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Baseline{}, nil, err
	}
	if rel, err := filepath.Rel(absRoot, absPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return Baseline{}, nil, fmt.Errorf("security: baseline %q escapes the workspace", relativePath)
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Baseline{}, []string{"baseline file not found; no suppressions applied"}, nil
		}
		return Baseline{}, nil, fmt.Errorf("security: read baseline: %w", err)
	}
	if len(data) > 1<<20 {
		return Baseline{}, nil, fmt.Errorf("security: baseline exceeds the 1 MiB limit")
	}
	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return Baseline{}, nil, fmt.Errorf("security: parse baseline: %w", err)
	}
	if baseline.Version != 1 {
		return Baseline{}, nil, fmt.Errorf("security: unsupported baseline version %d", baseline.Version)
	}
	return baseline, nil, nil
}

// Apply marks findings matched by a reviewed and unexpired suppression. Invalid
// entries are ignored and reported. An expired entry is visible and does not
// suppress. New findings never match an unrelated entry because the fingerprint
// is exact.
func (b Baseline) Apply(findings []Finding, now time.Time) ([]Finding, []string) {
	out := append([]Finding(nil), findings...)
	var diagnostics []string
	active := make(map[string]BaselineEntry, len(b.Entries))
	expired := make(map[string]BaselineEntry, len(b.Entries))
	for i, entry := range b.Entries {
		entry.Fingerprint = strings.TrimSpace(entry.Fingerprint)
		entry.Reason = strings.TrimSpace(entry.Reason)
		entry.Owner = strings.TrimSpace(entry.Owner)
		if entry.Fingerprint == "" || entry.Reason == "" || entry.Owner == "" || entry.ExpiresAt.IsZero() {
			diagnostics = append(diagnostics, fmt.Sprintf("baseline entry %d ignored: fingerprint, reason, owner, and expires_at are required", i))
			continue
		}
		if !entry.ExpiresAt.After(now) {
			expired[entry.Fingerprint] = entry
			continue
		}
		active[entry.Fingerprint] = entry
	}
	for i := range out {
		f := &out[i]
		if entry, ok := active[f.Fingerprint]; ok {
			if !baselineEntryMatches(entry, *f) {
				continue
			}
			f.Suppressed = true
			f.Suppression = &SuppressionMatch{
				Reason:    SanitizeText(entry.Reason, MaxMessageBytes),
				Owner:     SanitizeText(entry.Owner, 256),
				ExpiresAt: entry.ExpiresAt.UTC(),
				Expired:   false,
			}
			continue
		}
		if entry, ok := expired[f.Fingerprint]; ok {
			if !baselineEntryMatches(entry, *f) {
				continue
			}
			f.Suppressed = false
			f.Suppression = &SuppressionMatch{
				Reason:    SanitizeText(entry.Reason, MaxMessageBytes),
				Owner:     SanitizeText(entry.Owner, 256),
				ExpiresAt: entry.ExpiresAt.UTC(),
				Expired:   true,
			}
			diagnostics = append(diagnostics, fmt.Sprintf("expired suppression for %s (%s) is visible and no longer effective", f.RuleID, f.Path))
		}
	}
	return out, diagnostics
}

func baselineEntryMatches(entry BaselineEntry, finding Finding) bool {
	if entry.RuleID != "" && entry.RuleID != finding.RuleID {
		return false
	}
	if entry.Path != "" {
		path, _ := NormalizePath(entry.Path)
		if path != "" && path != finding.Path {
			return false
		}
	}
	return true
}

// Identity returns a deterministic policy identity that includes the baseline
// content, so changing a suppression invalidates resumed evidence.
func (b Baseline) Identity() string {
	data, err := json.Marshal(b)
	if err != nil {
		return ""
	}
	return DigestBytes(data)
}

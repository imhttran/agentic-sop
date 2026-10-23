package hipcompare

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ResumeDecision reports whether a stored comparison may be resumed.
type ResumeDecision struct {
	// Resumed is true only when every fingerprint component matches.
	Resumed bool
	// DifferingComponent names the first component whose fingerprint differs, or
	// "" when the fingerprints match. A mismatched resume is refused, naming it.
	DifferingComponent string
	// Stored is the comparison read from disk, if any.
	Stored *Comparison
}

// ResumeFingerprint reads a stored comparison from dir (comparison.json) and
// decides whether it may be resumed with the supplied fingerprint. It refuses to
// resume when any component differs, naming that component.
func ResumeFingerprint(dir string, want Fingerprint) (ResumeDecision, error) {
	data, err := os.ReadFile(filepath.Join(dir, ResultsFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ResumeDecision{Resumed: false, DifferingComponent: "(no stored comparison)"}, nil
		}
		return ResumeDecision{}, fmt.Errorf("read stored comparison: %w", err)
	}
	var stored Comparison
	if err := json.Unmarshal(data, &stored); err != nil {
		return ResumeDecision{}, fmt.Errorf("parse stored comparison: %w", err)
	}
	d := ResumeDecision{Stored: &stored}
	if stored.Fingerprint.Version != want.Version {
		d.DifferingComponent = "fingerprint version"
		return d, nil
	}
	if stored.Fingerprint.Digest != want.Digest {
		// Name the component, not just "fingerprint": every part is compared so the
		// mismatch is actionable.
		d.DifferingComponent = differingPart(stored.Fingerprint.Parts, want.Parts)
		if d.DifferingComponent == "" {
			d.DifferingComponent = "fingerprint digest"
		}
		return d, nil
	}
	d.Resumed = true
	return d, nil
}

// differingPart returns the first sorted key whose part differs, or a key present
// on only one side.
func differingPart(a, b map[string]string) string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		if a[k] != b[k] {
			return k
		}
	}
	return ""
}

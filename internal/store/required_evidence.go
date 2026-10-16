package store

import (
	"encoding/json"
	"fmt"
)

// encodeRequiredEvidence serializes a task's required-evidence declarations to
// the JSON stored in the tasks.required_evidence column (RAG-SOP-003). An empty
// or nil list is encoded as "[]" so the column is never NULL and a migrated
// default round-trips. Declaration order is preserved.
func encodeRequiredEvidence(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// decodeRequiredEvidence parses the tasks.required_evidence column. It NEVER
// converts malformed stored data into an empty requirement list: an invalid
// value is returned as an error so a corrupt row is surfaced rather than
// silently treated as "no requirements". An empty string is rejected too, so a
// missing value can never silently drop a requirement list; only the JSON array
// "[]" means "no required evidence" and preserves the pre-existing behavior.
func decodeRequiredEvidence(raw string) ([]string, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty required_evidence value; a stored task must carry a JSON array")
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid required_evidence JSON: %w", err)
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items, nil
}

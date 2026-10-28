package quality

import "testing"

// TestMatchPath pins `**` semantics at the start, end, and interior of a pattern.
func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"crates/**/shared.rs", "crates/core/src/shared.rs", true},
		{"crates/**/shared.rs", "crates/shared.rs", true},
		{"crates/**/shared.rs", "crates/core/src/other.rs", false},
		{"**/migrations/**", "db/migrations/001.sql", true},
		{"crates/**", "crates/core/lib.rs", true},
		{"**/*.sql", "schema.sql", true},
		{"crates/core/**", "crates/cli/main.rs", false},
		{"**", "", false},
	}
	for _, c := range cases {
		if got := MatchPath(c.pattern, c.path); got != c.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

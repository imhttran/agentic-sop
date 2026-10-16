package validate

import "testing"

func TestEnsureGoTestJSON(t *testing.T) {
	cases := map[string]string{
		"go test ./db/... -run TestX": "go test ./db/... -run TestX -json",
		"go test -json ./db/...":      "go test -json ./db/...",
		"psql -f migrate.sql":         "psql -f migrate.sql",
	}
	for in, want := range cases {
		if got := ensureGoTestJSON(in); got != want {
			t.Errorf("ensureGoTestJSON(%q) = %q, want %q", in, got, want)
		}
	}
}

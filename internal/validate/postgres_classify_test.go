package validate

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// TestClassifyPostgresCommandRequiresPostgresTag proves that classification is
// PostgreSQL-SPECIFIC: a generic `-tags=integration` Go test is NOT classified,
// because such a run may exercise no database and must never stand in for
// pgvector operations. Only an explicit postgres/pgvector tag qualifies.
func TestClassifyPostgresCommandRequiresPostgresTag(t *testing.T) {
	cases := []struct {
		command string
		want    bool
	}{
		{"go test -tags=postgres ./...", true},
		{"go test -tags pgvector ./...", true},
		{"go test -tags=integration ./...", false},
		{"go test -tags=postgres,integration ./...", true},
		{"go test ./...", false},
		{"go test ./... -run postgres", false},
		{"psql -f migrate.sql", true},
		{"go build ./...", false},
		{"true", false},
	}
	for _, tc := range cases {
		if got := ClassifyPostgresCommand(tc.command); got != tc.want {
			t.Errorf("ClassifyPostgresCommand(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

// TestPostgresCategoryOnlyFromIntegrationField proves that the category is
// satisfied ONLY by a classified command in validation.integration: a
// PostgreSQL-looking command placed under a generic build/test/lint list does not
// count, and a unit-only integration list does not count either. The designated
// field is the single configuration surface for the category.
func TestPostgresCategoryOnlyFromIntegrationField(t *testing.T) {
	if PostgresConfigured(config.Validation{Test: []string{"psql -f migrate.sql"}}) {
		t.Fatal("a PostgreSQL command under validation.test must not satisfy the category")
	}
	if !PostgresConfigured(config.Validation{Integration: []string{"psql -f migrate.sql"}}) {
		t.Fatal("a PostgreSQL command under validation.integration must satisfy the category")
	}
	if PostgresConfigured(config.Validation{Integration: []string{"go test ./..."}}) {
		t.Fatal("a unit-only validation.integration command must not satisfy the category")
	}
}

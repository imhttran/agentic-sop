package quality

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// testPolicy builds the deterministic default quality policy used by the
// PostgreSQL-category tests: tests required, a three-cycle fix budget, and the
// standard blocking severities. It mirrors the built-in default so the tests
// exercise the gate exactly as a default project would.
func testPolicy() config.Quality {
	requireTests := true
	return config.Quality{
		RequireTests: &requireTests,
		MaxFixCycles: config.DefaultMaxFixCycles,
		FailOn:       []string{"critical", "high"},
	}
}

func TestPostgresIntegrationRequiredMatchesDatabaseSensitivePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"migration dir", "db/migrations/0001_init.sql", true},
		{"migration dir at root", "migrations/0001_init.sql", true},
		{"plain sql", "internal/store/schema.sql", true},
		{"migrate go", "internal/store/migrate.go", true},
		{"migrate prefix go", "internal/store/migrate_up.go", true},
		{"schema go", "internal/store/schema.go", true},
		{"pg go", "internal/store/pg.go", true},
		{"pg prefix go", "internal/store/pgvector.go", true},
		{"vector go", "internal/store/vector.go", true},
		{"pgvector name", "internal/store/pgvector_index.go", true},
		{"non-database change", "internal/retrieval/retrieval.go", false},
		{"readme", "README.md", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PostgresIntegrationRequired([]string{tc.path}); got != tc.want {
				t.Fatalf("PostgresIntegrationRequired(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestPostgresIntegrationRequiredIsDeterministicOverSets(t *testing.T) {
	paths := []string{"docs/README.md", "internal/quality/quality.go", "internal/pgdb/pg.go"}
	if !PostgresIntegrationRequired(paths) {
		t.Fatal("expected a database-sensitive path in the set to require the category")
	}
	if PostgresIntegrationRequired([]string{"docs/README.md", "internal/quality/quality.go"}) {
		t.Fatal("non-database paths must not require the category")
	}
}

// TestPostgresPatternsIgnoreDotPrefixedSegments proves the deterministic matcher
// does not let a wildcard match a leading-dot (hidden) segment: a dotfile-like
// path cannot silently acquire the PostgreSQL integration category.
func TestPostgresPatternsIgnoreDotPrefixedSegments(t *testing.T) {
	for _, p := range []string{".sql", "internal/store/.pgvector", "internal/.migrations/x"} {
		if PostgresIntegrationRequired([]string{p}) {
			t.Errorf("hidden segment %q must not require the PostgreSQL category", p)
		}
	}
}

func TestPostgresIntegrationPatternsFrozen(t *testing.T) {
	patterns := PostgresIntegrationPatterns()
	if len(patterns) == 0 {
		t.Fatal("pattern list must not be empty")
	}
	// Mutating the returned copy must not change policy.
	patterns[0] = "mutated"
	if PostgresIntegrationPatterns()[0] == "mutated" {
		t.Fatal("PostgresIntegrationPatterns must return a copy")
	}
}

// TestUnitOnlyValidationDoesNotSatisfyPostgresCategory proves that a run whose
// only configured validation is unit-level fails closed when the policy marks
// the PostgreSQL integration category required: the deterministic gate never
// accepts unit-only validation as proof of pgvector operations.
func TestUnitOnlyValidationDoesNotSatisfyPostgresCategory(t *testing.T) {
	req := PostgresIntegrationRequired([]string{"internal/store/migrations/0002_add_vectors.sql"})
	if !req {
		t.Fatal("migration path must require the PostgreSQL integration category")
	}
	// Unit-only configuration: no integration category command is configured, so
	// the gate fails closed.
	res := Evaluate(testPolicy(), Input{
		BuildPassed:          true,
		TestPassed:           true,
		ValidationRequired:   req,
		ValidationConfigured: false,
	})
	if res.Decision != Fail {
		t.Fatalf("unit-only validation must not satisfy the postgres-integration category, got %v", res.Decision)
	}
}

// TestNonDatabaseChangeUnaffected proves non-database changes do not acquire the
// category and are decided solely by the ordinary policy.
func TestNonDatabaseChangeUnaffected(t *testing.T) {
	req := PostgresIntegrationRequired([]string{"internal/retrieval/retrieval.go"})
	if req {
		t.Fatal("non-database change must not require the PostgreSQL integration category")
	}
	res := Evaluate(testPolicy(), Input{
		BuildPassed:          true,
		TestPassed:           true,
		ValidationRequired:   req,
		ValidationConfigured: false,
	})
	if res.Decision != Pass {
		t.Fatalf("non-database change must be unaffected, got %v (%v)", res.Decision, res.Reasons)
	}
}

package quality

// PostgreSQL integration validation category policy.
//
// This file implements the deterministic file-pattern policy required by
// RAG-SOP-004 task 1: migration and database-sensitive changes must require the
// `postgres-integration` validation category, which can only be satisfied by a
// real PostgreSQL/pgvector integration command. It is deliberately separate
// from retrieval quality: it consumes changed paths only, never retrieval
// metrics, and unit-only validation is never accepted as proof of pgvector
// operations.
//
// The policy is pure and has no model dependency, so classification is exactly
// reproducible from the changed paths alone.

// PostgresIntegrationCategory is the validation category name required for
// migration and database-sensitive changes.
const PostgresIntegrationCategory = "postgres-integration"

// postgresPathPatterns is the frozen, deterministic list of glob patterns that
// select the PostgreSQL integration validation category. Semantics (see
// matchGlob):
//
//   - `**` matches zero or more whole path segments.
//   - `*` and `?` match within a single path segment only.
//
// A pattern is anchored to whole path segments, so `**/pg*.go` matches
// `internal/store/pg.go` and `internal/store/pgvector.go` and `db/pg/conn.go`
// would only match via `**` if the named file segment matches. It does NOT match
// an arbitrary path that merely contains the substring "pg" (for example
// `internal/payloads/render.go` is not matched by `**/pg*.go`, because the
// segment `payloads` is not a `pg*.go` file).
var postgresPathPatterns = []string{
	"**/migrations/**",
	"**/*.sql",
	"**/migrate*.go",
	"**/schema*.go",
	"**/pg*.go",
	"**/vector*.go",
	"**/*pgvector*",
}

// PostgresIntegrationPatterns returns a copy of the deterministic pattern list,
// for documentation and tests. Callers must not mutate the policy through it.
func PostgresIntegrationPatterns() []string {
	out := make([]string, len(postgresPathPatterns))
	copy(out, postgresPathPatterns)
	return out
}

// PostgresIntegrationRequired reports whether any of the changed paths matches
// the deterministic PostgreSQL integration pattern set. It is the sole input to
// the required-category decision: no retrieval-quality signal participates.
func PostgresIntegrationRequired(changedPaths []string) bool {
	for _, p := range changedPaths {
		if matchesPostgresPattern(p) {
			return true
		}
	}
	return false
}

// matchesPostgresPattern reports whether one changed path matches the policy.
func matchesPostgresPattern(path string) bool {
	for _, pattern := range postgresPathPatterns {
		if MatchPath(pattern, path) {
			return true
		}
	}
	return false
}

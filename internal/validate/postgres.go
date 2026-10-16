package validate

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// PostgreSQL integration validation classification.
//
// RAG-SOP-004 task 1 requires that migration and database-sensitive changes be
// verified by the postgres-integration validation category, and that unit-only
// validation can NEVER stand in for it. The category is a deterministic property
// of the configured commands, not a name an operator can attach to an arbitrary
// command: a command counts only when it is classified as real PostgreSQL/pgvector
// integration work (see ClassifyPostgresCommand). This is deliberately separate
// from retrieval quality: nothing here inspects recall, citations, latency, or any
// RAG metric.

// postgresToolTokens are the executable/tool names that classify a command as
// PostgreSQL/pgvector integration work. Matching is on WHOLE TOKENS (or path
// basenames), not raw substrings, so an unrelated argument such as
// `go test ./... -run postgres` does not classify: `postgres` there is the value
// of `-run`, not a PostgreSQL tool invocation. A plain unit-test harness
// (go test ./..., go test -short ./...) matches none of these tokens.
var postgresToolTokens = map[string]bool{
	"pgvector":   true,
	"postgres":   true,
	"postgresql": true,
	"psql":       true,
	"pg_ctl":     true,
	"pg_isready": true,
	"createdb":   true,
	"dropdb":     true,
	"initdb":     true,
}

// integrationTagTokens are the Go build tags that mark a `go test` run as a
// PostgreSQL/pgvector integration test. Classification is deliberately
// PostgreSQL-SPECIFIC: a generic `-tags=integration` run is NOT classified,
// because such a run may exercise no database at all and must never stand in for
// pgvector operations. Only an explicit `postgres`/`pgvector` tag qualifies.
var integrationTagTokens = map[string]bool{
	"postgres": true,
	"pgvector": true,
}

// ClassifyPostgresCommand reports whether a single configured command is a
// genuine PostgreSQL/pgvector integration command. It is a pure function of the
// command text: no model, no retrieval-quality signal, and no state participates.
//
// Classification rules, in order:
//
//  1. A `go test` command classifies ONLY when it is explicitly tag-gated as
//     PostgreSQL/pgvector integration (`-tags=postgres`, `-tags=pgvector`). A
//     plain `go test ./...`, a generic `-tags=integration` run, or one whose only
//     mention of "postgres" is an argument such as `-run postgres` is NOT
//     classified.
//  2. Any other command classifies when it invokes a PostgreSQL/pgvector tool as
//     a whole token (a command or an argument that is exactly a tool name or a
//     path whose basename is a tool name), or when it is a `docker compose` /
//     `docker-compose` invocation that names a postgres/pgvector service token.
//
// Because a miss fails closed, the classifier errs toward NOT classifying when a
// token is ambiguous.
func ClassifyPostgresCommand(command string) bool {
	c := strings.TrimSpace(command)
	if c == "" {
		return false
	}
	fields := strings.Fields(c)

	// Special-case Go test runs: only an explicit integration tag classifies.
	if isGoTest(fields) {
		return hasIntegrationTag(fields)
	}

	// docker compose / docker-compose: only classify when a postgres/pgvector
	// service token is also present.
	if isDockerCompose(fields) {
		for _, f := range fields {
			if postgresToolTokens[tokenName(f)] {
				return true
			}
		}
		return false
	}

	// General case: a whole-token PostgreSQL/pgvector tool invocation.
	for _, f := range fields {
		if postgresToolTokens[tokenName(f)] {
			return true
		}
	}
	return false
}

// isGoTest reports whether the field list is a `go test` invocation.
func isGoTest(fields []string) bool {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "go" && fields[i+1] == "test" {
			return true
		}
	}
	return false
}

// hasIntegrationTag reports whether a `go test` field list carries an explicit
// PostgreSQL/pgvector build tag, in either `-tags=x`, `-tags=x,y`, or `-tags x` form.
func hasIntegrationTag(fields []string) bool {
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch {
		case strings.HasPrefix(f, "-tags="):
			if tagListContainsIntegration(strings.TrimPrefix(f, "-tags=")) {
				return true
			}
		case f == "-tags" && i+1 < len(fields):
			if tagListContainsIntegration(fields[i+1]) {
				return true
			}
		}
	}
	return false
}

// tagListContainsIntegration reports whether a comma-separated build tag list
// contains an integration tag token.
func tagListContainsIntegration(list string) bool {
	for _, tag := range strings.Split(list, ",") {
		if integrationTagTokens[strings.TrimSpace(tag)] {
			return true
		}
	}
	return false
}

// isDockerCompose reports whether the field list invokes docker compose or
// docker-compose.
func isDockerCompose(fields []string) bool {
	if len(fields) >= 2 && fields[0] == "docker" && fields[1] == "compose" {
		return true
	}
	if len(fields) >= 1 && tokenName(fields[0]) == "docker-compose" {
		return true
	}
	return false
}

// tokenName reduces a shell token to its bare executable/argument name: it
// strips any `KEY=value` assignment prefix and returns the basename of a path,
// lowercased. It never inspects substrings inside a token, so an argument such as
// `-run postgres` or a comment-like `# postgres` cannot classify a command.
func tokenName(token string) string {
	t := token
	if eq := strings.Index(t, "="); eq >= 0 {
		t = t[eq+1:]
	}
	t = strings.Trim(t, "'\"`")
	if i := strings.LastIndexByte(t, '/'); i >= 0 {
		t = t[i+1:]
	}
	return strings.ToLower(t)
}

// PostgresIntegrationCommands returns the configured commands classified into the
// postgres-integration category. It reads ONLY validation.integration — the
// designated field for the category — so a PostgreSQL-looking command placed under
// a generic build/test/lint list never silently satisfies the category, and a
// command listed under validation.integration that is NOT classified (for example
// a plain unit-test command) is omitted too. Either way, unit-only or
// mis-placed validation can never satisfy the category.
func PostgresIntegrationCommands(v config.Validation) []string {
	var out []string
	seen := map[string]bool{}
	for _, command := range v.Integration {
		if strings.TrimSpace(command) == "" {
			continue
		}
		if !ClassifyPostgresCommand(command) {
			continue
		}
		if seen[command] {
			continue
		}
		seen[command] = true
		out = append(out, command)
	}
	return out
}

// PostgresConfigured reports whether at least one configured, classified
// PostgreSQL/pgvector integration command exists. It is the single source of
// truth for whether the postgres-integration category is satisfiable, so a
// unit-only suite (an empty or unclassified integration list) fails closed.
func PostgresConfigured(v config.Validation) bool {
	return len(PostgresIntegrationCommands(v)) > 0
}

// PostgresChecks maps the classified PostgreSQL integration commands to ordered
// checks carrying the PostgreSQL integration category. Only classified commands
// are returned, so an unclassified (unit-only) command is never run or counted as
// satisfying the category.
func PostgresChecks(v config.Validation) []testrunner.Check {
	commands := PostgresIntegrationCommands(v)
	checks := make([]testrunner.Check, 0, len(commands))
	for _, command := range commands {
		checks = append(checks, testrunner.Check{Category: testrunner.PostgresIntegration, Command: ensureGoTestJSON(command)})
	}
	return checks
}

// ensureGoTestJSON makes a `go test` integration command emit structured
// evidence, so the runner can prove a test actually executed and passed rather
// than trusting exit code 0 (an all-skipped run exits 0). Non-Go commands are
// returned unchanged; they never receive a passing PostgreSQL status without a
// supported execution-proof mechanism.
//
// It appends `-json` only when the command does not already request JSON output
// (`-json`, `-json=true`, `-json=false`, or an existing `-json=...`), so an
// explicit `-json=false` is not silently flipped and no bare `-json` is appended
// after `--` (which would be passed to the test binary instead of `go test`).
func ensureGoTestJSON(command string) string {
	fields := strings.Fields(command)
	if len(fields) < 2 || fields[0] != "go" || fields[1] != "test" {
		return command
	}
	for _, f := range fields {
		if f == "--" {
			return command // flags after -- belong to the test binary
		}
		if f == "-json" || strings.HasPrefix(f, "-json=") {
			return command
		}
	}
	return command + " -json"
}

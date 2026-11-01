# Git hooks and agent scripts

## Git hooks

`.githooks/pre-commit` runs `go mod tidy`, `go fmt ./...`, `go vet ./...`,
`staticcheck ./...`, `go test ./...`, and `go build ./...`, and aborts the commit
if any step fails. It also aborts when `go mod tidy` or `go fmt` changed a file
that is part of the commit, so the fix can be staged before committing again.
When `govulncheck` is on `PATH` it also runs `govulncheck ./...` (skipped with an
install hint otherwise).

It also runs `jq empty` over every tracked `*.json`, so a malformed
`evals/retrieval.json` or `.zed/settings.json` fails the commit instead of the
command that reads it. `jq` is skipped with a note if it is not on `PATH`.
`make fmt-json` is the reformatting counterpart, for when you want jq's layout
rather than just a syntax check.

`staticcheck` must be on `PATH` (`brew install staticcheck` on macOS). It catches
unused code — functions, methods, and types — which `go vet` does not.

`.githooks/pre-push` runs what the pre-commit hook skips, using one
project-agnostic script in `.agents/scripts/`. The hook supplies only what is
specific to this project.

`.agents/scripts/integration-test-agent.sh` runs the database tests
(`internal/retrieval`, `internal/ingestion`) given by `INTEGRATION_TEST_CMD`. It
probes `INTEGRATION_TEST_DB` with `psql` first — this project points that at
`RAG_TEST_DATABASE_URL` (a throwaway database, `rag_test` by default), because the
tests truncate the `documents` table and must never touch `DATABASE_URL`. An
unreachable database is skipped with a note, so a stopped Docker daemon never
blocks a push; `INTEGRATION_TEST_STRICT` (`RAG_REQUIRE_INTEGRATION=1`) fails
instead. It also fails when the command passes but no test ran, so a `-run` filter
that matches nothing cannot turn the gate green.

`.agents/scripts/review-agent.sh` reviews the diff against `origin/main` and is
advisory — a finding never fails anything. It is not wired into the hook either,
so run it by hand when you want a second pair of eyes: it is read-only
(`--tools ""`), budget-capped (`REVIEW_BUDGET`, default `0.25` USD), and skips
itself unless at least `REVIEW_MIN_LINES` lines of what `REVIEW_PATHS` matches
changed.

`.agents/scripts/audit-agent.sh` hunts over-engineering — dependencies the
standard library already ships, single-implementation interfaces, dead flags — and
writes its report to `AUDIT.md` at the root of the repository (gitignored). It is
not wired into the hook, so it never delays a push: run it by hand. It gets
read-only tools (`AUDIT_TOOLS`, default `Read,Grep,Glob`), so it can walk the tree
but cannot change it, and it is budget-capped (`AUDIT_BUDGET`, default `0.50` USD).
It skips itself unless at least `AUDIT_MIN_LINES` lines changed against
`AUDIT_BASE`.

The audit's rulebook is a skill file, `AUDIT_SKILL`, so one file drives the
script and an editor session that invokes the skill. It defaults to the global
ponytail-audit skill at `~/.agents/skills/ponytail-audit/SKILL.md`; point it
somewhere else to audit by different rules. Run it whenever you want a report:

```sh
AUDIT_MIN_LINES=0 sh .agents/scripts/audit-agent.sh
```

The scripts read their settings from environment variables, so another repository
can reuse them by copying the folder somewhere shared and pointing a hook (or a
manual run) at them:

```sh
AUDIT_AGENT=/path/to/shared/audit-agent.sh
INTEGRATION_TEST_AGENT=/path/to/shared/integration-test-agent.sh
REVIEW_AGENT=/path/to/shared/review-agent.sh
```

Set `REVIEW_MIN_LINES=0` to review any change, or `REVIEW_CMD=false` to switch the
review off; `AUDIT_MIN_LINES=0` and `AUDIT_CMD=false` do the same for the audit.
Each script documents the rest.

Git does not pick up `.githooks/` on its own. Enable it once per clone:

```bash
git config core.hooksPath .githooks
```

Bypass the commit hook with `git commit --no-verify`, or the push hook with
`git push --no-verify`.

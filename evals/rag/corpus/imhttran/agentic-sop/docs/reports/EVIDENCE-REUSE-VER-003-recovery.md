# VER-003 targeted recovery — provenance record

Worktree: `agentic-sop-ver` (branch `plan/ver-evidence-reuse`, base `8980826`).
Scope: repair the verified VER-003 compile failure and run targeted validation.
No approval granted; no commit; no push.

## Failure (SOP-recorded evidence, preserved)
- `.agent-sdlc/runs/VER-003/gate.json` — `NEEDS_HUMAN`, reasons `["build failed","fix-loop limit reached (3/3)"]`.
- `.agent-sdlc/runs/VER-003/classification.json` — `AUTO_FIX_EXHAUSTED` / `NEEDS_HUMAN`.
- Compiler errors: `internal/cli/validate.go:109,125,144` (wrong arg count to `recordDecision`)
  and `internal/cli/validate_reuse.go:237` (`NewDecisionRecord` args reversed: `reason, pri`).

## Repair (minimal; `internal/cli/validate_reuse.go`)
`NewDecisionRecord(id, hit, source, sourceRun, pri, reason)` derives `PriorRun` itself from
`pri.Source`, so `recordDecision`'s extra `priorRun` parameter was redundant and its body
passed `pri`/`reason` in the wrong order. Aligned `recordDecision` to the call sites and
fixed the order:
- signature: `func recordDecision(path string, id verifcache.Identity, hit bool, source, reason string, pri verifcache.Entry) error`
- body: `rec := verifcache.NewDecisionRecord(id, hit, source, sourceRun(path), pri, reason)`
No call sites changed (they already passed the intended 6 args).

## Targeted validation (all in the worktree)
| Command | Result |
| --- | --- |
| `gofmt -l internal/cli/validate_reuse.go` | clean |
| `go build ./...` | exit 0 |
| `go vet ./internal/cli/... ./internal/verifcache/...` | exit 0 |
| `go test -count=1 -timeout 120s -run 'TestValidationReuse\|TestRunValidateCache\|TestRunValidateWithoutCache' ./internal/cli/` | PASS (4 tests) |
| `go test -race ./internal/verifcache/...` | ok |
| `git diff --check` | clean |

Full `go test ./...` intentionally NOT run: pre-existing `internal/cli` hang
(10m panic on a SQLite commit), preserved as FAILED evidence at
`.agent-sdlc/runs/VER-001/validation.full-suite.FAILED.json`.

## SOP recovery determination
- `sop resume` → "nothing to resume".
- `sop retry` applies to `BLOCKED` tasks only (VER-003 is `PLANNED` with a `PENDING` approval).
- Re-invoking `sop run` **re-raised** the gate: `VER-003 NEEDS_HUMAN` / "Run paused: human approval required".
- Conclusion: SOP has **no autonomous retry** past this boundary; it requires an explicit
  `sop approve VER-003` or `sop decline VER-003`, then `sop run`.
- This agent did **not** approve or decline the gate.

## Remaining limitation (requirement 8)
The worktree's standard validation set is scoped to `internal/verifcache` + `internal/archtest`
and does not run `internal/cli` tests (they hang in this environment), so VER-003's CLI
integration is validated only by build/vet + the targeted reuse tests above — not by the gate.

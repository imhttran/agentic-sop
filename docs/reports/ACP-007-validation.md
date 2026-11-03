# ACP-007 — Validation record

Task: ACP-007 of [PLAN-SOP-ACP-ZED.md](../plans/PLAN-SOP-ACP-ZED.md). This report records the deterministic validation executed for the ACP change set. It records only what was run; no result is inferred.

## Revision

| Item | Value |
| --- | --- |
| Repository | `github.com/imhttran/agentic-sop` |
| Branch | `feat/sop-acp-zed` |
| Commit | `79acbfc` (parent `82ad60b`) |
| Plan | `plan-sop-acp-zed` — 7 tasks, all `LOCAL_DONE`, final gate PASS |

## Commands and results

Run from the repository root with `TMPDIR` on RAM-backed storage (`/dev/shm/sop-run`), so the SQLite-heavy suite is not dominated by disk I/O.

| Command | Result |
| --- | --- |
| `go build ./...` | PASS |
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `go test ./internal/acp/...` | PASS |
| `go test ./internal/cli/...` | PASS |
| `go test ./internal/archtest/...` | PASS |

SOP recorded the same validation for ACP-007 (`go build ./...`, `go test ./...`, `go vet ./...` all PASS) and executed the declared acceptance checks (`go test ./internal/acp/...` and `go build ./...`), producing outcome `VERIFIED`.

## Continuous integration

PR #197 runs the repository's `fmt, vet, build, test` job. On run `41517465111`, `gofmt`, `vet`, `build`, and `test` passed; the `documentation links` step failed on relative links in the ACP-007 reports, which is corrected in a follow-up commit (the links now resolve from `docs/reports/`).

The same job now also runs the ACP stdio smoke (`make acp-smoke`, added in PR #205). It passed on run `41558842274` (job `125664254691`, step `ACP stdio smoke`), exercising negotiation, ordering, capability refusal, malformed-frame recovery, a governed start, and a confirmed cancellation against the real stdio endpoint. The recorded Linux result is in [ACP-007-acceptance.md](ACP-007-acceptance.md).

## Notes

- Commit `79acbfc` was also verified in a clean worktree: `go build ./...`, `go vet ./...`, and the `internal/acp`, `internal/archtest`, and `internal/cli` suites passed.
- ACP-007 itself changes documentation only; it adds no production behaviour.
- The ACP package's own behaviour is covered by the adapter tests referenced from [ACP-007-discovery.md](ACP-007-discovery.md).

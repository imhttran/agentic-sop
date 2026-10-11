# AUTONOMY-001 scenario evidence — one-prompt multi-package unused-code cleanup

**Scenario:** a single prompt drives the governed lifecycle to remove unused code from a
**multi-package** Go module, end to end, autonomously, with no new framework.
**Provenance:** operator-executed on a disposable workspace (the governed tool harness is
repo-root confined and cannot run a nested `sop prompt`, mirroring the PREVIEW-002 pattern).

## Workspace (disposable, outside the repo)

`/tmp/sop-auto-multi` — a 3-package module seeded with four genuinely unused symbols:

```text
go.mod                      module example.com/multicleanup
main.go                     package main; uses mathx.Add + strx.Join
mathx/mathx.go              func Add (used); func Sub (unused); const unusedConst (unused)
strx/strx.go                func Join (used); func Repeat (unused); type unusedType (unused)
```

Config: `agent.harness: tool`, `provider: ollama`, `model: deepseek-v4.1-flash:cloud`,
`validation: go build ./... / go test ./... / go vet ./...`, `human.approval_before_commit: true`,
`autonomy.level: high`.

## The one prompt

```sh
sop prompt --capability implement \
  "Remove all unused code from this multi-package Go module: every function, constant, \
variable, and type that is not referenced anywhere in the module. Update imports so the \
module still builds and vets cleanly. Do not change the behavior of the used code."
```

## Observed result (exit 0)

```text
run prompts/prompt-20261009-192346: PASS
  - all required checks passed
fix cycles: 0/2
performance: 19.9s total (agent 17.2s, validation 374ms, review 2.3s) | agent calls 2, validation runs 1, fix cycles 0
report: .agent-sdlc/runs/prompts/prompt-20261009-192346/report.md
human approval required before commit; completed locally without committing.
```

- Stage `PASSED`, gate `PASS`, fix cycles `0/2`.
- Validation: `BUILD go build ./... PASS`, `UNIT_TEST go test ./... PASS`, `LINT go vet ./... PASS`.
- Review (self engine): "…the removed symbols were genuinely unused and no imports became
  dangling (both packages remain imported for the used functions)… No issues found."

## Workspace result (independent re-verification)

- All four unused symbols removed; `grep` for `func Sub|unusedConst|func Repeat|unusedType`
  returns **none remaining**.
- Independent `go build ./...` → OK; `go vet ./...` → OK.
- `git status`: `M mathx/mathx.go`, `M strx/strx.go` (18 deletions) — **not committed**
  (the human commit boundary held).

```diff
 mathx/mathx.go |  6 ------
 strx/strx.go   | 12 ------------
```

## Verdict

**VERIFIED** — the single-prompt governed path performs a realistic multi-package unused-code
cleanup end to end using only existing components (`sop prompt --capability implement` →
`internal/cli` lifecycle → `internal/autonomy` policy → validation/review/quality gate), and
preserves the human commit boundary. No single-command composition is required (AUTONOMY-004
is NOT_REQUIRED; see the baseline report).

# SOP-RUN-ACCEPT-001 — ACC-008 controlled manual intervention

**Status:** Edit applied and verified; awaiting human review (not committed; SOP state unchanged).
**Authorized by:** Tom Tran (operator), 2026-10-11, for ACC-008 only.
**Applied by:** Claude (manual edit outside the SOP lifecycle).

## Scope

`internal/cli/outcome.go` only: keep each executed acceptance check's recorded run
(argv, exit code, bounded output) and pass it to the existing
`outcome.Result.PropagateEvidence` after `outcome.Verify`. No change to command
authorization, security hardening, outcome semantics, report formatting, or the seven
operator acceptance files (`zz_operator_acceptance_*`).

## Rationale

ACC-008 (narrowed to evidence propagation) could not be completed through SOP with
either available tool-capable model, for model/harness compatibility reasons rather
than task difficulty:

| Attempt | Model | Result |
|---|---|---|
| 1 (corrective run) | `deepseek-v4.1-flash:cloud` | Hardening landed; FIX-1 emitted its `write_file` call as text in the summary (never applied); FIX-2 `FIX_NO_PROGRESS` → BLOCKED |
| 2 (continuation) | `nemotron-3-super:cloud` (`--model-class medium`) | CONTINUE: asked what the intended change was; no mutation |
| 3 (continuation) | `nemotron-3-super:cloud` (`--model-class medium`) | Three invalid internal plans (unknown stage kinds `discovery`, `validation`; undeclared capability `go`) → NEEDS_HUMAN; no mutation |

Root cause of the remaining gap: in `deriveOutcome`, each check's `Check` closure
fills a local `rec` from `runAcceptanceCheck`, but `checkEvidence` stores only the
criterion ID and `rec` is discarded (`_ = rec`), so `PropagateEvidence` is never
called and `Argv` / `ExitCode` / `Output` stay empty. Output is already bounded to
400 bytes at capture (`boundOutput`).

## Preserved state

SOP state was not edited. Before the edit, the ACC-008 run artifacts, the pending
`NEEDS_HUMAN` approval (`PENDING`), `sop task` / `sop status` output, and the
`.agent-sdlc/state.db` checksum were copied with a SHA-256 manifest to
`/tmp/sop-acc008-manual-20261011T183454/`.

- `.agent-sdlc/state.db` SHA-256 before: `450c30ef4bfad4709dde7f8b578faf3f2d3ffbb803a4e8ed6d6846d3d395cd3c`
- `internal/cli/outcome.go` SHA-256 before: `35feb4ed90e3a753698bb642f87f150ad60725d06d0e70995200b4e28657a93d`

## Change

Exact delta in `internal/cli/outcome.go:deriveOutcome` (+12/−18; verified to be the only
change by reconstructing the pre-edit file and matching its recorded SHA-256):

- `checkEvidence` holds the criterion ID and a `*checkRun` instead of unused copies of its fields.
- Each authorized check's `Check` closure stores its `runAcceptanceCheck` result in that
  `*checkRun` (replacing the discarded anonymous `rec` and `_ = rec`).
- After `outcome.Verify`, each entry is passed to `res.PropagateEvidence(id, argv, exitCode, output)`
  (replacing `_ = evidence`). Rejected checks never execute and have no entry.

Output stays bounded to 400 bytes by the existing `boundOutput` at capture.

## Verification

| Check | Result |
|---|---|
| ACC-008 operator suite (`go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC008 ./internal/cli/`) | 6/6 PASS, including `EvidenceRecorded` (previously the only failure) |
| Seven operator acceptance-file SHA-256 hashes | all OK (unchanged) |
| ACC-007 / ACC-010 operator suites (regression) | PASS |
| `go build ./...`, `go vet ./...` | OK |
| `go test -count=1 ./...` | 79/79 packages OK |
| gofmt | only `internal/planflow/reconcile_test.go` (pre-existing; owned by ACC-011) |
| Report rendering (production `deriveOutcome` → `writeOutcomeReport`, scratch copy) | executed failing check shows `argv`, `exit: 1`, bounded output; passing check shows `argv`, `exit: 0`; rejected `sh -c` check shows no evidence, outcome HOLD, marker file absent (never executed) |
| `.agent-sdlc/state.db` SHA-256 after | `450c30ef4bfad4709dde7f8b578faf3f2d3ffbb803a4e8ed6d6846d3d395cd3c` (unchanged) |

## Supported SOP completion mechanism (not executed)

`sop task complete ACC-008 --external --commit <sha>` records work performed outside the
SOP lifecycle: model-free, fails closed unless the configured validation passes and the
commit is contained by HEAD; it moves the task to `LOCAL_DONE`, writes
`.agent-sdlc/runs/ACC-008/external-completion.json`, preserves existing run evidence, and
keeps the pending `NEEDS_HUMAN` request in the append-only approval history while
superseding its head (`internal/cli/task_complete_test.go:TestTaskCompleteExternalPreservesRunEvidence`).
It requires a commit that contains this change, so it follows the human-approved commit.

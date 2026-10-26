# HIP-002 — Existing Facilities and Implementation Choices

Status: INSPECTION NOTE. This plan records the observed facilities, the
integration points, and the implementation choices for the Harness Improvement
Pilot. It does not modify any source file.

## 1. Starting baseline

The starting commit and the implementation branch come from
`docs/plans/HIP-001-bootstrap-baseline.md`.

| Item | Value |
| --- | --- |
| Starting commit | `fab5dd1f28d31cd43e478d72f4f5054697ecd31f` |
| Implementation branch | `feat/sop-harness-improvement-pilot` |
| Source | `docs/plans/HIP-001-bootstrap-baseline.md`, sections 1 and 1.1 |

The inspection ran on branch `feat/sop-harness-improvement-pilot`. The note reuses
the HIP-001 baseline as prior evidence. It declares no cross-plan dependency.

## 2. Repository instructions

| File | State |
| --- | --- |
| `CLAUDE.local.md` | PRESENT — local lessons for review order and RAG tests |
| `docs/PLAN.md` | Repository plan guidance |
| `docs/PRD.md` | Product requirements |
| `docs/specs/WORKFLOW.md` | SOP workflow specification |
| `docs/reference/CLI.md` | CLI reference, including `sop run` |
| `docs/architecture/OVERVIEW.md` | Architecture overview and the runner |

## 3. Integration points

| Integration point | File path or command |
| --- | --- |
| Runner (entry) | `cmd/sop` |
| Runner (loop) | `internal/run` |
| Runner (controller) | `internal/controller` |
| Budget and limits | `internal/budget` |
| Run metrics | `internal/runmetrics` |
| Runtrace | `internal/runtrace` |
| Verification-evidence reuse | `internal/verifcache` (used by `sop validate --cache`) |
| Skills | `skills/`, `internal/skill`, `internal/prompt`, `internal/promptcache`, `internal/prompttuning` |
| Advisory memory | `internal/decisionmemory`, `internal/retrieval`, `internal/memindex` |
| Evidence layout | `.sop-evidence/` |
| Worktree isolation | `internal/worktreecleanup`, `docs/plans/SPIKE-001-disposable-environment.md` |
| Acceptance checks | `docs/plans/ACCEPTANCE-CHECKS-AUTHORING.md` |

## 4. Extension pattern for opt-in behavior

Choice: the pilot uses an opt-in script or an existing command flag. It does not
add a new public CLI command.

Reason: the repository root has a `scripts/` directory and a `Makefile`, and
`internal/cli` already hosts the command surface. An opt-in script or flag keeps
the default behavior unchanged. A new public CLI command needs a design reason
first.

Inspected paths that establish the pattern: `internal/cli` (subcommand and flag
registration), `scripts/`, and `Makefile`.

## 5. Advisory-memory provenance verdict

Inspected paths: `internal/decisionmemory/decisionmemory.go`,
`internal/memindex`, `internal/retrieval`.

Verdict: advisory memory does NOT confirm lesson provenance. The inspected
records hold no confirmed source-evidence field, input fingerprint, and outcome
set together. Advisory memory does not support lesson provenance.

Consequence: the pilot keeps its lessons in the pilot report. The pilot does not
write lessons to shared advisory memory.

## 6. No existing pilot implementation or active plan

Confirmation: no existing pilot implementation or active plan covers this work.

Evidence from read-only inspection:

- `git status --short --branch` printed
  `## feat/sop-harness-improvement-pilot...origin/feat/sop-harness-improvement-pilot`
  and showed no tracked file change.
- `git diff` printed no output. No tracked source file changed.
- `git status --porcelain=v1 -uall` listed only untracked entries. These are
  `.sop-evidence/` and untracked plan notes under `docs/plans/`. No tracked source
  file changed.
- `docs/plans/` holds `HIP-001-bootstrap-baseline.md`, which is a bootstrap note
  that modifies no source file. `.sop-evidence/` holds run evidence only.

No active plan implements the pilot. The HIP-001 note is prior evidence, not an
implementation.

## 7. Preserved unrelated working-tree changes

These untracked entries are user-owned. This stage does not modify, revert,
discard, or clean them.

- `.sop-evidence/`
- `docs/plans/HIP-001-bootstrap-baseline.md` (created by this stage; untracked)
- `docs/plans/HIP-002-existing-facilities-and-choices.md` (created by this stage; untracked)
- `docs/plans/PLAN-Executore-Integration.md`
- `docs/history/PROMPT-SOP-Harness-Improvement-Pilot.md`

The stage adds only the two HIP plan notes. Each listed entry was left
unchanged. It records no fabricated run results
and launches no model calls.

## 8. Acceptance mapping

| Criterion | Evidence |
| --- | --- |
| Starting commit and branch stated, citing HIP-001 | Section 1 |
| Each integration point listed with a file path or command | Section 3 |
| Extension pattern named with inspected path | Section 4 |
| Advisory-memory provenance verdict stated | Section 5 |
| No existing pilot or active plan, citing git status and git diff | Section 6 |
| Only the two HIP notes added; untracked entries unchanged | Section 7 |
| No fabricated results or model calls | Section 7 |

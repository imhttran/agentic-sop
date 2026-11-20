# PLAN — Ponytail Complexity Audit Cleanup

**Status:** PROPOSED, not approved.
**Baseline:** `main` at `e28348d3`.
**Rule:** Delete before abstracting. A capability with no production caller is either wired to a real entry point or removed; it is not kept "for later" without an owner and a trigger.

## Project

agentic-sop

## Summary

A repo-wide `ponytail-audit` pass over 1,468 Go files found roughly 7,000 non-test
lines that no `cmd/*` binary executes. The dead weight is concentrated in three
shapes:

1. **Unwired packages.** Eleven packages have no production importer. Ten of
   them are already marked **Not wired** in
   [PACKAGES.md](../reference/PACKAGES.md); the eleventh (`evaltarget`) is
   imported only by build-tagged live tests.
2. **Dead subsystems inside live packages.** The `handoff` compression manager,
   the `agent.Harness` factory, the `shadow` live-evaluation path, and several
   convenience emitters have callers only in tests or in the unwired packages.
3. **Duplicated implementations.** Process-group handling, file locking,
   `clef`/`nimble` SystemOne transport, scanner lifecycle, and several one-line
   helpers are copied rather than shared.

This plan removes that complexity in reviewable slices, with no behavior change
for a wired path. It also adds a guard so the same accumulation does not return
silently.

## Objective

Reduce the production surface to the code a binary can actually reach, keep one
implementation of each shared mechanism, and make "not wired" a failing state
rather than a documentation footnote.

## Evidence

Measured at `e28348d3`:

- `git ls-files '*.go' | wc -l` → 1,468 files; 136,413 non-test lines.
- `go list -deps ./cmd/...` → 102 internal packages reachable from a binary.
- `go mod why -m` on every indirect dependency → all four trace to
  `modernc.org/sqlite`; **no module can be dropped**.

| Area                           | Non-test lines | Why it can go                                                                                                  |
| ------------------------------ | -------------: | -------------------------------------------------------------------------------------------------------------- |
| `internal/evaltarget`          |          2,865 | Imported only by 11 `_live_test.go` files under the `llm02_live` build tag.                                    |
| `internal/completion`          |            254 | **Not wired**; imported only by `e2e/dogfood_test.go`.                                                         |
| `internal/worktreecleanup`     |            244 | **Not wired**; zero importers.                                                                                 |
| `internal/parallel`            |            206 | **Not wired**; imported only by `e2e/dogfood_test.go`.                                                         |
| `internal/skill`               |            179 | **Not wired**; imported only by `internal/dist` tests.                                                         |
| `internal/prompttuning`        |            179 | **Not wired**; zero importers.                                                                                 |
| `internal/ciremediation`       |            123 | **Not wired**; imported only by `e2e/dogfood_test.go`.                                                         |
| `internal/mergegate`           |            121 | **Not wired**; imported only by `e2e/dogfood_test.go`.                                                         |
| `internal/handoff/caveman`     |            108 | **Not wired**; zero importers.                                                                                 |
| `internal/commitgate`          |             62 | **Not wired**; imported only by `e2e/dogfood_test.go`.                                                         |
| `internal/ci`                  |             41 | **Not wired**; zero importers.                                                                                 |
| `handoff` compression cluster  |            313 | `Manager`/`Mode`/`SelectCompressor`/`NoOpCompressor`/`RedactAll` called only by the unwired `completion` loop. |
| `agent.Harness` factory        |           ~215 | `newAgent` returns `agent.Agent`; the interface and `HarnessFromConfig` have no production caller.             |
| `shadow` live path + metrics   |           ~250 | CLI uses only `RunSheet`, `ProduceEvidence`, `MarshalEvidence`.                                                |
| Other dead exports             |           ~600 | ~163 exported symbols with no production caller (comment- or test-only).                                       |
| `clef`/`nimble` duplication    |       ~500 net | Identical SystemOne wire types and 120+ shared 8-line blocks.                                                  |
| `cargoaudit`/`osv` duplication |       ~150 net | 26 shared 8-line blocks of scanner lifecycle/status logic.                                                     |
| Process-group copies           |           ~150 | Four packages duplicate `internal/procgroup`, which 12 others already use.                                     |
| File-lock copies               |            ~80 | Three packages duplicate the same lock helper.                                                                 |
| One-line helpers               |            ~80 | `firstNonEmpty` ×8, `oneLine` ×4, `writeJSON` ×5, `truncate` ×3, `fileExists` ×3.                              |

The audit method is reproducible:

```sh
go list -deps ./cmd/... | grep 'agentic-sop/internal/' | sort -u
git --no-pager ls-files '*.go' | grep -v _test.go | xargs grep -l 'internal/<pkg>'
```

## Non-goals

- No behavior change on a wired path.
- No new abstraction, registry, plugin, or configuration option.
- No change to security policy, approval semantics, redaction, or evidence
  formats.
- No deletion of the AISEC acceptance intent: `evaltarget` may move to a
  test-only tree instead of being deleted.

## Decision required

Before PONY-006 runs, the operator chooses one disposition for the unwired
packages:

- **A — Delete.** Remove the package and its tests. Use this when no roadmap
  item owns it.
- **B — Wire.** Name the binary/command that calls it and add the test that
  proves the call. A package without that proof is not "complete".
- **C — Quarantine.** Move the code under `testdata/` or a `testonly` build tag
  so `go build ./...` and the production import graph exclude it.

The plan recommends **A** for `ci`, `ciremediation`, `commitgate`, `mergegate`,
`completion`, `parallel`, `prompttuning`, `worktreecleanup`, and
`handoff/caveman`; **C** for `evaltarget`; and **B or A** for `skill`,
depending on whether the skill-surface validator is wanted in CI.

## PONY-001 — Delete dead code inside live packages

**Priority:** P1. Remove code with no production caller without touching a wired
path.

### Dependencies

None.

### Deliverables

- `internal/handoff/manager.go`, `mode.go`, `noop.go`, `compressor.go`,
  `redact.go` — remove the compression manager and its helpers. Keep `Record`,
  `Capsule`, `Builder`, `Select`, and the `Store` interface.
- `internal/agent/harness.go`, `command.go`, `provider.go` — remove the
  `Harness` interface, `HarnessFunc`, `CommandHarness`,
  `NewCommandHarness`, `HarnessFromEnv`, `HarnessFromConfig`, and the now-unused
  `FromEnv`/`EffectiveModel` wrappers.
- `internal/decisionadapter/shadow/shadow.go` — remove `Evaluate`, the `Source`
  interface, and the metric accessors (`AgreementRate`, `Disagreements`,
  `InvalidChoiceRate`, `IndeterminateRate`, `TimeoutFailureRate`,
  `LatencyP50`, `LatencyP95`, `LatencyPercentile`, `ConfidenceDistribution`,
  `NimbleAgreement`, `NimbleComparableCases`, `Records`, `Total`). Keep the
  types and `RecordSupplied`/`Evidence` that `ProduceEvidence` uses.
- `internal/cli/lifecycle_notify.go` — remove `NewLifecycleEmitter`,
  `EmitCompleted`, `EmitFailed`, `EmitApproval`, `MemoryEventSink`,
  `NewMemoryEventSink`, and their tests; keep `EmitRequest`, `Emit`, and
  `EventSink`.
- `internal/security/detect.go` — remove the convenience wrappers
  `DetectRepository`, `ClassifyCapabilities`, `SelectCandidates`,
  `AutoAdapterConfigs`, `MissingLockfiles`, `PresentLockfiles`, and
  `MissingObligations`; keep the `...WithDatabases` variants.
- `internal/security/policy.go`, `exclude.go`, `normalize.go`, `exec.go` —
  remove `EvaluatePolicy`, `ExcludedPath`, `StringSet`, and
  `boundedBuffer.Overflow/Len/String` plus the write-only `overflow` field.
- `internal/config/config.go` — remove `Features` and `JevDecisions`.
- `internal/domain/task.go`, `selection.go` — remove `IsReady`, `IsDone`, and
  `IsBlockedSelectable` (or wire the latter into the scheduler in the same
  change).
- `internal/store/repository.go` — remove the unused `TaskRepository` interface.
- `internal/perf/jev_duration.go` — inline `WriteJEVDuration` at its one call
  site.
- Any remaining symbol in the audit's dead-export list that still has no
  production caller after the above.

### Acceptance Criteria

- `rg` finds no production reference to a removed symbol.
- `go build ./...` and `go vet ./...` pass.
- The wired handoff path still records `Record`/`Builder`/`Select` and the
  notification path still emits through `emitTransition*`.
- No behavior test changes its expected output; only tests for removed symbols
  are deleted.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `./scripts/checks/test-fast.sh`
- `rg -n 'SelectCompressor|HarnessFromConfig|AgreementRate|EvaluatePolicy' internal cmd decision` → no production hit

### Failure Behavior

If a removed symbol turns out to have a production caller, restore it and record
the caller in this plan; do not replace it with a new wrapper.

## PONY-002 — Consolidate process-group and file-lock helpers

**Priority:** P1. One implementation of each platform mechanism.

### Dependencies

None.

### Deliverables

- `internal/agent/proc_{unix,other}.go`, `internal/security/proc_{unix,other}.go`,
  `internal/toolharness/proc_{unix,other}.go`, and
  `internal/evaltarget/proc_{unix,other}.go` — delete the local
  `terminateGroup`/`sysProcAttrKill` copies; call `internal/procgroup`.
  `evaltarget`'s `processAlive` may move into `procgroup` if it is still needed
  after PONY-006.
- `internal/question/lock_{unix,windows}.go`,
  `internal/projectlock/lock_{unix,windows}.go`, and
  `internal/verifcache/lock_{unix,windows}.go` — replace the three copies with
  one small lock helper (for example `internal/filelock`) that preserves each
  package's current unsupported-platform behavior.

### Acceptance Criteria

- `internal/procgroup` is the only process-group implementation; a `rg` for
  `Setpgid` outside it finds none.
- The lock helper is the only `syscall.Flock` implementation; each caller keeps
  its current error contract (`ErrLockUnsupported`, no-op, or fail).
- `go build ./...` passes for both `GOOS=darwin` and `GOOS=windows`.

### Acceptance Checks

- `GOOS=darwin go build ./...`
- `GOOS=windows go build ./...`
- `go vet ./...`
- `./scripts/checks/test-fast.sh`

### Failure Behavior

If a platform needs different semantics, add a documented parameter or build
file to the shared helper; do not restore a private copy.

## PONY-003 — Consolidate one-line helpers and use the standard library

**Priority:** P2. Same logic, fewer copies.

### Dependencies

None.

### Deliverables

- One shared helper for each of: `firstNonEmpty`, `oneLine`, `writeJSON`,
  `truncate`, `fileExists`, `readJSON`.
- Replace `maxInt` in `internal/router/router.go` and
  `internal/runmetrics/aggregate.go` with the builtin `max`.
- Replace `hasPrefix` in `internal/commandpolicy` with `slices.HasPrefix`.
- Replace the two `dedupe` helpers with `slices.Compact` (after the caller's
  required sort) or one shared helper.

### Acceptance Criteria

- A `rg` for each helper name finds one definition.
- No call site changes behavior: the trim/no-trim and empty-string semantics of
  the original helper are preserved per caller and covered by an existing or
  added table test.
- `go vet ./...` and the fast suite pass.

### Acceptance Checks

- `go vet ./...`
- `./scripts/checks/test-fast.sh`

### Failure Behavior

If two callers disagree on semantics, keep two clearly named helpers rather than
one ambiguous one.

## PONY-004 — Merge the `clef` and `nimble` SystemOne adapters

**Priority:** P2. Remove the largest duplication in the decision layer.

### Dependencies

PONY-001 (so the dead-symbol set is stable).

### Deliverables

- A shared `internal/decisionadapter/providers/systemone` package containing the
  wire types, transport, normalization, and availability logic currently copied
  in `clef` and `nimble`.
- `clef` and `nimble` become thin configurations over that package, preserving
  their names, defaults, environment variables, selection behavior, and
  `decision.Provider` implementation.
- Delete the duplicate `systemone.go`, `transport.go`, and translation helpers
  from both providers.

### Acceptance Criteria

- The golden/transport/translate tests for both providers pass unchanged.
- `decisionadapter` still reports `nimble`, `julia`, and `clef` as available
  providers.
- `clef`'s selection/availability behavior is unchanged, including the
  `clef.EnabledWhenSelected()` rule.

### Acceptance Checks

- `go test ./internal/decisionadapter/...`
- `go vet ./...`
- `./scripts/checks/test-fast.sh`

### Failure Behavior

If the two providers diverge during the merge, stop and document the difference
instead of forcing a shared abstraction.

## PONY-005 — Share the scanner lifecycle between `cargoaudit` and `osv`

**Priority:** P2.

### Dependencies

None.

### Deliverables

- Extract the shared execution/status/provenance lifecycle from
  `internal/security/cargoaudit.go` and `internal/security/osv.go` into one
  unexported helper in `internal/security`.
- Keep each scanner's command construction, parsing, and finding shape separate.

### Acceptance Criteria

- Both scanners still produce the same findings and status for the existing
  test fixtures.
- The real-scanner integration script still passes its offline fixture.

### Acceptance Checks

- `go test ./internal/security/...`
- `bash scripts/checks/security-integration.sh`
- `./scripts/checks/test-fast.sh`

### Failure Behavior

If the shared lifecycle cannot express one scanner without flags that only one
uses, keep the scanners separate and record the reason.

## PONY-006 — Resolve the unwired packages

**Priority:** P1 after the decision in "Decision required".

### Dependencies

The operator decision above.

### Deliverables

- For each unwired package, the chosen disposition:
  - **Delete:** remove the package, its tests, and its row from
    [PACKAGES.md](../reference/PACKAGES.md).
  - **Wire:** add the production call and the acceptance test that proves it;
    keep the row and remove the **Not wired** mark.
  - **Quarantine:** move the code under `testdata/` or a `testonly` build tag and
    mark it test-only in PACKAGES.md.
- Update `docs/reference/PACKAGES.md` so no stale **Not wired** row remains.
- If `evaltarget` is quarantined, the live tests keep their build tag and import
  the moved package.

### Acceptance Criteria

- `go list -deps ./cmd/...` accounts for every package that remains under
  `internal/`.
- No package row says **Not wired** unless the operator explicitly chose to keep
  it with a named owner and trigger.
- Deleting a package removes its tests and any now-unused fixture files.

### Acceptance Checks

- `go build ./...`
- `go vet ./...`
- `./scripts/checks/test-fast.sh`
- `bash scripts/checks/check-doc-links.sh`

### Failure Behavior

If a package has a caller discovered during the change, stop and reclassify it
as wired; do not delete a reachable path.

## PONY-007 — Document and guard the reduction

**Priority:** P2.

### Dependencies

PONY-001 through PONY-006.

### Deliverables

- `docs/reference/PACKAGES.md` — regenerate the inventory from the package doc
  comments and the import graph; remove the **Not wired** explanation if no row
  uses it.
- Add an architecture test (following `internal/archtest`) that fails when a
  package under `internal/` has no non-test importer and is not allow-listed as
  test-only. The allow-list must name each exception and its reason.
- Update this plan's status to **IMPLEMENTED** with the merged PR numbers.

### Acceptance Criteria

- The guard test fails on a newly added unreachable package and passes on the
  intended test-only packages.
- The package map matches the tree.
- Documentation links resolve.

### Acceptance Checks

- `go test ./internal/archtest/...`
- `bash scripts/checks/check-doc-links.sh`
- `./scripts/checks/test-fast.sh`

### Failure Behavior

If the guard cannot distinguish a deliberate test-only package from dead code,
require an explicit allow-list entry rather than weakening the check.

## Rollout

1. PONY-001, PONY-002, PONY-003 can land independently and in parallel.
2. PONY-004 and PONY-005 land after PONY-001 to keep the dead-symbol set stable.
3. PONY-006 lands only after the operator decision.
4. PONY-007 lands last and turns the result into a guard.

## Expected net

Approximately **-7,000 non-test lines** and **-0 modules**. The exact number is
recorded per task as each lands.

## Out of scope

Correctness bugs, security findings, and performance work surfaced by the same
audit. They belong in a normal review, not this cleanup.

# CONV-005 — Regression and Safety Verification

Verification stage for the convergence enforcement change in `internal/ollamaagent`.
All commands were run from the repository root against the current working tree
(uncommitted, user-owned changes preserved; nothing was reverted or cleaned).

## 1. Scope of the change under verification

The convergence enforcement is the pre-mutation guard in the phased IMPLEMENT/FIX
engine. Inspected implementation files:

- `internal/ollamaagent/orchestrate.go` — `executePhased` applies the
  pre-mutation convergence enforcement before dispatching a controlled tool:
  once `st.mutationConvergenceRequired(iteration)` holds and the request is a
  `nonMutatingRepositoryTool`, the call is `RecordDenied` and answered with
  `implementConvergenceCorrection`; the run then falls through to the unchanged
  no-progress termination.
- `internal/ollamaagent/state.go` — `executionState.mutationConvergenceRequired`,
  `nonMutatingRepositoryTool`, `recordInspected`, `inspectedSummary`,
  `maxCheckpointFiles` / `maxCheckpointShown`, `stalled`.
- `internal/ollamaagent/convergence_enforcement_test.go` — the discriminating
  IMPLEMENT/FIX regression.
- `internal/ollamaagent/convergence_regression_test.go` — the credited-discovery
  bound and determinism regressions.
- `internal/ollamaagent/checkpoint_continuation_test.go` — continuation
  checkpoint / retry lifecycle regressions.

## 2. Full repository gates (required validations)

| Command | Result |
| --- | --- |
| `go build ./...` | exit 0 (pass) |
| `go test ./...` | exit 0 (pass); every listed package `ok` |
| `go vet ./...` | exit 0 (pass) |

`go test ./...` reported `ok` for all packages including
`internal/ollamaagent`, `internal/approval`, `internal/continuation`,
`internal/e2e/lifecycle`, `internal/completion`, `internal/review`,
`internal/provider`, and `internal/router`.

## 3. IMPLEMENT and FIX convergence regressions (CONV-005-S1)

Focused command:

```
go test ./internal/ollamaagent/ -run TestConvergence -v
```

Discovered test identifiers (package `github.com/imhttran/agentic-sop/internal/ollamaagent`):

- `TestConvergenceEnforcementFiresBeforeStaleTermination` — subtests `implement`, `fix`
- `TestConvergenceImplementCreditedDiscoveryStopsAtNoProgressBound`
- `TestConvergenceImplementIsDeterministic` — subtests `run=0..2`
- `TestConvergenceFixCreditedDiscoveryStopsAtNoProgressBound`
- `TestConvergenceFixIsDeterministic` — subtests `run=0..2`

Result: `PASS` (`ok ... internal/ollamaagent 0.353s`), exit 0.

`TestConvergenceEnforcementFiresBeforeStaleTermination` is the discriminating
regression: it asserts at least one non-mutating repository tool request is
**denied** before the run terminates (the inert implementation records zero
denials), while `*noProgressError` still bounds the run at
`implementNowAfter + maxNoProgressIterations` model turns.

## 4. Safety and lifecycle checks

### 4.1 Retry semantics: retryable incomplete disposition, never silently reset

- **Evidence (test):** `checkpoint_continuation_test.go` —
  `TestUnmutatedRunCarriesContinuationCheckpoint` requires `*noProgressError`
  with the `IMPLEMENT_NO_PROGRESS` marker, `no repository progress`, and
  `termination=no_progress`; `TestUnmutatedFixIsNoProgressNotFinalizationLimit`
  requires `FIX_NO_PROGRESS` and asserts the diagnostic is *not*
  `finalization_limit` or `iteration_limit`.
- **Evidence (code):** `orchestrate.go` `implementNoProgressError` retains the
  `"a retry may succeed"` marker (the retryable-incomplete disposition) and the
  `termination=no_progress` suffix. Nothing in the enforcement change clears the
  stale counter or resets a retry: `stalled` is unchanged and only
  `observeMutation`/`observeDiscovery` reset the streak.
- **Conclusion:** a non-converging run still yields the retryable incomplete
  (`no_progress`) disposition; retries are never silently reset.

### 4.2 Continuation/checkpoint behavior: still produced and still bounded

- **Evidence (test):** `TestCheckpointIsBoundedAndDeduplicated` pins first-seen
  order, deduplication, blank rejection, and the `maxCheckpointFiles` bound;
  `TestUnmutatedRunCarriesContinuationCheckpoint` requires the emitted
  `continuation checkpoint (… inspected=pkg/f0.go)` in the diagnostic.
- **Evidence (code):** `state.go` `recordInspected` dedupes and stops at
  `maxCheckpointFiles` (12); `inspectedSummary` shows at most `maxCheckpointShown`
  (5) plus a `(+N more)` count; `orchestrate.go` appends the checkpoint to both
  `implementNoProgressError` and `noChangeError`. Paths only — no contents or
  secrets.
- **Conclusion:** the inspected-path checkpoint is still produced and still
  bounded.

### 4.3 Approval boundaries, fail-closed behavior, lifecycle transitions

- **Evidence (test):** full-suite `go test ./...` passes, including
  `internal/approval`, `internal/commitgate`, `internal/mergegate`,
  `internal/e2e/lifecycle`, and `internal/completion`; the phased lifecycle
  regressions (`TestConvergence*`, `TestUnmutatedRunStopsBeforeLateMutation`)
  pass unchanged.
- **Evidence (code):** the enforcement only adds a `RecordDenied` path for
  non-mutating repository tools; it introduces no new approval bypass, no new
  human-gate, and does not change any `termination*` constant. FINALIZE remains
  terminal (all tool requests denied) and the fail-closed default
  (`!policy.Allows(name)` → deny) is untouched in `orchestrate.go`.
- **Conclusion:** approval boundaries, fail-closed behavior, and lifecycle
  transitions are unchanged.

### 4.4 Provider neutrality

- **Evidence (diff inspection):** `convergence_enforcement_test.go` and the
  enforcement code use the in-process `httptest` fake (`newFakeOllama`), the
  `ollamaagent` package's own request/policy helpers, and `testConfig`. No new
  provider, model, or domain name and no provider-specific branch is added by the
  enforcement change. `internal/provider/*` packages are unchanged by this work.
- **Conclusion:** provider neutrality holds.

### 4.5 A run that mutates does not regress

- **Evidence (test):** the full `internal/ollamaagent` suite passes, covering the
  mutating path (`mutation_verification_test.go`, `mutation_observed` handling,
  CHANGE/FORCE-FINALIZE lifecycle). `mutationConvergenceRequired` returns `false`
  once `st.mutationObserved` is set, so a mutating run is never denied by the new
  enforcement.
- **Conclusion:** a run that mutates is not regressed.

## 5. Uncovered checks

No acceptance criterion was left without evidence. Two behaviors are covered by
focused tests rather than the required commands alone and are recorded here so
they are not over-asserted:

- The pre-mutation *denial* discriminator is covered by
  `TestConvergenceEnforcementFiresBeforeStaleTermination`; the full `go test ./...`
  run exercises it as part of the package but does not isolate it.
- Provider neutrality is established by diff inspection (no machine-checked
  guard beyond `internal/archtest`), not by a dedicated new test.

## 6. Result

All required gates (`go build ./...`, `go test ./...`, `go vet ./...`) pass, the
IMPLEMENT and FIX convergence regressions pass, and every safety/lifecycle
criterion is satisfied with the test and code evidence above.

# PREJEV012 — Full Agent Harness Regression Suite (Decomposition)

This document restructures the umbrella milestone **PREJEV012** into focused,
independently executable regression tasks. It is the updated PREJEV012 plan/task
decomposition; the umbrella stays, and S6–S12 become tasks that can each be
understood, implemented, validated, and completed on their own.

## Status

- **PREJEV012 remains the umbrella regression milestone.** It completes only when S6–S12 have all been validated (see [Umbrella completion criteria](#umbrella-completion-criteria)).
- **The active plan source is left unchanged.** `docs/PLAN-Pre-JEV-Stabilization.md`
  and `.agent-sdlc/plan.json` are not edited, so the existing task graph's
  provenance is preserved and the next `sop run` does not stop with
  `NEEDS_HUMAN: plan changed`. PREJEV012 continues to exist as the umbrella;
  S6–S12 execute as independent task files.
- **S6–S12 are standalone task files** under `docs/tasks/prejev012/`, each runnable
  in isolation:

  ```bash
  sop run --task docs/tasks/prejev012/PREJEV012-S6.md
  sop run --task docs/tasks/prejev012/PREJEV012-S7.md
  sop run --task docs/tasks/prejev012/PREJEV012-S8.md
  sop run --task docs/tasks/prejev012/PREJEV012-S9.md
  sop run --task docs/tasks/prejev012/PREJEV012-S10.md
  sop run --task docs/tasks/prejev012/PREJEV012-S11.md
  sop run --task docs/tasks/prejev012/PREJEV012-S12.md
  ```

## Why this decomposition exists

The previous PREJEV012 IMPLEMENT invocation was too broad. It ran repository
discovery across several unrelated areas and then exited `NEEDS_HUMAN` before
making any repository change:

- `.agent-sdlc/runs/PREJEV012/report.json` → `"decision": "NEEDS_HUMAN"`,
  `validation_runs: 0`, `agent_calls: 2`.
- `.agent-sdlc/runs/PREJEV012/attempt.txt` → discovery completed but
  "no repository files were created or modified".

The fix is **not** a larger iteration budget. It is to make each regression area a
small, single-responsibility task with its own acceptance criteria, its own
validation commands, and its own resumable run directory. No agent budget was
increased.

## Design rules applied

Each task S6–S12 satisfies the decomposition requirements:

1. **One responsibility** — a single regression area (see each task's _Objective_).
2. **One subsystem** — the relevant package(s) are named explicitly.
3. **Concrete acceptance criteria** — checkable statements, not prose.
4. **Explicit validation commands** — focused package tests first, broader
   regression checks after (see each task's _Requirements_ and this document's
   [validation matrix](#validation-matrix)).
5. **No unrelated discovery** — scope is limited to the named subsystem.
6. **Independently resumable and idempotent** — a task runs via `sop run --task`
   and writes to its own `.agent-sdlc/runs/<ID>/`; re-running is safe.
7. **No manufactured changes** — a requirement already covered by existing tests is
   _recorded_, not rewritten. Covered requirements complete with no change
   (`changes_expected=false`).
8. **No duplicated orchestration** — tasks are task files; the SOP lifecycle
   (`PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`) is reused unchanged.

### Execution mode and per-task loop

All of S6–S12 run in the ordinary `implement` mode. Each one follows the same
three-step loop:

1. **Verify existing coverage first** — inspect the tests that already exist for
   the area before changing anything.
2. **Implement only the missing coverage** — add a test only where a real gap is
   demonstrated; otherwise change nothing.
3. **Validate independently** — SOP's own VALIDATE stage runs the deterministic
   validation; the agent's self-report is not the validation.

`implement` (rather than `verify-first`) is deliberate: `verify-first` runs the
deterministic validation _first_ and, when it passes, short-circuits with no agent
at all — so it could never add the missing coverage that step 2 requires. A task
with no real gap still completes with no change (`changes_expected=false`), and a
requirement already proven by existing tests is recorded, not rewritten.

## Existing coverage map

The suite already exists (largely from PREJEV007–PREJEV011). The table maps each
regression area to the tests that already cover it.

| Area                                                   | Subsystem                                                                | Existing tests                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | Status  |
| ------------------------------------------------------ | ------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------- |
| Scaffold / fake harness                                | `internal/e2e/harness`                                                   | `TestFakeProviderIsDeterministic`, `TestFakeProviderError`, `TestCapabilitiesRestriction`, `TestClockIsExplicit`, `TestIDsAreStable`, `TestAdapterContract`, `TestRepoCleanAndDirty`, `TestRepoUntrackedIsDirty`                                                                                                                                                                                                                                                                                                                                       | covered |
| PLAN early completion / forced synthesis               | `internal/e2e/lifecycle`, `internal/ollamaagent`                         | `TestPlanEarlyCompletion`, `TestPlanForcedSynthesis`, `TestPlanInvalidForcedSynthesisFails`, `TestPlanAgentErrorIsSurfaced`, `TestPlanEarlyFinalCompletesImmediately`, `TestPlanForcedSynthesisAfterDiscoveryLimit`, `TestPlanSynthesisExhaustion`, `TestPlanCannotMutateRepository`                                                                                                                                                                                                                                                                   | covered |
| IMPLEMENT mutation / no-mutation / forced finalization | `internal/e2e/lifecycle`, `internal/ollamaagent`                         | `TestImplementEarlyMutation`, `TestImplementNoMutationTransition`, `TestImplementForcedFinalization`, `TestImplementMutationTransitionsToChange`, `TestImplementEarlyMutationBeforeThresholdSucceeds`, `TestImplementWithoutMutationStopsAtHardCeiling`, `TestImplementFinalizationExhaustionAfterMutation`, `TestImplementWritingPastThresholdIsNotFinalized`                                                                                                                                                                                         | covered |
| IMPLEMENT invocation isolation                         | `internal/e2e/lifecycle`, `internal/ollamaagent`                         | `TestImplementInvocationIsolation`, `TestImplementIsolationAcrossProviders`, `TestMutationEvidenceIsInvocationScoped`                                                                                                                                                                                                                                                                                                                                                                                                                                  | covered |
| REVIEW inspect → synthesize / tool denial              | `internal/ollamaagent`, `internal/e2e/lifecycle`                         | `TestReviewEarlyFinalCompletesImmediately`, `TestReviewForcedSynthesisAfterDiscoveryLimit`, `TestReviewInspectNudgeAtSixthCall`, `TestReviewSynthesisExhaustion`, `TestReviewCannotMutateRepository`, `TestReviewSynthesisToolDenied`, `TestReviewSynthesisUnknownToolDenied`, `TestReviewSynthesisDenialRecorded`, `TestSynthesisOutcomeNotSilentlySucceeded`, `TestReviewCapabilityGuard`                                                                                                                                                            | covered |
| **S6** Repository-change reconciliation                | `internal/ollamaagent`, `internal/toolharness`                           | `TestObservedRepositoryChangeMutationEvidenceWins`, `TestObservedRepositoryChangeNoEvidenceNoBaselineUnknown`, `TestChangedSinceReportsBaselineDivergence`, `TestChangedSinceIgnoresRestoredFile`, `TestReconcileOutcomeOverrideBothDirections`, `TestReconcileOutcomeFingerprintFailureKeepsModelClaim`, `TestReconcileOutcomeGroundsChangesExpected`, `TestReconcileOutcomeHandlesProseAndDoesNotChange`, `TestOutcomeWireShapeIsSOPCompatible`, `TestEnsureStructuredOutcome*`, `TestWorkingTreeChanged`, `TestRestoreFileRecoversCommittedContent` | covered |
| **S7** Pre-dirty working tree                          | `internal/ollamaagent`, `internal/e2e/harness`, `internal/e2e/lifecycle` | `TestPreDirtyNoInvocationChangeIsNotAttributed`, `TestPreDirtyWithInvocationChangeIsAttributed`, `TestPreDirtyStagedChangeNotAttributed`, `TestPreDirtyContentPreservedAcrossInvocation`, `TestPreDirtyRepoAcrossCapabilities`, `TestRepoCleanAndDirty`, `TestRepoUntrackedIsDirty`, `TestImplementInvocationIsolation`                                                                                                                                                                                                                                | covered |
| **S8** Provider / model selection                      | `internal/agent`, `internal/config`                                      | `TestFromEnvSelection`, `TestFromConfigProvider`, `TestEffectiveProvider`, `TestEffectiveModel`, `TestEffectiveHarness`, `TestFromConfigModel`, `TestNewOllamaValidation`, `TestNewLlamaCppValidation`, `TestNewOllamaFromEnvRequiresModel`, `TestNewOllamaFromEnvUsesConfiguredModel`, `TestConfigDefaultsToDeepSeek`, `TestConfigModelEnvOverride`                                                                                                                                                                                                   | covered |
| **S9** Command adapter compatibility                   | `internal/agent`, `internal/e2e/harness`                                 | `TestCommandAgentReturnsStdout`, `TestCommandAgentSerializesCapability`, `TestCommandAgentAcceptsAllCapabilities`, `TestCommandAgentRejectsUnknownCapabilityWithoutInvokingHarness`, `TestCommandAgentFailureIncludesDiagnostics`, `TestCommandAgentReturnsOutcome`, `TestCommandAgentProseHasNoOutcome`, `TestCommandHarnessSatisfiesHarnessInterface`, `TestCommandHarnessExecuteDelegates`, `TestCommandHarnessExecutePreservesOutcome`, `TestHarnessFromConfigLegacyCommand`, `TestAdapterContract`                                                | covered |
| **S10** Bootstrap / known-good binary                  | `internal/agentbin`, `scripts/*.sh`, `internal/ollamaagent`              | `TestResolvePrefersOverride`, `TestResolveUsesHomeDirectory`, `TestResolveMissingIsAnActionableError`, `TestResolveSkipsDirectories`, `TestResolveSkipsNonExecutable`, `TestResolveFallsBackPastNonExecutableOverride`, `TestCandidatesOrder`, `TestDefaultHomeIsOutsideProjectTree`, `TestBinaryPathIsAbsolute`, `TestBootstrapWrapperNeverSelfBuilds`                                                                                                                                                                                                | covered |
| **S11** Determinism / race hardening                   | `internal/e2e/harness`, `internal/e2e/lifecycle`, `internal/ollamaagent` | `TestFakeProviderIsDeterministic`, `TestClockIsExplicit`, `TestIDsAreStable`, `TestFakeProviderConcurrentUseIsRaceFree`, the `*Isolation*` tests, plus the suite run under `-race`                                                                                                                                                                                                                                                                                                                                                                     | covered |
| **S12** Coverage closure                               | all of the above                                                         | the full suite                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         | to do   |

## Coverage gaps (confirmed and closed)

These were the only acceptance criteria without an explicit test. Each was
confirmed against the existing suite and then closed with a single focused test
(all pass, including under `go test -race`). S12 only re-runs them; it adds
nothing more unless a new gap appears.

| ID  | Area | Gap                                                                                                                         | Proving test                                                                              |
| --- | ---- | --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| G1  | S6   | `ChangedSince` after a change is **reverted to baseline** ("restored file") was not asserted; only unchanged → changed was. | `TestChangedSinceIgnoresRestoredFile` (`internal/ollamaagent/reconcile_test.go`)          |
| G2  | S7   | Pre-existing modified file **content is preserved** across a completed invocation was not asserted (only attribution was).  | `TestPreDirtyContentPreservedAcrossInvocation` (`internal/ollamaagent/reconcile_test.go`) |
| G3  | S7   | The pre-dirty fixture was not exercised across PLAN, IMPLEMENT, and REVIEW in the e2e lifecycle suite.                      | `TestPreDirtyRepoAcrossCapabilities` (`internal/e2e/lifecycle/pre_dirty_test.go`)         |
| G4  | S10  | No deterministic test that the bootstrap wrapper does **not** compile candidate source.                                     | `TestBootstrapWrapperNeverSelfBuilds` (`internal/agentbin/wrapper_test.go`)               |
| G5  | S11  | No explicit concurrent shared-state test for the fake harness under `-race`.                                                | `TestFakeProviderConcurrentUseIsRaceFree` (`internal/e2e/harness/harness_test.go`)        |

## Task definitions

### PREJEV012-S6 — Repository Change Reconciliation

- **Responsibility:** verify repository-change detection and reconciliation —
  changed, unchanged, and restored files — and that observed reality overrides the
  model's claim.
- **Subsystem:** `internal/ollamaagent` (reconcile/outcome), `internal/toolharness`
  (`WorkingTreeFingerprint`/`ChangedSince`).
- **Dependencies:** none (the S0 scaffold exists).
- **Execution:** `implement`.
- **Acceptance criteria:** see `docs/tasks/prejev012/PREJEV012-S6.md`.
- **Focused validation:**
  `go test ./internal/ollamaagent/ -run 'Reconcile|ObservedRepositoryChange|ChangedSince|MutationEvidence|PreDirty|OutcomeWire'`
  then `go test ./internal/ollamaagent/... ./internal/toolharness/...`.

### PREJEV012-S7 — Pre-Dirty Working Tree

- **Responsibility:** verify that pre-existing user changes are preserved and are
  never falsely attributed to the agent.
- **Subsystem:** `internal/ollamaagent` (pre-dirty reconciliation),
  `internal/e2e/harness` + `internal/e2e/lifecycle` (`Repo.Dirty`/`Untracked`).
- **Dependencies:** S6.
- **Execution:** `implement`.
- **Focused validation:** `go test ./internal/ollamaagent/ -run 'PreDirty'` then
  `go test ./internal/e2e/... -run 'Repo|Isolation'`.

### PREJEV012-S8 — Provider and Model Selection

- **Responsibility:** verify provider/model configuration, defaults, overrides,
  precedence, and Ollama selection — and that invalid selections fail clearly.
- **Subsystem:** `internal/agent` (`provider.go`), `internal/config`.
- **Dependencies:** none.
- **Execution:** `implement`.
- **Focused validation:**
  `go test ./internal/agent/ -run 'Provider|Model|FromEnv|FromConfig|Effective|Ollama|LlamaCpp'`
  then `go test ./internal/agent/... ./internal/config/...`.

### PREJEV012-S9 — Command Adapter Compatibility

- **Responsibility:** verify the command-based adapter remains compatible with the
  stabilized Agent Harness contract (JSON request on stdin, raw response on
  stdout, structured-outcome passthrough, diagnostic failures).
- **Subsystem:** `internal/agent` (`command.go`, `harness.go`),
  `internal/e2e/harness` (`Adapter`).
- **Dependencies:** none.
- **Execution:** `implement`.
- **Focused validation:** `go test ./internal/agent/ -run 'Command'` then
  `go test ./internal/e2e/harness/ -run 'Adapter'`.

### PREJEV012-S10 — Bootstrap Behavior

- **Responsibility:** verify the known-good `sop-ollama-agent` bootstrap/install
  path and that runtime execution does not depend on self-building candidate
  source.
- **Subsystem:** `internal/agentbin`, `scripts/install-sop-ollama-agent.sh`,
  `scripts/sop-ollama-agent.sh`, `internal/ollamaagent/version.go`.
- **Dependencies:** S8, S9.
- **Execution:** `implement`.
- **Focused validation:** `go test ./internal/agentbin/...` then
  `go vet ./... && go build ./...`.

### PREJEV012-S11 — Determinism and Race Hardening

- **Responsibility:** verify invocation isolation, deterministic lifecycle
  behavior, and race-sensitive shared state.
- **Subsystem:** `internal/e2e/harness`, `internal/e2e/lifecycle`,
  `internal/ollamaagent`.
- **Dependencies:** S6, S7, S8, S9, S10.
- **Execution:** `implement`.
- **Focused validation:** `go test -race ./internal/e2e/...` and
  `go test -race ./internal/ollamaagent/...`.

### PREJEV012-S12 — Coverage Closure

- **Responsibility:** run the complete Agent Harness regression suite, identify
  remaining uncovered acceptance criteria, and add only genuinely missing
  coverage (G1–G5, if confirmed).
- **Subsystem:** the whole suite (`internal/e2e/...`, `internal/ollamaagent/...`,
  `internal/agent/...`, `internal/agentbin/...`).
- **Dependencies:** S6, S7, S8, S9, S10, S11.
- **Execution:** `implement` (may complete with `changes_expected=false` when no
  gap is confirmed).
- **Focused validation:** `go test ./internal/ollamaagent/...`,
  `go test ./internal/e2e/...`, then the full validation matrix below.

## Validation matrix

Focused commands per task are above; the umbrella gate is:

```bash
go test ./internal/ollamaagent/...
go test ./internal/e2e/...
go test ./...
go vet ./...
go build ./...
```

Concurrency-sensitive code additionally requires:

```bash
go test -race ./internal/ollamaagent/...
go test -race ./internal/e2e/...
```

## Umbrella completion criteria

PREJEV012 is complete only when **all** of the following hold:

1. Every task PREJEV012-S6 … PREJEV012-S12 has run to a passing result
   (`Passed` / `LOCAL_DONE`) via `sop run --task …`, having verified existing
   coverage, added only real gaps, and validated independently.
2. Each of G1–G5 is covered by its proving test (listed above) and passes, including
   under `go test -race`; S12 re-runs them and adds nothing more unless a new gap
   appears.
3. The validation matrix above passes, including the `-race` runs.
4. A final coverage map marks every PRD coverage bullet (below) as covered by at
   least one passing regression test.
5. No working implementation was rewritten merely to force a mutation.

### PRD coverage bullets → task

| PRD coverage bullet                                                   | Task             |
| --------------------------------------------------------------------- | ---------------- |
| PLAN early completion and forced synthesis                            | S1–S5 (existing) |
| IMPLEMENT early mutation, no-mutation transition, forced finalization | S1–S5 (existing) |
| IMPLEMENT invocation isolation                                        | S1–S5, S11       |
| REVIEW early completion and forced synthesis                          | S1–S5 (existing) |
| REVIEW synthesis tool denial                                          | S1–S5 (existing) |
| structured outcome reconciliation                                     | S6               |
| pre-dirty repository behavior                                         | S7               |
| provider/model selection                                              | S8               |
| command adapter compatibility                                         | S9               |
| bootstrap binary behavior                                             | S10              |
| regression coverage closure                                           | S12              |

## Workflow authority (unchanged)

This decomposition does not change SOP's workflow authority or lifecycle:

- Lifecycle remains `PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`.
- Human gates are unchanged; nothing is committed, pushed, or merged
  automatically.
- No destructive reset/cleanup; `.agent-sdlc/state.db` is never hand-edited.
- No regression task re-implements orchestration; each is a task file driven by
  the existing lifecycle.
- No CI/PR/merge result is fabricated.

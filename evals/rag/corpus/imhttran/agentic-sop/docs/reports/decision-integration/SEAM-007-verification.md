# SEAM-007 — Full Verification and Architecture Review

Status: **COMPLETE (review + verification only; no production change).**
Scope: `agentic-sop` only.
Authority: `docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md` §SEAM-007.

This record is the verification and architecture-review evidence for the cumulative
provider-neutral decision integration seam (SEAM-001 … SEAM-006). It exists so
SEAM-008 can make the final readiness decision without repeating the investigation.

---

## 1. Repository truth

| Fact | Value |
|---|---|
| Branch | `main` |
| HEAD | `b743cfa93c057eb1499ce72332d6481edb4643a6` |
| Upstream | `origin/main` (in sync; neither ahead nor behind) |
| Commit made by SEAM work | **none** (the entire seam is uncommitted working-tree state) |

`git status --short`:

```
 M internal/cli/cli.go
 M internal/cli/run.go
 M internal/config/config.go
 M internal/decision/decision.go
?? docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md
?? docs/reports/decision-integration/
?? internal/archtest/decision_seam_test.go
?? internal/cli/decision_evidence.go
?? internal/cli/decision_evidence_crossmodule_test.go
?? internal/cli/decision_evidence_hardening_test.go
?? internal/cli/decision_evidence_scheduled_test.go
?? internal/cli/decision_evidence_test.go
?? internal/cli/decision_provider.go
?? internal/cli/decision_provider_test.go
?? internal/config/decision_test.go
?? internal/decision/command/
?? internal/decision/hardening_test.go
?? internal/decision/validator_test.go
?? testdata/
```

`git diff --stat` (tracked files only; most seam files are untracked):

```
 internal/cli/cli.go           |  15 +++-
 internal/cli/run.go           |   8 ++
 internal/config/config.go     |  14 ++-
 internal/decision/decision.go | 163 +++++++++++++++++++++++++++++++++--
 4 files changed, 186 insertions(+), 14 deletions(-)
```

Every changed/added path above is seam work. There are no unrelated or
user-owned changes mixed in (the earlier SOP-PROMPT-REPORT-001 and Clef-readiness
work is already committed at `b743cfa`).

---

## 2. Final implemented architecture (traced from source)

```
External Decision Provider            (sop-decision-adapters; any module/process)
        │  provider-neutral JSON request  (stdin)
        ▼
internal/decision/command.Adapter.Decide           command.go:108
        │  provider-neutral JSON result  (stdout)
        ▼
SOP-owned validation:  command.go:171  req.ValidateResult(res)
                       decision.go:255 Request.ValidateResult
                       decision.go:136 ValidConfidence / decision.go:33 Choice.Known
                       cli/decision_evidence.go:147 decision.Validate
        ▼
SOP-owned translation: cli/decision_evidence.go:194 decisionEvidenceAdverse (HIGH|HUMAN)
                       cli/decision_evidence.go:203 decisionEvidenceClassification
                       → failure.Classification{Kind:BLOCKING_FINDINGS, Disposition:NEEDS_HUMAN}
        ▼
Single policy path:    internal/autonomy.Decide      autonomy.go:162
        ├── Continue / AUTO_*                        (provider cannot authorize)
        ├── Block / TERMINAL                         (provider cannot weaken)
        └── Human Boundary                           (unchanged)
        ▼
SOP-owned lifecycle/approval enforcement:
   cli/approval.go:400 humanBoundary → :410 recordHumanApprovalRequest → internal/approval
   cli/drive.go:770  policyForcesHuman → recoverTask(..., NEEDS_HUMAN, ...)
   cli/run.go:219    runSingleTask human gate message
```

Call site: `cli/run.go:301` (`executeLifecycle`), one optional call
`res = applyDecisionEvidence(ctx, cfg, d, spec, res)` before the classification
artifact/report are written, so the persisted evidence reflects the escalation.

Composition root: `cli/decision_provider.go:21 decisionProviderFromConfig` (returns
`(nil, nil)` when `decision.enabled` is false), wired through `deps.newDecisionProvider`
(`cli/cli.go:69`).

There is exactly **one** policy interpretation path: `autonomy.Decide`. The seam adds
no autonomy entry point and no second policy engine.

---

## 3. Governance verification — providers evaluate, SOP governs

Verified from source and tests. A provider result is data until SOP validates and
interprets it; a provider has **no** mechanical surface for any of:

| Provider cannot… | Why (structural) | Test evidence |
|---|---|---|
| transition task/plan state | the provider is a separate process returning JSON; only `internal/cli`/`runpkg` transition state | `TestDecisionEvidenceDoesNotTouchLifecycleState`; cross-module lattice |
| approve/reject work | approval is created only by `recordHumanApprovalRequest`; the seam never calls it | `TestRunApprovalBoundaryUnaffectedByDecisionEvidence` |
| create/satisfy approvals | the seam returns a `lifeResult`; it holds no approval handle | `TestScheduledTaskDecisionEvidenceEscalates` (SOP records the approval) |
| authorize execution/commit/merge | commit/merge are `commitgate`/`mergegate`; the seam touches neither | `TestDecisionEvidenceUsesSinglePolicyPath`; diff has no commit/merge path |
| bypass validation/review | validation/review gates run before policy; the seam runs after and only modifies a decision value | `TestDecisionCarriesNoLifecycleAuthority` (structurally) |
| bypass human approval | `attentionable` is false for a human/terminal baseline; escalation only ever *adds* a boundary | `TestDecisionEvidenceGovernedBaselinePreserved` |
| select lifecycle transitions | the only action vocabulary comes from `autonomy.Decide` | `TestDecisionEvidenceUsesSinglePolicyPath` |

`provider output = data`, never a command: `TestDecisionEvidenceProviderTextIsData`,
`TestDecisionEvidenceAdversarialOutputHasNoAuthority`,
`TestDecideDoesNotExecuteProviderOutput`.

---

## 4. Monotonic-authority verification

Invariant: `governed(baseline + evidence)` is never more permissive than
`governed(baseline)`.

Enforced structurally by `attentionable` (`decision_evidence.go:107`): only
`AUTO_RETRY|AUTO_CONTINUE|AUTO_FIX|AUTO_RECONCILE` baselines may be escalated; the
escalation target is always `autonomy.Decide`'s human boundary. Tests:

| Claim | Test | Result |
|---|---|---|
| Block + evidence ≠ Continue | `TestDecisionEvidenceAuthorityLattice`, `TestCrossModuleAuthorityLattice` | PASS |
| Human Boundary + evidence ≠ Continue | same | PASS |
| failure/rejection + evidence ≠ authorization | `TestDecisionEvidenceFailureMatrix`, `TestCrossModuleEndToEndScenarios` | PASS |
| confidence = 1.0 ≠ authority | `TestDecisionEvidenceConfidenceIsNotPermission` | PASS |
| property: `permits(out) ⇒ permits(base)` | `TestDecisionEvidenceMonotonic` (6×5) | PASS |

Verified **through the real process adapter** in `TestCrossModuleAuthorityLattice`.

---

## 5. Validation-boundary review (fail-closed)

`decision.ValidateResult` (`decision.go:255`) + `decision.Validate` (`decision.go:229`)
+ `ValidConfidence` (`decision.go:136`). Fail-closed cases verified:

| Case | Result |
|---|---|
| unknown status / contract version / kind echo | `ErrInvalidResult` (`TestValidateResultStatusVocabularyIsClosed`, `…ContractVersionIsExact`, `…KindEchoMismatch`) |
| unsupported capability | `ErrUnsupportedCapability` |
| malformed response | `ErrInvalidResult` (`TestDecideProcessFailureMatrix/truncated_json`) |
| missing required fields | `ErrInvalidResult` / `ErrIndeterminate` (`TestValidateResultRequiredFields`) |
| unknown choice / outside allowed set | `ErrInvalidResult` (`TestValidateResultAllowedChoiceMembership`) |
| confidence <0 / >1 / NaN / ±Inf | `ErrInvalidResult` (`TestValidateResultConfidenceBounds`) |
| missing required confidence | `ErrIndeterminate` |
| contradictory fields (OK+error; non-OK + success payload) | `ErrInvalidResult` / typed (`TestValidateResultContradictoryFields`, `…NonOKNeverSucceeds`) |
| indeterminate result | `ErrIndeterminate` |
| empty output / protocol failure | `ErrProviderFailure` (adapter) |

Invalid/unvalidated evidence **cannot reach** `autonomy.Decide`: the seam calls
`decision.Validate` before `decisionEvidenceAdverse`/`decisionEvidenceClassification`,
and any error returns the baseline unchanged (byte-for-byte). No gap found.

---

## 6. Failure / fallback review

Against production behavior (SEAM-005 matrix). All failure modes preserve the
governed baseline; none authorizes.

| Situation | Production behavior | Never |
|---|---|---|
| no provider / disabled | strict no-op, no process started | error, approval |
| provider unavailable / executable missing / startup failure | typed `ErrProviderFailure`, recorded, baseline | success |
| non-zero exit / crash | typed `ErrProviderFailure`, baseline | success |
| timeout / cancellation | SOP deadline, process terminated, typed failure | authorization |
| malformed output | `ErrInvalidResult` (or `ErrProviderFailure` for empty/oversized) | pass |
| unsupported capability | `ErrUnsupportedCapability` | low-confidence guess |
| indeterminate result | `ErrIndeterminate` | pass |

**OFF/absent vs configured-and-failed** are *distinguished*: absent/disabled is a
silent strict no-op; a configured provider that fails is **recorded distinctly**
(`emitDecisionEvidenceUnavailable`, secret-free activity event) while the governed
baseline still stands. Both preserve governed behavior, as SEAM-005 acceptance
requires; the distinction is observability, not authority.

Fallback = return to SOP-owned governed behavior. There is no provider chaining and
no alternative provider trusted on failure.

---

## 7. Default-OFF / backward-compatibility

`decision.enabled` defaults false (`config.go` `DecisionConfig`); `decisionEvidenceEnabled`
short-circuits and `decisionProviderFromConfig` returns `(nil, nil)`.

| Claim | Test |
|---|---|
| disabled ⇒ no provider constructed, no subprocess | `TestDecisionEvidenceDisabledNeverConstructsProvider` |
| disabled ⇒ byte-for-byte unchanged | `TestDecisionEvidenceDisabledIsStrictNoOp`, `TestCrossModuleLivePathEndToEnd/provider_off_is_unchanged` |
| absent ⇒ no activity emitted | `TestDecisionEvidenceAbsentRecordsNothing` |
| no provider ⇒ no approval created / no lifecycle change / no model change | `TestScheduledTaskDecisionEvidenceEscalates/no_provider_continues` |

No provider is mandatory or default; no new configuration is required.

---

## 8. Human-approval review

Enforcement path is SOP-owned and unchanged: `humanBoundary` (`approval.go:400`) →
`recordHumanApprovalRequest` (`:410`) → `internal/approval`; `policyForcesHuman`
(`cli/autonomy.go:31`) → `recoverTask(..., NEEDS_HUMAN, ...)`. Provider evidence
cannot create approval evidence, mark one satisfied, remove `NeedsHuman`, bypass a
pending approval, turn a Human Boundary into Continue, or reinterpret confidence as
approval. Diagnostics are never read as approval (`emitDecisionEvidenceUnavailable`
emits labels only; `decisionEvidenceReason` contains only the closed choice).

Evidence: `TestRunApprovalBoundaryUnaffectedByDecisionEvidence` (pending approval
unchanged; provider not consulted), `TestScheduledTaskDecisionEvidenceEscalates`
(SOP records the approval for provider-added attention), `TestDecisionEvidenceConfidenceIsNotPermission`.

---

## 9. Provider-neutrality review

Search of the cumulative production additions for `clef|nimble|ollama|omlx|systemone|laya|julia`
finds **no production reference**; the only matches are test-only name lists used to
assert the absence of provider-specific behavior (`decision_provider_test.go`,
`command_test.go`, `governance_test.go`). Pre-existing model-id strings in
`config.go`/`internal/model` (SMALL/MEDIUM/LARGE defaults) are untouched by SEAM.

Policy branches on provider/model/executable name: **none**. The only name switch is
`decisionProviderFromConfig` selecting a neutral transport (`"command"` vs the
in-process default) — construction, not policy, and vendored-name-free.

Identity independence: `TestDecisionEvidenceProviderIdentityIsIrrelevant`,
`TestCrossModuleProviderIdentityIndependence` (different names, and different
metadata, identical evidence → identical policy).

---

## 10. Execution-model separation

- `internal/model` is **not** in the changed set; SMALL/MEDIUM/LARGE defaults and
  `DefaultClass=medium` are unchanged.
- `internal/decision` has no import edge to `internal/model` (leaf: only
  `context errors fmt math strings`), enforced by `TestDecisionSeamStaysProviderAndModelNeutral`.
- The seam never maps a decision to a model class; it touches only `res.decision`
  (routing/modelSelection untouched).
- `TestDecisionEvidenceNeverSelectsAModelClass`, `TestDecisionEvidenceIsIndependentOfModelContext`,
  `TestCrossModuleModelIndependence` all PASS.

---

## 11. Cross-module boundary review

The SEAM-006 fixture (`testdata/fake-decision-provider`, its own Go module) is
genuinely external: stdlib-only imports, no `internal/` import, no agentic-sop
import, no lifecycle/approval authority, no SOP governance type; it communicates only
through the documented JSON DTO. `internal/decision` remains internal.

**SEAM-007 conclusion:** yes — a future provider (a Clef adapter) can live in
`sop-decision-adapters` and supply decision evidence to the live policy path
**without any provider-specific change to `agentic-sop` policy**: it needs only the
SEAM-002 JSON protocol; SOP owns validation, translation, cancellation, and the
governed outcome.

---

## 12. Process / security review

| Property | Evidence |
|---|---|
| direct executable invocation (no shell) | `command.go:133` `exec.CommandContext(ctx, argv[0], argv[1:]...)`; `TestAdapterHasNoShellInterpreter` |
| argv separate, no shell expansion | `TestDecideArgumentsAreNotShellInterpreted`, `TestDecideRequestIsNeverShellInterpreted` |
| request on stdin, never a command line | `command.go:135`; protocol-shape capture |
| provider output never executed | `TestDecideDoesNotExecuteProviderOutput` |
| bounded output (1 MiB stdout, 512 B stderr), overflow fails closed | `command.go:33,154`; `TestDecideOversizedOutputFailsClosed`, `TestDecideHostileStderrIsDiagnosticOnly` |
| stderr diagnostic only | same |
| context propagation / timeout / cancellation | `TestDecideTimeoutFailsClosed`, `TestDecideCancellationFailsClosed`, `TestCrossModuleTimeoutAndCancellation` |
| child cleanup / no orphan | `command.go:134` `cmd.WaitDelay`; `TestCrossModuleTimeoutAndCancellation` (PID liveness) |
| diagnostics not instructions | `TestDecisionEvidenceAdversarialOutputHasNoAuthority` |
| secrets not leaked | `decisionEvidenceFailureLabel` (sentinels only); bounded stderr |

Residual (non-blocking): `cmd.Env` is inherited (the provider executable is trusted
operator configuration, matching the repo's other command adapters); a provider that
daemonizes grandchildren is not reaped (bounded stall via `WaitDelay`, not a hang).

---

## 13. New public API inventory

All new exported symbols live under `internal/`, so they are **not** importable by an
external module. None exposes governance authority.

| Symbol | Package | Purpose | Consumer | Why exported | Governance authority |
|---|---|---|---|---|---|
| `Choice.Known` | `internal/decision` | known-choice set | validator, adapter | shared definition | no |
| `ValidConfidence` | `internal/decision` | `[0,1]` finite check | validator, policy | shared rule | no |
| `ContractVersion`, `Status`, `StatusOK/…`, `Status.Valid` | `internal/decision` | result DTO vocabulary | adapter, seam | one contract | no |
| `Result` | `internal/decision` | result DTO mirror | adapter | protocol | no |
| `ErrInvalidResult/ErrUnsupportedCapability/ErrIndeterminate/ErrProviderResult/ErrProviderFailure` | `internal/decision` | typed failure sentinels | adapter, seam | classification | no |
| `Validate`, `Request.ValidateResult` | `internal/decision` | fail-closed validation | adapter, seam | boundary | no |
| `Request.Choices` (field) | `internal/decision` | bounded allowed set | seam | request DTO | no |
| `command.New`, `command.Adapter`, `Adapter.Name/Decide` | `internal/decision/command` | process adapter | composition root | transport | no |
| `DecisionConfig.Command` | `internal/config` | adapter argv | composition root | configuration | no |

No symbol is broader than the SEAM-002 contract requires. No refactor performed.

---

## 14. Architecture guards

`internal/archtest` (all PASS): `TestArchitectureGuard`, `TestCorePackageDirectoriesExist`,
`TestPhase8CoveragePreserved`, `TestProviderImplementationsExist`,
`TestGuardDetectsRepresentativeForbiddenDependency`, `TestGuardAcceptsNeutralImports`,
`TestProviderRegistryCoversProviderAdapters`, `TestSameCorePipelineAcrossAdapters`,
`TestCapabilityGateIsBehavioral`, `TestPromptCacheIsolatesProviderModel`,
`TestAdaptiveRoutingIsCapabilityAndEvidenceDriven`,
`TestDecisionSeamStaysProviderAndModelNeutral`, `TestDecisionSeamPackagesExist`,
`TestFakeDecisionProviderIsOutOfModule`.

Coverage of the ten required guards:

1. no provider-specific policy branch — guard + `TestDecisionEvidenceProviderTextIsData` — durable
2. no provider/model-name policy authority — `…ProviderIdentityIsIrrelevant` + factory fail-closed — durable
3. no execution-model coupling — `TestDecisionSeamStaysProviderAndModelNeutral` — durable (imports)
4. validation before policy interpretation — seam ordering + `TestDecisionEvidenceFailsClosed` — durable (behavioural)
5. no lifecycle mutation by provider — `TestDecisionEvidenceDoesNotTouchLifecycleState` — durable
6. no approval mutation by provider — `TestRunApprovalBoundaryUnaffectedByDecisionEvidence` — durable
7. single policy path — `TestDecisionEvidenceUsesSinglePolicyPath` — durable
8. no shell-command transport regression — `TestAdapterHasNoShellInterpreter` — durable
9. optional/default-OFF — `TestDecisionEvidenceDisabledNeverConstructsProvider`, `…AbsentRecordsNothing` — durable
10. external-provider compatibility — `testdata/fake-decision-provider` + proof tests — durable

Invariants protected only by convention (noted, non-blocking):

- The **seam's placement** in `executeLifecycle` before the artifact write is asserted
  by behaviour, not by a structural guard (a future refactor could move it).
- `attentionable`'s action list is keyed to the current autonomy action vocabulary;
  a future new action would default to *no escalation* (safe-but-quiet) and should be
  reviewed when added.

Neither is a material readiness gap.

---

## 15. Cross-module proof re-run

`go test -count=1 -run TestCrossModule ./internal/cli/` — PASS (real process boundary):
protocol shape; benign/adverse; Block/Human lattice; malformed; unsupported;
provider failure; timeout/cancellation (+cleanup); adversarial diagnostics;
identity independence; model independence; live-path end-to-end.
`TestScheduledTaskDecisionEvidenceEscalates` (SEAM-007 addition) — PASS.

---

## 16. Verification — exact commands and outcomes

| Command | Outcome |
|---|---|
| `gofmt -l .` | (no output) — clean |
| `go vet ./...` | exit 0 — clean |
| `go test -count=1 ./...` | exit 0 — all packages pass |
| `go test -race -count=1 ./...` | exit 0 — all packages pass, no data races |
| `go build ./...` | exit 0 |
| `git diff --check` | exit 0 — clean |
| `go test ./internal/decision/ ./internal/decision/command/ ./internal/archtest/` | 61 top-level tests PASS |
| `go test ./internal/cli/` | PASS (52 decision-seam tests among them) |

No failing required gate. No unrelated failure hidden.

---

## 17. Cumulative production diff review

| File | Purpose | Introduced by | Public API | Policy | Approval | Lifecycle | Provider-specific | Security risk | Residual risk |
|---|---|---|---|---|---|---|---|---|---|
| `internal/decision/decision.go` | contract DTO + fail-closed validator | SEAM-003 | yes (internal) | no | no | no | no | low | low |
| `internal/decision/command/command.go` | provider-neutral process adapter | SEAM-003 / bounded by SEAM-005 | yes (internal) | no | no | no | no | low | env inheritance; grandchild not reaped |
| `internal/config/config.go` | `decision.command` + validation | SEAM-003 | yes (internal) | no | no | no | no | low | none |
| `internal/cli/decision_provider.go` | neutral factory | SEAM-003 | no | no | no | no | no | low | none |
| `internal/cli/cli.go` | `deps.newDecisionProvider` | SEAM-003 | no | no | no | no | no | low | none |
| `internal/cli/decision_evidence.go` | the optional seam + interpretation | SEAM-004 / hardened SEAM-005 | no | no (adds attention only) | no | no | no | low | 60 s fixed timeout |
| `internal/cli/run.go` | one call site in `executeLifecycle` | SEAM-004 | no | no | no | no | no | low | placement asserted behaviourally |

`internal/autonomy`, `internal/failure`, `internal/model`, `internal/approval`,
`internal/quality`, `internal/commitgate`, `internal/mergegate`: **not modified**.

---

## 18. Findings by severity

**CRITICAL:** none.

**HIGH:** none.

**MEDIUM:** none. Explicitly evaluated candidates that were *not* downgraded to
complete the plan, and why they are not MEDIUM:

- Documentation staleness (§20) — affects only prose, not behaviour; not a governance gap.
- Fixed 60 s timeout, env inheritance, grandchild reaping — operational only; each is
  bounded and cannot add authority.

**LOW / NOTE:**

- L1 — `decisionEvidenceTimeout` is a fixed 60 s constant (not configurable).
- L2 — the provider process inherits the parent environment (trusted operator config).
- L3 — a provider that daemonizes grandchildren is not reaped (bounded stall).
- L4 — the seam's insertion point is guarded behaviourally, not structurally.
- L5 — `attentionable` is keyed to the current autonomy action vocabulary.
- L6 — documentation staleness in the architecture analysis (§20).

None blocks readiness.

---

## 19. Residual risks / notes

Carried forward to SEAM-008 as non-blocking: L1–L6 above. The seam is OFF by
default, monotonic, fail-closed, and provider-neutral; the external-adapter path is
proven; no production architecture change is required.

---

## 20. Documentation consistency

- `SEAM-architecture-analysis.md` selected design (§4), dependency direction (§8),
  desired-state (§9), risks (§10), anti-goals (§11) — **consistent** with the
  implementation.
- `SEAM-002-decision-contract.md` §4–§5 DTO, §7 failure table, §8 validation
  ownership, §11 no public package, §12 preferred Option A — **consistent**.
- Two **stale** (design-stage) statements, recorded but **not edited** (SEAM-007
  production scope does not authorize editing prior reports):
  1. `SEAM-architecture-analysis.md` §6 says a live provider failure "fail closes
     (human)". The implemented and plan-sanctioned (SEAM-005 acceptance) behavior is
     "provider failure preserves governed behavior" (baseline). This is a resolved
     design→plan evolution; the implemented behavior is the stricter-in-mechanics,
     authority-safe one (it never converts a failure into approval or authorization).
  2. §6/§7 describe a four-stage `disabled → shadow → selectable → live` ladder. The
     implementation provides OFF and enabled (=live); no runtime `shadow` mode was
     required by the plan (SEAM-004 was instructed not to invent modes).

Consumers (SEAM-008, adapters) should rely on SEAM-005/SEAM-007 semantics: **provider
absence and provider failure preserve governed behavior; never authorization.**

---

## 21. Readiness recommendation

**GO_WITH_NOTES** for SEAM-008.

Reasons:

- All six required gates pass, plus the full `internal/archtest` guard suite.
- Every required architecture property is verified with tests or source inspection:
  no provider-specific or provider-name policy branch; provider results cannot
  transition lifecycle state, create approvals, or authorize commits/merges;
  malformed evidence fails closed; provider absence and provider failure preserve
  governed behavior; execution-model routing is independent; an external provider
  crosses the module/process boundary; multiple providers implement the contract
  without modifying SOP policy.
- No CRITICAL, HIGH, or MEDIUM findings.
- The notes (L1–L6) are operational/documentation only and non-blocking; they are
  carried forward for SEAM-008 to acknowledge explicitly.

SEAM-008 may base its readiness result on this record without re-running the
investigation.

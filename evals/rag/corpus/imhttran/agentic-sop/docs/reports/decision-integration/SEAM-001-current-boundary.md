# SEAM-001 — Capture and Trace the Current Integration Boundary

Status: trace complete (read-only). This report is the SEAM-001 deliverable and the
only repository change of this stage.

- **Repository:** `~/agentic-workspace/agentic-sop`
- **Scope:** `agentic-sop` only; `sop-decision-adapters` not touched.
- **Current HEAD:** `b743cfa93c057eb1499ce72332d6481edb4643a6` (branch `main`, level
  with `origin/main` at trace time; working tree clean at start).
- **Method:** the architecture analysis and plan are treated as **hypotheses** and
  re-verified against live source at HEAD. Every claim below cites a file/symbol (and
  line where useful). Searches were run with `grep`, and package edges with
  `go list -deps`/`go list -f '{{join .Imports}}'`.

No production code is changed by this stage.

---

## 0. Current-state call-path diagram

```text
                         internal/cli  (composition root: defaultDeps, cli.go:106)
                         deps.newAgent / deps.newJEVAnalyzer / deps.readDiff ... (cli.go:48)
                                        │
      (A) optional early-JEV path       │            (B) lifecycle path
          OFF by default                │            internal/run + internal/cli/run.go
                                        ▼
        runEarlyGate -> runJEVCheckpoint (early_jev.go:137,151)
          earlyGateEnabled (early_jev.go:69)      failure.Classify(failure.Evidence) (failure.go:269)
          d.newJEVAnalyzer(cfg) (cli.go:61)                │  built from validation/review/
          runpkg.RunJEV -> JEVOutcome (run/jev.go:107)     │  outcome/error evidence
          ev.Validate() (early_jev.go:177)                 ▼
          autonomy.DecideEarly(...) (early_jev.go:184)  failure.Classification (failure.go:174)
          res.Escalate = Decision.RequiresHuman            │
                    │                                      ▼
                    │                       decideAutonomy(cfg, cls) (cli/autonomy.go:18)
                    │                         autonomy.Decide(cls, cfg.AutonomyPolicy()) (:22)
                    │                                      │
                    └──────────────┬───────────────────────┘
                                   ▼
                  internal/autonomy  (pure; imports ONLY internal/failure)
                    Decide (autonomy.go:162) / DecideEarly (early.go:72) /
                    DecidePlanChange (autonomy.go:227) / humanDecision (autonomy.go:294)
                                   │
                     autonomy.Decision{Action,Level,Risk,RequiresHuman,Reason,Classification}
                                   ▼
             lifecycle consumes:  AUTO_* -> requeue/fix loop; TERMINAL -> blockTask;
                                  HUMAN_APPROVAL_REQUIRED -> humanBoundary (cli/approval.go:400)
                                  -> recordHumanApprovalRequest (cli/approval.go:410)
                                  -> internal/approval + runpkg.WaitingForHuman

  (C) reconcile path:  reconcileAutoAccept -> autonomy.DecidePlanChange (cli/reconcile.go:126)

  SEPARATE, UNWIRED SURFACE (no production consumer):
  internal/decision  (leaf package: imports only context/fmt/math/strings)
     Request (decision.go:49) / Decision (decision.go:41) / Choice (decision.go:18) /
     Provider (decision.go:56) / NewProvider (decision.go:64) / Thresholds (decision.go:108) /
     Route (decision.go:142), validConfidence (decision.go:127), knownChoice (decision.go:30)
     production use of Provider/Request/Decision/Route/NewProvider: NONE
     production use of the package: only internal/config for decision.Thresholds (config.go:298)
```

---

## 1. The live `internal/autonomy` decision path

`internal/autonomy` is the single centralized, pure policy engine (`autonomy.go:1-24`).

- Entry points: `Decide(c failure.Classification, p Policy) Decision`
  (`autonomy.go:162`), `DecideEarly(ev jev.Evidence, failOn []string, p Policy) Decision`
  (`early.go:72`), `DecidePlanChange(kind PlanChangeKind, p Policy) Decision`
  (`autonomy.go:227`).
- `Decide` order: (1) authority boundary → human (`boundaryRisk`, `autonomy.go:251`);
  (1b) `NoProgress` → `TERMINAL` (`autonomy.go:173`); (2) `AutoFixExhausted` → human or
  terminal by level (`autonomy.go:181`); (3) disposition switch (`Retry`/`Continue`/
  `AutoFix`/`Replan`) gated by `Policy` flags (`autonomy.go:190`); (4) fail-closed to
  human (`autonomy.go:209`).
- The package doc states it has "no I/O, no clock, no LLM" and "starts no work and
  transitions no state" (`autonomy.go:18-23`).

### Production callers of autonomy policy functions (all in `internal/cli`)

| Callee | Call site | File:line |
|---|---|---|
| `autonomy.Decide` | `decideAutonomy` | `internal/cli/autonomy.go:22` |
| `autonomy.DecideEarly` | `runJEVCheckpoint` | `internal/cli/early_jev.go:184` |
| `autonomy.DecidePlanChange` | `reconcileAutoAccept` | `internal/cli/reconcile.go:126` |

`decideAutonomy` (`internal/cli/autonomy.go:18-23`) is the only wrapper of
`autonomy.Decide`; its production callers are:

- `internal/cli/run.go:903` (`runStages`: quality-gate failure classification)
- `internal/cli/run.go:1144` (`noChangesFailure`)
- `internal/cli/run.go:1243` (`outcomeResult` → `classifiedOutcome` path via `pendingResult`)
- `internal/cli/drive.go:727` and `internal/cli/drive.go:756` (`runScheduledTask`)

No other package calls `autonomy.Decide`/`DecideEarly`/`DecidePlanChange` in
production (verified by `grep` over `internal`).

## 2. Input to `autonomy.Decide`

- **Origin:** `failure.Classification` (`internal/failure/failure.go:174`), produced by
  `failure.Classify(failure.Evidence)` (`failure.go:269`) from a `failure.Evidence`
  struct (`failure.go:190`) built by the lifecycle: validation results, review report,
  agent outcome, errors, approval boundary, fix-cycle counts. Construction sites:
  `internal/cli/run.go:892`, `:903`, `:998` (`pendingClassification`), `:1143`,
  `:1242`; `internal/cli/drive.go:726`.
- **Constructor:** `failure.Classify` / `pendingClassification` / `noChangesFailure` /
  `outcomeResult` — all in `internal/cli`/`internal/failure`.
- **Fields that affect policy:** `Kind` and `Disposition` drive the `Decide` switch;
  `boundaryRisk` branches on `Kind`; `Reason` is descriptive only; `Confidence` is not
  a `Decide` branch (it is recorded). `Policy` flags (`AutoRetry`/`AutoContinue`/
  `AutoFix`/`AutoReconcileSafeChanges`, `Level`, `MaxContinuations`) gate the
  disposition.
- **Is it already "decision evidence"?** `failure.Classification` **is** typed
  evidence about a failure/outcome, not a general provider decision; it is
  SOP-constructed, not provider-supplied. `jev.Evidence` (`internal/jev/evidence.go`)
  is the other typed evidence input, consumed by `DecideEarly`. Neither is a
  provider-neutral *decision* result; no provider-neutral decision evidence reaches
  `autonomy` today.

## 3. Output of `autonomy.Decide` and its consumers

`autonomy.Decision{Action, Level, Risk, RequiresHuman, Reason, Classification}`
(`autonomy.go:151`). Consumers:

| Consumer | File:line | Handling |
|---|---|---|
| `humanBoundary` | `internal/cli/approval.go:400` | `d.RequiresHuman || d.Action==ActionHumanApproval || cls.Disposition==NeedsHuman || stage==WaitingForHuman` |
| `policyForcesHuman` | `internal/cli/autonomy.go:31` | `d.RequiresHuman && cls.Disposition != NeedsHuman` |
| `runStages` (Continue/Fix/block) | `internal/cli/run.go:908-932`, `:1256` | `ActionTerminal` → `Failed`; else stage from gate |
| `emitRunSummary` | `internal/cli/run.go:1059` | `ActionTerminal` forces `quality.Fail` |
| `traceTermination` | `internal/cli/trace.go:159` | records action/`RequiresHuman` |
| `runScheduledTask` | `internal/cli/drive.go:754-790` | terminal → `blockTask`; human → `recoverTask(...NEEDS_HUMAN...)` |
| `earlyGateResult.Escalate` | `internal/cli/early_jev.go:185` | drives the pre-mutation human boundary |

- **Continue:** an `AUTO_*` action → the existing bounded requeue/fix/continue machinery
  (`recoverTask`/`continueTask`, `run.go` fix loop).
- **Block:** `ActionTerminal` → `blockTask(saver, task, domain.RETRIES_EXHAUSTED` /
  `domain.NO_PROGRESS)` (`drive.go:763`), distinct from human approval.
- **Human approval:** `RequiresHuman`/`ActionHumanApproval` → `humanBoundary` →
  `recordHumanApprovalRequest` (`approval.go:410`) → `internal/approval` (`approval.New`)
  + `parkRunAtHumanBoundary` (`drive.go:643`, `runpkg.WaitingForHuman`).
- **Lifecycle-action owner:** `internal/cli` (the lifecycle in `run.go`/`drive.go` via
  `runpkg`); `autonomy` only *returns* a value, it never acts.

## 4. `internal/decision` — types and production usage

`internal/decision/decision.go` is a **leaf** package: `go list -f '{{join .Imports}}'`
shows **no** `agentic-sop/internal/...` import at all (only stdlib).

| Symbol | File:line |
|---|---|
| `Choice` (+ `Low/Medium/High/Human`) | `decision.go:18-25` |
| `knownChoice` (unexported) | `decision.go:30` |
| `Decision{Choice, Confidence, Metadata}` | `decision.go:41` |
| `Request{UseCase, Subject, Signals}` | `decision.go:49` |
| `Provider{Name, Decide}` | `decision.go:56` |
| `NewProvider` (hard-errors on unknown names) | `decision.go:64` |
| `DeterministicProvider` | `decision.go:74` |
| `Thresholds{RouteToStrongModel, RequireHuman}` | `decision.go:108` |
| `Target` (`SMALL_MODEL`/`STRONG_MODEL`/`HUMAN`) | `decision.go:116` |
| `validConfidence` (unexported) | `decision.go:127` |
| `Route` (fail-closed) | `decision.go:142` |

**Production consumers:** none for `Provider`/`Request`/`Decision`/`Choice`/`Route`/
`NewProvider`. `grep` for `decision.<Symbol>` over non-test `internal` finds only
`internal/config/config.go` (`decision.Thresholds` at `config.go:298`, `:394`, `:523-524`).
`decision.NewProvider` and `decision.Route` appear only in tests
(`internal/e2e/lifecycle/early_decision_test.go`, `internal/decision/governance_test.go`).

## 5. Confirmation: `internal/decision.Provider` is unwired from the live path

**Confirmed.** No production code constructs or invokes a `decision.Provider`; the
live policy path never reaches `internal/decision`. `config.DecisionConfig{Provider,
Enabled, Thresholds}` (`config.go:295`) is **validated only** — `validate` checks
`decision.provider ∈ {deterministic, jev}` (`config.go:573-576`) and threshold ranges
— and is never used to construct a provider (no production reference to
`cfg.Decision.Provider` or `cfg.Decision.Enabled` outside validation/defaulting).

## 6. Construction / bootstrap / configuration

- Composition root: `defaultDeps()` (`internal/cli/cli.go:106`). Injected boundaries:
  `deps` struct (`cli.go:48`) with `newAgent` (`:50`), `newResources` (`:51`),
  `readDiff` (`:52`), `snapshotRepository` (`:55`), `commit` (`:56`), `newGitHub`
  (`:57`), `newJEVAnalyzer func(cfg) (jev.Analyzer, error)` (`:61`), `newJEVAnalyzer`
  defaulted to `jev.NewOllamaAnalyzerFromEnv()` (`cli.go:163-168`).
- Optional-capability pattern (OFF by default): `config.EarlyJEV{Enabled *bool, ...}`
  (`config.go:250`) with `EarlyJEVActive()` = `Enabled != nil && *Enabled`
  (`config.go:677`); gates ANDed with enablement in `earlyGateEnabled`
  (`early_jev.go:69`).
- Policy config: `config.AutonomyPolicy()` (`internal/config/autonomy.go:40`) resolves
  `autonomy.Policy` from `config.Autonomy` (default `Balanced`, `config.go:528`).
- Execution stack: `resolveExecutionStack` / `defaultDeps.newAgent` (composition root);
  the agent factory is parameterised by harness/provider/model.
- `config.DecisionConfig` exists but is inert for construction (see §5).

## 7. Narrowest candidate evidence-insertion point

The narrowest point where externally produced, provider-neutral decision evidence can
enter **without** granting policy authority is an **optional, disabled-by-default
evidence checkpoint at the `internal/cli` composition seam**, structurally identical to
`runJEVCheckpoint` (`internal/cli/early_jev.go:151`):

1. build a bounded request from `taskfile.Spec` (cf. `buildEarlyJEVInvocation`,
   `early_jev.go:110`);
2. construct the provider through a `deps` factory (cf. `d.newJEVAnalyzer`,
   `cli.go:61`; `defaultDeps`, `cli.go:106`);
3. invoke it with a `context` deadline (cf. `runpkg.RunJEV`, `run/jev.go:107`);
4. **validate** the result fail-closed (cf. `ev.Validate()`, `early_jev.go:177`);
5. convert validated evidence into a typed policy input and call the pure policy
   (`autonomy.Decide`, `autonomy.go:162`);
6. return an action/escalation the existing lifecycle already enforces
   (`res.Escalate`, `early_jev.go:185`; `humanBoundary`, `approval.go:400`).

This keeps `internal/autonomy` pure (no new I/O) and keeps `internal/decision` a
passive contract/provider surface. It does **not** require touching the quality gate,
the review gate, the commit gate, or the approval boundary.

## 8. Where evidence validation would need to occur (SOP-owned)

At the seam, **before** policy consumes anything, mirroring the existing JEV seam:
`RunJEV` already discards a partial result on analyzer error and reports `FailClosed()`
(`internal/run/jev.go:92,119-124`), and `runJEVCheckpoint` calls `ev.Validate()` and
treats an invalid result as `ProviderFailed` (never a pass) (`early_jev.go:177-182`).
For decisions, SOP owns a fail-closed validator over the provider result: known status,
known choice (`knownChoice`, `decision.go:30` — currently unexported), finite
in-range confidence (`validConfidence`, `decision.go:127` — currently unexported),
missing confidence = indeterminate. Validation is a SOP responsibility, never the
provider's.

## 9. Where provider timeout/cancellation/error terminates without approval

- `RunJEV` measures and returns a fail-closed outcome; an analyzer error yields a zero
  `Result` and `Error` (`run/jev.go:119-126`), and `runJEVCheckpoint` records
  `ProviderFailed=true`, leaves `Decision` zero and `Escalate` false, and continues
  under deterministic policy (`early_jev.go:169-192`).
- Subprocess precedent enforces cancellation: `exec.CommandContext(ctx, ...)`
  (`agent/command.go:65`, `review/ocr.go:55`, `git/command.go:35`); the agent/review
  adapters surface `ctx.Err()` distinctly (`agent/command.go:72-75`,
  `review/ocr.go:62-65`), and `git` returns `ctxErr` (`git/command.go:40-42`).
- No `context` deadline is established at the decision boundary today (decision is a
  leaf package with no I/O). A future seam must own the deadline at the invocation
  point, like `RunJEV`/`exec.CommandContext`.

A provider failure therefore has a defined, non-approval termination path to copy:
record failure → never a success → deterministic policy decides (continue or human).
The live JEV seam chooses "continue under deterministic SOP policy"; whether the
decision seam continues or fails closed is a SEAM-005 policy choice, but it must never
become an approval.

## 10. Human-approval enforcement point

- `humanDecision` (`autonomy.go:294`) sets `Classification.Disposition = NeedsHuman`
  and returns `Action=HUMAN_APPROVAL_REQUIRED, RequiresHuman=true`; `boundaryRisk`
  (`autonomy.go:251`) selects the risk for authority-boundary kinds.
- Enforcement in the CLI: `humanBoundary` (`approval.go:400`) →
  `recordHumanApprovalRequest` (`approval.go:410`) via `approval.New` (`internal/approval`)
  + `parkRunAtHumanBoundary` → `runpkg.WaitingForHuman` (`drive.go:643`).
- `currentApprovalBoundary` (`approval.go:386`) is the only structured input that can
  produce an approval boundary. Approval is never inferred from prose or a task status.

## 11. Execution-routing separation

- Execution-model defaults are owned solely by `internal/model.DefaultRoute()`
  (`internal/model/model.go:294`): SMALL `ollama/qwen3:4b/local` (+cloud fallback),
  MEDIUM `ollama/nemotron-3-super:cloud`, LARGE `ollama/deepseek-v4.1-flash:cloud`,
  `DefaultClass=medium`.
- **No import edge between `internal/decision` and `internal/model` in either
  direction** (`go list -deps`: model↛decision, decision↛model; decision is a leaf).
- The policy/core packages (`autonomy`, `decision`, `approval`, `commandpolicy`,
  `commitgate`, `mergegate`, `quality`) have **no direct import** of `internal/provider`
  or a provider implementation (verified via `go list -f '{{join .Imports}}'` + `grep`).
- **Nuance (refinement, not a contradiction):** `internal/autonomy` imports only
  `internal/failure`, but `internal/failure` imports `internal/agent`, which transitively
  reaches `internal/provider` → `internal/model`. So `go list -deps ./internal/autonomy`
  lists `internal/model`. This is a **pre-existing transitive** chain via the failure
  vocabulary (a `Classification` can be derived from an `agent.Outcome`); it is not a
  decision-provider↔execution-routing coupling and is out of scope for this seam. It is
  worth recording because any future seam that adds a *direct* decision→model or
  evidence→model-class mapping would create a real coupling.

## 12. Cross-module visibility constraints

- Module: `github.com/imhttran/agentic-sop` (`go.mod`), `go 1.27.1`.
- **No public package exists.** `go list ./... | grep -vE '/internal/|/cmd/'` → none.
  Every package is under `internal/` or `cmd/`.
- An external module **cannot** import `internal/decision`: `Provider`, `Request`,
  `Decision`, `Choice`, `Thresholds`, `Route` are all under `internal/` and blocked by
  Go's internal-visibility rule.
- No existing public/contract package can host the contract today.
- **Reusable boundaries exist**: the `deps` factory-injection pattern (`cli.go:48,61`,
  `106`) and the JSON-over-subprocess pattern (`agent/command.go`,
  `review/ocr.go`, `git/command.go`).
- **The external adapter does not need SOP-owned Go types**: it can satisfy a
  provider-neutral JSON DTO over a process boundary. This is the finding that makes a
  process/transport boundary viable without exporting `internal/decision`.

## 13. Subprocess / transport precedent (evidence for SEAM-003)

| Property | `internal/agent/command.go` | `internal/review/ocr.go` | `internal/git/command.go` |
|---|---|---|---|
| Config source | `SOP_AGENT_COMMAND` (`:18`) | `SOP_REVIEW_COMMAND` (`:22`) | n/a (library) |
| Exec | `exec.CommandContext(ctx, "sh", "-c", command)` (`:65`) | `exec.CommandContext(ctx, "sh", "-c", p.command)` (`:55`) | `exec.CommandContext(ctx, "git", args...)` (`:35`) |
| Argument handling | command string via shell; request JSON on stdin (`:66`) | command string via shell; request JSON on stdin (`:56`) | direct argv, no shell (`:35`) |
| Env handling | inherits process env | inherits process env | `append(os.Environ(), "GIT_TERMINAL_PROMPT=0")` (`:38`) |
| Timeout/cancel | `ctx` (`:65`, distinguishes `ctx.Err()` `:72`) | `ctx` (`:55`, returns `ctx.Err()` `:63`) | `ctx` (`:35`, returns `ctxErr` `:40`) |
| stdout/stderr | separate buffers; stdout = content; stderr diagnostic (`:68-80`) | separate buffers; stdout parsed as report (`:58-73`) | separate buffers (`:40-43`) |
| JSON precedent | request `json.Marshal` (`:60`); response via `ParseOutcome` (`:128`) | request `json.Marshal` (`:50`); response `parseReport` (`:73`) | n/a |
| Security | command is trusted local config; request passed as JSON on stdin (never interpolated into the shell command) | `ErrNotConfigured` is non-fatal; command is "trusted local configuration … never built from untrusted input" (`:24-26`) | `GIT_TERMINAL_PROMPT=0` |

Facts recorded for SEAM-003 (not decided here): both a **shell** form (`sh -c`) and a
**direct-argv** form (`git`) already exist; cancellation is via `context`; stdout/stderr
are separated; JSON request/response is the established protocol; empty/erroneous
output is an error, never success.

## 14. Facts relevant to the future A-vs-B autonomy choice (SEAM-004)

Recorded, not chosen:

- **A — translate validated provider evidence into a typed `failure.Classification`
  and reuse `autonomy.Decide`.**
  - *Coupling introduced:* the seam/decision layer must speak the `failure`
    vocabulary (`failure.Kind`/`Disposition`), i.e. an outward dependency onto
    `internal/failure`.
  - *Duplication introduced:* a deterministic mapping table (provider result →
    `failure.Kind`/`Disposition`); possibly a new `failure.Kind` for
    "decision-evidence" cases.
  - *Semantic information lost:* a provider decision is not inherently a *failure*;
    forcing it into `Kind`/`Disposition` can lose the distinction between "not a
    failure but still an attention signal" and "a failure". Confidence/threshold
    semantics may not map cleanly (a confident LOW decision has no failure Kind).
  - *Does `Decide` already express all required policy inputs?* Yes for the output
    vocabulary (`AUTO_*`/`TERMINAL`/`HUMAN_APPROVAL`) and the gating inputs
    (`Kind`, `Disposition`, `Policy`). Its inputs are failure-shaped, not
    decision-shaped.
- **B — a dedicated autonomy entry point using an autonomy-owned provider-neutral
  evidence type.**
  - *Duplication introduced:* a second evidence type and a second policy entry point
    alongside `Decide`/`DecideEarly`/`DecidePlanChange`.
  - *Second policy path?* Risk of one, unless it delegates to the same `Decide`
    internally; `DecideEarly` is the precedent for a dedicated entry point that
    builds a `failure.Classification` and delegates to `Decide` (`early.go:82-88`).
  - *Avoids* forcing a decision into a failure-shaped type.
- **Which appears smaller on current evidence:** **A** appears smaller — `autonomy.Decide`
  already expresses the required policy inputs and the full action vocabulary, and
  `DecideEarly` shows the established pattern of a thin, deterministic translation
  into `failure.Classification` before calling `Decide`. B duplicates an evidence type
  and risks a second policy path. (Note: the option that best reuses the *existing*
  pattern may be a hybrid — a thin seam that constructs `failure.Classification`
  exactly as `DecideEarly` does — but this is a SEAM-004 decision; SEAM-001 does not
  choose.)

## 15. Unresolved questions

1. **A vs B** for autonomy consumption (§14) — deliberately deferred to SEAM-004.
2. **Shell (`sh -c`) vs direct executable** for the process boundary (§13) —
   deferred to SEAM-003.
3. **Exact call site(s)** that may invoke a decision provider (which lifecycle
   checkpoints) — the early-JEV seam is the closest precedent, but the plan leaves the
   precise set to SEAM-004.
4. **Config surface:** reuse `config.DecisionConfig{Provider,Enabled,Thresholds}`
   (currently inert) or add a new optional block — deferred to SEAM-003.
5. **Failure disposition** (continue-under-deterministic-policy vs fail-closed-to-human)
   when a configured provider fails — deferred to SEAM-005 (must never be approval).

## 16. Contradictions / refinements vs the architecture analysis

**No material contradiction.** The analysis's key hypotheses are confirmed at HEAD:
`internal/decision` has no production non-deterministic consumer; `config.DecisionConfig`
is validated but not wired; the live path is `internal/autonomy`; `internal/model` and
`internal/decision` are independent; no public package exists; a JSON-over-subprocess
pattern already exists.

Two **refinements** (not contradictions):

1. `internal/decision` is even more isolated than stated: it imports **no** internal
   package at all (a leaf, stdlib-only).
2. `internal/autonomy` is direct-import pure (only `internal/failure`) but has a
   **pre-existing transitive** dependency on `internal/model` via
   `failure → agent → provider → model`; this concerns the failure vocabulary, not
   decision-provider↔execution-routing coupling.

## 17. Invariant check — can a provider gain authority under this architecture?

Under the traced architecture, an external provider entering at the §7 seam would
receive a bounded request and return bounded data. It would have **no** method to:
transition task state (only `internal/cli`/`runpkg` transition), approve work (only
`internal/approval` + the human boundary), authorize execution (only the quality/review
gates + `autonomy`), bypass validation/review (the gates run before policy consumes
evidence), commit/merge (`commitgate`/`mergegate`), select lifecycle transitions (only
`autonomy.Decision` → `internal/cli`), or bypass human approval (`humanBoundary` →
`internal/approval`). Conclusion: **the proposed architecture does not necessarily grant
a provider policy/lifecycle authority**, provided validation and interpretation remain
SOP-owned at the seam. No stop condition is triggered.

---

## Scope statement

No production source, test, configuration, or default was modified by SEAM-001. This
report is the only repository change. No commit or push was performed.

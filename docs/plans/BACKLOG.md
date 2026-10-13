# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](../PLAN.md); this file is
the running list in between. Completed observations are preserved in
[RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md); shipped capabilities and known
limitations are catalogued in [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md).

## Agentic reliability and evaluation (AGENT-001–005)

**Status:** Complete. After the pre-Phase-6 closeout (complete), AGENT-001 through
AGENT-005 are delivered:

- **AGENT-001 Structured Run Trace** — **delivered**: a versioned observational
  `trace.json` (schema 1) per run, summarized by `sop report`.
- **AGENT-002 Progress Signals** — **delivered**: observational progress signals
  (discovery, repository mutation, verification, state transition) in `trace.json`
  (schema 2), summarized by `sop report`; they do not change lifecycle decisions.
- **AGENT-003 Evaluation Harness** — **delivered**: deterministic fixtures over
  `trace.json` under `evals/`, evaluated by `internal/eval` (see
  [EVALUATION.md](../reference/EVALUATION.md)).
- **AGENT-004 Agent Budgets** — **delivered**: the canonical execution budget
  (`internal/budget`: iteration, stale, tool-call limits), configurable via
  `SOP_OLLAMA_*` and recorded in `trace.json`.
- **AGENT-005 Replan Strategy** — **delivered**: opt-in, deterministic permission
  to change strategy once on the same class after a recoverable failure
  (`internal/recovery`; `models.replan_enabled` / `models.max_replans`), recorded in
  `trace.json` (schema 4) and asserted by `evals/replan/`.
- **Task-Scoped Discovery Budgets** (see
  [Task-Scoped Discovery Budgets](#task-scoped-discovery-budgets)) — deferred; the
  discovery allowance remains fixed (a soft threshold, not a hard budget).

Metrics to capture: run latency, token and tool-call counts, and reliability
measurements. This is distinct from the earlier, already-implemented
`PLAN-Phase-6-Interactive-Approval`; it is the reliability/evaluation workstream
that follows the closeout.


## Post-performance architecture (Phase 8) — delivered

**Status:** Delivered (Phase 8). The pre-performance closure and baseline completed
(recorded in [RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md) and
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md)) and deferred the new performance
architecture behind the readiness gate
([CLOSE-011](../reports/pre-performance-closure/CLOSE-011-readiness.md)). That
architecture then shipped as **Phase 8 (Context & Execution Efficiency)**, which is
complete and closed. The capabilities this section once listed as deferred are now:

- **DELIVERED** — Context Engine (CTX-001), structural repository index
  (CTX-002), BM25 retrieval (CTX-003), retrieval evaluation gate (CTX-004), prompt
  compiler (CTX-005), response normalizer (CTX-006), verification cache (CTX-007),
  prompt result cache (CTX-008), decision memory (CTX-010), adaptive routing (CTX-011),
  automatic prompt tuning (CTX-012).
- **EVALUATED / REJECTED** — vector retrieval (CTX-009): a deterministic
  evaluation found no improvement over BM25, so BM25 is retained and no vector
  retrieval shipped.
- **DELIVERED BUT OPT-IN** — the verification cache (`sop validate --cache`),
  the prompt result cache (`sop prompt --cache`), and decision memory on the run
  path (`context.decision_memory`); adaptive routing is live within the opt-in
  routing path.
- **DEFERRED FOLLOW-UP** — a persistent cross-run verification cache, tracked
  below as VERIFCACHE-PERSISTENCE.

The Git-SHA summary cache once named here was not shipped as a distinct component; the
structural repository index (CTX-002) and the verification cache (CTX-007) carry
repository-state identity instead. Phase 8 evidence:
[history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md](../history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md);
post-Phase-8 governance:
[history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md](../history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md).

## Persistent cross-run verification cache (VERIFCACHE-PERSISTENCE)

**Status:** Backlog (deferred). The Phase 8 verification cache (`internal/verifcache`,
CTX-007) is reachable today only as the per-invocation `sop validate --cache`, which is
consulted for a clean working tree. A persistent, cross-run verification cache is
deliberately **not** wired: it would reuse verification evidence across execution
sessions, repository states, and environments, a different trust boundary from the
existing in-session `sessionValidation` reuse. It requires a separate, evidence-gated
design (identity inputs, invalidation, and false-hit analysis) and is recorded here as
`VERIFCACHE-PERSISTENCE` rather than implemented.

## Performance telemetry gaps

**Status:** Backlog (low). `internal/perf` records stage durations and counts but
not every measurement the baselines would like. Documented gaps (recorded as
`UNAVAILABLE` rather than invented in the baselines):

- a tool-call count — `Counts` has no field (only the JEV record has `ToolCalls`);
- a per-category (`build`/`test`/`lint`) `validation_ms` breakdown for a safely
  *reused* validation;
- per-step PLAN timing (a slow plan is one number, not a profile).

Sources: `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` and the PREJEV018
remaining-issues register (I4–I6).


## Local network service (team mode)

Share SOP state and control across a team on the local network.

## Small-device dashboard

A read-only view of SOP state for small devices.

## Jev adapter and Jev-vs-deterministic evaluation

An adapter for the Jev planner, and an evaluation comparing it with the
deterministic planner.

## OpenAI-style tool calling for `openai_compatible` (Phase 3)

`openai_compatible` (Phase 1 identity/configuration, Phase 2 execution) currently
runs against the shared `internal/agent` `OpenAICompatible` transport, which is
text-only: its `chatRequest` has no `tools` field and it parses no tool calls, so
the checked wrapper honestly declares the text capabilities and rejects
`IMPLEMENT`/`FIX`. The only tool-calling loop in the tree is Ollama-native
(`internal/ollamaagent`, `POST /api/chat`).

Make the generic provider a coding agent by giving the shared OpenAI-compatible
transport real tool calling:

- add `tools` / `tool_calls` to the shared request/response (one implementation,
  reused by every OpenAI-compatible identity — do not fork a second HTTP client),
  and route the tool-call loop through the existing `internal/toolharness`
  surface (`internal/ollamaagent`-equivalent controls, no parallel harness);
- declare `IMPLEMENT`/`FIX` only when the server/model establishes tool support
  (never inferred from the model name), keeping the honest capability model;
- preserve the Phase 2 endpoint precedence
  (`SOP_OPENAI_COMPATIBLE_BASE_URL` > `providers.openai_compatible.endpoint` >
  `http://127.0.0.1:8000`), locality-as-configuration, and the IMPLEMENT/FIX
  no-progress guard semantics; do not raise the iteration ceilings.

Related: `internal/agent/openai.go` (the shared transport), `internal/agent/openaicompatible.go`
(the generic identity), `internal/toolharness`, `internal/ollamaagent` (the native
tool loop to mirror), and [../specs/PROVIDERS.md](../specs/PROVIDERS.md) §2a.

## Task-Scoped Discovery Budgets

**Priority:** High  
**Status:** Backlog  
**Depends on:** Bounded productive IMPLEMENT/FIX discovery (`8f426e2`)

### Problem

IMPLEMENT/FIX currently allow productive discovery through a fixed turn threshold
before stale/no-progress semantics take over.

This prevents short premature termination, but the fixed threshold still assumes
different agents/models require roughly the same amount of discovery.

A future agent may legitimately require more discovery turns before mutation.

We should avoid provider/model-specific limits such as:

- Codex = 18 turns
- DeepSeek = 12 turns
- Nemotron = 10 turns

## Earlier multi-package discovery observations

The original P3-006/P3-009/011/014/017 evidence and successive steering fixes are
preserved in [the resolved backlog](../history/RESOLVED-BACKLOG.md#bootstrap-agent-exhausts-its-budget-on-multi-package-tasks).
Remaining candidates were a concrete file/work scope, incremental continuation,
and a stronger configured model for multi-package tasks. Task-scoped discovery
budgets above track the next refinement; current bounded discovery behavior is
owned by [AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md#9-tool-harness-bounds-and-phases).

## Autonomy `BLOCK` hardening

**Status:** Backlog (defensive). `autonomy.Decide` has no `case failure.Block`:
only `NoProgress` produces `Disposition: BLOCK` today and is handled explicitly by
the `Kind == NoProgress` branch. A future non-`NoProgress` `BLOCK` classification
would fall through to a human boundary, contradicting `BLOCK`'s documented "not a
human approval" definition. Add an explicit terminal case plus a test. Surfaced by
the pre-Phase-6 closeout; currently unreachable.


## SOP-EXTERNAL-COMPLETION

**Status:** Delivered. `sop task complete <task-id> --external` records a task completed
outside the SOP execution as an explicit, model-free operator action backed by repository and
validation evidence (see `RECOVERY.md` §11). The description below is preserved for the
history of why it was needed.

SOP records a task's completion only when its own lifecycle
runs the task and produces a verified repository mutation (or a verified `ALREADY_SATISFIED`
claim). When the work was performed outside the SOP runtime — for example by a coding agent
that implemented and merged it directly — `sop run` cannot complete the task: the model
discovers that the work already exists, makes no mutation, and the stale guard classifies
`IMPLEMENT_NO_PROGRESS` (a spurious BLOCK). There is no model-free way to record the
completion, so the task's SOP state cannot reflect the merged work.

Provide a deterministic, model-free operation to record a task as complete against
authoritative evidence (for example a merged change plus a green validation), so SOP's task
state can reflect work performed outside its runtime without bypassing the lifecycle,
fabricating a PASS, or re-running the model.

Requirements:

```text
model-free
no task execution
requires authoritative evidence (merged change + green validation)
no fabricated PASS
no fabricated approval
does not advance dependent tasks beyond what the evidence supports
```

Surfaced when CTX-001 (Context Engine) was implemented and merged by a coding agent and
`sop run` could not record it: the model recognized the existing implementation, produced no
repository change, and the task was classified `IMPLEMENT_NO_PROGRESS` and blocked.


## Phase 10 — Dynamic Worker Action Space

**Priority:** Low
**Status:** Backlog (evidence-gated). Deliberately **not** scheduled or implemented.

Phase 10 would give every worker assignment a deterministic, harness-derived
**action space** — the bounded operations, tools, read scope, write scope, and
compatible targets the harness derives from authoritative inputs
(`WorkAssignment + RepositoryState + TaskState + Scope + Policy + WorkerCapabilities + Budget`)
— with execution-time revalidation and stale-action rejection. A model would propose
within that space; the harness would validate the proposal and grant authority. The
invariant is unchanged: a model-generated action is **data until the harness validates
it against the current worker action space**, and a worker can never gain authority by
naming a tool, operation, path, target, or provider command.

This is deferred until **real Phase 9 usage shows a measurable limitation** that the
current static orchestration cannot absorb. Revisit it when run evidence records any of:

- repeated `BLOCK` / `NO_PROGRESS` caused by static orchestration;
- manual replanning an operator performs by hand today;
- undiscovered dependencies between decomposed work units;
- another measurable orchestration limitation.

No orchestration complexity is added ahead of that evidence. Until then, static
orchestration under the existing Phase 9 boundaries remains the design. Design
inspiration only — no dependency, no code reuse: the constrained-action architecture of
`browser-use/jev-ultrafast`. Provider/model neutrality, the existing Context Engine, the
single lifecycle authority, and the single-agent defaults are all preserved.

## Phase 9 (complete)

**Status:** Complete. Phase 9 (Multi-Agent Orchestration) shipped as ORCH-001–ORCH-012;
all twelve tasks are `LOCAL_DONE`. The deterministic adoption gate
([PHASE-9-MULTI-AGENT-ADOPTION.md](../reports/PHASE-9-MULTI-AGENT-ADOPTION.md)) keeps
multi-agent execution **opt-in**, so default execution stays single-agent and Phase 9
changes no default behavior. Plan:
[PHASE-9-MULTI-AGENT-ORCHESTRATION.md](PHASE-9-MULTI-AGENT-ORCHESTRATION.md).

The post-Phase-8 governance pass (POST8-001) is complete. The remaining Phase 8
follow-up is the persistent cross-run verification cache, tracked above as
`VERIFCACHE-PERSISTENCE`.

## Execution profiles and task-state closure

The following items were recorded under the former top-level `docs/BACKLOG.md` and are
consolidated here (the single backlog owner):

## SOP-LC-001 — Automatic Task-State Closure with Evidence Reuse

- **Priority:** P2
- **Status:** IMPLEMENTED (2026-10-12)
- **Problem:** `sop run` could not close a plan whose tasks were already implemented
  and committed: an empty-diff task still invoked the optional JEV analysis, whose
  fail-closed result stopped the run at a retry/human boundary (`LC-001 RETRY
(requeued)`), so a green tree left the plan ACTIVE. Completing the tasks required a
  separate operator command per task, each re-running the full validation suite.
- **Requirements:**
  - Skip JEV when there is no reviewable change (an empty diff with no task-owned
    change evidence), mirroring review's "no changes to review" guard.
  - Reuse valid, task-scoped validation evidence across the plan (already provided by
    the run-session verification cache).
  - Complete already-satisfied verify-first tasks without an agent call, progress the
    lifecycle automatically, and close the plan on completion.
  - Preserve deterministic acceptance, audit history, and human approval boundaries;
    never fabricate completion for insufficient evidence.
- **Acceptance:** A 12-task plan with sufficient evidence closes in one governed
  workflow without 12 redundant validation runs.
- **Evidence:** `internal/cli/run.go` gates the JEV invocation on a reviewable change;
  `internal/cli/sop_lc_001_test.go` proves one `sop run` closes a 12-task
  already-committed plan (archived `COMPLETE`), with 0 agent calls, 0 JEV invocations,
  and 1 validation run reused across the 12 tasks; a second invocation is idempotent;
  a task with insufficient evidence fails closed and does not complete.

## SOP-EXEC-001 — Lightweight Execution Profiles

P2 · Proposed
Target: agentic-sop/docs/plans/BACKLOG.md
Scope

- Add --mode=light|standard|full to sop run.
- Support the same flag in SOP skills.
- Default to standard.
- Keep mandatory targeted verification in every mode.
- Minimize preflight for low-risk tasks.
- Automatically escalate when scope or risk increases.
- Preserve Git and policy safeguards.
  Acceptance criteria
- Small tasks execute without unnecessary full-suite checks.
- Targeted tests remain mandatory where applicable.
- Failed targeted tests prevent PASS.
- No applicable tests are reported explicitly.
- Existing behavior remains backward-compatible.

## Decision-model evaluation follow-ups (DME-FU-001–005)

Follow-ups from the DME decision-model evaluation; the consolidated findings live in
[DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) and the promotion
criteria in [AI-DECISION-PROMOTION-CRITERIA.md](../reference/AI-DECISION-PROMOTION-CRITERIA.md).

## DME-FU-001 — Investigate `DecideEarly` handling of `INCOMPLETE`/`ERROR`

- **Priority:** P1
- **Status:** Resolved (investigated 2026-10-13) — no production defect
- **Finding:** Production never routes `INCOMPLETE`/`ERROR` to `DecideEarly`: the
  early-JEV checkpoint (`internal/cli/early_jev.go`) short-circuits fail-closed,
  `ERROR`, and `INCOMPLETE` results into a non-escalating `ProviderFailed` outcome
  before policy is applied, and the quality-gate path (`quality.NewJEVEvidence`)
  fails closed on them. The `CONTINUE` mapping was an evaluation-only tooling
  artifact (`internal/deceleval/jev.go`). See
  [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) §5.
- **Residual (evaluation-only):** addressed — the eval mapping was renamed to
  `MapJEVHypotheticalDirective`, its hypothetical (non-production) scope documented,
  and the production distinction pinned by tests; a production-fidelity evaluation is
  proposed as DME-FU-005.
- **Problem:** In the JEV shadow-mode evaluation, `autonomy.DecideEarly` mapped a
  result with no blocking finding — including `INCOMPLETE`/`ERROR` with no
  HIGH/CRITICAL items — to `CONTINUE`, while the quality gate
  (`quality.NewJEVEvidence`/`quality.Evaluate`) treats `INCOMPLETE`/`ERROR` as
  fail-closed evidence that is never a pass. The two consumers disagree on the
  safety semantics of an incomplete analysis.
- **Requirements:** Confirm whether this is reachable in the real execution path
  (the early-JEV checkpoints), whether the divergence is intentional per consumer,
  and whether `DecideEarly` should fail closed on `INCOMPLETE`/`ERROR`.
- **Acceptance:** A written finding with the production reachability path (or a
  "not reachable" determination) and a decision on fail-closed semantics; any code
  change carries targeted tests.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) §5;
  `evals/decision-model/jev-shadow.results.json` (`dme-provider-fault-002`,
  `dme-real-as-clef-005` ERROR; `dme-real-close-001`, `dme-real-rsh-002` INCOMPLETE).

## DME-FU-002 — Review the `dme-real-close-004` label and `BLOCK` vs `NEEDS_HUMAN`

- **Priority:** P2
- **Status:** Resolved (investigated 2026-10-13) — labelling inconsistency; label correction proposed
- **Finding:** `BLOCK` (terminal stop, no approval) and `NEEDS_HUMAN` (human approval)
  both withhold automatic execution; they differ only in operational responsibility.
  The deterministic `BLOCK` is not an unsafe continuation: across all 61 scenarios the
  baseline has 0 less-conservative mismatches (`internal/deceleval/disposition.go`,
  `AnalyzeExecutionPosture`). `dme-real-close-004` is the only `NO_PROGRESS` scenario
  labelled `NEEDS_HUMAN` (six siblings are labelled `BLOCK`), so the label — not the
  classifier — is the outlier.
- **Proposed correction (pending explicit approval):** relabel `dme-real-close-004` to
  `BLOCK` and re-freeze the corpus. The frozen label is unchanged; under the
  correction the deterministic baseline reads 48/61 exact with 0 unsafe misses.
- **Problem:** `dme-real-close-004` expects `NEEDS_HUMAN` while the deterministic
  classifier produces `BLOCK` (counted as the corpus's single unsafe miss). `BLOCK`
  and `NEEDS_HUMAN` both withhold automatic execution but carry different
  operational responsibilities (a terminal stop with no approval vs a human approval
  boundary). Whether the frozen label should be `BLOCK` or `NEEDS_HUMAN` has not been
  adjudicated.
- **Requirements:** Independently review the label against the recorded failure and
  document the intended semantics; if the label changes, re-freeze the corpus and
  record a new corpus revision (labels are otherwise frozen).
- **Acceptance:** A written adjudication with rationale; the corpus hash recorded for
  any revision; downstream metrics re-derived where the label changes.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) §5.

## DME-FU-003 — Define evidence-based criteria for reconsidering AI failure disposition

- **Priority:** P2
- **Status:** Documentation complete (2026-10-13) — threshold approval pending
- **Deliverable:** [AI-DECISION-PROMOTION-CRITERIA.md](../reference/AI-DECISION-PROMOTION-CRITERIA.md) —
  separate evaluation scopes, the deterministic baseline, hard safety gates,
  effectiveness requirements, evaluation methodology, staged promotion, and decision
  outcomes (with established vs proposed thresholds).
- **Problem:** The evaluated AI configurations produced zero useful escalations and
  more errors than deterministic Go, so no AI failure-disposition configuration is
  promoted. There is no agreed, pre-registered bar for when reconsideration would be
  justified.
- **Requirements:** Turn the plan's decision rule into concrete, pre-registered
  thresholds (unsafe misses = 0, false-escalation ceiling, latency budget, and at
  least one of accuracy or useful-escalation rate showing a supported improvement),
  and name the evidence a candidate configuration must produce first.
- **Acceptance:** A written, pre-registered acceptance bar a candidate must meet
  before it can influence production decisions.
- **Evidence:**
  [PLAN-Decision-Model-Effectiveness-Evaluation.md](PLAN-Decision-Model-Effectiveness-Evaluation.md#decision-rule-retain--tune--disable).

## DME-FU-004 — Evaluate JEV's code-review effectiveness on real changes

- **Priority:** P2 · Conditional
- **Status:** Proposed (only if warranted)
- **Problem:** JEV was evaluated for failure disposition using only terse failure
  text (no diffs or repository context), which is outside its primary review use
  case. Its value as a code-quality/review mechanism therefore remains unmeasured.
- **Requirements:** If JEV's review value matters, evaluate it on real diffs and
  repository context against the quality gate's own criteria — a distinct
  experiment, not a re-run of the decision-disposition corpus. Do not promote JEV as
  a failure-disposition classifier regardless of that result.
- **Acceptance:** A scoped evaluation plan with labeled review fixtures and
  reproducible evidence, or a documented decision to defer.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) §4, §6.

## DME-FU-005 — Production-fidelity JEV evaluation (proposed)

- **Priority:** P3
- **Status:** Proposed
- **Problem:** The JEV shadow experiment (`jev-shadow-v1`) measures a *hypothetical*
  use of JEV as a failure-disposition classifier, not production early-JEV behavior:
  production short-circuits fail-closed, `INCOMPLETE`, and `ERROR` results before
  policy runs. There is no evaluation of JEV's actual production mapping.
- **Requirements:** Design a production-fidelity evaluation that replays the real
  early-JEV checkpoint (`internal/cli/early_jev.go` `runJEVCheckpoint`) over a corpus
  and records its actual outcomes (escalate / provider-failed / continue under
  deterministic policy). Do not reuse the hypothetical `jev-shadow` metrics.
- **Acceptance:** A scoped evaluation plan and reproducible evidence, or a documented
  decision to defer.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md) §5;
  `internal/deceleval/jev.go` (`MapJEVHypotheticalDirective`).

## Developer Preview carry-forward (from the superseded plan)

Carried forward when `plan-sop-developer-preview` was disposed `SUPERSEDED`
(2026-10-13); each item's status was re-checked against `main` at that time. The original
register and its HOLD verdict are preserved in
[history/developer-preview/](../history/developer-preview/README.md); the plan remains at
[PLAN-SOP-Developer-Preview.md](PLAN-SOP-Developer-Preview.md). Two preview findings are
no longer open and are **not** carried: the documentation link gate (KI-03) now passes
(`scripts/checks/check-doc-links.sh` → `broken links: 0`), and the acceptance-verification
escape (KI-01/KI-05) now has a mechanism — structured acceptance checks plus the opt-in
`lifecycle.verdict_gate` (default OFF). Three items remain:

### DP-01 — `examples/` is referenced but absent (was KI-09)

- **Priority:** P3
- **Status:** Open (re-verified 2026-10-13)
- **Problem:** the shipped SOP skill tells users examples live in `examples/`
  (`review.md`, `plan.md`, `diagnose.md`, `implement.md`), but no `examples/` directory
  exists in the repository, so the reference dangles.
- **Requirements:** add the referenced example files, or remove/redirect the reference.
- **Acceptance:** `examples/` exists with the named files, or the reference is corrected;
  `scripts/checks/check-doc-links.sh` still passes.
- **Evidence:** `ls -d examples` → absent; [skills/sop/SKILL.md](../../skills/sop/SKILL.md).

### DP-02 — the governed harness cannot run `./install.sh` (was KI-04)

- **Priority:** P3
- **Status:** Open (re-verified 2026-10-13)
- **Problem:** `internal/commandpolicy` classifies any unrecognized program as
  `REQUIRES_APPROVAL`, so a governed task cannot verify install/quickstart: the PREVIEW-002
  run parked at the approval boundary and its deliverable had to be operator-produced.
  There is no sanctioned, unattended install-verification step.
- **Requirements:** admit a sanctioned install step in the governed harness, or document
  install verification as an explicit operator-run step.
- **Acceptance:** a deterministic, governed install-verification path exists, or the
  operator-run nature is documented in the install/quickstart guidance.
- **Evidence:** `internal/commandpolicy/commandpolicy.go` (`Classify` default →
  `RequiresApproval`); [history/developer-preview/QUICKSTART-VERIFICATION.md](../history/developer-preview/QUICKSTART-VERIFICATION.md).

### DP-03 — cross-repository controller/adapter verification is unverified (was KI-02/KI-07)

- **Priority:** P2
- **Status:** Open (re-verified 2026-10-13)
- **Problem:** the governed harness is repository-root confined, so governed tasks cannot
  reach the sibling `sop-controller` / `sop-decision-adapters` repositories and cannot run
  remote-ref Git commands. Controller runtime parity and live `decision.provider: command`
  dispatch remain unverified, so no production/GO claim can cover the controller
  integration surface. Adapter *shadow* evidence now exists
  ([TEL-003](../reports/decision-telemetry/TEL-003-integration.md)), but live dispatch is
  still unobserved.
- **Requirements:** provide an operator-authorized cross-repository integration workspace
  (or equivalent) that can exercise controller parity and live decision-adapter dispatch.
- **Acceptance:** a recorded, reproducible run covering the controller runtime and live
  decision-adapter dispatch, or a documented decision to defer the claim.
- **Evidence:** [ETOE-010 final reliability report](../reports/end-to-end-reliability/ETOE-010-final-reliability-report.md);
  [history/developer-preview/KNOWN-ISSUES.md](../history/developer-preview/KNOWN-ISSUES.md) KI-02/KI-07.

## Status and roadmap

See [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md) for implemented capabilities
and [its known limitations](../reference/PROJECT-STATUS.md#known-limitations-current).
The numbered implementation plans remain in [this directory](.).

## Known limitations (current)

See [the current limitations reference](../reference/PROJECT-STATUS.md#known-limitations-current).

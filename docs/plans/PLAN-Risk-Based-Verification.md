# PLAN — Risk-Based Verification and Risk-Tiered Execution

**Status:** ACTIVE (`plan-risk-based-verification`). The prior plan
`plan-post-wave3-production-readiness` was superseded with operator authorization;
RD-03/RD-04 are preserved as SUPERSEDED history under `.agent-sdlc/archive/`.
Implemented and committed: RBV-001, RBV-002, RBV-003, RBV-004, RBV-008, RBV-012, the
dedup guarantee of RBV-011, and RBV-013 (input+output size; provider tokens
UNAVAILABLE). RBV-005 (acceptance verification) and RBV-007 (validation/review reuse)
are satisfied by existing components and test coverage - no change. RBV-009 is
documented. RBV-010 is satisfied by the deterministic fixture. RBV-011's
retrieval/handoff wiring remains, pending a design decision.

**Basis:** review of the current verification, JEV, autonomy, and approval
workflows. Only RBV-001, RBV-002, and RBV-008 are implemented so far; the rest are
pending.
**Rule:** deterministic behavior and every existing safety boundary are preserved.
The risk tier may only _escalate_ verification; it never weakens a gate, raises a
budget, changes approval behavior, or bypasses an approval.

## Project

agentic-sop

## Summary

SOP today runs the **same** configured validation set (`build`, `test`, `lint`) for
every ordinary mutating task, regardless of how local, low-risk, or narrow the
change is. JEV is a separate optional stage that, once enabled, runs on every green
pass. The result is duplicated validation work across tasks, unconditional JEV
invocations, and no deterministic link between a change's actual risk and the
verification it receives.

This plan introduces a **deterministic, escalate-only verification risk tier**
(`LOW` | `MEDIUM` | `HIGH`) that selects the _verification strategy_ — which
configured checks run and whether JEV is required — while leaving the deterministic
quality gate, the failure classifier, the autonomy policy, the acceptance
verification, and the human approval surface unchanged.

The tier is derived only from typed evidence SOP already produces (the model
router's `Risk`/`Complexity`/`Scope` signals and the task's declared execution
mode). It is never derived from prose, never chosen by a model, and never lowers a
`HIGH` signal.

```text
typed evidence -> deterministic tier -> verification plan -> existing gate -> existing approval
                                            |
                       (targeted tests | tests+build | full+acceptance+JEV)
```

The design reuses existing mechanisms rather than adding a second engine:
`internal/router` (typed signals), `internal/quality` (the single verdict),
`internal/autonomy` (recovery policy), the validation runner
(`internal/validate`/`testrunner`), the acceptance-check seam
(`deriveOutcome`/`authorizeAcceptanceCommand`/`runAcceptanceArgv`), and the
verification/review reuse cache (`internal/cli/session.go`, `internal/verifcache`).

## Approved scope decisions

1. **Reuse the globally loaded `AGENT-POLICY.md`; do not create a repository copy.**
2. **Implement risk-based validation with LOW, MEDIUM, and HIGH tiers.**
3. **Use a `verification.*` configuration namespace, disabled by default.**
4. **Preserve existing approval behavior.** No new approval gate is added; the former
   standalone approval task (RBV-006) is removed.
5. **JEV is optional for LOW/MEDIUM and required for HIGH when JEV is enabled.**
6. **Deterministic risk classification**, escalating unresolved material risk
   (escalate-only merge; unknown evidence resolves to the safe default, never LOW).
7. **Measure validation time, JEV invocations, and agent calls** (RBV-010).
8. **Preserve deterministic acceptance verification and existing safety boundaries.**

## Risk tiers

| Tier       | Required validation                                                  | JEV                           | Review                              | Approval  |
| ---------- | -------------------------------------------------------------------- | ----------------------------- | ----------------------------------- | --------- |
| **LOW**    | relevant/targeted tests (+ build)                                    | optional (off unless enabled) | existing (self)                     | unchanged |
| **MEDIUM** | tests + build (today's default)                                      | only when warranted           | existing                            | unchanged |
| **HIGH**   | full validation (build+test+lint) **and** executed acceptance checks | required when JEV is enabled  | existing (+ external if configured) | unchanged |

Tier rules (all deterministic, all in code):

- The tier is computed from typed evidence only; a model cannot select or lower it.
- Unknown/uncomputable risk resolves to `MEDIUM`/`HIGH` (fail closed to _more_
  verification), never `LOW`.
- A high-risk, high-complexity, or cross-cutting signal floors the tier at `HIGH`.
- The tier is **escalate-only** (`verify.Max`): pre-execution evidence sets an
  initial tier; the observed post-implementation change set may only raise it.
- Tiers change _what is verified_, never _how the verdict is decided_. The gate's
  `PASS`/`FAIL`/`NEEDS_HUMAN` computation, the fix budget, the retry/continuation
  budgets, the acceptance verification, and the approval surface are unchanged.

## RBV-001 — Deterministic risk-tier model and derivation

Introduce a pure, model-free `internal/verify` package: the tier type, the
escalate-only merge, and the derivation function. It consumes the existing
`router.Signals` and the task's declared execution mode, and returns the tier plus
fixed-phrase reasons. It performs no I/O, calls no model, and transitions no state.

### Dependencies

None

### Deliverables

- `internal/verify`: `Tier` (`LOW`/`MEDIUM`/`HIGH`), `Valid`, `Max`, `RequiresJEV`,
  `Derive`.
- Escalate-only merge and a fail-closed default (`MEDIUM`/`HIGH` when evidence is
  missing).
- Unit tests: LOW/MEDIUM/HIGH derivation, high-risk floor, escalate-only,
  fail-closed-on-unknown, purity/determinism.

### Acceptance Criteria

- `Derive` is deterministic: identical typed inputs yield an identical tier and
  identical reasons.
- A high-risk / high-complexity / cross-cutting signal never derives below `HIGH`.
- Missing typed evidence derives `LOW` only from an explicit verify-first
  declaration, never from absent evidence alone.
- No model call, network access, or state write occurs in the package.

### Execution

- implement

## RBV-002 — Opt-in `verification` configuration surface

Add an optional, validated, off-by-default `verification` configuration block that
supplies the LOW-tier targeted command set. An omitted block leaves existing
behavior byte-for-byte unchanged. Unknown keys are rejected by the existing strict
decoder.

### Dependencies

- RBV-001

### Deliverables

- `verification.risk_based` (bool, default false) and `verification.low.build` /
  `verification.low.test` (targeted command lists).
- A `Verification.Enabled()` gate and a `LowValidation()` helper that reuses the
  existing `validate`/`testrunner` runner.
- Load-time validation and tests, plus field-reference documentation (RBV-009).

### Acceptance Criteria

- With no `verification` block, configuration load and every run are unchanged.
- An unknown key is rejected at load time.
- The block alone activates nothing; `risk_based: true` is required.
- A LOW tier with no targeted commands configured falls back to the standard set
  (never to zero verification).

### Execution

- implement

## RBV-003 — Tier-selected validation in the run lifecycle

Wire the derived tier into `runStages` (`internal/cli/run.go`) so the _configured
command set executed for the gate_ is selected by the tier: LOW uses the targeted
set, MEDIUM uses tests+build, HIGH uses the full set. The gate
(`internal/quality.Evaluate`) is unchanged; it still decides from the observed
results. When `risk_based` is off, the full configured set runs exactly as today.

### Dependencies

- RBV-002

### Deliverables

- Tier computation at the validation seam, and re-evaluation (escalate-only) after
  the observed change set.
- Validation selection using existing `validate`/`testrunner` mechanisms.
- The run report records the tier and the actual command set executed.
- Tests: LOW runs only the targeted set; HIGH runs the full set; `risk_based: false`
  is unchanged; a post-implementation scope growth escalates LOW→HIGH.

### Acceptance Criteria

- The executed command set is a deterministic function of the tier and the config.
- A targeted (LOW) pass is the _gate's own_ required evidence for that tier, never a
  bypass of a required full gate: when the tier is HIGH the full set is mandatory.
- `risk_based: false` reproduces the current behavior (same commands, same gate).

### Execution

- implement

## RBV-004 — Tier-gated and de-duplicated JEV

Invoke the existing optional JEV stage (`runOptionalJEV`) only when the tier warrants
it (`Tier.RequiresJEV`, i.e. HIGH), preserving disabled-by-default behavior. JEV is
optional at LOW/MEDIUM. Do not re-invoke JEV when its inputs are unchanged within a
bounded attempt.

### Dependencies

- RBV-001

### Deliverables

- A deterministic `ShouldRun(tier, enabled)` predicate consumed by the JEV seam.
- JEV invocation guarded by the predicate; the existing "cleared while validation is
  red" and bounded-retry behavior is preserved.
- Tests: no JEV when unwarranted; JEV at HIGH when enabled; disabled-by-default
  unchanged; no duplicate JEV on an unchanged input within one bounded attempt.

### Acceptance Criteria

- JEV is never invoked when the tier/config says it must not run.
- Enabling JEV with `risk_based: false` behaves exactly as today.
- JEV evidence reaching the gate is unchanged in shape and severity handling.

### Execution

- implement

## RBV-005 — Acceptance verification distinct from generic tests

Make a `HIGH` task's completion derive from **executed acceptance checks**, not from
a generic green unit-test suite, by threading the tier into the existing
`deriveOutcome`/`authorizeAcceptanceCommand`/`runAcceptanceArgv` seam. A task whose
acceptance cannot be established from an observed check result stays `PARTIAL`/
`UNAVAILABLE`, never `VERIFIED`. No new execution or authorization path is added and
no new approval gate is introduced.

### Dependencies

- RBV-003

### Deliverables

- Tier-to-acceptance wiring: HIGH requires executed acceptance checks;
  missing/unexecutable checks fail closed.
- Tests: generic tests green but acceptance check unmet ⇒ not `VERIFIED`; a passing
  authorized check ⇒ `VERIFIED`; unauthorized/malformed check ⇒ `UNAVAILABLE`.

### Acceptance Criteria

- `VERIFIED` is never produced from generic passing tests alone when acceptance checks
  are required.
- Only operator-declared, authorized commands can establish acceptance (model-authored
  claims cannot).
- Existing acceptance-check semantics and the command policy are unchanged.

### Execution

- implement

## RBV-007 — Reuse: avoid duplicate validation and JEV

Extend the existing reuse rules so a LOW/MEDIUM tier whose selected command set and
diff are unchanged reuses a prior passing result, and JEV is not re-invoked on
unchanged inputs. The rules stay as specified in
[../reference/PERFORMANCE.md](../reference/PERFORMANCE.md): only a passing/clean
result is cacheable, reuse never crosses a run or plan, and a miss is always safe.

### Dependencies

- RBV-003
- RBV-004

### Deliverables

- Reuse keyed by (tier, command set, diff) for validation and (task, diff, engine),
  plus JEV-input identity for JEV.
- Tests: a targeted pass is reused across consecutive unchanged tasks; a changed tree
  or command set invalidates; JEV is not called twice on identical inputs.

### Acceptance Criteria

- No duplicate validation or JEV invocation occurs when the inputs are provably
  identical within a run.
- Correctness never depends on a cache hit; a miss re-runs the work.

### Execution

- implement

## RBV-008 — Distinguish review infrastructure failure from code failure

Confirm and harden the bounded distinction between a review **infrastructure/provider
failure** (fail-closed, never a pass, non-code) and a **blocking finding** (a code
defect that enters the existing bounded fix loop). This is provider-neutral and
raises no budget. The malformed-output path already maps deterministically to the
human boundary; this task proves and documents it and closes any remaining conflation.

### Dependencies

None

### Deliverables

- Tests: a provider/transport review failure is classified as infrastructure
  (retry/fail-closed), not as a blocking finding; a `HIGH`/`CRITICAL` finding beside a
  malformed sibling still blocks; malformed output never yields a pass.
- A short spec note recording the distinction.

### Acceptance Criteria

- A review infrastructure failure never auto-fixes or silently passes.
- A genuine blocking finding always enters the bounded fix loop.
- No budget, threshold, or approval boundary changes.

### Execution

- implement

## RBV-009 — Documentation alignment

Update the normative specs and references to describe the risk tier, the
verification-plan selection, and the unchanged boundaries, and reconcile the
documentation index. **Do not create a repository copy of `AGENT-POLICY.md`**; it is
read from its global location.

### Dependencies

- RBV-003
- RBV-004
- RBV-005

### Deliverables

- Updates to [../specs/VALIDATION.md](../specs/VALIDATION.md),
  [../specs/QUALITY.md](../specs/QUALITY.md), [../specs/OPENJEV.md](../specs/OPENJEV.md),
  [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md),
  [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md),
  [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md) (PERF008), and the docs
  index.
- A doc-links check passes.

### Acceptance Criteria

- Every behavioral change is described in exactly one normative place, cross-linked,
  with no spec restating another's rules.
- The documentation states explicitly that tiers escalate only and never bypass a
  gate, approval, or acceptance verification.

### Execution

- implement

## RBV-010 — Regression and operation-count benchmark

Add a deterministic, model-free fixture that exercises all three tiers and asserts
**operation counts** (validation runs, JEV invocations, agent calls, fix cycles) and
the **measured validation time**, never wall-clock thresholds, so a slow machine
cannot make it fail. Extend the existing benchmark in `internal/cli/perf_test.go`.

### Dependencies

- RBV-003
- RBV-004
- RBV-005
- RBV-007
- RBV-013

### Deliverables

- A multi-task fixture with a LOW, a MEDIUM, and a HIGH task and a dependency edge.
- Assertions proving: LOW runs only the targeted set; HIGH runs the full set; JEV runs
  only when warranted; no duplicate validation/JEV; `risk_based: false` matches the
  current baseline counts.
- The three measured quantities (validation time, JEV invocations, agent calls)
  reported from `internal/perf`.

### Acceptance Criteria

- The fixture is deterministic and passes offline.
- The asserted counts match the design and the disabled-flag baseline is unchanged.

### Execution

- implement

## Prompt and output efficiency (added scope)

This scope was added after the initial review. It reuses Phase 8 and the existing
metrics/evaluation plans and must not duplicate them.

### Reconciliation with existing work

Phase 8 context/efficiency is largely implemented and is reused as-is: CTX-001
Context Engine (`internal/context`), CTX-002 structural index (`internal/repoindex`),
CTX-003 retrieval (`internal/retrieval`), CTX-004 retrieval gate
(`internal/retrievalgate`), CTX-005 Prompt Compiler (`internal/prompt`), CTX-006
Response Normalizer (`internal/normalize`), CTX-007 verification cache
(`internal/verifcache`), CTX-008 prompt-result cache (`internal/promptcache`),
CTX-009 vector evaluation (`internal/vectoreval`), CTX-010 decision memory
(`internal/decisionmemory`), CTX-011 adaptive routing, CTX-012 prompt tuning
(`internal/prompttuning`). The handoff capsule/compressor (`internal/handoff`)
exists but is not wired into the run lifecycle. `internal/perf` records stage
latency, agent calls, validation/review/fix counts, and JEV invocations, but not
context/prompt size or tokens. `sop report` is concise by default with no detailed
view. Two existing plans already cover parts of this:
`PLAN-Run-Metrics-And-Observability.md` (provider-reported tokens are MISSING and are
explicitly not policy/budget/acceptance authority) and
`PLAN-EV-Context-Run-Evaluation.md` (live-path context evaluation). Reuse both; do not
rebuild their subject matter.

### Missing functionality (gaps to close)

- **Input:** no dedup guarantee across compiled sections; retrieval and the handoff
  capsule are not wired into execution-context assembly.
- **Output:** no detailed view; the concise default does not state a fixed
  completed-work / verification / blockers / next-action shape.
- **Measurement:** context/prompt input size is not recorded; provider-reported
  input/output tokens are not captured; normalized output size is not recorded.

## RBV-011 — Input efficiency: dedup, relevant context, between-task compression

Guarantee the compiled model input carries no duplicated instruction, task, or
globally inherited policy text; assemble only relevant, already-identified context
(reusing CTX-001/CTX-003/CTX-010); and compress between tasks via the existing handoff
capsule — never during an active task's execution. Acceptance criteria and safety
boundaries are carried verbatim. No new dependency and no second context or handoff
implementation.

### Dependencies

- RBV-003

### Deliverables

- A dedup guarantee in `internal/prompt` (sections never repeat acceptance criteria or
  policy text; the task and its criteria are carried by the request's own fields).
- Opt-in, off-by-default wiring of retrieval context and the handoff capsule into
  context assembly for a task, reusing `internal/retrieval`, `internal/handoff`, and
  `internal/context`.
- Tests: byte-identical compiled input for identical input; no duplicated criterion or
  policy line across sections; between-task compression is bounded and never applied
  mid-task; boundaries and acceptance criteria unchanged.

### Acceptance Criteria

- A compiled prompt contains each acceptance criterion and each policy line at most
  once.
- Retrieval and handoff wiring is opt-in and off by default, so an existing run is
  unchanged.
- Context compression happens only between tasks (handoff), never inside an active
  task's execution.
- No gate, approval, or acceptance-verification behavior changes.

### Execution

- implement

## RBV-012 — Output efficiency: concise default, detailed on request

Add an explicit detailed view (`sop report --detailed`) that renders the full existing
evidence (validation, review findings, JEV, classification, routing, performance) on
request, while the default stays concise and states completed work, the verification
outcome, blockers, and the next action. Full technical evidence remains in the
existing run artifacts; responses are normalized through CTX-006 before
interpretation. No new reporting subsystem.

### Dependencies

None

### Deliverables

- A `--detailed` flag on `sop report` (reusing the persisted `report.json` and the
  existing renderers); the default output shape is unchanged.
- A fixed concise summary shape: completed work, verification outcome, blockers, next
  action.
- Tests: default output is concise and unchanged in shape; `--detailed` adds the full
  sections; no task-state or gate effect.

### Acceptance Criteria

- The default `sop report` remains concise; `--detailed` shows the full evidence.
- No evidence is lost: everything shown in detail is already in the run artifacts.
- No behavior change to task state, gates, or approvals.

### Execution

- implement

## RBV-013 — Input/output measurement: size, tokens, latency, calls, JEV

Record the deterministic input size (canonical context bytes and compiled-prompt
bytes) per task in `internal/perf`, and capture provider-reported input/output tokens
when a provider exposes them; a value SOP did not measure is `UNAVAILABLE`, never
estimated. Latency (stage durations), model calls (agent calls), and JEV invocations
are already recorded and are reported alongside. Reuses the existing perf and metrics
plans; introduces no new metrics store or dependency.

### Dependencies

- RBV-003
- RBV-011

### Deliverables

- `internal/perf` fields for context/prompt bytes (and normalized output bytes), with
  reporting in `metrics.json`, `report.json`, and the perf summary.
- Provider-reported token capture where available, else `UNAVAILABLE`.
- Tests: sizes are recorded and rendered; an unmeasured value is `UNAVAILABLE` and
  never inferred; no decision reads a metric.

### Acceptance Criteria

- A measured size is recorded; a value SOP did not measure is `UNAVAILABLE`, never
  inferred from text length.
- No metric influences a gate, routing, budget, or approval decision.
- No new dependency and no second metrics store.

### Execution

- implement

## Dependency graph

```text
RBV-001 ──► RBV-002 ──► RBV-003 ──┬─► RBV-005 ──► RBV-009
                                  ├─► RBV-007 ──► RBV-010
RBV-001 ──► RBV-004 ──────────────┘
RBV-003, RBV-004 ──► RBV-007
RBV-003 ──► RBV-011 ──► RBV-013 ──► RBV-010
RBV-012   (independent)
RBV-008   (independent)
```

## Parallelizable groups

- **Group A (independent):** RBV-001, RBV-008, RBV-012.
- **Group B (after RBV-001):** RBV-002, then RBV-003.
- **Group C (after RBV-001 / RBV-003; disjoint files):** RBV-004, RBV-005.
- **Group D (after the above):** RBV-007, RBV-010.
- **Group E (docs, after RBV-003..005):** RBV-009.
- **Group F (added scope, after RBV-003):** RBV-011, then RBV-013; RBV-010 also depends on RBV-013.

Assign non-overlapping files: RBV-001 owns `internal/verify`; RBV-002 owns
`internal/config`; RBV-003/004 own `internal/cli/run.go` and the JEV seam; RBV-005
owns the acceptance seam; RBV-009 owns `docs/`. Do not let two tasks edit the same
orchestration file concurrently.

## Validation

Per-task deterministic acceptance plus the repository bar as appropriate to what was
touched: `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`;
`go test -race -count=1 ./...`; `git diff --check`; `scripts/checks/check-doc-links.sh`.

## Non-goals

- No second orchestrator, model, or provider.
- No change to the gate verdict, the failure classifier, the autonomy policy, the
  acceptance verification, or the approval surface beyond the deterministic tier
  input.
- No raised budget, no bypassed approval, no auto commit/push/merge.
- No model-selected or prose-inferred tier.
- No new approval gate (decision 4).
- Do not vendor `AGENT-POLICY.md` into the repository (decision 1).
- No second context, handoff, or reporting subsystem; reuse CTX-001..012 and `internal/handoff`.
- Tokens are never policy, budget, routing, or acceptance authority; an unmeasured value is `UNAVAILABLE`, never estimated.
- No new dependency.

## Estimated impact (to be confirmed by RBV-010)

Directional only; the RBV-010 fixture and `internal/perf` provide the authoritative
before/after counts for validation time, JEV invocations, and agent calls.

- **Validation time:** LOW tasks run a targeted command subset; MEDIUM is unchanged;
  HIGH is unchanged or slightly higher. No gate is weakened.
- **JEV invocations:** from "once per green pass when enabled" to "only when
  warranted", removing the analyzer call on the common LOW/MEDIUM case.
- **Agent calls:** unchanged for MEDIUM/HIGH; a LOW task provable by `verify-first`
  avoids the IMPLEMENT call, and de-duplication avoids repeated validation/JEV calls.
- **Safety:** zero change to approvals, budgets, or boundaries; every tier change is
  escalate-only.

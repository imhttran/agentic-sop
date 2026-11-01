# RM-005 — Decision Record: Offline Aggregate vs Opt-in Command

Decision-record stage of the Run Metrics and Observability plan. It records a
single **GO / HOLD / NO-GO** decision on whether to surface the RM-004 verified
offline aggregate as an opt-in `sop metrics` / `sop report --all` command under a
separate, evidence-gated plan. **No production surface is implemented here.** This
is the only artifact this task writes; it is documentation-only and changes no
production runtime path.

Terminology is preserved from the plan and RM-001…RM-004: **artifact**, **run**,
**denominator**, **eligible runs**, **UNAVAILABLE**, **coverage**, **census
denominators**, **provider/model neutrality**, and the seven dimensions.

## 0. Authority and inputs

| Input | Role |
| --- | --- |
| `docs/plans/PLAN-Run-Metrics-And-Observability.md` | The plan that defers `sop metrics` / `sop report --all` to this decision (Non-Goals; RM-005; Dependency Graph `GO --> [human decision] --> SEPARATE plan`). |
| `docs/reports/run-metrics/RM-004-verification.md` | The independent verification of determinism, coverage reconciliation, zero-vs-`UNAVAILABLE`, and production-path neutrality — the evidence this decision cites. |
| `docs/reports/run-metrics/RM-003-aggregation-results.md` | The measured aggregate over the live `.agent-sdlc/runs/` evidence (per-metric denominators and coverage). |
| `docs/reports/run-metrics/RM-002-metric-register.md` | The seven-dimension register, classifications, and the binding token rule. |
| `docs/reports/run-metrics/RM-001-artifact-inventory.md` | The artifact census and the 164-run census denominator (2026-10-08). |
| `docs/reports/ev-context-run/EV-005-acceptance-criteria.md` | The Option A / Option B decision precedent: objective GO/HOLD/NO-GO predicates, fail-closed stop conditions, enumerated prerequisite approvals. |

## 1. Decision

> **Decision: GO — but only with respect to authorizing a future, separate,
> evidence-gated surface plan. No surface is implemented by RM-005.**

The verified aggregate is fit to be surfaced as an opt-in `sop metrics` /
`sop report --all` command **under a separate plan**. RM-005 itself authors no
command, wires no CLI, and changes no production path; it records that the evidence
now supports proceeding to a distinct plan, and it fixes the approvals and
fail-closed stop conditions any such plan must satisfy.

### 1.1 Why GO (and not HOLD / NO-GO)

- **The GO preconditions the plan fixed are all met.** The plan's Baseline /
  Evaluation Acceptance Criteria required every metric to state a denominator and a
  coverage percentage, an absent artifact to be `UNAVAILABLE` and excluded (never
  zero-filled), reproducibility, a clean toolchain, token usage `UNAVAILABLE` and
  absent from gate/budget/routing, and no `internal/cli/` or production runtime
  change. RM-004 records all of these as PASS.
- **HOLD is not required.** HOLD is reserved for "a required metric cannot be
  established without inference or the coverage denominator is ambiguous." RM-004
  §2 reconciles every emitted metric to its RM-001 denominator; the one genuinely
  unavailable dimension (`changed-files.json` change footprint) is recorded as an
  explicit **UNKNOWN** and is **not** part of any emitted metric, so no denominator
  is ambiguous and no value is inferred.
- **NO-GO is not indicated.** NO-GO is reserved for a metric that "would require
  inventing a value, changing an artifact schema, or using a token count as
  authority." No emitted metric invents a value, no schema changed (RM-003 added a
  pure library and tests only), and token usage is `UNAVAILABLE` and carries no
  authority (RM-004 §4.4; RM-002 §4).

Recording **GO** means the aggregate is authorized to be *considered* for a future
opt-in surface; it does **not** mean the surface exists, is wired, or is enabled.

### 1.2 The offline-only option (Option B), retained

The alternative — **leave the aggregate offline-only** and wire no command at all —
remains a legitimate choice of the future plan's human decision, exactly as EV-005
§2.3 retains Option B (E2 = NOT_REQUIRED). Under Option B the aggregate stays a
pure library reachable only through its tests and the RM-003 report; `sop metrics`
and `sop report --all` are never added. This record does not foreclose it.

## 2. Cited RM-004 evidence

The decision rests on the RM-004 verification, not on assertion. Specific evidence:

| RM-004 section | Verified property | Recorded verdict |
| --- | --- | --- |
| §1 — Determinism | Two independent aggregations over identical inputs compare byte-for-byte (`TestReproducible`; `TestGoldenOutput`); inputs are fixture corpora `internal/runmetrics/testdata/{present,absent,zero}` plus `.agent-sdlc/runs/`; no model or network call; the `Aggregate` struct carries no wall-clock field. | **PASS** |
| §2 — Coverage reconciliation | Every emitted metric reconciles to the RM-001 census denominators (164, 2026-10-08) with the documented 164→168 corpus growth; recomputed percentages match reported. Sample: `agent_success_failure_rate` 126/135 = 93.3%; `execution_latency` 17,000,500 ms / 150 = 113,336.7 ms; `token_usage` 0/0 = 0.0% `UNAVAILABLE`. One recorded **UNKNOWN** (`changed-files.json`, not emitted). | **PASS** (one UNKNOWN) |
| §3 — Zero vs `UNAVAILABLE` | Missing-artifact fixture emits `"value": "UNAVAILABLE"` with `denominator: 0`, `coverage_pct: 0`; recorded-zero fixture emits `"value": "0"` with `denominator: 1`, `coverage_pct: 100`. The two are never conflated. | **PASS** |
| §4 — Production-path neutrality | `TestNoProductionImportOfRunmetrics` finds **zero** offenders; `grep -rn 'internal/runmetrics' --include='*.go' .` (excluding the package) returns no lines; `grep -rn 'runmetrics' internal/cli/` returns no matches. Provider/model neutrality grounded: no runtime path can be influenced by the aggregate; token usage never an input to budget, routing, or acceptance. | **PASS** |
| §5 — Toolchain gates and clean diff | `gofmt -l .` (empty), `go vet ./...`, `go build ./...`, `go test ./...`, `go test -race ./...` all exit 0; `git diff --check` clean. The only file RM-004 added is `docs/reports/run-metrics/RM-004-verification.md`. | **PASS** |
| §6 — Final reconciliation checklist | All acceptance criteria map to a section, command, and result; no criterion asserted without its command. | **PASS** |

Supporting figures from RM-003 (the aggregate RM-004 verifies): seven emitted
metrics with explicit numerator/denominator/coverage — `agent_success_failure_rate`
93.3% (126/135), `routing_finding_severity` 53/53 (PARTIAL, 31.5% coverage),
`fix_loop_convergence` 84/150 (mean ≈ 0.56), `retry_efficiency` 61/61 (PARTIAL,
36.3%), `human_approval_frequency` 6.7% (9/135), `execution_latency`
17,000,500 ms / 150 (mean ≈ 113 s/run), `token_usage` `UNAVAILABLE` (0/0, 0.0%).

## 3. No production surface is implemented

**RM-005 implements no production surface.** Specifically:

- No `sop metrics`, no `sop report --all`, and no other CLI command is added, wired,
  enabled, or registered. RM-004 §4.3 records that no `internal/cli/` file references
  `internal/runmetrics`, and RM-005 leaves that state unchanged.
- No flag, subcommand table, help text, output format, or configuration key is added.
- No file under `internal/` — CLI, orchestration, perf, run, or runmetrics — is
  created, modified, or deleted.
- The only artifact RM-005 writes is this report,
  `docs/reports/run-metrics/RM-005-decision.md`.
- No `.agent-sdlc` state is created, modified, or deleted; no plan activation is
  performed; no commit or push is made.

GO here authorizes a *future* plan to be considered; it does not itself create the
surface. Any such plan is separate, evidence-gated, and subject to the approvals in
§5.

## 4. Stop conditions (fail-closed)

Any future surface plan must **stop and report HOLD or NO-GO — never proceed** — if
any of the following holds. The default on ambiguity is to stop. These mirror the EV
fail-closed discipline (EV-005 §3).

1. **No token authority.** A token count must never be used as budget, routing, or
   acceptance authority. Provider-reported token counts are MISSING
   (`internal/context/context.go:10-11`, `internal/prompt/compile.go:15-16`,
   `internal/orchestration/budget.go:23-25`; CLOSE-008 §6); `token_usage` is
   `UNAVAILABLE` (RM-003 §4; RM-002 §4). No token count may enter any gate, budget,
   routing, or acceptance field. Token counts are never inferred from text, byte, or
   diff size (CLOSE-008 §10 rule 3; EV-004 §4). Informational at most, never policy.
2. **No provider/model-specific behavior.** The surface must stay provider- and
   model-neutral. No provider- or model-specific branch, no special-casing of
   provider/model names, no non-neutral dependency. Provider and model names, when
   present, are data to be counted, never logic (RM-003 §2; RM-004 §4.4). The command
   must not change routing, budget, or selection semantics.
3. **No lifecycle change.** The surface must not change routing, retry, replan,
   budget, verification, approval, or commit semantics. The aggregator is a reader,
   not an authority (plan, Architecture Invariant); a surface that lets its output
   drive a lifecycle decision is fail-closed. Any correctness, lifecycle, approval,
   retry, replan, budget, or verification-semantics change stops the work.
4. **No denominator fabrication.** An absent artifact stays `UNAVAILABLE` with a
   coverage impact, never a zero (RM-001 §5.1; RM-004 §3). A surface that zero-fills
   an absent artifact, widens a denominator by inference, or reports a value without
   its denominator and coverage is fail-closed.
5. **No unverified surface.** A surface whose output is not reproducible
   byte-for-byte over identical inputs, or that regresses the toolchain gates
   (`gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go test -race ./...`),
   is fail-closed.
6. **No missing approval.** If any prerequisite approval in §5 is absent, the work
   stops. The GO recorded here is *conditional on* those approvals.

## 5. Prerequisite approvals for a future surface plan

Each item is enumerated individually with what it gates. **None is requested or
implied by RM-005;** this record asks for no approval and activates no plan.

| # | Prerequisite approval | Who approves | What it gates |
| --- | --- | --- | --- |
| 1 | **Surface authorization** — human decision to actually wire an opt-in `sop metrics` / `sop report --all` command (plan Prerequisites item 1; Dependency Graph `GO --> [human decision] --> SEPARATE plan`). | Human (architecture owner) | Any surface work at all; selects GO-to-build or the offline-only Option B of §1.2 |
| 2 | **Separate plan authorization** — approve a distinct, evidence-gated plan for the surface; RM-005 does not activate it. | Human (plan owner) | The separate surface plan's creation and execution |
| 3 | **CLI / spec authorization** — any change to the CLI command surface, help text, or a spec (`docs/specs/…`) documenting the command. | Human (spec owner) | CLI wiring and any spec edit the surface requires |
| 4 | **Offline-first / no-instrumentation authorization** — the surface reads recorded artifacts only; it adds no live-path instrumentation and no artifact-schema change. | Human (architecture owner) | Keeps the surface a reader, preserving the Architecture Invariant |
| 5 | **Token-metric authorization** — continued confirmation that token consumption is informational only, never policy (plan Prerequisites item 3; RM-002 §4). | Human (policy owner) | Any token recording at all; keeps tokens out of authority |
| 6 | **Commit authorization** — RM-001…RM-005 are not committed and the plan is not closed without separate human approval (plan Prerequisites item 2). | Human (repository owner) | Committing the reports and closing the plan |

**RM-005 requires none of these.** Authoring this decision record is read-only and
model-free; it changes no production runtime path. Only the transition to an actual
surface (§6) requires the approvals above.

## 6. Decision procedure (ordered, deterministic)

```text
1. RM-004 verification = PASS (determinism, coverage, zero-vs-UNAVAILABLE, neutrality).
2. Branch on this record's decision:
   - GO    -> [human decision: §5 item 1] -> SEPARATE evidence-gated plan wires
              `sop metrics` / `sop report --all`; author no surface work here.
   - HOLD  -> expand/refine evidence and re-verify; never proceed to a surface.
   - NO-GO -> keep the aggregate offline-only (§1.2 Option B); record; stop.
```

The procedure is ordered and deterministic: RM-004 precedes any surface decision,
and no surface work follows without the enumerated approvals and the explicit human
decision. **RM-005 stops at step 2 with GO recorded; it performs no step-3 work.**

## 7. Change-scope statement

- No production change is made, described, or performed by RM-005.
- No CLI command (`sop metrics`, `sop report --all`) is added, wired, or enabled.
- No `.agent-sdlc` state is created, modified, or deleted.
- The only artifact added is `docs/reports/run-metrics/RM-005-decision.md`.
- Pre-existing user-owned working-tree changes are preserved unchanged.
- No plan activation is performed; no commit or push is made.

## 8. Verification record (RM-005 self-verification)

| # | Acceptance criterion | Inspection method | Result |
| --- | --- | --- | --- |
| 1 | The decision names the chosen option and cites the RM-004 evidence | Read §1 (GO) and §2 (RM-004 §1–§6 table with verdicts and figures) | PASS |
| 2 | No production surface is implemented; the report states that explicitly | Read §3 and §7; confirm no CLI is added and the only artifact is this report | PASS |
| 3 | Stop conditions mirror the EV fail-closed discipline (no token authority, no provider/model-specific behavior, no lifecycle change) | Read §4 items 1–3; confirm each maps to the EV-005 §3 discipline, with items 4–6 as additional fail-closed conditions | PASS |
| 4 | Prerequisite approvals for a future surface plan are enumerated, or the offline-only option is stated | Read §5 (six enumerated approvals) and §1.2 (offline-only Option B retained) | PASS |

Required validations (`go build ./...`, `go test ./...`, `go vet ./...`, `gofmt -l .`)
are run clean; this task adds only a Markdown report, so the working tree shows only
the expected report artifact on top of the preserved pre-existing user-owned changes.

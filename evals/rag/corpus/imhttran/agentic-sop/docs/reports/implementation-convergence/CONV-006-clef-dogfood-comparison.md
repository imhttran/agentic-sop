# CONV-006 — Clef Dogfood Comparison (harness before vs after CONV-004)

**Type:** External, read-only dogfood verification (comparison report)
**Status:** COMPLETE — no production change made; no Clef production code changed
**Scope:** `internal/ollamaagent` phased IMPLEMENT/FIX engine, compared against the
preserved CLEF-014 case as external read-only evidence.

This report compares the convergence behavior of the harness **before** the CONV-004
change (the CONV-001 baseline: bounded but not enforced) against the harness **after**
CONV-004 (pre-mutation convergence enforcement), using the same repaired CLEF-014
task/context and the same default budget. It performs no live CLEF-014 re-run, rewrites
no preserved CLEF-014 evidence, modifies no CLEF-014 task definition, and writes exactly
one file: this report.

---

## 1. Evidence base actually read

| Artifact | Location | Read this stage? |
| --- | --- | --- |
| CONV-001 baseline ("before") | `docs/reports/implementation-convergence/CONV-001-convergence-baseline.md` | Yes (in-repo) |
| CONV-005 verification ("after") | `docs/reports/implementation-convergence/CONV-005-regression-safety.md` | Yes (in-repo) |
| Governing plan | `docs/plans/PLAN-Implementation-Convergence.md` | Yes (in-repo) |
| CLEF-014 raw run artifacts (`attempt.txt`, `trace.json`, `classification.json`, `report.md`) | External `sop-decision-adapters` repo, `.agent-sdlc/runs/CLEF-014/` | **Not accessible — not read** |

### 1.1 CLEF-014 raw artifacts: not accessible (limitation)

The preserved CLEF-014 run artifacts live in the **external** `sop-decision-adapters`
repository, outside this repository, and are **not present in this repository**. This
is recorded explicitly in CONV-001 §4:

> "The CLEF run artifacts live in the external `sop-decision-adapters` repository
> (`.agent-sdlc/runs/CLEF-014/`: `attempt.txt`, `trace.json`, `classification.json`,
> `report.md`) and are **not present in this repository**."

The controlled tool surface for this stage cannot escape the repository root, so no raw
CLEF-014 artifact was opened. **Limitation:** this comparison does **not** rest on a
raw-artifact read of CLEF-014. The CLEF-014 side of the comparison rests on the
in-repo authoritative record: the observed-run row carried in the governing plan
(`docs/plans/PLAN-Implementation-Convergence.md` line 123) and reproduced in the CONV-001
baseline §4. That row is the only concrete CLEF-014 evidence asserted here, and no CLEF-014
artifact is described as if it had been inspected.

No CLEF-014 file was read, written, retried, or otherwise touched.

---

## 2. Identity preconditions (held constant on both sides)

| Precondition | Value |
| --- | --- |
| Task / context | The repaired CLEF-014 task definition, **unchanged** (not modified by this stage) |
| Capability | IMPLEMENT (phased IMPLEMENT/FIX engine) |
| Budget knob | `SOP_OLLAMA_STALE_ITERATIONS` = **5** (default) — **not increased** |
| Discovery window | `implementNowAfter` = 12 |
| Stale allowance | `staleIterations` = 5 |
| Convergence bound | `implementNowAfter + staleIterations` = 12 + 5 = **17** |

Both the before and after sides are reported at the **same** `staleIterations = 5` and the
**same** bound of **17**. No iteration, stale, tool-call, or retry budget is increased,
and no new budget knob is introduced.

**Sources:** `docs/reference/CONFIGURATION.md` (default `SOP_OLLAMA_STALE_ITERATIONS` = 5);
`internal/budget/budget.go` (`EnvStaleIterations = "SOP_OLLAMA_STALE_ITERATIONS"`,
`DefaultStaleIterations = 5`); `internal/ollamaagent/policy.go` (`implementNowAfter = 12`,
`staleIterations`). Arithmetic reproduced in CONV-001 §4.

---

## 3. BEFORE — CONV-001 baseline (bounded but not enforced)

The pre-CONV-004 engine (`internal/ollamaagent` shared `executePhased`) is **bounded but
not enforced**: it permits an unmutated run to consume the full discovery window and then
the stale allowance doing read-only work, and only then stops. As recorded in CONV-001 §4
and §8:

- A continuously credited-discovery, non-mutating run stops at
  `implementNowAfter + staleIterations = 12 + 5 = 17`.
- Termination diagnostic: `<CAPABILITY>_NO_PROGRESS` → **`IMPLEMENT_NO_PROGRESS`** (and
  `FIX_NO_PROGRESS` for the FIX capability).
- `termination=no_progress` (`terminationNoProgress` in `internal/ollamaagent/protocol.go`).
- `repository_mutations=0` for a stalled discovery run.
- Disposition: retryable-incomplete — the trailing `; a retry may succeed` marker keeps the
  failure classifier at **CONTINUE** (resumable, not a hard block); returned as
  `*noProgressError` ("a resumable continuation (retry, do not block)").

**CLEF-014 row (from the governing plan, reproduced in CONV-001 §4):**

| Run | Capability | staleIterations | iterations at stop | mutations |
| --- | --- | --- | --- | --- |
| **CLEF-014** | IMPLEMENT | 5 | 17 | 0 |

The CLEF-014 observed stop lands exactly on `12 + 5 = 17` with zero mutations — the
signature the baseline predicts for a bounded-but-not-enforced run.

---

## 4. AFTER — CONV-004 enforcement (per CONV-005 verification)

CONV-004 adds a **pre-mutation convergence guard** to `executePhased`. Once
`st.mutationConvergenceRequired(iteration)` holds (discovery window closed and stale
allowance elapsed with no observed mutation) and the request is a
`nonMutatingRepositoryTool`, the call is **denied** (`RecordDenied`) and answered with
`implementConvergenceCorrection`. In effect:

- Non-mutating repository tools — `read_file`, `list_files`, `search_files`, and
  non-mutating `run_command` — are **denied** once the discovery window has closed and the
  stale allowance has elapsed with no mutation.
- **Mutation tools remain available**, so the model is forced either to attempt a mutation
  or to return a truthful `needs_human` / `failed` outcome.
- A still-non-converging run then **falls through to the unchanged no-progress
  termination** and reaches the same bounded terminal: `*noProgressError`,
  `termination=no_progress`, at `implementNowAfter + maxNoProgressIterations`.
- `mutationConvergenceRequired` returns `false` once `st.mutationObserved` is set, so a
  mutating run is never denied by the enforcement (no regression to the mutating path).

**After-side evidence base (stated honestly):** the above is grounded in the **reused
in-repo CONV-005 verification** (`docs/reports/implementation-convergence/CONV-005-regression-safety.md`)
— its code inspection of `orchestrate.go` / `state.go` and its discriminating test
`TestConvergenceEnforcementFiresBeforeStaleTermination` (asserting at least one
non-mutating repository tool request is denied before the run terminates, while
`*noProgressError` still bounds the run at `implementNowAfter + maxNoProgressIterations`).

**Operator-authorization position on live after-side evidence:** no **live** CLEF-014
re-run was performed or is authorized by the plan (CONV-006 Mutation Targets: "None in
`sop-decision-adapters` (external, read-only). No CLEF-014 retry is authorized by this
plan."). No operator-supplied live after-side evidence was provided for this stage, so
none is asserted. The after column is grounded in the in-repo CONV-005 evidence alone; no
run is fabricated or implied.

---

## 5. Convergence or termination? Fail-closed and retryable?

**Converged or terminated?** Under the after (CONV-004) harness, the enforcement does not
force a mutation — it removes the read-only escape after the discovery window closes and
the stale allowance elapses. The run therefore either **(a) converges**, in the sense of
making a **mutation attempt** or emitting a **concrete blocker** (`needs_human` / `failed`),
or **(b) terminates** at the same bounded terminal when it still cannot converge. For the
compared CLEF-014 observed run (an unmutated IMPLEMENT run), the outcome class is
**termination**: it stopped at iteration 17 with `repository_mutations=0`.

**Fail-closed and retryable?** Yes, on both sides:

- **Fail-closed:** termination is the `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS` guard;
  FINALIZE remains terminal (all tool requests denied) and the default `!policy.Allows(name)`
  → deny branch is untouched (CONV-005 §4.3).
- **Retryable:** the `termination=no_progress` diagnostic retains the `; a retry may
  succeed` marker and `*noProgressError`'s resumable-continuation disposition; nothing in
  the enforcement clears the stale counter or resets a retry (CONV-005 §4.1). **No silent
  retry reset** occurs; the retryable-incomplete (CONTINUE) disposition is preserved.

---

## 6. Non-modification and budget assertions

- **Preserved CLEF-014 evidence not rewritten.** No CLEF-014 file (`attempt.txt`,
  `trace.json`, `classification.json`, `report.md`) was read, written, or otherwise touched.
  The external repository is out of this plan's mutation scope, and this stage wrote no file
  there.
- **CLEF-014 task definition not modified.** The repaired CLEF-014 task/context is reused
  **unchanged** on both sides; this stage edits no CLEF-014 task definition.
- **No Clef production code changed.** The only file this stage writes is this report;
  Production-Change Scope is "None".
- **No budget increase.** `SOP_OLLAMA_STALE_ITERATIONS` stays at its default of **5**; no
  increase to any iteration, stale, tool-call, or retry budget; no new budget knob is added.
- **No behavioral change beyond CONV-004.** The only production-behavior change on the
  after side is CONV-004's pre-mutation enforcement (CONV-005 §1); no budget, approval,
  lifecycle, retry, or provider-selection change rides along.

---

## 7. Limitations

1. **No raw-artifact read of CLEF-014.** The preserved CLEF-014 artifacts are external and
   were not accessible from this repository; the CLEF-014 side rests on the in-repo
   authoritative record (governing plan observed-run row, reproduced in CONV-001 §4), not on
   a raw-artifact comparison. This is stated rather than papered over.
2. **No live after-side CLEF-014 re-run.** A live re-run is not authorized by the plan and
   was not performed; the after side rests on the reused in-repo CONV-005 evidence.
3. **Budget neutrality by construction, not by re-measurement.** Both sides are reported at
   `staleIterations = 5` / bound 17; the value is the documented default and is not
   re-derived from a live CLEF-014 measurement.

---

## 8. Result

Using the **same repaired CLEF-014 task/context** and the **same default budget**
(`SOP_OLLAMA_STALE_ITERATIONS = 5`, bound `17 = implementNowAfter 12 + staleIterations 5`,
**not increased**), the harness **before** CONV-004 is bounded but not enforced (credited
run stops at 17 with `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS`, `termination=no_progress`,
`repository_mutations=0`, retryable-incomplete; CLEF-014 observed row: IMPLEMENT,
staleIterations 5, iterations 17, mutations 0), and the harness **after** CONV-004 enforces
pre-mutation convergence (non-mutating repository tools denied after the discovery window
closes and the stale allowance elapses, mutation tools still available, mutation attempt or
concrete blocker required; a still-non-converging run reaches the same bounded terminal).
The compared CLEF-014 observed run **terminated** at the bounded terminal; the terminal
remains **fail-closed** and **retryable** (no silent retry reset). The preserved CLEF-014
evidence was not rewritten, its task definition was not modified, and **no Clef production
code was changed** by this task.

---

## 9. Addendum - external CLEF-014 dogfood result (separately authorized)

This addendum records the outcome of the **separately authorized external dogfood**,
performed **after** CONV-006 and **not** part of CONV-006's own read-only execution.
CONV-006 did not execute or retry CLEF-014; the live before/after experiment below was
an explicit, separately authorized operator operation in `sop-decision-adapters`.
(Sections 1-8 above were written at CONV-006 execution time, before the live after-run;
section 7.2 recorded that accurately then.)

Harness under test (verified provenance): `sop-ollama-agent` `80cc54ec49e4ae39a7196d4c30fba36d47c87ffa`
(sha256 `754b6a19f74cc3f23a995628391d5c57e8c44ef944c7df4d406ada93a36e9c7e`), built from
convergence HEAD `80cc54ec`; the runtime path resolves to exactly that binary.
Default budgets unchanged (`SOP_OLLAMA_STALE_ITERATIONS = 5`; no budget change).

BEFORE (preserved pre-fix failure, same unchanged CLEF-014 task):
- `IMPLEMENT_NO_PROGRESS`, discovery-only, zero mutation attempts.
- iterations=17, discovery_inspections=10, repository_mutations=0, changed_files=0,
  tool_calls=16, termination=no_progress, last_action="run_command go test -count=1 ./...".

AFTER (unchanged CLEF-014 task, default budgets, `80cc54ec` harness):
- CLEF-014 PASS (LOCAL_DONE); validation BUILD/UNIT_TEST/LINT all PASS.
- iterations=26, repository_mutations=1, first mutation at iteration 16,
  changed_files=["internal/providers/clef/governance_test.go"].

Result: the pathological trajectory changed. Under the corrected harness, unchanged
CLEF-014 escaped the zero-mutation discovery-only loop, produced its declared mutation
target at iteration 16, and passed - resolving the convergence failure class this report
was written to compare.

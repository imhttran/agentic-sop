# ETOE-007 — Controller parity against the disposable project

**Task:** ETOE-007 — exercise the `sop-controller` against the same disposable SOP
project produced by ETOE-005/ETOE-006 and verify display and delegated-action
parity with the SOP CLI, including pending approvals, without allowing the
controller to become a second authority.

**Status:** BLOCKED (controller side) / UNAVAILABLE — the sibling `sop-controller`
checkout and its runtime are **outside this run's authorized tool root**, so no
controller process was built, served or observed. The SOP-side half of the
contract (authority surface, disposable fixture, recorded pending gate) is
verified from preserved, inspected artifacts. Every controller-dependent criterion
is recorded as **UNAVAILABLE/BLOCKED with the failing probe**, never asserted.

> **Scope and method note.** ETOE-007 is an *observation/refusal* task. No
> orchestrator source (`cmd/sop`, `internal/`) was modified and no SOP state was
> edited by hand. The controller-dependent stages (ETOE-007-RUNTIME, -DISPLAY,
> -ACTIONS, -AUTHZ) require the sibling `sop-controller` repository and its
> runtime; that checkout is **not reachable** from this run's tool surface (probe
> below), exactly as ETOE-003 recorded (C-G1). The report therefore consolidates
> the reachable SOP-side evidence, restates the contract from the SOP-side
> documents that *are* reachable, and records the controller-side observations as
> UNAVAILABLE with the failed probe. This report is the sole repository change.

---

## 1. Reachability probe — exact attempt and outcome (recorded)

The first work item (resolve ETOE-003 C-G1) was to reach the sibling
`sop-controller` checkout and read its four named contract documents. The run's
tool surface exposes no authorized root outside `agentic-sop`, so the probe is
the same class of rejection ETOE-003 §2.1 recorded. Confirmed by direct
inspection of this run's tool root: the repository root is
`/Users/imhttran/agentic-workspace/agentic-sop` and the sibling lives at
`/Users/imhttran/agentic-workspace/projects/sop-controller`, outside it.

| # | Surface attempted | Exact attempt | Outcome |
|---|-------------------|---------------|---------|
| 1 | Explicit sibling root via file tools | `read_file`/`list_files` root `.../projects/sop-controller` | **Rejected:** not an authorized repository root (no such surface offered by this run's tool protocol; all tool paths are relative to the repository root and may not escape it) |
| 2 | Relative path escape | path `../projects/sop-controller` | **Rejected:** path escapes the repository |
| 3 | Shell listing outside root | `ls` of the sibling path | **Rejected:** not admitted by the command allow-list |

**Probe outcome:** the sibling `sop-controller` checkout and its runtime remain
**UNAVAILABLE** to this run. Consequently the four controller-side contract
documents (`docs/architecture/SOP-BOUNDARY.md`, `docs/reference/CLI.md`,
`docs/specs/WORKFLOW.md`, `docs/specs/HUMAN-APPROVAL.md`) are **UNAVAILABLE**
(not read), and every controller-dependent observation below is recorded as
UNAVAILABLE/BLOCKED. This is consistent with ETOE-003 C-G1 and the parent plan's
Assumption *"…If the sibling checkout remains unreachable, the controller-document
reading and the controller-dependent checks are recorded as UNAVAILABLE
findings (not PASS/FAIL), the /healthz/parity criteria are reported BLOCKED for
the controller side…"*.

---

## 2. Provenance

### 2.1 SOP authority surface (verified, reachable)

| Item | Value / reference |
| ---- | ----------------- |
| SOP CLI surfaces used as the comparison baseline | `sop status`, `sop task <id>`, `sop approvals --json`, `sop approval <task-id>`, `sop report`, `sop run` |
| Documented delegation target | `docs/reference/CLI.md` (reachable) |
| Documented approval semantics | `docs/guides/APPROVALS.md` (reachable) |
| Documented controller integration contract (SOP side) | `docs/guides/SOP-CONTROLLER-DASHBOARD.md` (read in full this run) |
| SOP state-ownership / second-authority rule | `docs/architecture/SOP-BOUNDARY.md`; `docs/plans/PLAN-SOP-End-to-End-Reliability.md` line 25 |

### 2.2 Controller revision / serve command (UNAVAILABLE)

| Item | Status |
| ---- | ------ |
| Controller revision (HEAD/branch/worktree) | **UNAVAILABLE** — sibling checkout not reachable this run (probe §1). ETOE-001 §1 carried an `[operator-verified]` revision `ed7df68906f2f14edc8cc36d0de5d151747cc78b` (branch `main`), cited here as prior evidence, not re-verified. |
| Serve command | **UNAVAILABLE (not observed).** Documented (SOP side) as `make run` / `make start` producing `http://127.0.0.1:8080` from the controller repo (`docs/guides/SOP-CONTROLLER-DASHBOARD.md` § 1/§ 3). Not executed this run. |
| Build command | Documented as `go build ./cmd/sop-controller` / `make build` → `.run/sop-controller`. Not executed this run. |

### 2.3 Disposable project used (verified via preserved evidence)

The disposable, deterministic fixture is the ETOE-005 fixture
(`scripts/etoe-005-fixture-setup.sh`, `scripts/etoe-005-fixture-run.sh`,
`scripts/etoe-005-fixture-agent.sh`), driven offline with
`SOP_AGENT_HARNESS=command` / `SOP_AGENT_PROVIDER=command`.

| Scenario | Disposable root | Own state.db | git baseline | Run id | Exit | Stage | Validation |
| -------- | --------------- | ------------ | ------------ | ------ | ---- | ----- | ---------- |
| FIX-SUCCESS | `/private/tmp/etoe-005-success` | `.agent-sdlc/state.db` | `f8960fd` | `run-20261009-053027` | 0 | `PASSED` | `PASS` |
| FIX-FAIL | `/private/tmp/etoe-005-fail` | `.agent-sdlc/state.db` | `1c17271` | `run-20261009-053030` | 1 | `WAITING_FOR_HUMAN` (`AUTO_FIX_EXHAUSTED`, FIX ×3) | `FAIL` |
| FIX-GATE | `/private/tmp/etoe-005-gate` | `.agent-sdlc/state.db` | `78e5143` | `FIX-003` | 1 | `WAITING_FOR_HUMAN` | `UNAVAILABLE` |

Evidence sources: `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/{success,fail,gate}/`
and the consolidated `docs/reports/end-to-end-reliability/ETOE-006-e2e-evidence.md`.
The ETOE-006 gate scenario (`FIX-003`) is the pending gate reused for parity.

### 2.4 Recorded pending gate (inspected, reachable)

From `ETOE-005-execution-evidence/gate/etoe-005-run-records.txt`
(`sop approvals --json` after FIX-GATE):

```json
{
  "version": 1,
  "approvals": [
    {
      "task_id": "FIX-003",
      "kind": "NEEDS_HUMAN",
      "target": "FIX-003",
      "reason": "the requested fixture operation is destructive and irreversible, so it requires authorization",
      "evidence": "the agent reported a boundary that requires a human: the requested fixture operation is destructive and irreversible, so it requires authorization",
      "stage": "WAITING_FOR_HUMAN",
      "disposition": "NEEDS_HUMAN",
      "status": "PENDING",
      "requested_at": "2026-10-09T05:31:19.762621Z",
      "task_status": "PLANNED"
    }
  ]
}
```

The gate is a single **PENDING** approval; `sop approve` was never invoked
(ETOE-006 §5) and the task status remains `PLANNED`/not completed.

---

## 3. PT-item traceability (PT-1…PT-12)

Every ETOE-003 PT item is traced to its matrix row and its observed outcome.
Because the controller is unreachable, **no PT item that requires a running
controller could be executed**; each such item is recorded `UNAVAILABLE/BLOCKED`
with the reason (the §1 probe). No PT item is asserted pass/fail from quotation.

| PT | Matrix row / gap | Planned observation | Outcome this run | Reason |
|----|------------------|---------------------|------------------|--------|
| PT-1 | AN-4, U3, C-G4 | `/healthz` responds without a token; exact response recorded | **BLOCKED** | Controller not buildable/servable (sibling unreachable, §1) |
| PT-2 | AN-1, C-G5 | Non-loopback bind silently rewritten to `127.0.0.1` | **BLOCKED** | Same |
| PT-3 | AN-2, C-G5 | Refuse-to-start without token; gated route 401; `/healthz` reachable | **BLOCKED** | Same |
| PT-4 | AN-3, C-G5 | Non-CSRF POST → 403; nothing recorded SOP-side | **BLOCKED** | Same |
| PT-5 | S-1…S-10, C-G4 | Display parity vs `sop status`/`sop task`/`sop approvals --json`/`sop report` | **UNAVAILABLE** | Controller display not observable; SOP-side baseline recorded (§2.3) |
| PT-6 | D-1…D-6, D-9, C-G4 | Action parity vs hand-run SOP CLI, single sequential driver | **UNAVAILABLE** | No controller action surface reachable |
| PT-7 | PA-4/PA-5, C-G2, C-G3, PA-6 | Observe dashboard during/after parked run | **UNAVAILABLE** | Controller dashboard not observable; SOP-side gate recorded PENDING (§2.4) |
| PT-8 | D-7, PH-1…PH-4, C-G2 | Verify approval-route gating; delegate one decline | **UNAVAILABLE** | Approval action presence unknown (controller unreachable). No decline delegated. |
| PT-9 | D-8, S-9, S-11, PH-6, U6, C-G7 | Second-authority refusal checks | **UNAVAILABLE** | No controller process/route to probe |
| PT-10 | S-8, C-G7 | Routing display-only check | **UNAVAILABLE** | Controller display not observable |
| PT-11 | PA-6, C-G6 | Compare `runs/<id>/gate.json` with `sop approvals --json` | **UNAVAILABLE** | `gate.json` capture requires the parked controller-driven run; only the SOP-side approvals listing is preserved. |
| PT-12 | PH-3, U4, U5, C-G5 | Mobile read-only check on isolated network | **UNAVAILABLE** | No isolated network/device workflow executed |

**Summary:** 0 of 12 PT items executed; 12 recorded UNAVAILABLE/BLOCKED with the
reason. None is asserted as pass.

---

## 4. Display parity (S-1…S-11) vs SOP CLI

Because the controller display could not be rendered, this section records the
**documented** display contract (SOP side) and the SOP CLI baseline that parity
would be compared against. No controller value is asserted equal to any SOP value.

| S | Documented display surface (SOP-side guide) | SOP CLI baseline for comparison | Parity observation |
|---|--------------------------------------------|--------------------------------|--------------------|
| S-1 | `/projects` progress + total/done/running/blocked | `sop status` (project counts) | **UNAVAILABLE** — controller view not rendered |
| S-2 | `/projects/{id}` task states, deps, blocked reason | `sop status`, `sop task <id>` | **UNAVAILABLE** |
| S-3 | `/projects/{id}/tasks/{task}` state, attempts, branch, deps, latest failure | `sop task <id>` | **UNAVAILABLE** |
| S-4 | Activity panel (attempt events) | run artifacts under `.agent-sdlc/runs/<id>/` | **UNAVAILABLE** |
| S-5 | Review panel (findings + severity) | `report.json` / review artifact | **UNAVAILABLE** |
| S-6 | CI / validation panel (build/test/lint + reasons) | `validation.json` (e.g. FIX-FAIL `Status: FAIL`) | **UNAVAILABLE** |
| S-7 | Handoff panel (status, compressor error, carry-forward) | `report.json` / handoff artifact | **UNAVAILABLE** |
| S-8 | Routing data display, no policy mutation | `docs/specs/MODEL-ROUTING.md` (display-only rule) | **UNAVAILABLE** — no controller route observed |
| S-9 | Reads through SOP boundary; browser never reaches SQLite | `docs/guides/SOP-CONTROLLER-DASHBOARD.md` § Security notes (DOC) | **UNAVAILABLE (live)** |
| S-10 | HTMX polling default 3s (`SOP_CONTROLLER_POLL`) | — | **UNAVAILABLE (live cadence)** |
| S-11 | No parallel copy of SOP state; cannot bypass human gates | `docs/architecture/SOP-BOUNDARY.md` § State Ownership (DOC) | **UNAVAILABLE (code-level, C-G7)** |

**Pending-approval visibility (C-G3, PT-7):** the SOP-side gate is a single
PENDING approval for `FIX-003` (§2.4). Whether the dashboard renders it is
**UNAVAILABLE — not assumed**. The reachable SOP-side dashboard guide's
view table lists no approvals view/field (S-1…S-10), consistent with ETOE-003
PA-4 (UNDOC), but absence of the live view cannot be confirmed without a running
controller, so C-G3 is carried forward rather than closed.

**Mismatch/defect list:** none can be recorded — no controller display values
were observed, so no value could be compared. Absence of observation is not
absence of defect.

---

## 5. Delegated-action parity (D-1…D-9) vs SOP CLI

No controller action was triggered (unreachable, §1). The table records the
documented action → SOP CLI mapping (SOP side) and the SOP-side state effect that
a hand-run of the same CLI command produces (from ETOE-006 evidence); the
controller-side effect is UNAVAILABLE.

| D | Documented action | SOP CLI command | SOP-side effect (hand-run, from ETOE-006) | Controller effect |
|---|-------------------|-----------------|-------------------------------------------|-------------------|
| D-1 | Run | `sop run` | stage transitions START→…→PASSED/WAITING_FOR_HUMAN | **UNAVAILABLE** |
| D-2 | Resume | `sop resume` | acts on interrupted work | **UNAVAILABLE** |
| D-3 | Validate | `sop validate` | `validation.json` build/test/lint status | **UNAVAILABLE** |
| D-4 | Review | `sop review` | configured review engine | **UNAVAILABLE** |
| D-5 | Report | `sop report` | summary of latest run (read-only) | **UNAVAILABLE** |
| D-6 | Retry | `sop retry <task>` | requeue a single `BLOCKED` task | **UNAVAILABLE** |
| D-7 | Approval delegation | `sop approvals --json` → `sop approve`/`sop decline` | resolves only the raised gate (DOC) | **UNAVAILABLE** — no approval action observed |
| D-8 | Direct state mutation | (none — documented absence) | — | **UNAVAILABLE** — no route probed |
| D-9 | Command timeout | `SOP_CONTROLLER_COMMAND_TIMEOUT` default 15m | (documented) | **UNAVAILABLE** |

**Pending-approval handling (PT-7/PT-8/PT-11):**

- The FIX-003 gate remains **PENDING** in the SOP-side record; viewing or
  re-running does not resolve it (ETOE-006 §5, §2.4 here). This is the SOP-side
  half verified; the controller's rendering/resolution path is UNAVAILABLE.
- **No decline was delegated** (no controller approval route reachable); **no
  approve was delegated** (no operator authorization recorded, and no controller
  route observed). Absence of an approval action is recorded here as **C-G2's
  carried-forward resolution**, not as a confirmed feature absence.
- **PT-11 (`gate.json` vs `sop approvals --json`):** the `sop approvals --json`
  listing is preserved (§2.4); the corresponding `.agent-sdlc/runs/<id>/gate.json`
  capture requires the controller-driven parked run and is **UNAVAILABLE**.
  C-G6 remains open with the same single-producer caveat (ETOE-002 G1).

---

## 6. Permission findings (auth/network boundary, AN-1…AN-8)

The documented auth/network boundary (SOP side) is restated; **live enforcement
is UNAVAILABLE** (server unreachable).

| AN | Documented contract (SOP-side guide) | Live observation |
|----|--------------------------------------|------------------|
| AN-1 | Loopback by default; non-loopback bind silently rewritten unless `SOP_CONTROLLER_ALLOW_NETWORK=true` | **UNAVAILABLE (BLOCKED — server not started)** |
| AN-2 | Network mode requires a token; refuses to start without one; every route except `/healthz`/`/static/` token-gated; wrong/missing token → `401 access token required` | **UNAVAILABLE (BLOCKED)** |
| AN-3 | State-changing requests are `POST` + double-submit CSRF; otherwise `403 invalid CSRF token` | **UNAVAILABLE (BLOCKED)** |
| AN-4 | `/healthz` exists and is exempt from the token gate | **UNAVAILABLE (BLOCKED — PT-1 not executable)** |
| AN-5 | LAN transport is plain HTTP (documented trade-off) | **UNAVAILABLE** |
| AN-6 | No secrets/env rendered into pages; access token is a bearer credential, rotated by restart | **UNAVAILABLE** |
| AN-7 | `SOP_BIN`/PATH determines which SOP executes; stale provider env breaks commands | **UNAVAILABLE** |
| AN-8 | Network exposure does not transfer workflow authority; a token holder can only *trigger* SOP commands | **UNAVAILABLE** |

**Who can trigger SOP commands via the controller and how that affects workflow
authority (SOP-side reasoning, documentation basis):** a token holder on the
controller's network can *trigger* the six documented SOP commands (D-1…D-6),
but workflow authority does not move: every triggered command executes inside
SOP's own gates, budgets and approval boundaries, and the controller is
documented to hold no second source of truth and no direct state-write path
(`docs/guides/SOP-CONTROLLER-DASHBOARD.md` § Security notes; State Ownership
rule). This authority statement is **documentation-level** only; the code-level
enforcement (C-G7) and the live token/CSRF enforcement (C-G5) remain UNAVAILABLE.

---

## 7. Second-authority assessment

SOP remains the sole workflow authority. SOP-side, every documented delegation
terminates in an SOP CLI command (D-1…D-6), and SOP-side state transitions occur
only through `sop run`. **No controller-driven mutation was attempted or observed
this run** (the controller was unreachable), so the criterion *"the controller
never mutates authoritative state except through SOP's approved interface"* is
**not contradicted and not independently observed**: it is UNAVAILABLE with the
reason that the controller process and its routes could not be exercised. The
SOP-side state used for comparison (the disposable fixtures) was not mutated by
this task; the only reachable state reads were of preserved run artifacts.

---

## 8. Acceptance-criteria mapping (ETOE-007)

| # | ETOE-007 acceptance criterion | Evidence / status |
|---|-------------------------------|-------------------|
| 1 | Report names the controller revision/serve command and the disposable project, and records the exact SOP CLI commands compared against (`sop status`, `sop task <id>`, `sop approvals --json`, `sop approval <task-id>`, `sop report`, ETOE-006 run artifacts) | §2.1 (CLI baseline), §2.2 (controller revision/serve **UNAVAILABLE** with failing probe), §2.3 (disposable project + run IDs), §2.4 (pending gate). SOP CLI set recorded exactly. — MET for the SOP side; controller revision/serve recorded UNAVAILABLE with reason |
| 2 | Every PT-1…PT-12 traced to its matrix row and observed outcome, unexecuted items labeled UNAVAILABLE/BLOCKED with a reason | §3 — all 12 traced; 12 UNAVAILABLE/BLOCKED with the §1 reason |
| 3 | The four sop-controller contract documents read (with divergences) or explicitly recorded UNAVAILABLE with the failing probe | §1 — explicitly UNAVAILABLE (sibling unreachable), failing probe recorded; no divergence asserted |
| 4 | No agentic-sop source, configuration, or `.agent-sdlc` state modified; no parallel SOP mutations against the same disposable state database | §10 mutation statement — only this report added; no `.agent-sdlc` edits; no SOP commands executed against any state database this run; no parallel writers |
| 5 | No approval granted or bypassed; the pending FIX-003 gate remains the gate ETOE-006 recorded unless operator authorization is recorded | §2.4/§5 — gate remains PENDING; `sop approve` never invoked; no decline delegated; no operator authorization recorded |
| ETOE-007-RUNTIME criteria (`/healthz`, loopback rewrite, token gate, CSRF) | §3 PT-1…PT-4, §6 AN-1…AN-4 — **BLOCKED** (controller runtime unavailable) |
| ETOE-007-DISPLAY criteria (display parity, pending-approval visibility, no mutation) | §4 — display parity **UNAVAILABLE**; pending-approval visibility **UNAVAILABLE (C-G3 carried)**; no display check mutated state (none ran) |
| ETOE-007-ACTIONS criteria (action parity, gate.json vs approvals, single driver) | §5 — action parity **UNAVAILABLE**; gate.json vs approvals **UNAVAILABLE** (C-G6 open); no parallel SOP mutation occurred |
| ETOE-007-AUTHZ criteria (auth/network, second-authority refusals, state.db hash) | §6/§7 — **BLOCKED/UNAVAILABLE**; no state.db hash comparison possible (no controller host exercised); C-G1 UNAVAILABLE |
| ETOE-007-REPORT criteria (gap closure, mutation statement, validation record) | §9 (gap closure), §10 (mutation), §11 (validation record) |

### Overall ETOE-007 top-level acceptance criteria

| Criterion | Status |
|-----------|--------|
| Controller display and delegated actions are shown to match the SOP CLI for the same disposable project, including pending approvals | **BLOCKED** — controller unreachable; SOP-side baseline and pending gate recorded (§2.3, §2.4), controller side UNAVAILABLE |
| The controller never mutates authoritative state except through SOP's approved interface | **UNAVAILABLE** — not independently observed; not contradicted; SOP-side documentation basis only (§7) |
| `/healthz` and SOP state/action parity are verified in the disposable environment | **BLOCKED** — `/healthz` not reachable (no running controller); SOP-side state recorded from preserved disposable-run artifacts (§2.3) |

---

## 9. ETOE-003 gap closure (C-G1…C-G7)

No gap is silently dropped; none is asserted closed without observation.

| Gap | Status after ETOE-007 | Reason |
|-----|------------------------|--------|
| C-G1 (sibling contract documents unread) | **UNAVAILABLE — carried forward** | Sibling checkout unreachable (§1); failed probe recorded; content still never read |
| C-G2 (approval-action asymmetry) | **UNAVAILABLE — carried forward** | No controller approval route reachable to confirm presence/absence; no approve/decline delegated |
| C-G3 (pending-approval visibility on dashboard) | **UNAVAILABLE — carried forward** | Dashboard not rendered; SOP-side gate PENDING recorded, view presence not observed |
| C-G4 (runtime display/action readiness) | **BLOCKED — carried forward** | Controller not built/served; PT-1/PT-5/PT-6 not executable |
| C-G5 (live auth/network enforcement) | **BLOCKED — carried forward** | No isolated-network server; PT-2/PT-3/PT-4/PT-12 not executable |
| C-G6 (gate.json single producer, no covering test; live gate parity) | **UNAVAILABLE — carried forward** | PT-11 requires the controller-driven parked run; only SOP-side approvals listing preserved |
| C-G7 (code-level second-authority enforcement) | **UNAVAILABLE — carried forward** | Sibling code unreachable; refusal checks PT-6/PT-9 not executable |

---

## 10. Mutation statement

- The **only** repository change made by ETOE-007 is this report file,
  `docs/reports/end-to-end-reliability/ETOE-007-controller-parity.md`.
- No orchestrator source (`cmd/sop`, `internal/`), configuration, or CLI behavior
  was changed. No `.agent-sdlc` state was edited by hand.
- No production checkout was used: all SOP-side evidence is read from preserved,
  disposable ETOE-005/ETOE-006 artifacts outside any production checkout.
- No parallel SOP mutations ran against any disposable state database; this task
  executed **no** SOP commands against any project state database this run.
- Nothing was committed or pushed. Pre-existing worktree entries are user-owned
  and were left untouched.

---

## 11. Required-validation record

Per the parent plan, the validation set distinguishes *plan-required* commands
from SOP's configured gate (build/test/lint + gofmt emptiness).

| Command | Kind | Status |
|---------|------|--------|
| `gofmt -l .` (emptiness test) | SOP-gate + this run's contract | **Ran — exit 0** (see structured outcome) |
| `go build ./...` | SOP-gate + this run's contract | **Ran — exit 0** |
| `go vet ./...` | SOP-gate + this run's contract | **Ran — exit 0** |
| `go test ./...` | SOP-gate + this run's contract | **Ran — exit 0** |
| `go test -race -count=1 ./...` | plan-required, not gate-enforced | **UNAVAILABLE** — not admitted by this run's command allow-list |
| `git diff --check` | plan-required, not gate-enforced | **UNAVAILABLE** — not admitted by this run's command allow-list; additive-only change attested in §10 |

No command above is reported as passed unless it ran. Exact per-command results
are confirmed by the structured outcome of this run and SOP's independent gate.

---

## 12. Findings / hand-off (ETOE-009, ETOE-010)

| Finding | Status | Matrix row | Gap |
|---------|--------|------------|-----|
| Controller runtime, `/healthz`, display/action parity not observed (sibling unreachable) | UNAVAILABLE/BLOCKED | PT-1…PT-6, S-1…S-10, D-1…D-9 | C-G1, C-G4 |
| Pending-approval visibility on dashboard not observed; SOP-side gate PENDING | UNAVAILABLE | PT-7, PA-4/PA-5 | C-G2, C-G3 |
| Auth/network + CSRF + token enforcement not exercised | BLOCKED | PT-2…PT-4, AN-1…AN-8 | C-G5 |
| Second-authority refusals not exercised; no direct write path probed | UNAVAILABLE | PT-9, D-8, S-9/S-11, PH-6 | C-G7 |
| `gate.json` vs `sop approvals --json` live parity | UNAVAILABLE | PT-11, PA-6 | C-G6 |
| Sibling contract documents read | UNAVAILABLE (failed probe) | — | C-G1 |

**For ETOE-009 (metrics):** no controller latency/health metric was observed; the
only quantified SOP-side facts are the disposable run/exit/stage/validation
values in §2.3. No readiness or metric is fabricated.

**For ETOE-010 (final report):** ETOE-007 does not close C-G1…C-G7; every one is
carried forward with the status above and requires a tool surface authorized for
the sibling `sop-controller` checkout plus a controller runtime to close.

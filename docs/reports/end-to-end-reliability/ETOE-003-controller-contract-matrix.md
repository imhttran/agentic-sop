# ETOE-003 — Controller integration audit (controller contract matrix)

Task: **ETOE-003** — Controller integration audit: read-only state display, delegated actions, pending approval visibility, phone-safe approval workflow, and the auth/network boundary. **Read-only audit.** May run in parallel with ETOE-002 and ETOE-004.

The audited system is the **sop-controller** sibling repository (`/Users/imhttran/agentic-workspace/projects/sop-controller`); SOP (`agentic-sop`) remains the sole workflow authority. This report is the sole repository change intended by ETOE-003.

## 1. Scope and method

- **Read-only statement:** this audit executed **zero** controller processes, zero network/phone workflows, and zero SOP-state actions. Its only intended repository change is this report file. No file in the sibling `sop-controller` checkout was read or modified this run (§2.1 records why and proves it was attempted), and no `.agent-sdlc` file was modified.
- **Audited surfaces:** the controller's documented behavior, as documented **on the SOP side of the boundary** — the in-repo controller-integration contracts that define what the controller may observe and command:
  - `docs/guides/SOP-CONTROLLER-DASHBOARD.md` (dashboard wiring guide: display views, delegated buttons, phone/LAN mode, auth/network rules — read in full this session);
  - `docs/architecture/SOP-BOUNDARY.md` (SOP ownership, state-ownership rule, routing display boundary, human authority — read in full this session);
  - `docs/guides/APPROVALS.md` § *Controller and MCP usage* (the documented controller client protocol for pending gates — read in full this session);
  - `docs/reference/CLI.md` (the delegation target: SOP's commanded surface — read in full this session);
  - corroborating in-repo notes located by exact-line content search: `docs/plans/PLAN-SOP-End-to-End-Reliability.md:25` (binding invariant), `docs/specs/MODEL-ROUTING.md:337`, `docs/specs/PROVIDERS.md:354`, `internal/config/config.go:47,780`, `docs/requirements/PRD-Phase-4-Provider-Runtime.md:83`.
- **Basis statement (no assumptions):** the controller's **own** contract documents (the four sop-controller paths named by ETOE-001 §5) could not be read this run — the sibling checkout is outside this run's authorized tool root (probe outcomes in §2.1). They are therefore **UNAVAILABLE**, not assumed. Every matrix assertion is cited from a reachable, in-repo document that documents the controller's designed behavior; anything beyond that documented basis is marked **UNDOC** or **UNAVAILABLE** and never asserted. This report's DOC rows are statements of the documented integration contract; they are *not* fresh observations of controller code or a running controller.
- **Verification-level legend (satisfies acceptance criterion 3):**
  - **DOC** — asserted from a reachable repository document read this session (documented behavior; SOP-side integration contract).
  - **UNDOC** — absent from every reachable controller-integration document; recorded as a gap candidate, never asserted present (treated as UNAVAILABLE for the capability).
  - **UNAVAILABLE** — cannot be verified in this run: either the sibling checkout was not an authorized tool root (probes §2.1) or the capability requires runtime execution (deferred to ETOE-007's disposable environment by plan design). **No mobile or network readiness is asserted beyond these labels.**
  - **R** — requires a real disposable run; deferred to ETOE-007 (controller parity) / ETOE-005 (authority-side live run), never executed here.
- **Method:** document reading plus exact-line content search inside `agentic-sop` only. Authority-side approval/gate semantics are covered by ETOE-002 and are cited (not re-derived) here. Cross-checks are bidirectional: every controller-documented action is matched against SOP's CLI list, and SOP-side commands absent from the controller's documented surface are enumerated (§2.5).

## 2. Evidence inventory (stage E3-S1)

### 2.1 Reachability probe — exact attempt and outcome (recorded)

Per the plan, the first work item was an explicit probe of the sibling checkout. Three surfaces were attempted; all failed; the exact attempts and results are:

| # | Surface attempted | Exact attempt | Outcome |
|---|-------------------|---------------|---------|
| 1 | File tools with an explicitly requested sibling root | `read_file` with root `/Users/imhttran/agentic-workspace/projects/sop-controller` | **Rejected:** “unauthorized repository root … is not an authorized repository root” |
| 2 | Directory listing via relative path escape | `list_files` with path `../projects/sop-controller` | **Rejected:** “path escapes the repository” |
| 3 | Git subcommand override | command `git -C /Users/imhttran/agentic-workspace/projects/sop-controller rev-parse HEAD` | **Rejected:** “command not allowed: -C/--git-dir/--work-tree overrides are not allowed” |

**Probe outcome:** at the ETOE-003 execution the tool surface **is confined to the `agentic-sop` repository root**; the sibling sop-controller checkout was **not reachable** and no controller file was read this run. This supersedes, *for this run only*, ETOE-001 §1's operator-verified reachability correction (which described the ETOE-001 harness): the §2.1 rejections are direct observations of this run's boundary. Direct controller inspection is therefore classified **UNAVAILABLE** for this run, and no controller capability is asserted above its documented, reachable basis.

### 2.2 Controller revision

- Fresh observation of the sibling HEAD/worktree: **UNAVAILABLE** (probe outcome §2.1; no authorized root, no git command surface for siblings).
- Prior evidence, cited **as prior evidence and not re-verified here** (`[operator-verified]` at ETOE-001 §1): sop-controller HEAD `ed7df68906f2f14edc8cc36d0de5d151747cc78b`, branch `main` in sync with `origin/main`, worktree clean. All matrix rows describe documented contracts, not a freshly observed controller tree.

### 2.3 Contract-document inventory

| Document | Repository | Status | Reason / use |
|---------|------------|--------|--------------|
| `docs/architecture/SOP-BOUNDARY.md` | sop-controller | **UNAVAILABLE** | sibling not an authorized tool root this run (§2.1 probe 1); the controller-side copy was named by ETOE-001 §5 but never read by any stage |
| `docs/reference/CLI.md` | sop-controller | **UNAVAILABLE** | same |
| `docs/specs/WORKFLOW.md` | sop-controller | **UNAVAILABLE** | same |
| `docs/specs/HUMAN-APPROVAL.md` | sop-controller | **UNAVAILABLE** | same |
| `docs/guides/SOP-CONTROLLER-DASHBOARD.md` | agentic-sop | **DOC** (read in full this session) | the dashboard wiring guide; primary documented integration contract for display, commands, phone/LAN mode, auth/network |
| `docs/architecture/SOP-BOUNDARY.md` | agentic-sop | **DOC** (read in full this session) | SOP ownership and the consumer state-ownership rule |
| `docs/guides/APPROVALS.md` | agentic-sop | **DOC** (read in full this session) | pending-gate visibility and the controller client protocol (§ *Controller and MCP usage*) |
| `docs/reference/CLI.md` | agentic-sop | **DOC** (read in full this session) | the delegation target: SOP's commanded surface |
| `docs/plans/PLAN-SOP-End-to-End-Reliability.md` | agentic-sop | **DOC** (exact-line search hit :25) | binding second-authority invariant; safety invariants; required-validation list |
| `docs/specs/MODEL-ROUTING.md`, `docs/specs/PROVIDERS.md`, `internal/config/config.go`, `docs/requirements/PRD-Phase-4-Provider-Runtime.md` | agentic-sop | **DOC** (exact-line content-search hits: :337, :354, :47/:780, :83) | corroborating controller boundary notes (display-only routing; provider consumer; config block consumed by the controller) |
| `internal/cli/run.go:868` (gate.json producer) | agentic-sop | cited from recorded ETOE-002 §2.3 P9 / §3 G1 (repo-wide search recorded there) | authority-side gate artifact caveat (gap C-G6) |

### 2.4 State-display enumeration (documented)

From `docs/guides/SOP-CONTROLLER-DASHBOARD.md` § *What you'll see* (views) and § *Run it*:

| S | Documented display surface | Citation |
|---|----------------------------|----------|
| S1 | `/projects` — progress and total / done / running / blocked counts | dashboard guide § What you'll see |
| S2 | `/projects/{id}` — task states, dependencies, why a task is blocked | same |
| S3 | `/projects/{id}/tasks/{task}` — state, attempts, branch, deps, latest failure | same |
| S4 | Activity panel — attempt events; raw transcripts secondary | same |
| S5 | Review panel — review findings + severity from run artifacts | same |
| S6 | CI / validation panel — build / test / lint results and reasons | same |
| S7 | Handoff panel — handoff status, compressor error, carry-forward facts | same |
| S8 | Routing data display: `sop-controller` MAY display routing data but MUST NOT own routing policy | `docs/architecture/SOP-BOUNDARY.md` (§ The Model-Routing Boundary); `docs/specs/MODEL-ROUTING.md:337` |
| S9 | Read path: the browser never reaches SQLite — all reads go through the server's SOP boundary, and state is read-only | dashboard guide § Security notes |
| S10 | Freshness: task list, activity, review, CI, handoff refresh automatically via HTMX polling (default every 3s, `SOP_CONTROLLER_POLL`) | dashboard guide § What you'll see; § Environment reference |

### 2.5 Delegated-action enumeration and SOP-CLI cross-check (bidirectional)

Documented controller command surface — `docs/guides/SOP-CONTROLLER-DASHBOARD.md` § *Commands you can trigger*: “Buttons map to real SOP CLI calls (via `SOP_BIN`)”:

| D | Documented button | Runs (documented) | Character |
|---|-------------------|-------------------|-----------|
| D1 | Run | `sop run` | continue / execute the active plan (state-advancing) |
| D2 | Resume | `sop resume` | act on interrupted work |
| D3 | Validate | `sop validate` | run configured build / test / lint (verification) |
| D4 | Review | `sop review` | run the configured review engine (verification) |
| D5 | Report | `sop report` | summary of the latest run (read-only informational) |
| D6 | Retry | `sop retry <task>` | requeue a single `BLOCKED` task (state-advancing) |

**Cross-check forward (controller → SOP):** every documented controller action exists in the delegation target's command list (`docs/reference/CLI.md` § Commands: `sop run`, `sop resume`, `sop validate`, `sop review`, `sop report`, `sop retry`). **No controller-documented action lacks an SOP-CLI counterpart** — all six delegate to the SOP CLI via `SOP_BIN` (dashboard guide § Prerequisites: “whatever `sop` is on the dashboard process's `PATH` is what actually executes”). The documented surface is a strict subset of SOP's CLI: the controller documents no state-writing path of its own (“The dashboard never mutates state directly”, § *Commands you can trigger*).

**Cross-check reverse (SOP → controller), documented-absent set:** SOP CLI commands documented in `docs/reference/CLI.md` but **not** exposed through the controller's documented button set: `sop approve`, `sop decline`, `sop approvals [--json]`, `sop approval <task-id>`, `sop commit`, `sop pr`, `sop reconcile`, `sop continue --check`, `sop plan activate` / `plan supersede` / `plan complete` / `plan historicalize`, `sop task complete --external`, `sop init`, `sop plan`, `sop tasks`, `sop prompt`, `sop retry --all` / `--force`, `sop memory add`, `sop retrieve`, `sop providers [--models]`, `sop task <id>`, `sop version` / `help` / `eval` / `mcp`. Two asymmetries are recorded as findings rather than resolved by assumption:

- **C-G2:** approval *actions* (`sop approve` / `sop decline`) are documented as delegable to a controller by `docs/guides/APPROVALS.md` § *Controller and MCP usage* (“Discover pending gates with `sop approvals --json`, inspect one with `sop approval <task-id>`, and resolve it with `sop approve`/`sop decline` … No SOP policy — routing, providers, approval outcome — moves into the client”), but **no approval button appears in the dashboard guide's documented surface** (its § *Commands you can trigger* table lists only D1–D6). Whether the deployed controller offers approval actions is therefore **undocumented in reachable evidence** → UNAVAILABLE (§3.2 D-7, §3.4 PH-1, gap C-G2).
- **C-G3:** a dedicated pending-approvals *view* is likewise absent from the documented view table (§2.4 S1–S10); pending-gate visibility through the dashboard is undocumented → UNAVAILABLE (§3.3 PA-4).

### 2.6 UNAVAILABLE set with reasons (summary inventory)

| Item | Status | Reason |
|------|--------|--------|
| U1 Contents of the four sop-controller contract documents | **UNAVAILABLE** | sibling checkout not an authorized tool root this run (§2.1, all three probe surfaces rejected) |
| U2 Fresh controller revision/worktree observation | **UNAVAILABLE** | same; only ETOE-001's operator-verified record is citable (§2.2) |
| U3 Controller runtime readiness (build/serve, live `/healthz`, live HTMX polling) | **UNAVAILABLE** | no execution this run (§1); deferred to ETOE-007 |
| U4 Live network-mode serving and live token/CSRF enforcement | **UNAVAILABLE** | no execution this run; server/client behavior documented but not exercised |
| U5 Live mobile (iPhone) workflow including any approval action from a phone | **UNAVAILABLE** | no device/network workflow this run; approval actions additionally undocumented on the dashboard surface (C-G2) |
| U6 Code-level second-authority enforcement (no direct state write path; documented refusals) | **UNAVAILABLE** | sibling code unreachable (§2.1); documented posture cited, code unverified |
| U7 Pending-approval display on the dashboard | **UNDOC → UNAVAILABLE** | absent from the documented view table (C-G3); controller-side HUMAN-APPROVAL doc unreachable (U1) |
| U8 `go test -race -count=1 ./...` and `git diff --check` execution status | recorded in §7 | required by the parent plan but **not** enforced by SOP's configured gate |

## 3. Contract matrix (stage E3-S2)

Columns: **Level** per the §1 legend; every row carries its documented basis and a gap column. DOC rows are documented integration contracts (SOP-side); nothing is asserted from assumption.

### 3.1 State display

| ID | Contract | Documented basis (citation) | Level | Gap |
|----|----------|-----------------------------|-------|-----|
| S-1 | Project list view with progress counts (total/done/running/blocked) | dashboard guide § What you'll see (`/projects`) | DOC | live rendering: C-G4 |
| S-2 | Project view: task states, dependencies, blocked reason | same (`/projects/{id}`) | DOC | C-G4 |
| S-3 | Task view: state, attempts, branch, deps, latest failure | same (`/projects/{id}/tasks/{task}`) | DOC | C-G4 |
| S-4 | Activity: attempt events from runs; raw transcripts secondary | same (Activity row) | DOC | C-G4 |
| S-5 | Review findings + severity from run artifacts | same (Review row) | DOC | C-G4 |
| S-6 | CI/validation: build / test / lint results and reasons | same (CI / validation row) | DOC | C-G4 |
| S-7 | Handoff: status, compressor error, carry-forward facts | same (Handoff row) | DOC | C-G4 |
| S-8 | Routing data display permitted; routing policy ownership forbidden | `docs/architecture/SOP-BOUNDARY.md` § The Model-Routing Boundary; `docs/specs/MODEL-ROUTING.md:337` | DOC | C-G7 (display-only enforcement) |
| S-9 | All state reads through the server's SOP boundary; browser never reaches SQLite; state read-only | dashboard guide § Security notes | DOC | C-G7 |
| S-10 | Auto-refresh via HTMX polling, default 3s (`SOP_CONTROLLER_POLL`) | dashboard guide § What you'll see; § Environment reference | DOC | live cadence: C-G4 |
| S-11 | **Second-authority prohibition on display:** the consumer “MUST NOT keep a parallel copy of SOP's task state” — the dashboard observes persisted state (`.agent-sdlc/state.db`, read-only) and documents “It keeps no second source of truth” | `docs/architecture/SOP-BOUNDARY.md` § State Ownership; dashboard guide (header + § Commands you can trigger) | DOC | C-G1, C-G7 |

### 3.2 Delegated actions

| ID | Contract | Documented basis (citation) | Level | Gap |
|----|----------|-----------------------------|-------|-----|
| D-1 | Run delegation: `Run` invokes the SOP CLI (`sop run`) through `SOP_BIN`; SOP executes the plan | dashboard guide § Commands you can trigger; `docs/reference/CLI.md` `sop run` | DOC | C-G4 (live parity) |
| D-2 | Resume delegation: `Resume` invokes `sop resume` (act on interrupted work) | dashboard guide § Commands you can trigger; `docs/reference/CLI.md` `sop resume` | DOC | C-G4 |
| D-3 | Validate delegation: `Validate` invokes `sop validate` (build/test/lint per `.agent-sdlc/config.yaml`) | same; `docs/reference/CLI.md` `sop validate` | DOC | C-G4 |
| D-4 | Review delegation: `Review` invokes `sop review` (configured engine; the model never decides the verdict) | same; `docs/reference/CLI.md` `sop review` | DOC | C-G4 |
| D-5 | Report delegation: `Report` invokes `sop report` (informational, read-only) | same; `docs/reference/CLI.md` `sop report` | DOC | — (observation-only) |
| D-6 | Retry delegation: `Retry` invokes `sop retry <task>` (single `BLOCKED` task; SOP-owned retry budget) | same; `docs/reference/CLI.md` `sop retry`; `docs/specs/RECOVERY.md` (ref.) | DOC | C-G4 |
| D-7 | Approval delegation on a controller: documented as client-delegable (`sop approvals --json` → `sop approve`/`sop decline`; MCP `sop_approve`/`sop_decline`), resolving **only the gate SOP raised**; approve “never completes a task and never bypasses validation, review, or the quality gate” | `docs/guides/APPROVALS.md` § Controller and MCP usage; `docs/reference/CLI.md` `sop approve` | DOC (permission to delegate) / **UNDOC on the dashboard surface** (no documented button) | **C-G2** (presence + phone-safety unverified) |
| D-8 | Direct state mutation by the controller: **not part of the documented interface** — “The dashboard never mutates state directly, is double-click safe, and is restart-safe” | dashboard guide § Commands you can trigger; § Security notes | DOC (documented absence) | C-G7 (code-level enforcement) |
| D-9 | Command timeout bound for a single SOP command (`SOP_CONTROLLER_COMMAND_TIMEOUT`, default 15m) | dashboard guide § Environment reference | DOC | C-G4 |

### 3.3 Pending-approval visibility

| ID | Contract | Documented basis (citation) | Level | Gap |
|----|----------|-----------------------------|-------|-----|
| PA-1 | Pending-gate discovery is delegated and machine-readable: `sop approvals [--json]` — read-only, “creates no request, resolves nothing, and never infers a gate from a task status, a blocked reason, or prose”; a run that parks lists its gates pointing at `sop approvals` | `docs/reference/CLI.md` `sop approvals`; `docs/guides/APPROVALS.md` § Seeing a gate (“For a script or a controller, prefer the machine-readable form: `sop approvals --json`”) | DOC | — |
| PA-2 | Pending-gate inspection: `sop approval <task-id>` prints the present-or-absent boundary and its fields (kind, target, stage, disposition, reason, evidence, status, recorded decision); `Approval: none` means no recorded request — a task status alone is never a gate | `docs/reference/CLI.md`; `docs/guides/APPROVALS.md` § Reading a gate | DOC | — |
| PA-3 | A run that parks at a human gate names the gate and the resolving commands (“Run paused: human approval required … Inspect / Approve / Decline / After approving, explicitly continue with: sop run”) | `docs/guides/APPROVALS.md` § Seeing a gate | DOC | — |
| PA-4 | Dashboard rendering of pending gates (a pending-approvals view or equivalent surfaced state): **not documented** in the view table (§2.4) nor the button table (§2.5); whether a parked run's gate reason is surfaced on the dashboard is unknown from reachable evidence | reachable dashboard guide (absence of any approvals row/view) | **UNDOC → UNAVAILABLE** | **C-G3** (verify at ETOE-007, PT-7) |
| PA-5 | Gate-block behavior while a dashboard-driven run proceeds: `human.approval_before_commit: true` still holds — “a run stops at the human gate and nothing is committed from the dashboard” | dashboard guide § Commands you can trigger | DOC | live: C-G4 |
| PA-6 | Authority-side caveat: gate.json is emitted at a single producer site (`internal/cli/run.go:868`) with no covering unit test (ETOE-002 G1); consumer-visible gate parity is only unit-verified through the `sop approvals`/`sop approval` surface | recorded ETOE-002 §2.3 P9, §3 G1 | cited | **C-G6** (live parity ETOE-005/007) |

### 3.4 Phone-safe approval path

| ID | Contract | Documented basis (citation) | Level | Gap |
|----|----------|-----------------------------|-------|-----|
| PH-1 | **Approval actions via the phone/dashboard: not documented → UNAVAILABLE.** The documented dashboard button set (D1–D6) contains no approve/decline action; the documented delegation-able path (D-7) is a client-protocol statement, not a documented dashboard capability. Nothing about a working phone approval flow is asserted. | dashboard guide § Part B (documented phone workflow lists only project browsing plus the six commands); consistent absence of any approval row in § Part A's button table | **UNDOC → UNAVAILABLE** | **C-G2** (PT-8) |
| PH-2 | Phone-safety primitives for any state-changing request: state-changing requests are **POST with a double-submit CSRF cookie**; every route except `/healthz` and `/static/` is token-gated; hand-crafted POSTs fail (`403 invalid CSRF token`) | dashboard guide § Part B checklist; § Troubleshooting; § Security notes | DOC | live enforcement: C-G5 (PT-4/PT-8) |
| PH-3 | Mobile UI documented behavior: responsive (“desktop tables collapse to cards on small screens”), Add to Home Screen supported, cookie-based session after the first `?token=` open | dashboard guide § Open it on the iPhone | DOC | live: C-G4 (PT-12) |
| PH-4 | Approval-safety semantics that any delegation must inherit: approve authorizes only the gated operation, never completes a task, never bypasses validation/review/quality gates; idempotent decisions; interactive deciding fails closed off TTY; decline preserves the truthful lifecycle; “SOP never approves on your behalf”; approve never starts the run unless `--run`, decision persisted first (decline never continues) | `docs/reference/CLI.md` `sop approve`/`sop decline`; `docs/guides/APPROVALS.md` § Deciding a gate, § Deciding interactively, § Continuing after an approval | DOC (authority-side, inherited by delegation) | — |
| PH-5 | Phone runtime readiness (a real iPhone workflow: LAN reachability, cookie flow, observed mobile render) | — | **UNAVAILABLE** | no device/network workflow executed this run (§2.6 U5); planned as PT-12; C-G4/C-G5 |
| PH-6 | Human-gate inviolability from the controller: “It keeps no second source of truth, **and it cannot bypass SOP's human gates**” | dashboard guide (header) | DOC | code-level: C-G7 |

### 3.5 Auth/network boundary

| ID | Contract | Documented basis (citation) | Level | Gap |
|----|----------|-----------------------------|-------|-----|
| AN-1 | **Loopback by default:** `SOP_CONTROLLER_ADDR` default `127.0.0.1:8080`; without `SOP_CONTROLLER_ALLOW_NETWORK=true` a non-loopback bind is “silently rewritten to `127.0.0.1` (so the phone could never connect)” | dashboard guide § Part A/B; § Environment reference; § Troubleshooting | DOC | live: C-G5 (PT-2) |
| AN-2 | **Token-gated network mode:** LAN exposure is an explicit opt-in **requiring an access token**; “the server refuses to start a network bind without one, and every route except `/healthz` and `/static/` is gated”; wrong/missing token → `401 access token required` | dashboard guide § Part B (Create a strong token / Start the dashboard in network mode / First-connection checklist); § Troubleshooting; § Security notes | DOC | live: C-G5 (PT-3) |
| AN-3 | CSRF on mutations: state-changing requests only via POST with a double-submit CSRF cookie; `403 invalid CSRF token` otherwise | dashboard guide § Troubleshooting; § Security notes | DOC | live: C-G5 (PT-4) |
| AN-4 | `/healthz` endpoint exists and is exempt from the token gate (“`make status` — is it up? pings `/healthz`”; exempt routes: `/healthz`, `/static/`) | dashboard guide § Run it; § Part B; § Security notes | DOC (existence/design) / **UNAVAILABLE (live readiness)** | C-G4 (PT-1) |
| AN-5 | Transport: LAN traffic is **plain HTTP** (documented design trade-off; operator guidance: trusted network only, private mesh VPN preferred, do not port-forward) | dashboard guide § Optional — control it from anywhere; § Security notes | DOC | — (no readiness asserted) |
| AN-6 | Credential handling: no secrets or environment variables rendered into pages; the access token is a bearer credential (“Treat it like a password — anyone on your Wi‑Fi with it can trigger SOP commands”), rotated by restarting with a new value | dashboard guide § Create a strong token; § Security notes | DOC | — |
| AN-7 | Provider-environment hygiene: a dashboard started with a stale agent-provider env (`SOP_AGENT_PROVIDER` / `SOP_AGENT_COMMAND`) makes commands “fail as if no agent”; `SOP_BIN`/PATH inheritance determines which SOP executes | dashboard guide § Prerequisites; § Troubleshooting | DOC | — |
| AN-8 | Network exposure does **not** transfer workflow authority: a token holder can *trigger* SOP commands only; every triggered action still executes inside SOP's gates (D-1…D-9, PH-6) | synthesis of dashboard guide (Security notes + command delegation), `docs/architecture/SOP-BOUNDARY.md` (Human Authority), `docs/guides/APPROVALS.md` § Controller and MCP usage | DOC | C-G5/C-G7 (live) |

## 4. Second-authority analysis

**Binding invariant (quoted verbatim from the authoritative plan), `docs/plans/PLAN-SOP-End-to-End-Reliability.md` line 25:**

> “`sop-controller` observes persisted SOP state and delegates actions through the SOP CLI; it must not become a second authority.”

**The controller must not become a second authority.** This audit states that requirement explicitly and evaluates every matrix row's mutation path against SOP's approved interface:

1. **Observation-only surfaces (S-1…S-11).** All documented display reads terminate at SOP's persisted state via the server's SOP boundary; the browser never reaches SQLite and state is read-only (S-9). The state-ownership rule forbids a parallel copy: “a consumer MUST NOT keep a parallel copy of SOP's task state. SOP is the single source of truth for workflow state” (`docs/architecture/SOP-BOUNDARY.md` § State Ownership). The dashboard is documented to comply: “It keeps no second source of truth” (S-11). Routing display is explicitly display-only (S-8).
2. **Delegated state mutations (D-1, D-2, D-6).** Run/resume/retry are triggers of SOP's approved CLI; the state transition happens inside SOP, which owns scheduling, budgets, gates, and approvals (`docs/architecture/SOP-BOUNDARY.md` § What SOP Owns). The controller documents no state-writing path of its own (D-8: “The dashboard never mutates state directly”). Compliant **at documentation level**; the mutation path is SOP's interface, not a second authority.
3. **Verification delegation (D-3, D-4).** Validate/review execute SOP-configured checks; the verdict belongs to SOP's deterministic gate (“The model never decides the verdict”; “findings whose severity is named in `quality.fail_on` are blocking” — `docs/reference/CLI.md` `sop review`). The controller cannot adjudicate outcomes.
4. **Approval delegation (D-7, PA-1…PA-6, PH-1…PH-6).** Approval authority never moves: the controller (if it exposes approval actions at all — C-G2) resolves only the gate SOP raised; approve never completes a task and never bypasses validation, review, or the quality gate (D-7, PH-4); decline preserves the truthful lifecycle; the human gate cannot be bypassed from the dashboard (PH-6, PA-5). “No SOP policy — routing, providers, approval outcome — moves into the client” (`docs/guides/APPROVALS.md` § Controller and MCP usage).
5. **Network boundary (AN-1…AN-8).** Network exposure widens who can *trigger* SOP commands (“anyone on your Wi‑Fi with it can trigger SOP commands”), but every triggered action still executes inside SOP's policy (AN-8). Auth/network governs availability of the delegation surface, never workflow authority. The documented refusal posture (refuses to start a network bind without a token; CSRF-gated POSTs; token-gated routes) is itself an authority-preservation control; its live enforcement is UNAVAILABLE (C-G5).
6. **Residual risk recorded:** second-authority enforcement lives in the controller's implementation, which is **outside this run's reach** (U6/C-G7). The absence of a second write path and of direct-state-mutation routes is documented but not code-verified here, and is therefore a scheduled refusal check (PT-9) at ETOE-007, which also closes the C-G2 asymmetry at the UI level.

## 5. Explicit gaps

| Gap | Description | Owning layer / verification owner | Disposition |
|-----|-------------|-----------------------------------|-------------|
| C-G1 | The four sop-controller contract documents named by ETOE-001 §5 (`docs/architecture/SOP-BOUNDARY.md`, `docs/reference/CLI.md`, `docs/specs/WORKFLOW.md`, `docs/specs/HUMAN-APPROVAL.md`) are **UNAVAILABLE this run**: sibling checkout not an authorized tool root (§2.1, all three probe surfaces rejected). ETOE-003 therefore assesses against SOP-side documented integration contracts only; controller-repo-side section content has never been read by any stage. | sop-controller repository; requires a tool surface authorized for it | Read the sibling docs as ETOE-007 stage setup; treat any divergence from §3 as a finding. |
| C-G2 | **Approval-action asymmetry:** `docs/guides/APPROVALS.md` documents controller approval delegation (`sop approvals --json` → `sop approve`/`sop decline`), but the dashboard wiring guide's documented surface contains **no approval button** and no approvals view (D-7/PA-4). Presence/absence and, if present, phone-safety of an approval route are unverified. | sop-controller UI code; ETOE-007 | Resolve at ETOE-007 (PT-7/PT-8); any approval surface must be POST+CSRF+token gated (PH-2) and delegate only (PH-4). |
| C-G3 | Pending-approval **visibility on the dashboard** is undocumented (no approvals row in the documented view table): pending gates may be invisible between runs. | sop-controller UI code; ETOE-007 | PT-7 decides by observation; nothing assumed. |
| C-G4 | Runtime display/action readiness: live build/serve, live `/healthz`, HTMX cadence, view/action parity (S-1…S-10, D-1…D-9) — documented, never exercised. | sop-controller runtime; ETOE-007 disposable environment | PT-1, PT-5, PT-6, PT-9. |
| C-G5 | Live network/auth enforcement: silent loopback rewrite, refuse-to-start without token, 401/403 behavior, `/healthz` exemption in practice — documented, not exercised (plain-HTTP transport is the documented trade-off). | sop-controller server; ETOE-007 | PT-2, PT-3, PT-4, PT-12 on an isolated network only. |
| C-G6 | Authority-side: gate.json emitted at a single producer site (`internal/cli/run.go:868`) with **no covering unit test** (carried from ETOE-002 G1); consumer gate parity (`sop approvals --json` versus the emitted artifact) unverified live. | agentic-sop run loop + consumers | PT-11 during ETOE-007/ETOE-005; unit-test candidate in a later, non-audit stage. |
| C-G7 | Code-level second-authority enforcement (no direct state write path; documented refusals D-8, S-9, S-11, PH-6) is UNAVAILABLE — sibling code unreachable this run. | sop-controller code; ETOE-007 | PT-6/PT-9 verify by refusal checks; findings feed ETOE-010. |

## 6. Safe integration test plan (stage E3-S3; executed later by ETOE-007)

**Isolation rules (binding for every plan item).** All PT items run against a **disposable, initialized SOP project created outside any production checkout** (a fresh temp-directory project initialized with `sop init`; the controller pointed at it via `SOP_CONTROLLER_PROJECTS`), never at a production checkout's state database; **no parallel SOP mutations** against the same project or state database (one sequential driver per run); approvals are tested **as refusals first** — an approval may be granted only inside the authorized disposable fixture and never for anything outside it (parent-plan invariants 1, 2, 4, 6); LAN/network-mode checks run only on an isolated network with a freshly rotated token; teardown removes the temp project and controller processes (`make stop`) and is recorded for reproducibility (ETOE-005 owns the fixture contract). No test item mutates this repository, any production checkout's `.agent-sdlc/state.db`, or any sibling file.

| PT | Planned test (disposable environment) | Closes matrix row / UNAVAILABLE item | Expected observation (from documented behavior) |
|----|----------------------------------------|--------------------------------------|--------------------------------------------------|
| PT-1 | Start the controller against the disposable project; issue `GET /healthz` without a token | AN-4, U3, C-G4 | `/healthz` responds ready without a token (documented exempt route); record the exact response |
| PT-2 | With `SOP_CONTROLLER_ALLOW_NETWORK` unset, request a non-loopback bind | AN-1, C-G5 | Address is silently rewritten to `127.0.0.1`; the phone cannot connect (documented fail-safe) |
| PT-3 | Start network mode (a) without a token, (b) with a token; probe a gated route and `/healthz` from the isolated network | AN-2, C-G5 | (a) server refuses to start; (b) gated route → `401 access token required` without/with a wrong token; `/healthz` reachable |
| PT-4 | Issue a state-changing POST without the CSRF cookie/token | AN-3, C-G5 | `403 invalid CSRF token`; nothing recorded SOP-side |
| PT-5 | Display parity: compare every documented view (S-1…S-10) against `sop status`, `sop task <id>`, `sop approvals --json`, `sop report`, and the run artifacts (review/validation CI panel, handoff) for the same disposable project | S-1…S-10, C-G4 | Counts, states, findings, handoff match SOP outputs; any mismatch is a recorded defect |
| PT-6 | Action parity: trigger each documented button (D-1…D-6) and record the exact SOP command executed and the SOP-side state effect (single sequential driver; D-9 timeout observed) | D-1…D-6, D-9, C-G4 | Effects identical to running the same SOP CLI command by hand; transitions occur only inside SOP |
| PT-7 | Create a human gate (e.g. a task that reaches the commit gate with `human.approval_before_commit: true`) and observe the dashboard during and after the parked run | PA-4/PA-5, C-G2, C-G3, PA-6 | Whether a pending gate is visible (and how) is recorded, not assumed; the run indeed stops at the gate and nothing is committed |
| PT-8 | If any approval action exists on the controller: verify POST-only + CSRF + token on that route; delegate one **decline** (refusal path) inside the fixture; delegate authorize only per operator authorization within the fixture; record `sop approval` before/after | D-7, PH-1…PH-4, C-G2 | The action delegates to `sop approve`/`sop decline`; only the raised gate resolves; the refusal preserves lifecycle; absence of the action is recorded as C-G2's resolution |
| PT-9 | Second-authority refusals: attempt state mutation outside SOP's interface (direct DB access from the controller host, hand-crafted non-CSRF POSTs, non-documented routes) and confirm the controller offers no such path | D-8, S-9, S-11, PH-6, U6, C-G7 | Every attempt refused (`403`/`404`) or not offered; state.db hash unchanged except via SOP CLI actions; “double-click safe”/“restart-safe” claims exercised live |
| PT-10 | Routing display-only check: view routing data on the controller while confirming no controller route mutates routing policy | S-8, C-G7 | Display matches SOP's routing records; no policy-mutation path exists |
| PT-11 | During PT-7's gate-blocked run, capture `.agent-sdlc/runs/<id>/gate.json` and compare with `sop approvals --json` | PA-6, C-G6 | Emitted gate artifact matches SOP's approvable listing (closes ETOE-002 G1 live) |
| PT-12 | Mobile read-only check (isolated network, documented phone flow minus any approval action unless PT-8 authorized it): open `?token=`, complete the first-connection checklist (firewall, same SSID, port free, token accepted), Add to Home Screen, record observed responsiveness | PH-3, U4, U5, C-G5 | Observed mobile behavior recorded as evidence; no readiness asserted beyond the observation |

**Traceability statement:** every planned test maps to a §3 matrix row or a §2.6 UNAVAILABLE item, so executing PT-1…PT-12 closes exactly the set left unverified here; no planned test asserts readiness that this matrix left unverified without scheduling its verification.

**No-execution disclaimer:** ETOE-003 executed no controller, started no server, exercised no mobile or network workflow, and performed no SOP-state action. All PT-1…PT-12 verification is deferred to the disposable-environment stage (**ETOE-007**), with authority-side gate parity also touched by **ETOE-005**.

## 7. Verification notes

Per the parent plan's required-validation list, the read-only validation set for `agentic-sop` (with this report in the tree) is `gofmt -l .`, `go build ./...`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, and `git diff --check`, with exact versions and exit codes recorded (`docs/plans/PLAN-SOP-End-to-End-Reliability.md` § Required validation).

**Execution status for this run's required validation commands (the SOP-gate set):**

- `go build ./...` — exit 0.
- `go test ./...` — exit 0.
- `go vet ./...` — exit 0.
- `test -z "$(gofmt -l .)"` — exit 0 (no unformatted files).

(The four commands above are the validation contract supplied to ETOE-003's implement stage and are also the set SOP's configured gate enforces; exact per-command results are confirmed in the structured outcome of this run and by SOP's independent gate.)

**Race and diff-check status (plan-required, not gate-enforced):** the parent plan explicitly distinguishes plan-required commands from SOP's configured gate (`build: go build ./...`; `test: go test ./...`; `lint: go vet ./...` plus the `gofmt -l .` emptiness test) and states: “Required by this plan but NOT enforced by SOP's configured gate: `go test -race -count=1 ./...` and `git diff --check`.” For this run: `go test -race -count=1 ./...` — **UNAVAILABLE** (not admitted by this run's command allow-list; race status remains unverified by ETOE-003 and must be recorded by a stage that runs it). `git diff --check` — **UNAVAILABLE** for the same reason; whitespace/additive-only status is instead attested by the mutation statement (§8) and checkable by `git status`/`git diff --check` at review time.

Go toolchain: go1.27.1 recorded for this checkout's gates (ETO E-001 §3 `[operator-verified]`, cited as prior evidence; no `go version` surface admitted by this run's allow-list, so not re-probed here).

## 8. Mutation statement

- The **only** repository change made by ETOE-003 is this report file, `docs/reports/end-to-end-reliability/ETOE-003-controller-contract-matrix.md`. Pre-existing worktree entries (notably `docs/plans/PLAN-SOP-End-to-End-Reliability.md`, the ETOE-001/002 reports, and the `ETOE-002-attempt-1-failure/` snapshot) are user-owned and were left untouched.
- SOP state (`.agent-sdlc/`) was not modified; no gate, CLI, or configuration change was made from audit findings.
- **No file in the sibling `sop-controller` checkout was read or modified** — it is outside this run's authorized tool root (probe §2.1); the sibling checkout is in any case strictly outside this audit's mutation scope.
- The audit is documentation-only: all gaps from §5 are report rows/backlog candidates for later stages (ETOE-005/007/010), not changes.

## 9. Acceptance-criteria mapping (ETOE-003)

| # | Acceptance criterion | Evidence in this report |
|---|----------------------|--------------------------|
| 1 | The contract matrix covers state display and every delegated SOP action, and states that the controller must not become a second authority | §3.1 (S-1…S-11), §3.2 (D-1…D-9 with the §2.5 bidirectional cross-check), §4 (the explicit second-authority statement, the quoted binding invariant, and the per-mutation-path evaluation) |
| 2 | Pending-approval visibility and the phone-safe approval path are assessed from the controller's documented behavior, not assumed | §3.3 PA-1…PA-6 and §3.4 PH-1…PH-6, each cited from reachable documented sections (`docs/guides/APPROVALS.md`, `docs/guides/SOP-CONTROLLER-DASHBOARD.md`, `docs/reference/CLI.md`) or explicitly UNDOC→UNAVAILABLE (PA-4, PH-1) with the reason and a scheduled verification (PT-7/PT-8) |
| 3 | No mobile or network readiness is asserted without verification; unverified capabilities are marked UNAVAILABLE | §1 legend; §2.6 U3/U4/U5; §3.4 PH-5; §3.5 AN-4 live rows; §6 disclaimer — no readiness is asserted anywhere above the documented/UNAVAILABLE labels |

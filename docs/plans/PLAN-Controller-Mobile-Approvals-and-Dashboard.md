# Controller — Mobile Approvals, Execution Dashboard, and Documentation

> **Document class:** plan · **Lifecycle:** proposed · **Authority:** proposed planning — not yet ACTIVE with SOP (`.agent-sdlc/plan.meta.json` records no active plan).

## Project

sop-controller

## Summary

Deliver the operator-priority work requested for SOP Controller without changing the
ownership boundary: **P0** a phone-friendly pending-approvals inbox, **P1** execution-
dashboard model-selection and usage display, **P2** SOP-integration integrity, and **P3**
documentation reconciliation.

The controller already implements FR-1…FR-10: project/workflow/task views, structured
activity (SSE + bounded poll), review/CI/handoff panels, recovery, decisions, and the
delegated command set (run/resume/validate/review/report/retry/retry-force/retry-all/
reconcile/approve/decline/accept-changed). Per-task approve/decline and a per-project
"Human decisions" panel already exist. The work below fills the three genuine gaps found
in preflight — a **cross-project approvals inbox**, **`model_selection` display**, and
**stale documentation** — and adds a boundary guard for the new read.

SOP remains the workflow authority. Every new read goes through `internal/sopclient`;
every decision delegates to SOP's own operation. The controller never decides a gate
exists, never selects a model, and never writes SOP persistence.

## Capabilities

### SOP approval listing — EXISTS

- Evidence: `Client.Approvals` runs `sop approvals --json` and decodes SOP's listing
  verbatim (`internal/sopclient/approvals.go`, `service.go`); a failed or unparsable run
  surfaces `ErrApprovalsUnavailable` rather than an empty success.

### Delegated approval operation — EXISTS

- Evidence: `Client.ApproveTask` / `DeclineTask` delegate to `sop approve` / `sop decline`
  with `--by` / `--note` and no `--run` (`internal/sopclient/boundary.go`, `decision.go`);
  routes POST with CSRF (`internal/web/server.go`, `middleware.go`).

### Model-selection evidence — EXISTS (unread)

- Evidence: SOP's `report.json` carries `model_selection` (`class`, `provider`, `model`,
  `locality`, `source`, `fallback`, `reason`) and `performance` counts; the controller
  reads only `provider`/`model` today (`internal/sopclient/run.go`).

### Deterministic validation — EXISTS

- Evidence: `.agent-sdlc/config.yaml` declares build `go build ./...`, test `go test ./...`,
  lint `go vet ./...`; `make test`/`make vet` mirror them.

## Assumptions

### Local-first authentication is network-mode token + CSRF

- Evidence: `docs/specs/SECURITY.md` requires loopback binding by default and an access
  token for network mode; PRD V1 lists per-user accounts/RBAC as non-goals.
- Consequence: "authenticated/authorized/auditable" means network-mode token + CSRF +
  SOP-recorded provenance (`--by`/`--note`), not new account management.

### "When available" gates model and usage display

- Evidence: not every `report.json` persists `model_selection`; SOP records counts, not
  quota percentages.
- Consequence: display SOP's persisted class/selection and counts verbatim; render an
  explicit absence rather than a fabricated value or percentage.

## MOB-001 — Cross-project pending-approvals projection

Add a read-only projection that aggregates SOP's applicable gates across every observed
project.

### Deliverables

- `internal/sopclient/pending_approvals.go` — `Client.PendingApprovals(ctx)` returning
  `[]ProjectApprovals` (project id/name, `Reported`, applicable `Entries`).
- `internal/sopclient/pending_approvals_test.go` — aggregation, unread-vs-empty, order.

### Acceptance Criteria

- Runs `sop approvals --json` once per project via the existing `Client.Approvals`; keeps
  only entries SOP reports applicable.
- A failed/unparsable listing yields `Reported=false` with no entries, never an empty
  success; a well-formed empty listing yields `Reported=true` with no entries.
- No gate is reconstructed from task status, prose, or attempts; no local write.

## MOB-002 — `/approvals` inbox route and template

A single phone-friendly page listing every pending gate with approve/decline controls.

### Deliverables

- `GET /approvals` handler + route (`internal/web/handlers.go`, `server.go`).
- `templates/approvals.html`; nav link in `templates/partials/nav.html`.

### Acceptance Criteria

- Read-only on GET; approve/decline POST to the existing
  `/projects/{project}/tasks/{task}/commands/{approve|decline}` routes with CSRF.
- Mobile-first layout; each gate shows project, task, kind, and SOP-reported reason.
- An unreported listing is shown as an explicit absence, distinct from "no gates".
- Depends on MOB-001.

## MOB-003 — Attention badge on the projects list

### Deliverables

- `templates/projects.html` renders `ProjectSummary.NeedsAttention` linking to `/approvals`.

### Acceptance Criteria

- Hidden when zero; uses the existing summary field (no new SOP call).

## MOB-004 — Web tests for the inbox

### Deliverables

- `internal/web/*_test.go` covering aggregation render, empty/unread states, and CSRF.

### Acceptance Criteria

- New tests pass under `go test` and `go test -race`. Depends on MOB-002/003.

## DASH-001 — Decode `model_selection` into `RunInfo`

### Deliverables

- `internal/sopclient/run.go` — `ModelSelection` type + `RunInfo.ModelSelection`.
- `internal/sopclient/run_test.go` — verbatim read; absent when unrecorded.

### Acceptance Criteria

- `class`, `locality`, `source`, `fallback`, `reason` read verbatim; `Present=false` without
  a record. The controller selects nothing (docs/specs/AGENT-PROVIDER.md).

## DASH-002 — Display model class and locality

### Deliverables

- `templates/task.html`, `templates/partials/plan_performance.html`,
  `internal/web/render.go` as needed.

### Acceptance Criteria

- Task detail shows SOP's class (e.g. SMALL/MEDIUM/LARGE) with locality/source/fallback;
  explicit absence when unrecorded. Depends on DASH-001.

## DASH-003 — Usage counts ("when available")

### Deliverables

- Performance panels surface SOP-persisted counts already read (agent calls, validation/
  review reuse, JEV invocations/tool calls).

### Acceptance Criteria

- Only SOP-persisted values; no invented percentage. Depends on DASH-002.

## INT-001 — Boundary guard for the new read

### Deliverables

- `internal/sopclient/boundary_test.go` addition.

### Acceptance Criteria

- Asserts the inbox read goes through `sopclient`, adds no orchestration/decision logic,
  and introduces no mutation path.

## DOC-001 — Correct stale active-plan references

### Deliverables

- `docs/README.md`, `AGENTS.md`.

### Acceptance Criteria

- No document claims an ACTIVE plan that SOP does not record; the Pre-Performance Closure
  plan is described as archived (`COMPLETE`).

## DOC-002 — README and docs index for humans

### Deliverables

- `README.md`, `docs/README.md`.

### Acceptance Criteria

- Covers the approvals inbox, mobile access, model-selection display, local setup, and
  security considerations; references correct. Depends on MOB-002, DASH-002.

## DOC-003 — CLI routes, guides, and test target

### Deliverables

- `docs/reference/CLI.md` (add `/approvals`); mobile-approvals/security guide;
  `Makefile` `test-race` target.

### Acceptance Criteria

- Route and target tables accurate. Depends on MOB-002.

## DOC-004 — This plan

### Deliverables

- `docs/plans/PLAN-Controller-Mobile-Approvals-and-Dashboard.md`.

## Plan-wide constraints

- Do not add a second source of truth for SOP state; every read goes through
  `internal/sopclient`; every mutation delegates to SOP.
- Do not select a model or provider, and do not add a cancellation control (SOP exposes
  none).
- Keep changes minimal and idiomatic: Go standard library and HTMX; no new framework,
  Node, or PostgreSQL.
- Validation for each behavioral change: `gofmt -l .`, `go vet ./...`, `go test ./...`,
  `go test -race ./...`, `go build ./...`.
- Work on `main`; commit completed, validated changes; do not push.

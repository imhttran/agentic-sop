# Controller — Mobile Approvals, Execution Dashboard, and Documentation

> **Document class:** plan · **Lifecycle:** active · **Authority:** the ACTIVE implementation plan; recorded by SOP in `.agent-sdlc/plan.meta.json` (`sop status` reports it).

## Project

sop-controller

## Summary

Deliver the operator-priority work for SOP Controller without changing the ownership boundary: P0 a phone-friendly pending-approvals inbox, P1 execution-dashboard model-selection and usage display, P2 SOP-integration integrity, and P3 documentation reconciliation.

The controller already implements FR-1…FR-10: project/workflow/task views, structured activity (SSE + bounded poll), review/CI/handoff panels, recovery, decisions, and the delegated command set (run/resume/validate/review/report/retry/retry-force/retry-all/reconcile/approve/decline/accept-changed). Per-task approve/decline and a per-project "Human decisions" panel already exist. This plan fills the three genuine gaps found in preflight — a cross-project approvals inbox, model-selection display, and stale documentation — and adds a boundary guard for the new read.

The implementation described here is already present and validated in the working tree (gofmt, go vet, go test, go test -race, and go build all pass), written directly under operator instruction. Each stage is therefore verify-first: SOP first runs the configured deterministic validation and only invokes an implementation agent if verification fails. SOP remains the workflow authority: every new read goes through internal/sopclient, every decision delegates to SOP's own operation, and the controller never decides a gate exists or selects a model.

## Capabilities

### SOP approval listing — EXISTS

- Evidence: `Client.Approvals` runs `sop approvals --json` and decodes SOP's listing verbatim; a failed or unparsable run surfaces `ErrApprovalsUnavailable` rather than an empty success.

### Delegated approval operation — EXISTS

- Evidence: `Client.ApproveTask` / `DeclineTask` delegate to `sop approve` / `sop decline` with `--by` / `--note` and no `--run`; the routes POST with CSRF.

### Model-selection evidence — EXISTS

- Evidence: SOP's `report.json` carries `model_selection` (class, provider, model, locality, source, fallback, reason) and performance counts.

### Deterministic validation — EXISTS

- Evidence: `.agent-sdlc/config.yaml` declares build `go build ./...`, test `go test ./...`, and lint `go vet ./...` for this project.

## Assumptions

### Local-first authentication is network-mode token plus CSRF

- Evidence: `docs/specs/SECURITY.md` requires loopback binding by default and an access token for network mode; PRD V1 lists per-user accounts/RBAC as non-goals.
- Consequence: authenticated/authorized/auditable means network-mode token plus CSRF plus SOP-recorded provenance (--by/--note), not new account management.

### "When available" gates model and usage display

- Evidence: not every `report.json` persists `model_selection`; SOP records counts, not quota percentages.
- Consequence: display SOP's persisted class/selection and counts verbatim; render an explicit absence rather than a fabricated value or percentage.

## MOB-001 — Cross-project pending-approvals projection

Add a read-only projection that aggregates SOP's applicable gates across every observed project.

### Dependencies

- none

### Deliverables

- `internal/sopclient/pending_approvals.go` — `Client.PendingApprovals(ctx)` returning `[]ProjectApprovals` (project id/name, Reported, applicable Entries).
- `internal/sopclient/pending_approvals_test.go` — aggregation, unread-vs-empty, and project order.

### Acceptance Criteria

- Runs `sop approvals --json` once per project via the existing `Client.Approvals`; keeps only entries SOP reports applicable.
- A failed or unparsable listing yields `Reported=false` with no entries, never an empty success; a well-formed empty listing yields `Reported=true` with no entries.
- No gate is reconstructed from task status, prose, or attempts; no local write.

### Execution

- verify-first

## MOB-002 — approvals inbox route and template

Add a single phone-friendly page listing every pending gate with approve/decline controls.

### Dependencies

- MOB-001

### Deliverables

- `GET /approvals` handler and route in `internal/web/handlers.go` and `internal/web/server.go`.
- `templates/approvals.html` and a nav link in `templates/partials/nav.html`.

### Acceptance Criteria

- Read-only on GET; approve/decline POST to the existing `/projects/{project}/tasks/{task}/commands/{approve|decline}` routes with CSRF.
- Mobile-first layout; each gate shows its project, task, kind, and SOP-reported reason.
- An unreported listing is shown as an explicit absence, distinct from "no gates".

### Execution

- verify-first

## MOB-003 — Attention badge on the projects list

Surface the projects-list attention count without a new SOP read.

### Dependencies

- MOB-002

### Deliverables

- `templates/projects.html` renders `ProjectSummary.NeedsAttention` as a non-linked badge inside the project card.

### Acceptance Criteria

- The badge is hidden when the count is zero and uses the existing summary field (no new SOP call).

### Execution

- verify-first

## MOB-004 — Web tests for the inbox

Cover the new inbox and badge with focused web tests.

### Dependencies

- MOB-002

### Deliverables

- `internal/web/approvals_inbox_test.go` covering the aggregated render, the unread listing, and the projects-list badge.

### Acceptance Criteria

- The new tests pass under `go test ./...` and `go test -race ./...`.

### Execution

- verify-first

## DASH-001 — Decode model selection into RunInfo

Decode SOP's recorded model selection into the run record.

### Dependencies

- none

### Deliverables

- `internal/sopclient/run.go` with a `ModelSelection` type and `RunInfo.ModelSelection`.
- `internal/sopclient/run_test.go` covering the verbatim read and the absent case.

### Acceptance Criteria

- `class`, `locality`, `source`, `fallback`, and `reason` are read verbatim; `Present=false` without a record.
- The controller selects nothing (`docs/specs/AGENT-PROVIDER.md`).

### Execution

- verify-first

## DASH-002 — Display model class and locality

Show SOP's model selection on the task detail view.

### Dependencies

- DASH-001

### Deliverables

- `templates/task.html` and a `hasModelSelection` helper in `internal/web/render.go`.
- `internal/web/model_selection_test.go` covering the present and absent cases.

### Acceptance Criteria

- The task detail view shows SOP's class with locality/source/fallback, and an explicit absence when unrecorded.

### Execution

- verify-first

## DASH-003 — Usage counts when available

Surface SOP's persisted usage counts; no new parsing is required.

### Dependencies

- DASH-002

### Deliverables

- The existing per-task and per-plan performance panels that surface SOP-persisted counts (agent calls, validation/review reuse, fix cycles).

### Acceptance Criteria

- Only SOP-persisted values are shown; no percentage is invented when SOP reports none.

### Execution

- verify-first

## INT-001 — Boundary guard for the new read

Guard the new read against boundary drift with a focused test.

### Dependencies

- MOB-001

### Deliverables

- `internal/sopclient/boundary_test.go` additions: the new read is included in the no-mutation snapshot and asserted to drive only `sop approvals`.

### Acceptance Criteria

- The inbox read goes through `sopclient`, adds no orchestration or decision logic, and introduces no mutation path.

### Execution

- verify-first

## DOC-001 — Correct stale active-plan references

Remove stale active-plan claims from the documentation.

### Dependencies

- none

### Deliverables

- `docs/README.md` and `AGENTS.md` corrected.

### Acceptance Criteria

- No document claims an ACTIVE plan that SOP does not record; the Pre-Performance Closure plan is described as archived (`COMPLETE`).

### Execution

- verify-first

## DOC-002 — README and documentation index for humans

Update the landing README and documentation index for human readers.

### Dependencies

- MOB-002
- DASH-002

### Deliverables

- `README.md` and `docs/README.md` updated.

### Acceptance Criteria

- Covers the approvals inbox, mobile access, model-selection display, local setup, and security considerations, with correct references.

### Execution

- verify-first

## DOC-003 — CLI routes, guide, and test target

Update the CLI route reference, add the approvals guide section, and add a race-test target.

### Dependencies

- MOB-002

### Deliverables

- `docs/reference/CLI.md` (adds `/approvals`), `docs/guides/APPROVAL-AND-RECONCILE.md` (inbox section), and a `test-race` target in `Makefile`.

### Acceptance Criteria

- The route table and make targets are accurate.

### Execution

- verify-first

## DOC-004 — This plan

Keep this document as the governed source for the work above, compilable by SOP's structured Markdown parser.

### Dependencies

- none

### Deliverables

- `docs/plans/PLAN-Controller-Mobile-Approvals-and-Dashboard.md`.

### Acceptance Criteria

- The plan compiles into stages with the declared dependency edges and passes deterministic validation.
- The document names this project and a summary, with one section per stage.

### Execution

- verify-first

## Plan-wide constraints

- Do not add a second source of truth for SOP state; every read goes through `internal/sopclient`; every mutation delegates to SOP.
- Do not select a model or provider, and do not add a cancellation control (SOP exposes none).
- Keep changes minimal and idiomatic: Go standard library and HTMX; no new framework, Node, or PostgreSQL.
- Validation for each behavioral change: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, `go build ./...`.
- Work on `main`; commit completed, validated changes; do not push.

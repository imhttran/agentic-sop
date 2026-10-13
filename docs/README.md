# SOP Controller — Documentation

Central map of SOP Controller documentation. Each entry has a one-sentence
description so a human or an AI agent can decide what to read.

**New here?** Start with [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md),
then [guides/OPERATING.md](guides/OPERATING.md).

## Documentation Authority

When documents disagree, authority flows in this order:

```text
Product intent
      │
      ▼
requirements/PRD.md      — what we are building and why
      │
      ▼
specs/*.md               — how the system MUST behave
      │
      ▼
architecture/*.md        — structural boundaries and design constraints
      │
      ▼
plans/*.md               — what work needs to happen
      │
      ▼
SOP execution            — SOP remains the workflow authority
```

Plans MUST NOT override specifications. Reports and historical artifacts are
point-in-time and non-normative: they provide traceability only and MUST NOT
override current specifications. The CTRL001 boundary contract and inventory
retain historical paths but are maintained current, non-normative references
maintained against the code; verification reports remain
point-in-time evidence.

## Where a new document belongs

```text
requirements/  product intent (PRD)
architecture/  structural boundaries and ownership
specs/         normative required behavior
plans/         current implementation plans
reference/     commands, states, and values
guides/        task-oriented how-tos
reports/       point-in-time execution evidence (non-normative)
tasks/         single-task sources run via `sop run --task`
history/       completed or superseded work (non-normative)
```

Filesystem location is a **signal for readers and tooling, not lifecycle
authority**. The authoritative plan state is the one SOP records in
`.agent-sdlc/plan.meta.json` and reports via `sop status`. Some paths are kept
where they are because SOP or the code records them — see
[Kept operational paths](#kept-operational-paths) below.

## Getting Started

- [../README.md](../README.md) — project landing page: what SOP Controller is,
  its architecture, requirements, and quick start.
- [../AGENTS.md](../AGENTS.md) — routing guide for implementation/review agents.

## Requirements

- [requirements/PRD.md](requirements/PRD.md) — what problem we are solving, the
  goals, functional requirements, constraints, and success criteria.

## Architecture

- [architecture/OVERVIEW.md](architecture/OVERVIEW.md) — system structure,
  components, data flow, and repository layout.
- [architecture/SOP-BOUNDARY.md](architecture/SOP-BOUNDARY.md) — the SOP boundary:
  why SOP remains the workflow authority and the controller must not become a
  second source of truth.

## Specifications

Normative descriptions of required behavior (MUST / SHOULD / MAY).

- [specs/WORKFLOW.md](specs/WORKFLOW.md) — how the controller reflects SOP's
  workflow: status vocabulary, dependencies, scheduling ownership, restart safety.
- [specs/EXECUTION.md](specs/EXECUTION.md) — how the controller reports SOP task
  execution: run stages, change detection, and structured execution outcomes.
- [specs/REVIEW.md](specs/REVIEW.md) — the review lifecycle the controller
  displays, including findings, severity, and remediation progress.
- [specs/OPENJEV.md](specs/OPENJEV.md) — the OpenJEV (JEV) integration contract:
  what the controller reads and displays, and what remains proposed.
- [specs/HUMAN-APPROVAL.md](specs/HUMAN-APPROVAL.md) — the human-approval boundary:
  when the controller shows a human gate and how approval actions delegate to SOP.
- [specs/ACTIVITY.md](specs/ACTIVITY.md) — the live activity stream: source,
  delivery, cursor semantics, and safe-summary guarantees.
- [specs/AGENT-PROVIDER.md](specs/AGENT-PROVIDER.md) — the agent/provider contract
  the controller depends on: the SOP command provider and structured outcomes.
- [specs/SECURITY.md](specs/SECURITY.md) — local-first binding, network mode,
  CSRF, discovery boundary, and secret handling.

## Plans

What work needs to happen. Plans describe implementation work and MUST NOT
override specifications. The authoritative plan state is the one SOP records in
`.agent-sdlc/plan.meta.json` (`sop status` reports it); a file's location is a
reader signal, not lifecycle authority.

Current:

- **[plans/README.md](plans/README.md)** — what belongs in `plans/` versus
  `history/plans/`.

No plan is currently active: SOP records none (`.agent-sdlc/plan.meta.json` has an
empty source, and `sop status` reports no plan).

Completed and archived (disposition `COMPLETE`) — kept at their recorded source path
because SOP's archive matches a plan by that path:

- [plans/PLAN-Controller-Mobile-Approvals-and-Dashboard.md](plans/PLAN-Controller-Mobile-Approvals-and-Dashboard.md) —
  plan id `plan-controller-mobile-approvals-and-dashboard`: the phone approvals inbox,
  model-selection display, and documentation work. Archived `COMPLETE` (see
  `.agent-sdlc/archive/plan-controller-mobile-approvals-and-dashboard/`).
- The earlier Pre-Performance Closure plan (CLOSE-005…CLOSE-011) is likewise
  `COMPLETE`; its document lives at
  [history/plans/PLAN-Pre-Performance-Closure-Remaining.md](history/plans/PLAN-Pre-Performance-Closure-Remaining.md).

Historical (completed or superseded — non-normative evidence):

- [history/plans/](history/plans/) — see
  [history/plans/README.md](history/plans/README.md). Includes
  [PLAN-Finish-Pre-Performance-Closure.md](history/plans/PLAN-Finish-Pre-Performance-Closure.md)
  (FP-001…FP-010), the CLOSE-004 rerun-readiness plan
  ([PLAN-CLOSE-004-Rerun-Readiness.md](history/plans/PLAN-CLOSE-004-Rerun-Readiness.md)),
  the hardening plans
  ([PLAN-Hardening.md](history/plans/PLAN-Hardening.md),
  [PLAN-Hardening-2.md](history/plans/PLAN-Hardening-2.md)), the wrap-up plan
  ([PLAN-Wrap-Up.md](history/plans/PLAN-Wrap-Up.md)), and the organization and
  precheck material
  ([PLAN-ORGANIZATION.md](history/plans/PLAN-ORGANIZATION.md),
  [PRECHECK.md](history/plans/PRECHECK.md)).
- [PLAN-SOP-Controller.md](PLAN-SOP-Controller.md) — **superseded**
  run-control and live-activity plan (CTRL001–CTRL017). Kept at this path because
  SOP's plan archive records `docs/PLAN-SOP-Controller.md` as its plan source;
  see [history/RETIRED-plan-sop-controller.md](history/RETIRED-plan-sop-controller.md).

## Reports and Evidence

Point-in-time execution evidence (non-normative). Reports record what was
observed; they never override requirements, specifications, or architecture.
See [reports/README.md](reports/README.md) for the full map.

- [reports/pre-performance-closure/](reports/pre-performance-closure/) —
  CLOSE-004…CLOSE-011 evidence for the Pre-Performance Closure work (archived,
  `COMPLETE`).
- [reports/finish-pre-performance-closure/](reports/finish-pre-performance-closure/) —
  FP-001…FP-009 and
  [READINESS.md](reports/finish-pre-performance-closure/READINESS.md).
- [reports/PERFORMANCE-BASELINE.md](reports/PERFORMANCE-BASELINE.md) — CLOSE-010
  performance baseline report.

## Tasks

Single-task sources executed directly with `sop run --task`.

- [tasks/CLOSE-004.md](tasks/CLOSE-004.md) — CLOSE-004 controller deterministic
  baseline task source.

## Reference

Concrete interfaces, values, commands, and states.

- [reference/CONFIGURATION.md](reference/CONFIGURATION.md) — every environment
  variable, its default, and the command timeout.
- [reference/CLI.md](reference/CLI.md) — how to run the controller and the SOP CLI
  commands and HTTP routes the dashboard exposes.
- [reference/STATUS-AND-RECOVERY.md](reference/STATUS-AND-RECOVERY.md) — task
  statuses, run stages, recovery dispositions, and recovery actions.

## Guides

- [guides/GETTING-STARTED.md](guides/GETTING-STARTED.md) — install, run, and open
  the dashboard, including optional phone access.
- [guides/OPERATING.md](guides/OPERATING.md) — operator guide: architecture and
  ownership, run controls, activity, failure display, human gates, troubleshooting.
- [guides/RUN-CONTROLS.md](guides/RUN-CONTROLS.md) — how to start and continue a
  run, retry semantics, and the documented cancellation behavior.
- [guides/ACTIVITY-AND-PRIVACY.md](guides/ACTIVITY-AND-PRIVACY.md) — what the
  activity timeline shows and the privacy/safety rules governing it.
- [guides/FAILURE-DISPLAY.md](guides/FAILURE-DISPLAY.md) — how review, CI, and
  handoff failures and diagnostics appear to an operator.
- [guides/APPROVAL-AND-RECONCILE.md](guides/APPROVAL-AND-RECONCILE.md) — when a
  human gate appears, the cross-project approvals inbox (`/approvals`), and how
  approval/reconcile actions delegate to SOP.
- [guides/TROUBLESHOOTING.md](guides/TROUBLESHOOTING.md) — mapping observed
  states to recovery actions, and the CLI/make equivalents of controller controls.
- [guides/PROJECT-DISCOVERY.md](guides/PROJECT-DISCOVERY.md) — configure which
  SOP projects the controller observes, explicitly or by workspace discovery.
- [guides/DEVELOPMENT.md](guides/DEVELOPMENT.md) — build, test, format, and
  develop the controller.
- [guides/E2E-TESTING.md](guides/E2E-TESTING.md) — run, debug, and maintain the
  Playwright browser end-to-end tests.

## Current Boundary References at Retained Historical Paths

These references describe the current implementation and do not replace the
normative requirements, specifications, or architecture constraints.

- [history/CTRL001/BOUNDARY-CONTRACT.md](history/CTRL001/BOUNDARY-CONTRACT.md) —
  maintained operation table and test map.
- [history/CTRL001/BOUNDARY-INVENTORY.md](history/CTRL001/BOUNDARY-INVENTORY.md) —
  maintained code-derived inventory of controller reads and commands.

## Historical Documentation

Point-in-time verification artifacts kept for traceability. Non-normative: they
do not define current behavior.

- [history/CTRL001/BOUNDARY-VERIFICATION.md](history/CTRL001/BOUNDARY-VERIFICATION.md) —
  verification of the CTRL001 boundary.
- [history/PLAN-IDENTITY.md](history/PLAN-IDENTITY.md) — plan identity and
  named-plan resolution.
- [history/PLAN.md](history/PLAN.md) — the historical v1 implementation plan.
- [history/RETIRED-plan-sop-controller.md](history/RETIRED-plan-sop-controller.md) —
  retirement/supersession history of the `plan-sop-controller` plan.
- [history/HARD001-REPORT.md](history/HARD001-REPORT.md) — where the fixed
  240-second wait came from (external operator workflow only).
- [history/HARD004-REPORT.md](history/HARD004-REPORT.md) — inactivity and timeout
  are not lifecycle states.
- [history/HARD008-REPORT.md](history/HARD008-REPORT.md) — validation results and
  the final hardening/observability report.
- [history/WRAP/](history/WRAP/) — wrap-up verification reports
  ([WRAP-006](history/WRAP/WRAP-006-VERIFICATION.md),
  [WRAP-007](history/WRAP/WRAP-007-VERIFICATION.md),
  [WRAP-008](history/WRAP/WRAP-008-VERIFICATION.md),
  [WRAP-009](history/WRAP/WRAP-009-VERIFICATION.md),
  [WRAP-011](history/WRAP/WRAP-011-VERIFICATION.md),
  [WRAP-012](history/WRAP/WRAP-012-FINAL-REPORT.md)).
- [history/C2-009-REPORT.md](history/C2-009-REPORT.md) —
  human-decision dogfood report (Phase 2).

## Kept Operational Paths

These paths are deliberately left where they are: SOP or the code records them,
so moving them would change behavior or break provenance.

- `docs/PLAN-SOP-Controller.md` — SOP plan-archive source path (its `sha256` is
  recorded in `.agent-sdlc/archive/plan-sop-controller/plan.meta.json` and in the
  retirement record), so it is not moved.
- `docs/plans/PLAN-Controller-Mobile-Approvals-and-Dashboard.md` — the
  mobile-approvals plan (`COMPLETE`, archived): SOP's archive records this file as
  the plan's source path, so it is not moved.
- `docs/tasks/CLOSE-004.md` — task source executed via `sop run --task`.
- `docs/reports/pre-performance-closure/` — the (now archived) Pre-Performance
  Closure plan declares files in this directory as its task deliverables, so the
  path is operational.

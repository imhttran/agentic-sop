# AGENTS.md

Routing guide for implementation and review agents working on SOP Controller.

This is a pointer file, not a README. Start at the [documentation index](docs/README.md).

## Before changing behavior

1. Read the relevant product requirement: [docs/requirements/PRD.md](docs/requirements/PRD.md).
2. Read the relevant specification(s): [docs/specs/](docs/specs/).
3. Read the architecture constraints that apply: [docs/architecture/](docs/architecture/).
4. Read the SOP-recorded active implementation plan — the source path in `.agent-sdlc/plan.meta.json`, which `sop status` reports. Current planning lives in [docs/plans/](docs/plans/); completed and superseded plans live in [docs/history/plans/](docs/history/plans/).
5. Do not treat [docs/history/](docs/history/) as current specifications — it is point-in-time and non-normative.
6. SOP remains the workflow authority: read SOP state and delegate commands; never mutate SOP state or reimplement workflow logic (see [docs/architecture/SOP-BOUNDARY.md](docs/architecture/SOP-BOUNDARY.md)).

## Task-to-document mapping

```text
Execution changes:
  docs/specs/EXECUTION.md
  docs/specs/WORKFLOW.md
  docs/architecture/SOP-BOUNDARY.md

Review changes:
  docs/specs/REVIEW.md

OpenJEV changes:
  docs/specs/OPENJEV.md
  docs/specs/REVIEW.md

Approval changes:
  docs/specs/HUMAN-APPROVAL.md
  docs/architecture/SOP-BOUNDARY.md

Activity / streaming changes:
  docs/specs/ACTIVITY.md
  docs/reference/CLI.md

Agent / provider changes:
  docs/specs/AGENT-PROVIDER.md

Security changes:
  docs/specs/SECURITY.md

CLI / config changes:
  docs/reference/

Workflow status / recovery changes:
  docs/specs/WORKFLOW.md
  docs/reference/STATUS-AND-RECOVERY.md
```

## Authority order

```text
requirements/PRD.md  →  specs/*.md  →  architecture/*.md  →  plans/*.md  →  SOP execution
```

Specifications define required behavior. Plans describe work and MUST NOT
override specifications. Historical artifacts provide traceability only.

## Repository layout

```text
docs/requirements/   product intent (PRD)
docs/architecture/   structural boundaries and ownership
docs/specs/          normative required behavior
docs/plans/          implementation plans
docs/reference/      commands, states, and values
docs/guides/         how to use or develop
docs/reports/        point-in-time execution evidence (non-normative)
docs/tasks/          single-task sources run via `sop run --task`
docs/history/        non-normative point-in-time artifacts
```

SOP matches a plan by its recorded source path, so operational paths stay put:
the **active** plan's path is recorded in `.agent-sdlc/plan.meta.json`;
`docs/PLAN-SOP-Controller.md` is a **superseded** plan kept because SOP's plan
archive records it as a source; `docs/tasks/CLOSE-004.md` is a `sop run --task`
source; `docs/reports/pre-performance-closure/` holds the (now archived)
Pre-Performance Closure plan's declared deliverables. When SOP records no active
plan (`sop status` reports none), none of these is an active planning source. Do
not move these while SOP records them — see
[docs/README.md](docs/README.md#kept-operational-paths).

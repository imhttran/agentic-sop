# PLAN — Phase 5.4: Unified Work Items, Prompt Execution & SOP Skill

**Status:** Done (implemented and committed).

## Objective

Let SOP's existing governed execution workflow operate on three input forms — planned
project tasks, ad-hoc operator prompts, and agent/assistant requests through a skill
— by introducing:

- a unified `WorkItem` abstraction (`internal/workitem`);
- a new `sop prompt` command;
- a thin, reusable SOP skill (`skills/sop/SKILL.md`).

The prompt and skill paths MUST reuse SOP's existing capability model, JEV evidence
seam, deterministic SMALL/MEDIUM/LARGE routing, model resolution, provider
validation, agent/harness execution, and validation/review boundaries. No second
workflow engine is introduced, and no SOP policy is reimplemented in the skill.

## Core invariant

```text
Task / Prompt / Skill
      ↓
   WorkItem
      ↓
   Capability → JEV evidence → SOP router → SMALL|MEDIUM|LARGE
      ↓
 model.Resolve → provider validation → agent/harness → execution
      ↓
  Validation / Review → Run artifacts
```

> JEV analyzes. SOP decides. Providers report capabilities. Work items carry no policy.

## Deliverables

| ID       | Deliverable                                                                  | State |
| -------- | ---------------------------------------------------------------------------- | ----- |
| P5.4-001 | Fail closed when routing selects a model but no agent factory exists         | Done  |
| P5.4-002 | `internal/workitem`: `Kind`, `WorkItem`, `Validate`                          | Done  |
| P5.4-003 | `workitem.FromTask` task → WorkItem adapter                                  | Done  |
| P5.4-004 | `workitem.FromPrompt` prompt → WorkItem creation                             | Done  |
| P5.4-005 | `sop prompt` (read-only path)                                                | Done  |
| P5.4-006 | Prompt JEV triage → router → `model.Resolve`                                 | Done  |
| P5.4-007 | Provider validation + capability guard for prompts                           | Done  |
| P5.4-008 | Prompt artifacts (`prompt.md`, `routing.json`, `result.md`, `metadata.json`) | Done  |
| P5.4-009 | IMPLEMENT prompts reuse the governed implementation lifecycle                | Done  |
| P5.4-010 | Structured result (`--json` / `metadata.json`)                               | Done  |
| P5.4-011 | The SOP skill + examples                                                     | Done  |
| P5.4-012 | Skill validation tests                                                       | Done  |
| P5.4-013 | Regression, boundary, and documentation tests                                | Done  |

## Key decisions

- **Capability is explicit.** The default is `plan` (read-only); an unknown
  `--capability` is a usage error. `IMPLEMENT` is never inferred from prose.
- **No keyword routing.** Prompt routing reads typed JEV evidence only; without
  evidence it falls back to MEDIUM. No prompt-specific classes or router.
- **Two execution modes.** Read-only capabilities run one bounded call; `IMPLEMENT`
  runs the governed implementation lifecycle (a `taskfile.Spec` projected from the
  prompt), so it cannot shortcut to a bare `Generate`.
- **No automatic escalation for prompts** in this slice; `d.escalation` stays
  disabled on the prompt path. _(Superseded by the Phase 5 hardening follow-up below:
  bounded escalation now applies to an `implement` prompt, which runs the same
  governed lifecycle; a read-only prompt still never escalates.)_
- **Reuse, not duplication.** Provider validation, the capability guard, the router,
  and the JEV checkpoint interpretation (`runJEVCheckpoint`) are the same code the
  task path uses.

## Acceptance criteria

Met: `WorkItem` exists; tasks adapt without behavior change; prompts become work
items; `sop prompt` supports direct text, files, and explicit capabilities; the
default capability is read-only; routing is deterministic and evidence-based;
provider validation is reused without fallback; capability guards are enforced;
read-only prompts run on text-only providers; `IMPLEMENT` cannot run through an
incapable provider and uses the governed path; prompt runs are auditable under the
existing artifact contract; `sop run` is unchanged; the skill invokes SOP; all tests
pass.

## Known limitations

Tracked in [`BACKLOG.md`](BACKLOG.md) rather than left implicit:

- **Bounded escalation did not apply to prompt runs.** Resolved by the follow-up
  below: it now applies to an `implement` prompt.
- **Prompt run ids are second-resolution** with a collision counter; they are unique
  locally but not globally across machines.

## Follow-up (Phase 5 hardening)

- **Bounded escalation now applies to an `implement` prompt.** The prompt path
  populates the same recovery policy a task uses (`applyEscalationEnabled`), and an
  `implement` prompt runs the governed lifecycle through the same `runAttempts` seam,
  so when `models.escalation_enabled` is on it escalates `small → medium → large`
  within the bound and records one attempt per try under the prompt run. A read-only
  prompt is a single bounded call with no gate and never escalates. See
  [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) §8.

## Out of scope

Automatic escalation for **read-only** prompts, provider fallback, provider scoring,
cheapest/latency routing, prompt marketplaces, remote/distributed execution, MCP
orchestration, arbitrary shell execution from prompt text, LLM-selected capabilities
or classes, parallel competing models, learned routing, and any separate policy engine
in the skill.

## Validation

```bash
make check
go test -race ./internal/workitem/... ./internal/cli/... ./internal/router/... \
  ./internal/model/... ./internal/provider/... ./internal/agent/... \
  ./internal/jev/... ./internal/run/... ./internal/skill/...
scripts/check-doc-links.sh
```

## See also

- [`../specs/WORK-ITEMS.md`](../specs/WORK-ITEMS.md)
- [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md)
- [`../architecture/execution.md`](../architecture/execution.md)
- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md)

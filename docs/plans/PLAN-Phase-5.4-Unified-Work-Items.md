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

## Follow-up (Phase 5.4 hardening)

- **The Zed command surface (`/sop`, `/sop-prompt`, `/sop-plan`, `/sop-review`,
  `/sop-diagnose`, `/sop-test`, `/sop-implement`).** Zed discovers skills as flat
  folders under `~/.agents/skills/` or `<project>/.agents/skills/`, and exposes each as
  a slash command named after the folder. Six one-per-capability aliases ship beside
  the canonical `skills/sop` skill; each is a few lines that call
  `sop prompt --capability <capability>` and carry no policy. They install with
  `make install-skills` (`scripts/install-zed-skills.sh`, idempotent, uninstallable,
  never overwrites a non-SOP entry, needs no root). `/sop-implement` is hidden from the
  agent's autonomous catalog. See [`../guides/ZED-SKILLS.md`](../guides/ZED-SKILLS.md)
  and [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) §13.

- **The capability guard now applies to the final executing agent.** With routing on,
  an `IMPLEMENT` prompt is no longer rejected because the _default_ provider lacks
  `IMPLEMENT` when routing builds a different, tool-capable agent. The guard and
  provider validation apply to the routed final selection (and to the default agent at
  the routing seam when no class is selected); `sop run` and `--task` follow the same
  rule. See [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) §4.

- **`--file` is confined against symlink resolution.** A project-local symlink that
  points outside the project is rejected, not read. See
  [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) §12.

## Follow-up (multi-agent skill installation)

- **The SOP skill installs for Claude Code as well as Zed.** Both agents discover a
  skill the same way — a flat folder containing a `SKILL.md`, exposed as a `/` command
  named after the folder — so the **same** canonical tree under [`skills/`](../../skills)
  serves both and the capability mapping is shared. A single installer
  (`scripts/install-skills.sh`, with thin `install-zed-skills.sh` /
  `install-claude-skills.sh` wrappers) links the tree into `~/.agents/skills` or
  `~/.claude/skills` (or the project-local equivalents). It is idempotent, touches
  only SOP-owned entries, refuses to clobber a foreign entry at a SOP name, and needs
  no root; `make install-skills` installs every supported agent that is present, and
  `make install-skills-zed` / `make install-skills-claude` install one. Neither
  adapter carries routing, provider, model-class, approval, or lifecycle policy: both
  call `sop prompt`, and `/sop-implement` enters the same governed lifecycle on each.
  See [`../guides/ZED-SKILLS.md`](../guides/ZED-SKILLS.md) and
  [`../guides/CLAUDE-SKILLS.md`](../guides/CLAUDE-SKILLS.md).

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

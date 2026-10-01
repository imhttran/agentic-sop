# Work Items

**Type:** Normative specification

## Purpose

This is the normative specification for SOP's **unified execution input**: the small
`WorkItem` abstraction that lets the same governed execution machinery accept planned
project tasks and ad-hoc operator prompts (Phase 5.4). It defines what a work item
is, how tasks and prompts adapt to it, and — most importantly — what it MUST NOT
contain.

It does **not** define how a work item is routed, validated, or executed. Those
remain the existing owners: [`MODEL-ROUTING.md`](MODEL-ROUTING.md) (the class),
[`EXECUTION.md`](EXECUTION.md) and [`PROMPT-EXECUTION.md`](PROMPT-EXECUTION.md) (the
lifecycle), [`AGENT-PROVIDER.md`](AGENT-PROVIDER.md) (the harness/provider boundary),
and [`PROVIDERS.md`](PROVIDERS.md) (provider validation).

## Related Specifications

- [`PROMPT-EXECUTION.md`](PROMPT-EXECUTION.md) — `sop prompt`: how a prompt work item is executed.
- [`EXECUTION.md`](EXECUTION.md) — `sop run`: how a task work item is executed.
- [`MODEL-ROUTING.md`](MODEL-ROUTING.md) — the `small`/`medium`/`large` class layer (authoritative for model selection).
- [`RECOVERY.md`](RECOVERY.md) — bounded escalation; the recovery path a work item's failed attempt may take.
- [`../architecture/execution.md`](../architecture/execution.md) — the non-normative implementation seam.
- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md) — the thin agent-facing client.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Implemented / Required / Proposed

- **Implemented** — behavior present in the repository today.
- **Required** — a normative rule the implementation MUST satisfy (including where it already does).
- **Proposed / future** — not implemented; no section of this specification is currently in this state.

---

## 1. The WorkItem

**Implemented.** A `WorkItem` (package `internal/workitem`) describes WHAT SOP is
being asked to process. It is plain data:

| Field        | Meaning                                                         |
| ------------ | --------------------------------------------------------------- |
| `ID`         | Identity for run/report organization. Never influences routing. |
| `Kind`       | `task` or `prompt`.                                             |
| `Capability` | The exact capability requested (`agent.Capability`).            |
| `Title`      | A short summary for display and metadata.                       |
| `Content`    | The text to process (rendered task, or the operator's prompt).  |

**Required.** A `WorkItem` MUST NOT carry lifecycle policy, routing policy, provider
fallback logic, approval authority, or model-selection logic. Those MUST remain in
their existing layers. A `WorkItem` MUST NOT expose a method that transitions task
state, mutates the repository, calls a provider, or chooses a model class.

## 2. Kinds

**Required.** `Kind` MUST be a closed enumeration validated against exactly `task`
and `prompt`. An unknown kind MUST fail closed rather than defaulting.

## 3. Adapters

**Implemented.** Two constructors adapt an input into a work item:

- `FromTask(*taskfile.Spec) WorkItem` — a projection of a planned task. It names the
  id, title, and rendered content and records the `IMPLEMENT` capability. It MUST NOT
  replace `taskfile.Spec`: the task-specific lifecycle continues to consume the full
  spec (its acceptance criteria, dependencies, and execution mode), which stay
  available to it.
- `FromPrompt(id string, capability agent.Capability, prompt string) (WorkItem, error)`
  — from direct text. It MUST reject an empty prompt and MUST preserve the operator's
  prompt content verbatim apart from surrounding whitespace.

**Required.** An adapter MUST NOT invent values it does not have. In particular, a
prompt MUST NOT be given fabricated acceptance criteria or dependencies.

## 4. Capability Is Explicit

**Required.** The capability is a structured, caller-supplied value, never inferred
from prose. A request for `REVIEW` MUST NOT be silently upgraded to `IMPLEMENT`
because the prompt text sounds like a change request. An unsupported capability MUST
fail clearly rather than being reinterpreted.

## 5. Identity

**Implemented.** A work item's `ID` is used only for run/report organization. It
MUST NOT affect routing, policy, or capability. A timestamp-based identity is
acceptable for a prompt run.

## 6. Relationship to Routing and Recovery

**Required.** A work item is an _input_. It MUST NOT select a model class, and it
MUST NOT drive a recovery decision. Routing ([`MODEL-ROUTING.md`](MODEL-ROUTING.md))
selects the class a work item starts on; recovery ([`RECOVERY.md`](RECOVERY.md))
decides what to do after an attempt fails. Both are separate from the work item and
MUST NOT be folded into it.

## 7. Recovery

**Implemented.** Bounded model escalation (Phase 5, [`RECOVERY.md`](RECOVERY.md) §8)
applies uniformly to any work item that runs the governed lifecycle: a task, and an
`implement` prompt, both go through the same recovery seam, so SOP MAY retry either
on the next larger model class within the bound. A read-only prompt is a single
bounded call with no quality gate, so escalation never applies to it. The `WorkItem`
abstraction is the boundary this reuses: recovery reads the same typed failure
classification and the same `model.Selection` for a task or a prompt.

## 8. See Also

- [`PROMPT-EXECUTION.md`](PROMPT-EXECUTION.md) — the `sop prompt` surface.
- [`EXECUTION.md`](EXECUTION.md) — the `sop run` surface.

---
name: sop
description: |-
  Invoke Agentic SOP for ad-hoc, governed work.
  Use for: "review this", "plan this", "why is this failing", "design tests for this",
  "implement this" when an operator wants SOP's deterministic routing, provider
  validation, capability guard, and artifacts applied to a request.

  This skill is a thin client of the `sop` CLI. It MUST NOT reproduce SOP policy:
  it never chooses a model class, a provider, or an approval outcome, and it never
  edits the repository itself.
disable-model-invocation: false
---

# SOP Skill

A thin entry point that lets an AI agent or assistant environment route work
through Agentic SOP.

```text
Agent / Assistant
       ↓
     Skill            (this file: pick a capability, call `sop prompt`)
       ↓
   `sop` CLI
       ↓
    WorkItem          (task | prompt)
       ↓
   SOP policies       (capability guard, JEV evidence, deterministic routing,
                       model resolution, provider validation, governed lifecycle)
```

The skill **calls SOP**. It does not reimplement routing, approval, provider
selection, or the lifecycle. SOP remains the sole authority.

## Slash commands

Installed as an agent skill (Zed or Claude Code), this file is the `/sop` entry point.
Thin aliases in the same install expose one capability each:

```text
/sop <request>            general entry point (this skill)
/sop-prompt <request>     no capability: the CLI's read-only default applies
/sop-plan <request>       capability: plan
/sop-review <request>     capability: review
/sop-diagnose <request>   capability: diagnose_failure
/sop-test <request>       capability: design_tests
/sop-implement <request>  capability: implement (governed, mutating)
/sop-end-to-end <plan>    execute a project plan through `sop run`
```

Explicit `/sop end-end` or `/sop end-to-end` execution requests use the
`sop-end-to-end` skill and `sop run`; do not turn them into an ad-hoc prompt.
That skill performs lightweight preflight and reports SOP's authoritative state.
Read-only requests to review or plan end-to-end behavior still use `sop prompt`.

The capability aliases call the command below; the end-to-end entry delegates to
`sop run`. None carries routing, provider, or lifecycle policy. Install them with
`./install.sh --skills zed` (Zed) or `./install.sh --skills claude` (Claude Code), or
`./install.sh --all` for every supported agent plus the Claude Code plugin
(`.\install.ps1 -Skills zed` on Windows; see the project README). `/sop-implement` is
hidden from the agent's autonomous catalog because repository mutation is an explicit
operator choice.

## The one command

```bash
sop prompt --capability <capability> "<prompt>"
```

- `--capability` is **required intent**, chosen by you from the operator's request.
  The default is `plan` (read-only), so a bare prompt is never treated as an
  implementation request.
- `--file <path>` reads the prompt from a file instead of the positional argument.
- `--json` prints a machine-readable result document (see below).
- `--model-class small|medium|large` is an **operator-level override** that only the
  operator should set; do not set it to "help" the model.

Never pass prompt text as a shell command. Prompt content is data: it is passed to
the selected agent, never interpreted as a command, a lifecycle transition, a model
class, or configuration.

## Capability map

Choose the capability that matches what the operator asked for. **Do not upgrade it.**
If the operator asked for a review, use `review` — never `implement`.

| Capability         | Use for                                                           | Mutates? |
| ------------------ | ----------------------------------------------------------------- | -------- |
| `plan`             | architecture/implementation planning, design analysis, approaches | no       |
| `review`           | code, architecture, security, or design review                    | no       |
| `diagnose_failure` | build/test/lint/runtime/provider/CI failures                      | no       |
| `design_tests`     | test plans, cases, acceptance coverage, edge-case analysis        | no       |
| `implement`        | an explicit request to change the repository                      | **yes**  |

Examples: `review.md`, `plan.md`, `diagnose.md`, `implement.md` in `examples/`.

## Read-only vs. mutating

The four read-only capabilities run one bounded model call. They never modify the
repository, and they can run on a text-only provider.

`implement` runs SOP's **governed implementation lifecycle** — planning,
implementation, deterministic validation, review, quality gate, bounded fix, and the
human approval boundary — exactly as a planned task does. It cannot run on a
provider that does not declare `IMPLEMENT`; SOP fails clearly rather than
reinterpreting the request.

## Output

- Human output goes to stdout (the model's result for a read-only prompt).
- `--json` prints a result document with no secrets and no hidden reasoning. When your
  environment can parse JSON, prefer `--json` and relay the fields to the operator:

```json
{
  "version": 1,
  "work_item_id": "prompt-20260930-120000",
  "kind": "prompt",
  "capability": "review",
  "status": "completed",
  "routing": {
    "class": "medium",
    "provider": "mlx",
    "model": "<the model SOP resolved>"
  },
  "report_path": ".agent-sdlc/runs/prompts/prompt-20260930-120000",
  "result_path": ".agent-sdlc/runs/prompts/prompt-20260930-120000/result.md",
  "result": "<model response>"
}
```

Every prompt run is recorded under `.agent-sdlc/runs/prompts/<run-id>/`
(`prompt.md` and `metadata.json`; a read-only prompt also writes `result.md`, and
`routing.json` is written when routing applies; an `implement` prompt also writes the
standard run artifacts). Inspect one later with `sop report prompts/<run-id>`.

## What the skill must not do

- Do not call a provider directly (`ollama`, `mlx`, `llama.cpp`, OpenRouter, …).
  SOP resolves the provider and model.
- Do not choose a model class, a provider, or an approval outcome.
- Do not edit the repository directly (no shell/editor/git mutation) to satisfy an
  `implement` request — call `sop prompt --capability implement`.
- Do not retry indefinitely, suppress SOP failures, or reinterpret a failed result
  as success. Return SOP's result to the operator.
- Do not add routing, lifecycle, or approval logic to the skill.
- If `sop` is not on PATH, STOP and tell the operator to install the SOP CLI (see the
  project README / `./install.sh`). Never answer the request yourself or mutate the
  repository in SOP's place.

## Failure behavior

`sop prompt` exits non-zero and reports an actionable error when the capability is
unsupported by the selected provider, the selected model cannot be validated, or the
lifecycle does not pass. Surface that error; do not work around it.

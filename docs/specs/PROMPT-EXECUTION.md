# Prompt Execution

**Type:** Normative specification

## Purpose

This is the normative specification for `sop prompt`: how an ad-hoc operator request
is executed through SOP's existing governed machinery (Phase 5.4). It defines the
command surface, the default capability, routing, capability enforcement, the
read-only and mutating execution modes, provider validation, artifacts, and the
structured result.

It does **not** define a second workflow engine. A prompt is a second _input form_
for the same machinery a task uses. Model-class selection remains
[`MODEL-ROUTING.md`](MODEL-ROUTING.md); provider behavior remains
[`PROVIDERS.md`](PROVIDERS.md); the implementation lifecycle remains
[`EXECUTION.md`](EXECUTION.md).

## Related Specifications

- [`WORK-ITEMS.md`](WORK-ITEMS.md) — the `WorkItem` input abstraction a prompt becomes.
- [`EXECUTION.md`](EXECUTION.md) — the implementation lifecycle an `implement` prompt reuses.
- [`MODEL-ROUTING.md`](MODEL-ROUTING.md) — the `small`/`medium`/`large` class layer (authoritative for model selection).
- [`PROVIDERS.md`](PROVIDERS.md) — provider validation and the read-only provider boundary.
- [`AGENT-PROVIDER.md`](AGENT-PROVIDER.md) — the harness/provider/model boundary and the capability guard.
- [`RECOVERY.md`](RECOVERY.md) — bounded escalation (not applied to prompts in this slice; see §11).
- [`../reference/CLI.md`](../reference/CLI.md) — the command surface.
- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md) — the thin skill that calls `sop prompt`.
- [`../guides/ZED-SKILLS.md`](../guides/ZED-SKILLS.md) — installing the Zed command aliases (`/sop-plan`, `/sop-review`, …).

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. The Command

**Implemented.** `sop prompt` accepts:

```text
sop prompt [--capability CAP] [--model-class small|medium|large] [--json] [--file PATH | PROMPT]
```

**Required.** At most one of a positional prompt and `--file` MAY be given. An empty
prompt (no positional argument and no file) MUST be a usage error. A missing or empty
prompt file MUST be an error.

## 2. Capabilities

**Implemented.** The accepted capabilities are exactly:

```text
plan, design_tests, diagnose_failure, review, implement
```

They reuse the existing `agent.Capability` vocabulary; no prompt-specific capability
enum exists. `FIX` is an internal lifecycle capability and is not offered as a prompt
capability.

**Required.** An unknown capability MUST be a usage error naming the accepted set.

## 3. Default Capability

**Required.** A bare `sop prompt` (no `--capability`) MUST default to `plan`, a
read-only, text-oriented capability. A prompt MUST NOT default to `IMPLEMENT`.

## 4. Capability Enforcement

**Required.** Before execution, the prompt's capability MUST be checked against the
selected agent's declared capabilities (the existing `agent` capability guard). A
capability the selected model/harness cannot serve MUST fail clearly. SOP MUST NOT
reinterpret the capability and MUST NOT silently switch provider or model.

**Required.** The check MUST apply to the agent that will actually **execute** — the
FINAL per-prompt selection, after routing and provider validation (§5, §7) — not to a
default agent the router will replace. When automatic routing is enabled, the routed
class builds, validates, and guards its own agent, and the default agent MUST NOT be
rejected merely because it lacks the capability: a read-only default provider with a
tool-capable routed class is a valid configuration. When routing is off, the default
agent is the executing agent and is guarded before any work.

For example, an `IMPLEMENT` prompt against a text-only provider (OpenAI-compatible
llama.cpp or MLX/oMLX, which declare only `PLAN`, `DESIGN_TESTS`,
`DIAGNOSE_FAILURE`, `REVIEW`) MUST fail rather than being forwarded.

## 5. Routing

**Required.** A prompt MUST use the same deterministic `small`/`medium`/`large`
router a task uses. It MUST NOT introduce prompt-specific classes and MUST NOT
implement a second router.

**Required.** Routing MUST be deterministic and MUST NOT read the prompt text. There
MUST NOT be keyword-based routing (for example matching a word such as "simple").
Only typed JEV evidence participates; without usable evidence the router resolves to
the safe default (MEDIUM) rather than guessing a class from prose. A typed JEV
signal (for example high risk or cross-cutting scope) escalates according to the
existing router rules.

**Required.** A manual `--model-class` override MUST win over the router, exactly as
it does for a task.

**Required.** JEV evidence is evidence only. JEV MUST NOT return the authoritative
class; SOP's router decides it.

## 6. Execution Modes

**Implemented.** A prompt runs on one of two paths, chosen by whether its capability
mutates the repository:

- **Read-only** (`PLAN`, `DESIGN_TESTS`, `DIAGNOSE_FAILURE`, `REVIEW`): one bounded
  agent call. It MUST NOT modify the repository, and it MAY run on a text-only
  provider.
- **Mutating** (`IMPLEMENT`): SOP's **governed implementation lifecycle** —
  planning, implementation, deterministic validation, review, quality gate, bounded
  fix, and the human approval boundary — exactly as a planned task runs.

**Required.** An `IMPLEMENT` prompt MUST NOT shortcut to a bare `Generate` call. It
MUST run through the governed implementation lifecycle, and its repository mutation
MUST go through the existing tool/harness authorization path.

## 7. Provider Validation

**Required.** The FINAL per-prompt model selection MUST pass through the existing
provider validation seam ([`PROVIDERS.md`](PROVIDERS.md)) when
`providers.validate` is enabled. Routing and validation MUST run before any agent
work. An `implement` prompt's final per-task selection is validated by the lifecycle
exactly as a task's is.

**Required.** Provider validation MUST NOT substitute a provider or model. A model
that cannot be served MUST stop the prompt with an actionable error, and no silent
provider fallback is permitted.

## 8. Automatic Escalation

**Implemented.** A read-only prompt performs **no** automatic `small → medium →
large` escalation: it is a single bounded call with no quality gate, so there is
nothing to recover. An `implement` prompt runs the governed implementation lifecycle,
so when bounded escalation is enabled it follows the **same** policy as a task — SOP
MAY retry the prompt on the next larger model class within the bound, recording one
attempt per try. The policy is OFF by default (`models.escalation_enabled`, exactly as
for a task) and MUST NOT be applied to a read-only prompt. See
[`RECOVERY.md`](RECOVERY.md) §8.

## 9. Artifacts

**Implemented.** A prompt run is recorded under
`.agent-sdlc/runs/prompts/<run-id>/`:

- `prompt.md` — the operator's prompt with its kind and explicit capability.
- `routing.json` — the routing decision, using the **existing** routing artifact
  contract (no prompt-specific schema). Omitted when no routing applied.
- `result.md` — the model's result (read-only prompts).
- `metadata.json` — the structured result document (§10).
- An `implement` prompt additionally writes the standard run artifacts
  (`task.md`, `plan.md`, `implementation.md`, `report.json`, …), and — when bounded
  escalation applies (§8) — one attempt record per try under `attempts/NNN.json`.

**Required.** No artifact MAY contain a credential or hidden reasoning.

## 10. Structured Result

**Implemented.** `--json` prints a result document, and the same document is
persisted as `metadata.json`:

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
    "model": "mlx-community/Qwen3-4B-4bit"
  },
  "report_path": ".agent-sdlc/runs/prompts/prompt-20260930-120000",
  "result_path": ".agent-sdlc/runs/prompts/prompt-20260930-120000/result.md",
  "result": "<model response>"
}
```

**Required.** The document MUST NOT expose chain-of-thought or secrets.

## 11. Reporting

**Implemented.** `sop report` inspects a prompt run: `sop report prompts/<run-id>`
renders the prompt's kind, capability, status, routing, and result path. With no
argument it reports the newest run of either kind — a task run or a prompt run. This
reuses the existing report command; SOP MUST NOT add a separate prompt reporting
system.

## 12. Security

**Required.** Prompt text and skill input are untrusted input. SOP MUST NOT interpret
prompt text as a shell command, a lifecycle transition, routing policy, an approval,
configuration, or provider credentials. Prompt text MUST remain data passed to the
selected agent. Repository mutation MUST require the existing tool/harness
authorization path.

**Required.** `--file` MUST be confined to the project directory. The confinement MUST
be enforced against symlink resolution, not only lexically, so a project-local symlink
pointing outside the project is rejected rather than read.

## 13. The Skill

**Implemented.** A thin agent-facing skill (`skills/sop/SKILL.md`) calls
`sop prompt`. **Required.** The skill MUST NOT invoke a provider directly, MUST NOT
choose a model class or provider, and MUST NOT reimplement routing, approval, or the
lifecycle. It is a client of SOP.

**Implemented.** One Zed command alias per capability ships beside the canonical
skill, each a thin adapter that calls `sop prompt` with its capability fixed:

```text
/sop          general entry point (the canonical skill)
/sop-prompt   no capability: the CLI's read-only default applies
/sop-plan     plan
/sop-review   review
/sop-diagnose diagnose_failure
/sop-test     design_tests (/sop-test is the alias for the canonical design_tests)
/sop-implement implement (governed; the only mutating command)
```

**Required.** An alias MUST NOT contain routing, provider, model-class, approval, or
lifecycle policy, and MUST NOT mutate the repository itself: it selects a capability
and delegates. It MUST NOT upgrade a read-only request into `implement`, and an
`implement` request MUST enter the governed lifecycle (§6). When the `sop` CLI is
unavailable the alias MUST fail closed rather than let the calling agent do the work.
Installation is described in [`../guides/ZED-SKILLS.md`](../guides/ZED-SKILLS.md).

## 14. Failure Behavior

**Required.** `sop prompt` MUST exit non-zero and report an actionable error when the
capability is unsupported by the selected provider, the selected model cannot be
validated, or the lifecycle does not pass. A failure MUST NOT be reported as success.

## 15. See Also

- [`WORK-ITEMS.md`](WORK-ITEMS.md) — the input abstraction.
- [`EXECUTION.md`](EXECUTION.md) — the lifecycle an `implement` prompt reuses.

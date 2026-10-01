# Agent Provider

**Type:** Normative specification

## Purpose

This document is the normative specification for SOP's agent boundary: the **Harness**/**Provider**/**Model** separation, harness and provider options, model requirement rules, the configuration matrix and precedence, the command-agent protocol and structured outcome, provider capability detection, tool-harness bounds and phases, and diagnostic sinks. Configuration keys are catalogued in [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Agent Harness Adapter, Agent Providers.
- [../PRD.md](../PRD.md) — §10; [EXECUTION.md](EXECUTION.md) — the up-front provider check; [RECOVERY.md](RECOVERY.md) — requeue and retry; [VALIDATION.md](VALIDATION.md), [QUALITY.md](QUALITY.md) — checks and the gate.
- [PROVIDERS.md](PROVIDERS.md) — the provider/runtime abstraction (identity, health, discovery, capabilities, opt-in validation) that sits beneath the agent boundary.
- [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Harness, Provider, and Model

SOP MUST keep three concepts independent: **Harness** — the layer executing agent logic (calling the model via the provider, managing tools and tool-call loops, enforcing bounds/timeouts, handling structured outcomes, and applying safety boundaries for file access, git, and command execution); **Provider** — the raw model source, which MUST NOT be assumed to supply tool-calling, file I/O, git, or mutation, and MUST NOT be treated as a harness; and **Model** — the identifier within a provider. The workflow MUST NOT depend permanently on one model or harness.

## 2. Options

Harness MUST be `tool` (a local tool-calling harness such as `sop-ollama-agent`) or `command` (a subprocess adapter running an external command that implements the agent interface). Provider MUST be `ollama` (local text-only), `llamacpp` (OpenAI-compatible), `mlx` (an Apple-Silicon MLX / oMLX runtime behind an OpenAI-compatible boundary), `openai_compatible` (a generic OpenAI-compatible endpoint — oMLX, vLLM, LM Studio, LocalAI, ...), or `command` (an external subprocess already a full agent). `llamacpp`, `mlx`, and `openai_compatible` share one OpenAI-compatible execution transport; see [PROVIDERS.md](PROVIDERS.md) §2a.

## 3. Model Requirement Rules

For `ollama`/`llamacpp`/`mlx`/`openai_compatible` a model MUST be specified (config `agent.model` or the provider's env var); the run MUST fail clearly if neither provides one. The command provider MUST NOT require a model. The command harness MUST require `SOP_AGENT_COMMAND`; the run MUST fail if unset.

## 4. Configuration Matrix

| Harness   | Provider   | Config keys                                      | Environment variables                                                                         | Model? |
| --------- | ---------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------- | ------ |
| `tool`    | `ollama`   | `agent.harness`, `agent.provider`, `agent.model` | `SOP_OLLAMA_BASE_URL`, `SOP_OLLAMA_MODEL`, `SOP_OLLAMA_TIMEOUT`                               | Yes    |
| `tool`    | `llamacpp` | `agent.harness`, `agent.provider`, `agent.model` | `SOP_LLAMACPP_BASE_URL`, `SOP_LLAMACPP_MODEL`, `SOP_LLAMACPP_TIMEOUT`, `SOP_LLAMACPP_API_KEY` | Yes    |
| `tool`    | `mlx`      | `agent.harness`, `agent.provider`, `agent.model` | `SOP_MLX_BASE_URL`, `SOP_MLX_MODEL`, `SOP_MLX_TIMEOUT`, `SOP_MLX_API_KEY`                     | Yes    |
| `command` | `command`  | `agent.harness` (optional; defaults to command)  | `SOP_AGENT_COMMAND` (required), `SOP_AGENT_PROVIDER` (optional)                               | No     |

## 5. Configuration Precedence

Settings MUST resolve highest-first: (1) environment variables (`SOP_AGENT_PROVIDER`, `SOP_OLLAMA_MODEL`, `SOP_AGENT_COMMAND`, …); (2) `.agent-sdlc/config.yaml`; (3) hardcoded defaults (the command harness when nothing is specified). Env MUST override config; when it selects the command harness, the config's tool/provider/model settings MUST be ignored. See [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## 6. Turning a Text-Only Provider into a Coding Agent

A text-only Ollama/llama.cpp endpoint alone is **not** a coding agent and cannot mutate a repository. The `tool` harness MUST add the tools, safety boundaries, and structured logic that make it one: file tools (`read_file`, `write_file`, `create_file`, `delete_file`, `restore_file`, `list_files`, `search_files`), git tools (`git_status`, `git_diff`), a bounded allow-listed `run_command`, tool-loop bounds, and structured outcomes. Layers MAY be composed (the `command` harness running a `tool` harness subprocess, which holds the provider settings). SOP MUST retain validation, review, retries, human gates, and workflow state; the harness MUST remain an implementation adapter only.

## 7. Provider-Neutral Interface and Capability Detection

The boundary MUST expose a provider-neutral interface — conceptually `Execute(task, context) -> result`, `Review(diff, criteria) -> findings`, `Fix(task, findings) -> result` — so the workflow does not depend on one model or harness. Each provider MUST declare its capabilities; a checked wrapper MUST reject, naming the supported set, an unsupported-capability request. A run MUST check up front that the provider can `IMPLEMENT` before any task and MUST reject one that cannot. See [EXECUTION.md](EXECUTION.md).

## 8. Command-Agent Protocol and Structured Outcome

A command agent MUST read JSON requests from stdin and write JSON responses to stdout, implementing the agent interface. For mutating capabilities (`IMPLEMENT`, `FIX`) it MAY return a structured execution outcome — `{"status":"completed","summary":…,"changes_expected":true|false}`, `{"status":"needs_human","reason":…}`, or `{"status":"failed","reason":…}`. SOP's reaction MUST be structural, never inferred by scanning prose:

```text
completed + changes_expected true  + non-empty change set → validate → review → gate
completed + changes_expected true  + no changes           → FAIL (claimed changes, produced none)
completed + changes_expected false + no changes           → run configured validation, then PASS
needs_human                                               → stop with NEEDS_HUMAN (requeued)
failed                                                    → FAIL (terminal)
```

Any other output MUST keep the legacy behaviour, judged by the change produced. The `claimed changes, produced none` failure is SOP's own deterministic verdict, so it MUST be classified (`NO_CHANGES_PRODUCED`) rather than left unclassified; when bounded escalation is enabled it MAY then be retried on a larger model — see [RECOVERY.md](RECOVERY.md) §8. See also [VALIDATION.md](VALIDATION.md), [QUALITY.md](QUALITY.md).

## 9. Tool-Harness Bounds and Phases

Bounds MUST be per capability. `DESIGN_TESTS`/`DIAGNOSE_FAILURE` get 12 read-only turns; `PLAN` (8) and `REVIEW` (6) run bounded read-only discovery then a tool-free **synthesis** (2 turns) that MUST produce the document, so neither can fail by exploring forever; `IMPLEMENT` (32) and `FIX` (24) share a phased loop — **discover**, **change** (first write plus targeted checks), **finalize** — so a `FIX` that wrote its repair hands back instead of running the build/tests to the ceiling. Finalization MUST be **mutation-aware**: tools are withdrawn only once a change is observed _and_ the writer has stopped; a write during finalize MUST NOT be refused and resumes change.

A phased run MUST also carry a deterministic **no-progress guard**, independent of the iteration ceiling. The only progress signal is a **successful repository mutation**: it resets the consecutive counter to zero. Every non-mutating turn increments it — a read, a search, an inspection, narration (a planning turn with no tool call and no final object), a denied tool, a repeat, and a **novel read of a file not seen before** alike. Exploration is activity, not implementation progress. After `maxNoProgressIterations` (default 5) consecutive non-mutating turns the run MUST stop early with `termination=no_progress` and a distinct `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS` diagnostic recording the observed state (mutations and changed files) — never by consuming the ceiling, and never by substituting another provider or model. Repetition of the same action is detected independently by the repetition guard. The guard applies while no mutation has been observed: once the repository has been changed the finalization lifecycle governs the run, so a productive run is unchanged.

A run that never changes the repository MUST end as a **retryable** `needs_human` (SOP requeues it), never as a hard failure: the no-progress guard stops it with `termination=no_progress` (above). A model reporting `failed` after changing nothing is likewise a retryable no-op, not a hard failure.

The two mutating ceilings are built-in defaults an operator MAY raise:
`SOP_OLLAMA_IMPLEMENT_ITERATIONS` (default 32) and `SOP_OLLAMA_FIX_ITERATIONS`
(default 24). Each MUST be a positive integer to take effect; an unset, blank,
non-numeric, or non-positive value MUST keep the default, so behavior is unchanged
when it is not set. The soft thresholds (finalize, late-stage, force-finalize) MUST
scale with the raised ceiling, so a longer loop preserves their relative steering
position instead of stranding them near the start.

## 10. Operator-Set Diagnostic Sinks

A failed run — or one reporting a failure or human boundary — SHOULD write a safe per-turn diagnostic trail to stderr (capability, phase, iteration, tool, request, progress, recovery) that MUST NOT contain prompts, file contents, or secrets. The command provider discards that stderr, so the operator MAY set `SOP_OLLAMA_TRACE_LOG` (trace) and `SOP_TOOL_AUDIT_LOG` (tool audit). Both MUST be operator-set sinks; neither MUST be SOP's state database.

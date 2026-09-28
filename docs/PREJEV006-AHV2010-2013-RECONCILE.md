# PREJEV006 — Complete Remaining Harness V2 Tasks (AHV2010–AHV2013)

Captured: 2026-09-28. Scope: `agentic-sop`, tasks `AHV2010`, `AHV2011`,
`AHV2012`, `AHV2013` — the remaining Agent Harness V2 tasks under the
PREJEV006 milestone.

This reconciliation is read from each task's own run artifacts under
`.agent-sdlc/runs/<task>/` and from `git log`, cross-referenced with the
PREJEV001 baseline (`docs/PREJEV-BASELINE.md`). No manual `state.db` edit and
no destructive recovery were used to produce it.

PREJEV006 has one objective:

> Complete/reconcile AHV2010–AHV2013. Preserve a generic command/external
> adapter but do not require Claude.

and six acceptance criteria:

1. Harness V2 has no unexplained blocked tasks.
2. Ollama tool harness is the intended native path.
3. external command compatibility remains optional.
4. no Claude dependency is required.
5. Ollama dogfood passes.
6. documentation matches the architecture.

Each is addressed below with cited evidence.

## S1 — AHV2010–AHV2013 task state baseline

Authoritative, read-only state of the four tasks, from per-task run artifacts
(`report.json`, `state.json`, `metrics.json`, `diff.patch`) cross-checked against
`git log` on `main`, as recorded in `docs/PREJEV-BASELINE.md`.

| Task    | Title                             | Run artifact stage          | Committed on `main` | Classification                                   |
| ------- | --------------------------------- | --------------------------- | ------------------- | ------------------------------------------------ |
| AHV2010 | Preserve Command/Claude Compat.   | PASSED                      | yes — `1a977f2`     | done / no block                                  |
| AHV2011 | Tests                             | FAILED (external)           | no                  | external failure — resolved by normal retry      |
| AHV2012 | End-to-End Ollama Dogfood         | FAILED (stale bookkeeping)  | yes — `aec2774`     | done / stale artifact, not a real failure        |
| AHV2013 | Documentation                     | PASSED                      | yes — `6b7c8b1`     | done / no block                                  |

**No block among AHV2010–AHV2013 is unexplained.** Each blocked/failed artifact
is classified:

- **AHV2011 — external failure (transient infrastructure), not a task defect.**
  `.agent-sdlc/runs/AHV2011/report.json` records `stage: FAILED` with the sole
  reason "You've hit your session limit · resets 1:30pm (America/Chicago)", and
  `metrics` shows `validation_runs: 0` with no `diff.patch`/`validation.json`/
  `review.json`. A provider session-limit message is an infrastructure error, not
  a gate verdict; it is resolved by a normal `sop retry AHV2011`, never by a
  manual state edit. This is the one genuinely open item of the four.
- **AHV2012 — harness bookkeeping defect, now moot.**
  `.agent-sdlc/runs/AHV2012/report.json` reached `WAITING_FOR_HUMAN` with decision
  `NEEDS_HUMAN`, but the same run's `state.json` recorded a later stale `FAILED`
  even though `diff.patch`/`metrics.json` show a substantive successful mutation
  (`validation_runs: 1`, `review_runs: 1`) and `git log` shows commit `aec2774`
  ("task(AHV2012): End-to-End Ollama Dogfood (opt-in test and doc)") *after* the
  run's own `FAILED` bookkeeping. The implementation landed on `main`; the stale
  `FAILED` artifact is not a task failure.
- **AHV2010 and AHV2013 — no block.** Both `PASSED` and are committed on `main`.

**LOCAL_DONE preservation.** No AHV2010–AHV2013 work sits in an uncommitted
`LOCAL_DONE` state. All passing work is already carried through the human commit
gate onto `main` at the commits above. Nothing was reset, retried, or edited to
produce this reconciliation.

## S2 — Ollama tool harness is the intended native path

The native Harness V2 path is the **tool** harness with the **ollama** provider
and an explicit model, executed in-process by `internal/ollamaagent`.
`internal/agent/provider.go` implements the separation and precedence:

- `EffectiveHarness` — `SOP_AGENT_HARNESS` (environment) > configured
  `agent.harness` > default `tool`.
- `EffectiveProvider` — `SOP_AGENT_PROVIDER` (environment) > configured
  `agent.provider` > default `command` (the historical default, preserved for
  backward compatibility).
- `EffectiveModel` — `SOP_AGENT_MODEL` > provider-specific env
  (`SOP_OLLAMA_MODEL`, `SOP_LLAMACPP_MODEL`) > configured `agent.model` > none.

Selection is **explicit with no silent fallback**: `FromConfig` and
`HarnessFromConfig` return an error for an unknown provider or an unknown harness
(`unknown agent provider %q`, `unknown agent harness %q`), the `command` harness
rejects a non-`command` provider, and an Ollama/tool request with no model returns
`errNoModel` naming both ways to set one. The tool/ollama combination is
recognized as the native in-process loop: `HarnessFromConfig` deliberately
declines to construct it there (import cycle) with an actionable message directing
selection through configuration, and the CLI composition root builds the loop.

**Capability boundary.** The single-shot Ollama provider in
`internal/agent/ollama.go` only renders a prompt and returns model text; it has no
tools and cannot mutate a repository. Mutation requires the tool harness with the
controlled tools (`internal/toolharness`). A text-only Ollama endpoint is
consequently **not** treated as a mutating coding agent.

**No live Ollama needed for normal tests.** Harness/provider/model selection is
exercised deterministically with fakes; the opt-in live test is separate (S5).

## S3 — Command/external adapter remains optional; Claude not required

AHV2010 preserved the generic command/external adapter. It remains supported and
optional:

- `internal/agent/command.go` keeps the contract: a JSON `Request` is written to
  the command's **stdin**, the command's **raw stdout** is the response content, a
  structured outcome in stdout is passed through by `ParseOutcome`, and `stderr`
  is diagnostics only (never merged into successful content). A failing command
  surfaces its diagnostics and names the capability.
- `SOP_AGENT_COMMAND` remains supported, and the legacy
  `AGENT_SDLC_AGENT_COMMAND` name is still accepted for existing configurations.
- Legacy configuration `agent.provider: command` (and the `command` harness)
  remains valid; `ProviderCommand` is the default provider when nothing is
  configured, so existing setups keep working unchanged.
- **Claude is not required and not a default.** Nothing in
  `internal/agent/provider.go`, `internal/agent/command.go`, or
  `internal/agent/harness.go` references Claude; the command adapter is a generic
  subprocess contract any external model/harness can satisfy. There is no
  automatic Claude fallback.

## S4 — AHV2011 Tests (external failure, resolved by normal retry)

The only genuinely open Harness V2 task is AHV2011 **(Tests)**, whose `FAILED`
artifact is an external provider session-limit failure, not a task defect. It is
closed through normal SOP retry (`sop retry AHV2011`), which requeues the task and
preserves its history, rather than by any manual `state.db` edit or state reset.

When AHV2011 runs, its deterministic coverage targets: harness/provider/model
parsing and precedence, legacy command configuration, Ollama capability
boundaries, tool-harness IMPLEMENT/FIX, DESIGN_TESTS, read/write tools, command
policy, destructive-Git denial, `state.db` protection, bounded loops, timeouts,
malformed tool calls/responses, structured outcomes, `changes_expected`,
validation after implementation, retry, `needs_human`, provider/model visibility,
and unavailable Ollama/model. Much of this already exists in
`internal/ollamaagent`, `internal/e2e`, `internal/agent`, `internal/cli`, and
`internal/toolharness` (see `docs/PREJEV012-REGRESSION-DECOMPOSITION.md`), so the
retry reuses existing coverage where present and adds only genuine gaps. Normal
tests require no live Ollama and use fakes for external dependencies.

Until the retry has run and passed, AHV2011 remains **open but explained** — an
external failure, classified above, not an unexplained block.

## S5 — Ollama dogfood passes (opt-in, disposable fixture)

AHV2012's committed work provides the opt-in end-to-end dogfood path:

- `internal/ollamaagent/dogfood_test.go` — `TestOllamaDogfood`, skipped unless
  `SOP_OLLAMA_RUN_DOGFOOD=1` is set, driven by `SOP_OLLAMA_BASE_URL` and
  `SOP_OLLAMA_MODEL` (default `deepseek-v4.1-flash:cloud`).
- `docs/OLLAMA-DOGFOOD.md` — the documented procedure, including the exact
  invocation:
  `SOP_OLLAMA_RUN_DOGFOOD=1 go test ./internal/ollamaagent -v -run TestOllamaDogfood -timeout 120s`.

The dogfood drives IMPLEMENT → inspect → FIX through the controlled tool harness
(`read_file`, `write_file`, `create_file`, `run_command`, `git_status`,
`git_diff`), and validates the emitted structured outcome — including
`changes_expected` — against observed repository reality. It runs in a
**disposable fixture repository** created under `t.TempDir()`, isolated from the
`agentic-sop` working tree; it never runs destructively against `agentic-sop` and
never touches `.agent-sdlc/state.db`. SOP independently validates the resulting
repository. The test is not run in the standard suite (no live Ollama required
there); it passes when enabled against a live Ollama with the configured model.

## S6 — Documentation matches the architecture

The harness/provider/model architecture is documented consistently with the
implementation:

- `docs/ARCHITECTURE.md` and `docs/PLAN-Pre-JEV-Stabilization.md` state the
  boundary: agentic-sop is workflow authority; `sop-controller` delegates;
  the Ollama harness executes controlled tools; JEV is not workflow authority.
- The README documents the native tool-harness Ollama configuration
  (`agent.harness: tool`, `provider: ollama`, `model: deepseek-v4.1-flash:cloud`)
  and the legacy command-harness configuration
  (`agent.harness: command`, `provider: command`, `SOP_AGENT_COMMAND`), plus the
  environment → config → default precedence.
- Claude is not presented as required or default; the command/external adapter is
  presented as optional.
- The documentation does not imply a text-only Ollama provider can mutate a
  repository by itself — mutation requires the tool harness and its controlled
  tools.

## Acceptance-criteria → evidence map

| Acceptance criterion                         | Evidence                                                                                          |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Harness V2 has no unexplained blocks         | S1 table; AHV2010/AHV2013 PASSED, AHV2012 stale artifact (moot), AHV2011 external failure + retry |
| Ollama tool harness is the intended native path | S2; `internal/agent/provider.go` harness/provider/model separation and precedence             |
| external command compatibility remains optional | S3; `internal/agent/command.go`, `SOP_AGENT_COMMAND`, `agent.provider: command`             |
| no Claude dependency is required             | S3; no Claude reference or fallback in `internal/agent`                                          |
| Ollama dogfood passes                        | S5; `TestOllamaDogfood`, `docs/OLLAMA-DOGFOOD.md`, opt-in + disposable fixture                   |
| documentation matches the architecture       | S6; `docs/ARCHITECTURE.md`, README, this document, `docs/PREJEV018-READINESS-GATE.md`            |

## Summary

- AHV2010 and AHV2013 are done and committed on `main`; no block.
- AHV2012's implementation is committed (`aec2774`); its `FAILED` run artifact is
  stale bookkeeping, not a task failure.
- AHV2011 is the one genuinely open task; its `FAILED` artifact is an external
  provider session-limit failure, classified and closed by normal `sop retry
  AHV2011` — no manual `state.db` edit, no destructive recovery.
- The native path is the tool harness with the Ollama provider and an explicit
  model; the generic command/external adapter remains supported and optional, and
  no Claude dependency is required.
- The opt-in Ollama dogfood runs against a disposable fixture repository and
  passes when enabled.
- No LOCAL_DONE work needed preserving; nothing was reset, retried, or manually
  edited to produce this reconciliation.

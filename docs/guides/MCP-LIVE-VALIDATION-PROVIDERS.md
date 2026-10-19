# MCP live-validation provider setup

The `mcp-live-validation` workflow is provider-neutral. This guide maps the same
skill, scripts, `validation-handoff.json`, and approval boundaries onto the
agent environments we support. It does not fork the workflow: every adapter uses
the same artifact and the same deterministic scripts.

## Capability matrix

| Environment                         | Filesystem             | Process execution      | MCP tools              | Usual role                                    |
| ----------------------------------- | ---------------------- | ---------------------- | ---------------------- | --------------------------------------------- |
| ChatGPT with connected MCP tools    | no by default          | no                     | yes                    | independent MCP validator                     |
| Claude with MCP and local execution | yes                    | yes                    | yes                    | coding agent or independent validator         |
| DeepSeek-compatible coding harness  | harness-dependent      | harness-dependent      | harness-dependent      | coding agent, validator, or reviewer          |
| A tool-enabled coding harness       | depends on the harness | depends on the harness | depends on the harness | coding agent, validator, or reviewer          |
| A model API with no tools           | no                     | no                     | no                     | cannot run the workflow; can review text only |

A model API alone does **not** provide filesystem, process, or MCP access. An
agent that lacks a capability must say so and produce a portable handoff rather
than pretend to have completed the operation. The handoff records the writer's
`environment.agent` and `environment.capabilities`; the verifier warns when it is
run with `--artifact-only`.

## Common contract

Every adapter uses:

- `skills/mcp-live-validation/SKILL.md` — the workflow and approval boundaries.
- `scripts/planner-tunnel-candidate.sh` — provision, preflight, start, stop,
  status, rollback, and handoff.
- `scripts/validation-handoff.py` — read-only schema/identity/approval validation.
- `docs/reference/validation-handoff.schema.json` — the versioned artifact.
- `docs/testing/MCP-CANDIDATE-HANDOFF.md` — the acceptance contract.

The coding agent produces `validation-handoff.json`. The independent validator
fills the handoff's `results_template` and returns that results object. The coding
agent records it with `handoff --results-json`. No provider-specific command is
part of this exchange.

## ChatGPT with connected MCP tools

**Use it as:** the independent validator.

**It can:** call the MCP tools exposed by the running candidate tunnel.

**It cannot:** read the repository, run the helper scripts, or manage a local
process unless a connector explicitly provides those capabilities.

**Setup:**

1. The coding agent provides the `validation-handoff.json` (or its `request` and
   `results_template` sections) in the conversation.
2. ChatGPT runs the requested rounds against the MCP tunnel: by default 10
   concurrent rounds of `sop_status` and `sop_question_get`, 20 calls, no retries,
   no state changes.
3. ChatGPT fills the `results_template` with one entry per call, the summary,
   initialization errors, validator name, capabilities, timestamps, and `status`.
4. ChatGPT returns the results JSON in the conversation, or writes it to the
   path the coding agent provided if it has filesystem access.

If the MCP tools are unavailable, ChatGPT must report that limitation and stop;
it must not infer a pass from the artifact alone. If it has no filesystem access,
the coding agent records the returned JSON; that is expected and does not reduce
the evidence.

## Claude with MCP and local execution

**Use it as:** a coding agent (provision, local verification, deployment,
rollback, PR preparation) or an independent validator.

**Setup:**

- Use the repository skill through the normal Zed/Claude skill installation.
- Run the same `scripts/planner-tunnel-candidate.sh` commands as any other
  coding agent. There are no Claude-specific commands or artifacts.
- When Claude is the independent validator, it fills the same
  `results_template`; when it is the coding agent, it records the results with
  `handoff --results-json`.
- If Claude has only an API connection and no filesystem, process, or MCP access,
  it must report that limitation and treat the artifact as text, not as proof
  that the candidate ran.

## A DeepSeek-compatible coding harness

**Use it as:** a coding agent, a second reviewer, or an independent validator when
the harness exposes MCP tools.

**Capability requirement:** the harness must actually provide filesystem and
process execution. A DeepSeek model API or API key alone does not provide those
capabilities, and a model name does not imply MCP access.

**Setup:**

- Use the same repository skill and scripts; there are no DeepSeek-specific
  commands, artifacts, or APIs.
- Record the harness identity with `--agent deepseek-harness` (or the harness's
  real name) and its real capabilities with `--capabilities`.
- Run `python3 scripts/validation-handoff.py --handoff <file> --checkout <checkout>`
  before trusting a prior agent's results. If the harness has only text access,
  use `--artifact-only` and report that limitation.
- If it lacks MCP tools, it cannot be the independent validator; it can review the
  submitted results and prepare the PR.
- Do not rebuild or redownload when the artifact already matches the current
  candidate and the independent evidence is complete.

## A tool-enabled coding harness

**Use it as:** a coding agent or a second reviewer.

The harness must actually provide filesystem and process execution; a model name
or API key alone does not. When those capabilities are present:

1. Check out the repository and use the same skill and scripts.
2. Run `python3 scripts/validation-handoff.py --handoff <file> --checkout <checkout>`
   before trusting a prior agent's results.
3. Use `workflow.next_permitted_action` as the only next step. Do not rebuild or
   redownload when the handoff already matches the current candidate and the
   validator's evidence is complete.
4. If the harness lacks MCP tools, it cannot be the independent validator; it can
   still review the submitted evidence and prepare the PR.

## Cross-agent continuation

The forward and reverse orders use the same contract:

```text
Claude implements → Claude writes the handoff → ChatGPT validates →
ChatGPT returns results → a second coding agent verifies the artifact →
the coding agent prepares the PR
```

or:

```text
ChatGPT validates → a coding agent records the results →
another coding agent verifies and prepares the PR
```

Every handoff has a stable `workflow_id`, candidate SHA-256, source revision, and
`next_permitted_action`. The verifier rejects a stale commit, a different
candidate, missing evidence, an unsupported schema version, an unapproved
deployment action, and an old artifact. Preserve previous handoffs and the
original failure evidence; do not edit the JSON to make it pass.

## Approval boundaries

The same boundaries apply to every adapter:

- The operator approves stopping the verified baseline and starting the candidate.
- The independent validator reports actual live results; missing validation is
  `PENDING`, never success.
- The operator approves the PR and any production binary change. The workflow
  never merges automatically and never installs the candidate as the production
  binary.

## What the automated tests do and do not prove

`make test-mcp-portability` is **simulated**. It uses a temporary repository, a
fake candidate, and a canary process to exercise the Claude → ChatGPT → second
agent transitions and the invalid-handoff cases. It proves the artifact,
validator, and rejection rules work; it does not prove a live ChatGPT, Claude, or
third-party harness integration.

Actual live cross-provider evidence requires all of the following, which were not
available in this environment:

- a live coding agent harness with filesystem and process execution (for example
  Claude or a DeepSeek-compatible harness);
- a running candidate tunnel reachable by an independent MCP validator;
- a live independent validator with MCP tools (for example ChatGPT or Claude with
  MCP access) and the candidate's read-only tool surface;
- a second coding agent to resume from the recorded artifact.

Until those are run, report the compatibility tests as simulated and do not claim
full live cross-provider validation.

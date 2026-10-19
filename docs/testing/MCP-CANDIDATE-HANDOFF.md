# MCP validation handoff contract

This is the portable handoff contract for an MCP candidate. It exists because a
coding agent can build and locally test a candidate, but it cannot claim an
independent MCP integration result. The reusable end-to-end workflow is packaged
as the [`mcp-live-validation` skill](../guides/MCP-LIVE-VALIDATION.md), with
provider setup in
[`MCP-LIVE-VALIDATION-PROVIDERS.md`](../guides/MCP-LIVE-VALIDATION-PROVIDERS.md).

The two layers are different:

| Layer                        | What it proves                                                                                                              | Who runs it                                       |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| Local unit/integration tests | The source builds, the wrapper answers an MCP `tools/call` locally, and the candidate's recorded identity matches its files | The coding agent                                  |
| Live MCP validation          | The independent MCP client, tunnel, `tunnel-client` candidate, and `sop mcp --planner` work together under concurrent load  | ChatGPT, Claude, or another MCP-capable validator |

A clean local run is never a substitute for the live layer. A clean 20/20 live
run is supporting evidence, not proof that an intermittent defect can never
recur.

## Required handoff report

The coding agent produces `validation-handoff.json` before asking an independent
validator to run the live test. The
`scripts/planner-tunnel-candidate.sh handoff` command records the facts and
placeholders; it never invents results.

### Machine-readable form

Use `--format json` for the stable artifact. The schema is
`mcp-live-validation/validation-handoff/v1` with `schema_version: 1`. It records:

- workflow ID, repository, branch, commit, and working-tree state;
- environment agent, platform, and declared capabilities;
- candidate path, version, SHA-256, source revision, patch SHA-256, and profile;
- baseline path, version, and SHA-256;
- active process state, PID, redacted command, command SHA-256, and launch data;
- candidate and baseline log paths;
- local verification status and evidence;
- the requested live MCP calls and acceptance criteria;
- the results template the independent validator fills;
- independent validation status, validator, and results;
- deployment and merge approval requirements;
- the workflow status and `next_permitted_action`;
- the rollback command and safety assertions.

```sh
scripts/planner-tunnel-candidate.sh handoff \
  --format json \
  --issue <issue-reference> \
  --workflow-id <stable-workflow-id> \
  --agent <agent-or-harness> \
  --capabilities filesystem,process_execution \
  --local-status PASS \
  --tests <local-test-evidence> \
  --deployment-approval <PENDING-or-APPROVED> \
  --output <checkout>/.run/tunnel/candidate/validation-handoff.json
```

`PASS` and `FAIL` require `--results-json FILE` with the independent validator's
actual results. The results object uses
`mcp-live-validation/validation-results/v1` and must carry the same workflow ID,
candidate SHA-256, and source revision. The helper validates the artifact against
`docs/reference/validation-handoff.schema.json` and the current candidate before
writing it atomically; it refuses a status without evidence.

### Resume and validation

Any agent can verify the artifact before trusting it:

```sh
python3 scripts/validation-handoff.py \
  --handoff <checkout>/.run/tunnel/candidate/validation-handoff.json \
  --checkout <checkout> \
  --max-age-hours 24
```

The verifier is read-only. It rejects an unsupported schema version, a stale
repository commit, a different candidate SHA-256 or source revision, a missing or
mismatched process, missing independent evidence, an old artifact, and an
unapproved deployment action. It prints the recorded `next_permitted_action`; the
agent must perform only that action.

If the agent lacks filesystem or process access, use `--artifact-only` and report
that limitation; do not claim the candidate file or process was verified.

### Markdown form

The markdown form is for people; the JSON artifact is authoritative.

```markdown
# MCP candidate validation handoff

## Workflow

- ID:
- Status:
- Next permitted action:
- Validation status:

## Repository

- Branch:
- Commit:
- Working tree:

## Environment

- Agent:
- Platform:
- Capabilities:

## Candidate executable

- Path:
- Version:
- SHA-256:
- Source:
- Patch (SHA-256):
- Profile:
- Baseline (unchanged):

## Active process

- Manifest state:
- PID:
- Command:
- Command SHA-256:
- Candidate debug log:
- Baseline log:

## Local verification (not independent validation)

- Status:
<commands and results>

## Independent validation

- Status:
- Validator:
- Candidate SHA-256:
- Source revision:
<results or a pointer to the JSON artifact>

## Approval

- Deployment status:
- Approval reference:
- Merge status:

## Requested live MCP tests

<the request below, or a pointer to the JSON artifact for a custom request>

## Rollback

<exact command>

## Acceptance

<the criteria below>
```

Do not fill the independent-validation section yourself. Do not mark the candidate
verified until an independent validator reports actual results. Missing
validation is `PENDING`, never success.

## Exact read-only MCP test request

The default request is for a fixture with a pending `q-1`. Do not change SOP
state, answer a question, or retry a failed call.

Run **10 rounds**. In each round, call these two tools **concurrently** and wait
for both responses before starting the next round:

- `sop_status({})`
- `sop_question_get({"question_id":"q-1"})`

That is **20 calls total**. Record, per call:

- wall-clock time;
- success, or the exact error fields: HTTP status, JSON-RPC code, error type,
  and origin;
- every initialization failure, including HTTP `409` and
  `mcp_initialization_required`.

**Pass:** all 20 calls succeed with no retries and no state change.

**Fail:** any call fails. Record the exact error and the matching lines from the
candidate debug log.

For a different MCP surface, pass `--request FILE` to the handoff command with the
same fields and replace the required calls. Keep the no-retry, no-state-change,
and initialization-error rules.

## Rollback

The handoff contains the exact command. It is normally:

```sh
scripts/planner-tunnel-candidate.sh rollback --checkout <checkout> --start
```

Rollback stops only the verified candidate PID, restores the wrapper backup,
and starts the recorded baseline with the preserved launch arguments. Keep the
candidate binary and debug log until the evidence is recorded; do not delete
them before rollback.

## Evidence hygiene

Tunnel debug logs contain local usernames and paths, tunnel/client identifiers,
and request IDs. Before sharing outside the machine, reduce the log to the
correlation fields, replace local paths and identifiers, and attach the reduced
result to the issue rather than committing raw logs. Do not enable raw HTTP or
payload capture options.

Do not put credentials, tokens, or sensitive environment values in either handoff
form or the results JSON. The helper redacts common credential-like command values
and the verifier rejects credential-like keys, but review the artifact before
sharing it.

## Simulated vs live evidence

The automated compatibility tests are simulated with fixtures and mock processes.
They prove the artifact, validator, and rejection rules work; they do not prove a
live ChatGPT, Claude, or third-party harness integration. Report the exact missing
capability when a live validator or coding harness is unavailable.

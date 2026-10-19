---
name: mcp-live-validation
description: |-
  Run a provider-independent MCP fix-and-validation workflow.
  /mcp-live-validation <issue or request>, or "use the MCP live-validation skill".
  Use when an MCP integration failure needs a candidate build, a safe local
  deployment, an independent MCP validation, portable evidence, rollback, and a PR.
  It is designed for any compatible coding agent or validator (for example a
  Claude-compatible coding agent, a ChatGPT MCP validator, or a tool-enabled
  coding harness) and never treats a local test as independent validation.
disable-model-invocation: true
---

# MCP Live Validation

This skill turns an MCP issue into a candidate that can be validated by a
different agent, without asking the operator to copy patches or commands,
download dependencies, build binaries, or manage a running tunnel by hand.

It is a provider-neutral workflow over the repository's existing deterministic
tooling. The model, editor, and agent vendor are not part of the contract: the
same scripts, handoff artifact, and approval boundaries work for any compatible
agent. Provider setup belongs in the small adapter guide, not in a forked
workflow. It does not introduce a new orchestration framework, database, or
message broker.

## Non-claims

- A coding agent cannot invoke another agent's MCP tools or a provider API by
  itself. Where no automated handoff exists, generate the portable request and
  wait for the independent validator to return actual results.
- Local tests, a local MCP call, and a clean debug log are not independent
  validation. Only the independent validator's recorded results can set the
  artifact to `PASS`.
- A clean run is supporting evidence, not proof that an intermittent defect
  cannot recur.
- The workflow never installs a candidate as the production binary, never
  merges, and never weakens SOP's approval policy.

## Operating contract

The expected interaction is:

**Issue described → agent investigates and fixes → agent prepares the candidate →
an independent MCP validator validates → the coding agent records evidence and
prepares the PR.**

Ask the operator only for a genuine approval, authorization, security, or
unresolved-decision boundary. Discover paths, profiles, revisions, and existing
state from the repository instead of asking for them. Retry safe, recoverable
failures within the helper's bounded limits.

The shared contract is `validation-handoff.json`, validated against
`docs/reference/validation-handoff.schema.json`. Any agent with the required
capabilities can write it, validate it, or resume from it. There is no new
database, message broker, or agent-to-agent protocol.

## Boundaries

- Work on an isolated branch or worktree. Preserve unrelated local changes.
- Do not modify the active SOP working tree, the installed baseline binary, or an
  unidentified running process.
- Do not replace production binaries without explicit approval.
- Do not merge. Prepare the PR and stop.
- Preserve the candidate, its logs, the previous handoff, and the baseline
  rollback path until the evidence is recorded.
- Never put credentials, access tokens, or sensitive environment variables in a
  handoff or results artifact.
- If `sop` is not on `PATH`, fail closed for any SOP state check and report the
  installation prerequisite. The candidate helper is still usable when the issue
  is purely a tunnel-client or MCP-server problem.

## DISCOVER

Read the issue, the repository instructions, and the existing MCP surfaces.
Assume nothing and invent no option.

- Identify the failing layer: SOP MCP server, tunnel-client, transport,
  initialization, concurrency, or the client. A tunnel-client candidate cannot fix
  a server-side defect.
- Find the existing tooling before creating anything:
  - `scripts/planner-tunnel-candidate.sh` for provision, preflight, start, stop,
    status, rollback, and handoff.
  - `scripts/planner-tunnel-point.sh` for the SOP project/binary the tunnel
    wrapper serves.
  - `scripts/validation-handoff.py` for portable-handoff validation.
  - `docs/reference/validation-handoff.schema.json` for the versioned contract.
  - `docs/testing/MCP-CANDIDATE-HANDOFF.md` for the handoff and acceptance
    contract.
  - `docs/guides/MCP-LIVE-VALIDATION-PROVIDERS.md` for agent/harness setup.
  - `docs/testing/PLANNER-TWO-QUESTION-LIVE.md` for the planner runbook and the
    issue #90 evidence example. Treat its specific tool names as an example, not
    a requirement for every MCP surface.
- Record the repository root, branch, commit, MCP profile, candidate source,
  source revision, patch, baseline binary, and the read-only MCP calls the issue
  actually needs.
- Read SOP state without changing it when the repository is SOP-governed:

  ```bash
  sop status
  sop approvals --json
  ```

- Check the toolchain: `git`, `go`, `shasum` or `sha256sum`, `ps`, `python3`, and
  the installed `tunnel-client` baseline. A missing tool is a blocker only when
  the issue cannot proceed without it; report the exact command and why.

## DEPENDENCIES

Provision what the workflow needs automatically, from trusted and pinned inputs,
before giving up on a missing dependency.

- Reuse an existing source checkout, build directory, Go module cache, and Go
  build cache before fetching anything. The candidate helper already reuses a
  matching source/build directory.
- Fetch the candidate source only from the pinned repository and revision. Verify
  the revision and the patch checksum before building. Never substitute an
  unpinned `latest` artifact.
- If the installed `tunnel-client` baseline is missing, build the pinned
  unpatched baseline from the same source with `--no-patch` into a separate output
  and pass it as `--baseline`; do not use the candidate as its own baseline.
- A missing system toolchain (`git`, `go`, `python3`, or the hash utility) is an
  environment blocker, not a reason to install an arbitrary package or download
  an unverified binary. Check common local install locations first, report the
  exact missing command, and ask only when installation needs authorization.
- Bound retries. Clone, fetch, and safe network operations use the helper's
  bounded retries; a deterministic build or patch failure is diagnosed, not
  retried blindly.

## PROVISION

Prepare the fix and the candidate. Never start or stop a tunnel in this phase.

1. Create an isolated branch or worktree from the intended base. Keep the active
   working tree untouched.
2. Implement the smallest correct fix and add or update tests. Do not change
   unrelated behavior, SOP scheduling, approvals, checkpoints, or routing.
3. Run the relevant local tests. Typical checks are:

   ```bash
   go test ./...
   go vet ./...
   go build ./...
   make test-planner-tunnel
   make test-mcp-portability
   ```

4. Build and record the candidate with the existing helper. Pass the issue's real
   source, revision, patch, baseline, and profile; the defaults are examples:

   ```bash
   scripts/planner-tunnel-candidate.sh provision \
     --source <source-checkout-or-url> \
     --revision <pinned-revision> \
     --patch <patch-file> \
     --baseline <baseline-binary> \
     --profile <tunnel-profile> \
     --log-file <candidate-log-outside-the-project>
   ```

   The helper reuses a matching source checkout or build directory, verifies the
   patch against the pinned revision, builds into an isolated path, records the
   candidate path/version/SHA-256, source revision, patch SHA-256, baseline
   identity, profile, launch arguments, workflow ID, and wrapper backup, and does
   not start or stop anything. It does not replace the installed binary.

   If no installed baseline exists, build one from the same pinned source with
   `--no-patch` into a separate output and pass that path as `--baseline`. Do not
   use the candidate as its own baseline.

5. If the helper cannot apply the patch, do not hand the operator a command. Read
   the failure, verify the pinned revision and patch checksum, and retry safely
   within the helper's bounds. A genuinely mismatched source is a blocker, not a
   reason to edit the patch by hand.

## LOCAL VERIFY

Run the candidate's own tests and the helper's preflight. Local verification is
required, but it is not independent validation.

```bash
scripts/planner-tunnel-candidate.sh preflight
```

`preflight` checks the candidate checksum, version, baseline identity, wrapper,
provenance, rollback files, and the running-process set. A failure stops the
workflow; do not start the candidate.

## IDENTITY

Before deployment, the agent must be able to answer all of these from the
manifest and preflight output:

- Which branch, commit, source revision, and patch produced the candidate?
- What is the candidate's absolute path, version, and SHA-256?
- What is the baseline's path, version, and SHA-256?
- What exact command and PID will run the candidate?
- Where is the candidate log, and where is the baseline log?
- What is the exact rollback command?

If any answer is missing, fix the provenance before asking for approval.

## APPROVAL GATE

Stop and ask the operator for approval before stopping the verified baseline or
starting the candidate. This is the only deployment approval in the workflow.

Present the candidate identity, baseline identity, profile, log path, rollback
command, and local test results. Do not ask the operator to copy commands; the
agent runs them after approval. Record the approval in the handoff with
`--deployment-approval APPROVED` and `--approval-reference` naming where it was
given (a comment link or ticket) only after it is actually given. The helper and
the validator reject `APPROVED` without a reference.

## DEPLOY

After approval, use the verified lifecycle. Do not use a broad process kill and
do not start a second tunnel.

```bash
scripts/planner-tunnel-candidate.sh stop
scripts/planner-tunnel-candidate.sh start \
  --handoff <checkout>/.run/tunnel/candidate/validation-handoff.json
```

`start --handoff` validates the artifact against the schema and the current
candidate and refuses unless `workflow.next_permitted_action` is
`START_CANDIDATE` and the deployment approval is `APPROVED`. Regenerate the
handoff after startup so the next action becomes `RUN_INDEPENDENT_VALIDATION`.

`stop` signals only the PID whose exact command line matches the manifest. It
refuses when multiple tunnel processes exist, when the process identity changed,
or when the target is not verified. `start` refuses to launch while another
tunnel process is running, launches with the preserved profile and equivalent
arguments, confirms startup, and records the PID, command, and log.

If either command refuses, stop and report the actual process list. Do not
override the refusal.

## HANDOFF

Generate the portable handoff after each state change. The JSON form is the
authoritative cross-agent artifact; the markdown form is for people. Keep both
outside the target project's change set.

```bash
scripts/planner-tunnel-candidate.sh handoff \
  --format json \
  --issue <issue-reference> \
  --workflow-id <stable-workflow-id> \
  --agent <agent-or-harness> \
  --capabilities filesystem,process_execution \
  --local-status PASS \
  --tests <local-test-evidence> \
  --deployment-approval <PENDING-or-APPROVED> \
  --approval-reference <link-or-ticket; required with APPROVED> \
  --output <checkout>/.run/tunnel/candidate/validation-handoff.json

scripts/planner-tunnel-candidate.sh handoff \
  --issue <issue-reference> \
  --tests <local-test-evidence> \
  --output <checkout>/.run/tunnel/candidate/validation-handoff.md
```

The JSON artifact is `validation-handoff.json` with schema
`mcp-live-validation/validation-handoff/v1` and `schema_version: 1`. It records:

- the workflow ID, repository, branch, commit, and working-tree state;
- the writer's agent, platform, and declared capabilities;
- candidate path, version, SHA-256, source revision, patch SHA-256, profile;
- baseline path, version, and SHA-256;
- active process state, PID, redacted command, command SHA-256, and launch config;
- candidate and baseline log paths;
- local verification status and evidence;
- the requested live MCP calls and acceptance criteria;
- the results template the independent validator fills;
- independent validation status, validator, and results;
- deployment and merge approval requirements;
- the workflow status and `next_permitted_action`;
- the rollback command and safety assertions.

The default request is the issue #90-style read-only test: 10 rounds of
concurrent `sop_status` and `sop_question_get`, 20 calls total, no retries, no
state changes, recording every initialization error including HTTP `409` and
`mcp_initialization_required`. For a different MCP surface, pass `--request FILE`
with the same fields and replace the required calls; keep the no-retry,
no-state-change, and initialization-error rules.

The helper requires `--results-json FILE` for `PASS` and `FAIL`, and the results
object uses `mcp-live-validation/validation-results/v1`. It refuses a status
without that evidence. The handoff is written atomically and refuses a silent
overwrite.

A minimal view of the portable shape:

```json
{
  "schema": "mcp-live-validation/validation-handoff/v1",
  "schema_version": 1,
  "workflow_id": "...",
  "candidate": { "sha256": "..." },
  "baseline": { "sha256": "..." },
  "process": { "pid": "...", "command_sha256": "..." },
  "logs": { "candidate": "...", "baseline": "..." },
  "local_verification": { "status": "PASS" },
  "request": { "rounds": 10, "total_calls": 20 },
  "results_template": {
    "schema": "mcp-live-validation/validation-results/v1",
    "status": "PENDING"
  },
  "independent_validation": {
    "status": "PENDING",
    "results": { "status": "PENDING" }
  },
  "local_tests_are_not_validation": true,
  "approval": { "deployment": { "status": "PENDING" } },
  "workflow": {
    "status": "AWAITING_INDEPENDENT_VALIDATION",
    "next_permitted_action": "RUN_INDEPENDENT_VALIDATION"
  },
  "rollback": { "command": "..." },
  "safety": { "production_binary_replaced": false, "merged": false }
}
```

## INDEPENDENT VALIDATION

Hand the `request` and `results_template` from the handoff to an independent MCP
validator. The validator can be ChatGPT with connected MCP tools, Claude with MCP
access, another MCP-capable agent, or any harness that can execute the requested
calls. The coding agent cannot run this layer for the validator.

The validator must:

- use the handoff's `workflow_id`, candidate SHA-256, and source revision;
- run every required round concurrently where the request says concurrent;
- use no retries and change no SOP state;
- record one entry per call in `calls`, with success or the exact error fields;
- record every initialization failure, including HTTP status, JSON-RPC code,
  error type, and origin;
- fill `summary`, `initialization_errors`, `validator`,
  `validator_capabilities`, timestamps, and `status`;
- return the results JSON to the coding agent (or write it directly when the
  validator has filesystem access).

The coding agent records the results, never invents them:

```bash
scripts/planner-tunnel-candidate.sh handoff \
  --format json \
  --results-json <validation-results.json> \
  --validation-status <PASS-or-FAIL> \
  --local-status PASS \
  --tests <local-test-evidence> \
  --deployment-approval APPROVED \
  --approval-reference <link-or-ticket> \
  --output <checkout>/.run/tunnel/candidate/validation-handoff.json \
  --replace
```

Missing validation is `PENDING`, never success. A clean local log is not a
substitute.

## CROSS-AGENT CONTINUATION

Any agent can resume from the shared artifact. It must not repeat completed work
or trust stale evidence.

```bash
python3 scripts/validation-handoff.py \
  --handoff <validation-handoff.json> \
  --checkout <checkout> \
  --max-age-hours 24
```

The verifier checks the schema, workflow ID, repository commit, manifest, candidate
and baseline checksums, process identity, local verification, independent results,
approval state, freshness, and the next permitted action. It is read-only.

- Use `--artifact-only` only when the agent lacks filesystem or process access.
  Report that limitation; do not claim the candidate file or process was verified.
- If the verifier rejects the artifact, stop. Do not edit the JSON to make it pass;
  re-provision or regenerate it from the authoritative state.
- Resume only the recorded `next_permitted_action`:
  - `REQUEST_DEPLOYMENT_APPROVAL`: ask the operator, then record the approval.
  - `START_CANDIDATE`: stop the verified baseline and start the candidate.
  - `RUN_INDEPENDENT_VALIDATION`: give the request to an independent validator.
  - `DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE`: preserve the failure evidence and
    prepare a corrected candidate.
  - `PREPARE_PR`: prepare the PR and stop.
  - `STOP_AND_REPORT`: report the block or rollback; do not claim success.
- Reject a stale commit, a different candidate SHA-256 or source revision, a
  wrong workflow ID, missing evidence, an unsupported schema version, or an
  unapproved deployment action.
- Preserve the original failure evidence and every previous handoff. Use a new
  output path for a new round.
- The reverse role order is supported: any agent with the required capabilities can
  implement, validate, or review, as long as it reads and writes the same
  artifact and preserves the approval boundaries.

## FAILURE LOOP

If independent validation returns `FAIL`:

1. Preserve the failed handoff, candidate, logs, and every failing call. Do not
   delete the previous candidate or overwrite its manifest.
2. Diagnose against the evidence: the failing layer, whether the error came from
   the server, the tunnel transport, initialization, or the client, and whether
   the candidate's startup identity matches the artifact.
3. Fix the actual cause. Add a regression test that fails before the fix and
   passes after it.
4. Provision a new candidate to a new output/build path or with a new patch. Keep
   the failed candidate and its evidence intact.
5. Re-run local verification and generate a new `PENDING` handoff. Return to the
   independent validator with a new request and results template.

Do not retry the failed independent call to turn it into a pass. A retry hides
the failure and is valid only as a separate diagnostic, clearly labelled.

If the failure is outside the agent's authorization or the evidence is
insufficient to identify the layer, stop and report the boundary.

## EVIDENCE

Keep evidence outside the target project's change set. Collect:

- local test commands and results;
- candidate and baseline identities;
- `validation-handoff.json`, its markdown form, and the independent results JSON;
- candidate and baseline log paths;
- the independent validator's per-call results and error fields;
- the failing round's matching debug-log lines, if any;
- the verifier output and the rollback command.

Reduce logs before sharing: replace local usernames, paths, tunnel/client
identifiers, and request identifiers. Do not include credentials, tokens,
environment dumps, tool arguments, or tool results unless the operator explicitly
authorizes the specific diagnostic content. The helper redacts credential-like
command values and the verifier rejects credential-like keys, but neither is a
substitute for reviewing the artifact.

## ROLLBACK

Rollback is deterministic and does not depend on remembering a command:

```bash
scripts/planner-tunnel-candidate.sh rollback --start
```

It stops only the verified candidate PID, restores the wrapper backup, and starts
the recorded baseline with the preserved launch arguments. If the candidate is
still needed for evidence, do not delete its binary or log before rollback is
recorded.

## PR PREPARATION

Prepare the PR against the intended base, but do not merge it.

- Include the code fix, tests, and the evidence summary. Keep raw logs and local
  paths out of the repository.
- State the candidate identity and the independent validation status exactly:
  `PENDING` when the validator has not reported, `FAIL` with the failure evidence
  when it has failed. Never use `PASS` from local results.
- Link the issue and the handoff artifact location without embedding credentials.
- Run the repository's checks before opening the PR:

  ```bash
  bash -n scripts/planner-tunnel-candidate.sh
  python3 -m py_compile scripts/validation-handoff.py
  scripts/checks/check-doc-links.sh
  make test-planner-tunnel
  make test-mcp-portability
  go test ./...
  go vet ./...
  go build ./...
  ```

- Do not merge, do not force-push over unrelated work, and do not delete the
  candidate or rollback artifacts until the evidence is accepted.

## COMPLETION REPORT

Return a concise, evidence-backed summary:

- Issue and failing layer.
- Branch, commit, and PR.
- Workflow ID and `validation-handoff.json` location.
- Candidate identity: path, version, SHA-256, source revision, patch SHA-256.
- Baseline identity and the fact that it was not replaced.
- Local tests: commands and actual results.
- Independent validation: `PENDING`, `PASS`, or `FAIL`; who reported it and the
  exact acceptance result.
- Candidate and baseline log paths.
- Rollback command and whether it was exercised.
- Remaining manual approval: deployment approval, independent validation, and PR
  review/merge.
- Limitations: intermittent defects, untested surfaces, or evidence that could
  not be captured. A mocked compatibility test is not a live cross-provider test.

## REUSE MAP

Do not recreate these capabilities:

| Need                                         | Existing owner                                                  |
| -------------------------------------------- | --------------------------------------------------------------- |
| Candidate download, patch, build, provenance | `scripts/planner-tunnel-candidate.sh provision`                 |
| Candidate identity and environment checks    | `scripts/planner-tunnel-candidate.sh preflight`                 |
| Verified stop, start, and rollback           | `scripts/planner-tunnel-candidate.sh stop`, `start`, `rollback` |
| Running-process and manifest view            | `scripts/planner-tunnel-candidate.sh status`                    |
| Portable markdown and JSON handoff           | `scripts/planner-tunnel-candidate.sh handoff`                   |
| Portable handoff and resume validation       | `scripts/validation-handoff.py`                                 |
| Versioned artifact contract                  | `docs/reference/validation-handoff.schema.json`                 |
| SOP project/binary wrapper                   | `scripts/planner-tunnel-point.sh`                               |
| Handoff acceptance contract                  | `docs/testing/MCP-CANDIDATE-HANDOFF.md`                         |
| Agent and harness setup                      | `docs/guides/MCP-LIVE-VALIDATION-PROVIDERS.md`                  |
| Candidate lifecycle regression tests         | `make test-planner-tunnel`                                      |
| Cross-agent compatibility tests              | `make test-mcp-portability`                                     |

Add code only if a real capability is missing. Prefer extending an existing
helper over adding a second lifecycle or a provider-specific fork.

---
name: mcp-live-validation
description: |-
  Prepare, deploy, and independently validate an MCP fix end to end.
  /mcp-live-validation <issue or request>, or "use the MCP live-validation skill".
  Use when an MCP integration failure needs a candidate build, a safe local
  deployment, an independent ChatGPT MCP test, evidence collection, and a PR.
  It reuses the repository's deterministic candidate helper and never treats a
  local test as independent validation.
disable-model-invocation: true
---

# MCP Live Validation

This skill turns an MCP issue into a candidate that can be tested independently,
without asking the operator to copy patches or commands, download dependencies,
build binaries, or manage a running tunnel by hand.

It is a workflow over the repository's existing deterministic tooling. It does
not add an orchestration framework, a background service, or a second lifecycle.

## Non-claims

- The coding agent **cannot invoke ChatGPT tools**. Where no automated handoff
  exists, generate a ready-to-use request and wait for the operator or ChatGPT to
  return the actual results.
- Local tests, a local MCP call, and a clean debug log are not independent
  validation. Only the independent validator's reported results can set the
  artifact to `PASS`.
- A clean run is supporting evidence, not proof that an intermittent defect
  cannot recur.
- The skill never installs a candidate as the production binary, never merges,
  and never weakens SOP's approval policy.

## Operating contract

The expected interaction is:

**Issue described → agent investigates and fixes → agent prepares the candidate →
ChatGPT validates → agent collects evidence and prepares the PR.**

Ask the operator only for a genuine approval, authorization, security, or
unresolved-decision boundary. Discover paths, profiles, revisions, and existing
state from the repository instead of asking for them. Retry safe, recoverable
failures within the helper's bounded limits.

## Boundaries

- Work on an isolated branch or worktree. Preserve unrelated local changes.
- Do not modify the active SOP working tree, the installed baseline binary, or an
  unidentified running process.
- Do not replace production binaries without explicit approval.
- Do not merge. Prepare the PR and stop.
- Preserve the candidate, its logs, the previous manifest, and the baseline
  rollback path until the evidence is recorded.
- Never put credentials, access tokens, or sensitive environment variables in a
  handoff artifact.
- If `sop` is not on `PATH`, fail closed for any SOP state check and report the
  installation prerequisite. The candidate helper itself is still usable when the
  issue is purely a tunnel-client or MCP-server problem.

## DISCOVER

Read the issue, the repository instructions, and the existing MCP surfaces.
Assume nothing and invent no option.

- Identify the failing layer: SOP MCP server, tunnel-client, transport,
  initialization, concurrency, or the client. A tunnel-client candidate cannot fix
  a server-side defect.
- Find the existing tooling before creating anything. The reusable pieces are:
  - `scripts/planner-tunnel-candidate.sh` for provision, preflight, start, stop,
    status, rollback, and handoff.
  - `scripts/planner-tunnel-point.sh` for the SOP project/binary the tunnel
    wrapper serves.
  - `docs/testing/MCP-CANDIDATE-HANDOFF.md` for the handoff and acceptance
    contract.
  - `docs/testing/PLANNER-TWO-QUESTION-LIVE.md` for the planner runbook and the
    issue #90 evidence format. Treat its specific tool names as an example, not a
    requirement for every MCP surface.
- Record the repository root, branch, commit, configured MCP profile, candidate
  source, source revision, patch, baseline binary, and the read-only MCP calls the
  issue actually needs.
- Read SOP state without changing it when the repository is SOP-governed:

  ```bash
  sop status
  sop approvals --json
  ```

- Check the toolchain: `git`, `go`, `shasum` or `sha256sum`, `ps`, and the
  installed `tunnel-client` baseline. A missing tool is a blocker only when the
  issue cannot proceed without it; report the exact command and why.

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
- A missing system toolchain (`git`, `go`, or the hash utility) is an environment
  blocker, not a reason to install an arbitrary package or download an unverified
  binary. Check common local install locations first, report the exact missing
  command, and ask only when the installation needs authorization.
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
   identity, profile, launch arguments, and wrapper backup, and does not start or
   stop anything. It does not replace the installed binary.

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
command, and the local test results. Do not ask the operator to copy commands;
the agent runs them after approval.

## DEPLOY

After approval, use the verified lifecycle. Do not use a broad process kill and
do not start a second tunnel.

```bash
scripts/planner-tunnel-candidate.sh stop
scripts/planner-tunnel-candidate.sh start
```

`stop` signals only the PID whose exact command line matches the manifest. It
refuses when multiple tunnel processes exist, when the process identity changed,
or when the target is not verified. `start` refuses to launch while another
tunnel process is running, launches with the preserved profile and equivalent
arguments, confirms startup, and records the PID, command, and log.

If either command refuses, stop and report the actual process list. Do not
override the refusal.

## HANDOFF

Generate the handoff after the candidate is running. The markdown form is for
people; the JSON form is the machine-readable artifact. Keep both outside the
target project's change set.

```bash
scripts/planner-tunnel-candidate.sh handoff \
  --format json \
  --issue <issue-reference> \
  --tests <local-test-evidence> \
  --output <checkout>/.run/tunnel/candidate/handoff.json

scripts/planner-tunnel-candidate.sh handoff \
  --issue <issue-reference> \
  --tests <local-test-evidence> \
  --output <checkout>/.run/tunnel/candidate/handoff.md
```

The artifact contains:

- repository root, branch, commit, and working-tree state;
- candidate path, version, SHA-256, source revision, patch SHA-256, profile;
- baseline path, version, and SHA-256;
- active process state, PID, redacted command, and launch configuration;
- candidate and baseline log paths;
- local test evidence;
- rollback command;
- the required ChatGPT calls and acceptance criteria;
- independent validation status: `PENDING`, `PASS`, or `FAIL`.

Use a new output path for each validation round. If an artifact already exists
and you intentionally reuse its path, preserve the old file first; the helper
refuses a silent overwrite unless `--replace` is passed explicitly.

`--validation-status PASS` or `FAIL` requires `--results FILE` with the
independent validator's actual report. The helper refuses a status without
evidence. It also redacts common credential-like command values, but review the
artifact before sharing it.

The machine-readable shape is stable and intentionally small:

```json
{
  "schema": "mcp-live-validation/handoff/v1",
  "repository": { "root": "...", "branch": "...", "commit": "..." },
  "candidate": { "path": "...", "version": "...", "sha256": "..." },
  "baseline": { "path": "...", "version": "...", "sha256": "..." },
  "process": { "state": "...", "pid": "...", "command": "[redacted]" },
  "logs": { "candidate": "...", "baseline": "..." },
  "local_tests": "...",
  "independent_validation": { "status": "PENDING", "results": "..." },
  "local_tests_are_not_validation": true,
  "request": { "rounds": 10, "total_calls": 20, "retries": 0 },
  "rollback": { "command": "..." }
}
```

The default request is the issue #90-style read-only test: 10 rounds of
concurrent `sop_status` and `sop_question_get`, 20 calls total, no retries, no
state changes, recording every initialization error including HTTP `409` and
`mcp_initialization_required`. For a different MCP surface, replace the required
calls in the artifact with that surface's read-only calls while keeping the same
no-retry, no-state-change, and initialization-error rules.

## INDEPENDENT VALIDATION

Hand the generated request to the operator or ChatGPT. State plainly that the
coding agent cannot run the independent layer and that local tests are not a
substitute. Wait for the actual results.

The independent validator must:

- run every required round concurrently where the request says concurrent;
- use no retries;
- change no SOP state;
- record per-call success or the exact error fields;
- record every initialization failure, including HTTP status, JSON-RPC code,
  error type, and origin;
- report the exact tool calls used and the wall-clock times.

Do not fill in the results yourself. Do not set `PASS` from a clean local log.

## FAILURE LOOP

If independent validation returns `FAIL`:

1. Preserve the failed handoff, candidate, logs, and the exact failing calls.
   Do not delete the previous candidate or overwrite its manifest.
2. Diagnose against the evidence: the failing layer, whether the error came from
   the server, the tunnel transport, initialization, or the client, and whether
   the candidate's startup identity matches the artifact.
3. Fix the actual cause. Add a regression test that fails before the fix and
   passes after it.
4. Provision a **new** candidate to a new output/build path or with a new patch.
   Keep the failed candidate and its evidence intact.
5. Re-run local verification and the handoff. Return to the independent
   validator with a new request and a new `PENDING` artifact.

Do not retry the failed independent call to turn it into a pass. A retry hides
the failure and is valid only as a separate diagnostic, clearly labelled.

If the failure is outside the agent's authorization or the evidence is
insufficient to identify the layer, stop and report the boundary.

## EVIDENCE

Keep evidence outside the target project's change set. Collect:

- local test commands and results;
- candidate and baseline identities;
- the handoff JSON and markdown;
- candidate and baseline log paths;
- the independent validator's per-call results and error fields;
- the failing round's matching debug-log lines, if any;
- the rollback command and whether it was exercised.

Reduce logs before sharing: replace local usernames, paths, tunnel/client
identifiers, and request identifiers. Do not include credentials, tokens,
environment dumps, tool arguments, or tool results unless the operator
explicitly authorizes the specific diagnostic content.

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
- State the candidate identity and the independent validation status exactly.
  Use `PENDING` when the validator has not reported; use `FAIL` with the failure
  evidence when it has failed. Never use `PASS` from local results.
- Link the issue and the handoff artifact location without embedding credentials.
- Run the repository's checks before opening the PR. At minimum:

  ```bash
  bash -n scripts/planner-tunnel-candidate.sh
  scripts/checks/check-doc-links.sh
  make test-planner-tunnel
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
  not be captured.

## REUSE MAP

Do not recreate these capabilities:

| Need                                         | Existing owner                                                  |
| -------------------------------------------- | --------------------------------------------------------------- |
| Candidate download, patch, build, provenance | `scripts/planner-tunnel-candidate.sh provision`                 |
| Candidate identity and environment checks    | `scripts/planner-tunnel-candidate.sh preflight`                 |
| Verified stop, start, and rollback           | `scripts/planner-tunnel-candidate.sh stop`, `start`, `rollback` |
| Running-process and manifest view            | `scripts/planner-tunnel-candidate.sh status`                    |
| Markdown and JSON handoff                    | `scripts/planner-tunnel-candidate.sh handoff`                   |
| SOP project/binary wrapper                   | `scripts/planner-tunnel-point.sh`                               |
| Handoff acceptance contract                  | `docs/testing/MCP-CANDIDATE-HANDOFF.md`                         |
| Planner/issue #90 example                    | `docs/testing/PLANNER-TWO-QUESTION-LIVE.md`                     |
| Candidate lifecycle regression tests         | `make test-planner-tunnel`                                      |

Add code only if a real capability is missing. Prefer extending an existing
helper over adding a second lifecycle.

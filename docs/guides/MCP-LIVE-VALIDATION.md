# MCP live validation skill

The `mcp-live-validation` skill is a reusable workflow for MCP integration
failures: it discovers the environment, prepares a candidate fix, verifies the
candidate's identity, deploys it through an approval gate, hands the live test to
an independent validator, and collects evidence for a PR.

It is **not** a new orchestrator. It is instructions over the deterministic
candidate helper added for issue #90:

- `scripts/planner-tunnel-candidate.sh` — provision, preflight, start, stop,
  status, rollback, and handoff.
- `scripts/planner-tunnel-point.sh` — choose the SOP project and binary the
  tunnel wrapper serves.
- `docs/testing/MCP-CANDIDATE-HANDOFF.md` — the handoff and acceptance contract.
- `docs/testing/PLANNER-TWO-QUESTION-LIVE.md` — the planner runbook and the
  issue #90 evidence example.

## Discovery

The skill is part of the canonical `skills/` tree and is installed with the other
SOP skills:

```sh
scripts/install/install-skills.sh zed
scripts/install/install-skills.sh claude
```

Zed and Claude Code expose it as `/mcp-live-validation`. The `/sop` entry point
lists it as an alias. The Claude Code plugin mirror is generated from the same
canonical tree:

```sh
scripts/packaging/build-claude-plugin.sh
```

A future agent can be given:

> Use the MCP live-validation skill to investigate and fix this issue. Prepare a
> candidate, coordinate independent ChatGPT testing, and open a PR.

The skill is reusable for any MCP surface. Its issue #90-style request is a
template, not a hardcoded requirement. For a different surface, keep the default
rules (concurrent read-only calls, no retries, no state changes, record
initialization errors) and replace the required calls in the handoff artifact.

## Workflow

1. **Discover** the failing layer, existing tooling, SOP state, candidate source,
   pinned revision, patch, baseline, and profile.
2. **Provision** the fix and candidate in an isolated branch/worktree. The helper
   reuses matching source/build caches, verifies the patch, builds in isolation,
   and records provenance. It never starts or stops a tunnel in this phase.
3. **Local verify** with the relevant Go/shell tests and
   `scripts/planner-tunnel-candidate.sh preflight`.
4. **Verify identity** from the manifest: candidate path/version/SHA-256, source
   revision, patch SHA-256, baseline, process command, logs, and rollback.
5. **Request deployment approval.** This is the only deployment gate.
6. **Deploy** with the verified `stop` and `start` lifecycle. No broad process
   kill and no second tunnel.
7. **Handoff** with `handoff --format json` (machine-readable) and the markdown
   form. The artifact records the required ChatGPT calls and
   `PENDING`/`PASS`/`FAIL`.
8. **Independent validation** by ChatGPT. The coding agent cannot invoke ChatGPT
   tools; it waits for actual results and never sets `PASS` from local tests.
9. **Failure loop** on `FAIL`: preserve the failed candidate and evidence,
   diagnose, fix, provision a new candidate, and return with a new `PENDING`
   artifact.
10. **Rollback, PR preparation, and completion reporting.** Rollback is
    deterministic; the PR is prepared but not merged.

## Handoff artifact

```sh
scripts/planner-tunnel-candidate.sh handoff \
  --format json \
  --issue <issue-reference> \
  --tests <local-test-evidence> \
  --output <checkout>/.run/tunnel/candidate/handoff.json
```

The JSON shape is `mcp-live-validation/handoff/v1`. It records the repository,
candidate identity, baseline identity, active process (redacted), candidate and
baseline logs, local tests, rollback, the required independent calls, and
`independent_validation.status`.

`PASS` and `FAIL` require `--results FILE` with the independent validator's
actual report. The helper refuses to record a status without evidence. It also
redacts common credential-like command values, but the artifact must be reviewed
before sharing.

For the issue #90-style test, the default request is 10 rounds of concurrent
`sop_status` and `sop_question_get`, 20 calls total, no retries, no state changes,
recording every initialization error including HTTP `409` and
`mcp_initialization_required`.

## Scenario evidence

The isolated regression suite is the evidence for the skill's mechanics. It uses
a temporary git checkout and mock processes and never touches a live tunnel:

```sh
make test-planner-tunnel
```

**Scenario A — successful fix.** The suite provisions a candidate from a pinned
mock source and patch, records its checksum/version/source/patch, passes
`preflight`, starts it, records the verified PID and log, and generates both the
markdown and JSON handoffs. The JSON artifact is validated as JSON and carries
`"status": "PENDING"` plus the 10-round/20-call request.

**Scenario B — failed validation.** The suite records a `FAIL` handoff with
independent failure evidence, then provisions a corrected candidate to a new
output/build path from a second patch. It asserts that the original candidate's
checksum, the failed handoff, and a manifest backup are preserved.

**Scenario C — unsafe environment.** The suite covers a missing candidate, a
checksum/version mismatch, multiple tunnel processes, an unexpected PID/command,
and a candidate that exits during startup. Each case stops safely; the baseline
checksum is unchanged and no unverified process is signalled.

## Remaining manual approval boundaries

- Deployment: the operator approves stopping the verified baseline and starting
  the candidate.
- Independent validation: ChatGPT must run the live calls and report the actual
  results. A clean local run is not a substitute.
- Promotion and merge: the operator approves the PR and any production binary
  change. The skill prepares the PR and never merges.

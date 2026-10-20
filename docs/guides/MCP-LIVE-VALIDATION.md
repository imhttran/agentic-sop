# MCP live validation skill

The `mcp-live-validation` skill is a reusable, provider-neutral workflow for MCP
integration failures: it discovers the environment, prepares a candidate fix,
verifies the candidate's identity, deploys it through an approval gate, hands the
live test to an independent MCP validator, and collects evidence for a PR.

It is **not** a new orchestrator. It is instructions over the deterministic
candidate helper and portable handoff added for issue #90:

- `scripts/planner-tunnel-candidate.sh` — provision, preflight, start, stop,
  status, rollback, and handoff.
- `scripts/planner-tunnel-point.sh` — choose the SOP project and binary the
  tunnel wrapper serves.
- `scripts/validation-handoff.py` — read-only schema/identity/approval validator.
- `docs/reference/validation-handoff.schema.json` — the versioned artifact.
- `docs/testing/MCP-CANDIDATE-HANDOFF.md` — the handoff and acceptance contract.
- `docs/guides/MCP-LIVE-VALIDATION-PROVIDERS.md` — ChatGPT, Claude, and
  tool-enabled-harness setup, with the capability matrix.
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
> candidate, coordinate independent MCP validation, and open a PR.

The skill is reusable for any MCP surface and any compatible agent or validator.
Its issue #90-style request is a template, not a hardcoded requirement. For a
different surface, pass `--request FILE` and keep the default rules (concurrent
read-only calls, no retries, no state changes, record initialization errors).

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
6. **Deploy** with the verified `stop` and `start --handoff` lifecycle. The
   start gate refuses unless the artifact authorizes `START_CANDIDATE`; no broad
   process kill and no second tunnel.
7. **Handoff** with `handoff --format json` (the portable artifact) and the
   markdown form. The artifact records the required calls, the results template,
   approval state, and `PENDING`/`PASS`/`FAIL`.
8. **Independent validation** by an MCP-capable validator. The coding agent
   cannot invoke the validator's tools; it waits for actual results and never
   sets `PASS` from local tests.
9. **Resume** from the artifact with `python3 scripts/validation-handoff.py`; it
   verifies identity, freshness, evidence, approval, and `next_permitted_action`.
10. **Failure loop** on `FAIL`: preserve the failed candidate and evidence,
    diagnose, fix, provision a new candidate, and return with a new `PENDING`
    artifact.
11. **Rollback, PR preparation, and completion reporting.** Rollback is
    deterministic; the PR is prepared but not merged.

## Portable handoff

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
  --approval-reference <link-or-ticket; required with APPROVED> \
  --output <checkout>/.run/tunnel/candidate/validation-handoff.json
```

The JSON shape is `mcp-live-validation/validation-handoff/v1` with
`schema_version: 1`. It records the repository, environment, candidate and
baseline identities, active process (redacted), logs, local verification, the
requested calls, the results template, independent results, approval state,
workflow status, and `next_permitted_action`.

The helper validates the JSON against the schema and the current candidate before
writing it, uses an atomic write, and refuses a silent overwrite. `PASS` and
`FAIL` require `--results-json FILE` with the independent validator's actual
results object; the results must carry the same workflow ID, candidate SHA-256,
and source revision. The helper refuses a status without that evidence and
redacts common credential-like command values.

For the issue #90-style test, the default request is 10 rounds of concurrent
`sop_status` and `sop_question_get`, 20 calls total, no retries, no state changes,
recording every initialization error including HTTP `409` and
`mcp_initialization_required`.

## Cross-agent continuation

Any agent can resume from the artifact, but it must verify it first:

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

If the agent lacks filesystem or process access, run `--artifact-only` and report
that limitation. Do not claim the candidate file or process was verified, and do
not treat missing validation as success. Preserve previous handoffs and the
original failure evidence; use a new output path for each round.

## Scenario evidence

The isolated regression suites are the evidence for the skill's mechanics. They
use a temporary git checkout, mock processes, and a fake candidate; they never
touch a live tunnel:

```sh
make test-planner-tunnel
make test-mcp-portability
```

**Scenario A — successful fix.** The suite provisions a candidate from a pinned
mock source and patch, records its checksum/version/source/patch, passes
`preflight`, starts it, records the verified PID and log, and generates the
markdown and JSON handoffs. The JSON artifact is validated as JSON and carries the
portable request and `PENDING` status.

**Scenario B — failed validation.** The suite records a `FAIL` handoff with
independent failure evidence, then provisions a corrected candidate to a new
output/build path from a second patch. It asserts that the original candidate's
checksum, the failed handoff, and a manifest backup are preserved.

**Scenario C — unsafe environment.** The suite covers a missing candidate, a
checksum/version mismatch, multiple tunnel processes, an unexpected PID/command,
and a candidate that exits during startup. Each case stops safely; the baseline
checksum is unchanged and no unverified process is signalled.

The dedicated portability suite additionally covers the simulated Claude →
ChatGPT → second-agent flow and rejects stale commits, mismatched checksums,
missing evidence, missing candidate identity, wrong workflow IDs, unsupported
schema versions, stale timestamps, and unapproved deployment requests.

## Simulated vs live

The compatibility tests are **simulated**. They prove the artifact, validator, and
rejection rules work; they do not prove a live ChatGPT, Claude, or third-party
harness integration. Actual live cross-provider evidence requires a live coding
agent harness, a running candidate tunnel, an independent MCP validator, and a
second agent to resume from the artifact. When those are unavailable, report the
missing capability exactly and label the result as simulated.

## Remaining manual approval boundaries

- Deployment: the operator approves stopping the verified baseline and starting
  the candidate.
- Independent validation: the MCP validator runs the live calls and reports the
  actual results. A clean local run is not a substitute.
- Promotion and merge: the operator approves the PR and any production binary
  change. The skill prepares the PR and never merges.

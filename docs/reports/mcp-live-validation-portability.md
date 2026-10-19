# MCP live-validation portability evidence

This report records what was actually tested for the provider-neutral
`validation-handoff.json` workflow, separately from what remains simulated.

## Actual evidence

### Real candidate artifact

A real candidate was provisioned earlier from the pinned local
`openai/tunnel-client` source plus the issue #90 patch, in an isolated checkout.
The current DeepSeek-based coding harness then generated and validated a portable
handoff for that candidate:

- workflow ID: `mcp-live-20261019T224635Z-90350`
- candidate version: `0.0.16+d8ecb31`
- candidate SHA-256: `57c1c93a304437847645fc861a7d83d0ff0bd446bc7dcb65929e20c52ef0c737`
- source revision: `d8ecb31659c15fb66e8e8f9dff2e167816602a08`
- patch SHA-256: `5e4dc6ee454045a41073ce43cfa33102db277cced2b4bfdbf1ff0472cc991244`
- local verification: `PASS`
- independent validation: `PENDING`
- workflow status: `DEPLOYMENT_PENDING_APPROVAL`
- next permitted action: `REQUEST_DEPLOYMENT_APPROVAL`

The artifact passed `scripts/validation-handoff.py --checkout <checkout>` against
the real candidate and baseline files. Local absolute paths were sanitized before
sharing; the candidate and source identities were not changed.

### Actual Claude review

Claude Code `2.1.286` was run with **tools disabled** (`claude -p --tools ""`) on a
sanitized copy of the real artifact. It was asked to act as an independent
reviewer and return a structured verdict. It returned:

```json
{
  "reviewer": "claude",
  "workflow_id": "mcp-live-20261019T224635Z-90350",
  "candidate_sha256": "57c1c93a304437847645fc861a7d83d0ff0bd446bc7dcb65929e20c52ef0c737",
  "validation_status": "PENDING",
  "next_permitted_action_you_would_allow": "REQUEST_DEPLOYMENT_APPROVAL",
  "may_prepare_pr": false,
  "reasons": [
    "independent_validation.status is PENDING; missing validation is not success",
    "0 of 20 required calls recorded; calls array empty, started_at/finished_at empty",
    "validator is unspecified and validator_capabilities is empty",
    "deployment approval status is PENDING with empty reference, so deployment is not permitted",
    "local_verification PASS alone does not satisfy PREPARE_PR (local_tests_are_not_validation=true)",
    "candidate_sha256 is consistent across candidate, independent_validation, and results_template",
    "process state is 'prepared' with no command or pid; candidate not launched",
    "workflow.next_permitted_action REQUEST_DEPLOYMENT_APPROVAL matches artifact state",
    "repository working tree has 4 uncommitted paths; provenance of local verification build should be confirmed",
    "safety: production binary not replaced, not merged, credentials redacted"
  ]
}
```

This is real cross-provider artifact consumption: a DeepSeek-based coding harness
produced the handoff, and Claude read it and correctly refused to prepare a PR
without independent validation and deployment approval. It is **not** a live MCP
validation: Claude had no MCP tools in this run.

## Simulated evidence

The automated compatibility suite is simulated. It uses a temporary repository, a
fake candidate, and a canary process:

```sh
make test-mcp-portability
```

It covers:

- **Scenario A — Claude to ChatGPT:** a Claude-role writer prepares a candidate
  handoff, records deployment approval, and produces a running-candidate handoff;
  ChatGPT-role results are associated with the correct candidate SHA-256 and move
  the workflow to `PREPARE_PR`.
- **Scenario B — ChatGPT to a second agent:** a second agent reads the completed
  evidence, determines acceptance, and resumes at `PREPARE_PR` without rebuilding,
  redownloading, or changing the manifest.
- **Scenario C — invalid handoff:** stale commits, mismatched candidate checksums,
  missing evidence, missing candidate identity, wrong workflow IDs, unsupported
  schema versions, stale timestamps, wrong source revisions, and unapproved
  deployment requests are rejected without touching the candidate, baseline, or
  canary process.
- **Deployment gate:** `start --handoff` refuses an artifact that does not
  authorize `START_CANDIDATE` and launches only after an approved handoff.

These tests prove the artifact, validator, and rejection rules. They do not prove a
live ChatGPT, Claude, or third-party harness integration.

## Not tested, with the missing capability

- **Live ChatGPT MCP validation:** no ChatGPT agent interface with the candidate's
  MCP tools was available in this environment. The Secure MCP Tunnel's control
  plane is not a local MCP endpoint that the coding harness can call directly.
- **Live Claude MCP validation:** Claude Code is installed, but it was run with
  tools disabled for the artifact review. It was not configured against the running
  candidate tunnel, and the tunnel's OpenAI control plane is not exposed as a local
  MCP endpoint.
- **A second live coding harness:** no independent Claude/DeepSeek coding harness
  with its own filesystem and process execution was available to reproduce the full
  forward and reverse order.
- **Independent 20-call MCP run:** the live 20-call read-only test must be performed
  by ChatGPT or another MCP-capable validator against the running candidate. It was
  not performed here, so the artifact remains `PENDING`.

## Conclusion

The portable artifact, validator, and cross-agent continuation rules are tested,
including one actual Claude model review of a real candidate artifact. Full live
cross-provider MCP validation is not claimed. The remaining requirement is a live
independent validator with MCP tools against the running candidate, followed by a
second agent resuming from the recorded evidence.

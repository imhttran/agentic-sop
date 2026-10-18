# SOP Reliability Review — Final Report

## Scope and attribution

This report separates three kinds of work:

1. **Earlier SOP task outcomes (not replayed here):** `PLAN-SOP-EXECUTION-RELIABILITY.md`
   SOP-001..SOP-005 were completed in the prior execution and remain
   `LOCAL_DONE`. Their artifacts are under `.agent-sdlc/runs/SOP-00{1..5}/`.
2. **Direct development fixes in this review:** the product diff summarized
   below. They were made directly, not by a new broad SOP plan.
3. **SOP-006 bounded workflow:** a disposable Rust fixture driven by the
   candidate binary. It is a bounded demonstration, not full Rust-migration
   readiness.

No product commit, push, merge, state.db edit, or installed-binary replacement
was performed. The installed `sop` is still `/Users/imhttran/go/bin/sop`.

## Candidate binary

- Path: `.run/candidate/sop`
- SHA-256: `c161b0aab7e18bf75b90ce940dc878d64148dabf15214f676d0b9d35c6602d03`
- Module: `github.com/imhttran/agentic-sop`
- VCS revision: `e99cde03aff969b458e6d23579d6b3bb3a6fbe56`
- VCS modified: `true` (the candidate includes the uncommitted review fixes)
- Build command: `go build -trimpath -o .run/candidate/sop ./cmd/sop`

The candidate recorded its own path, checksum, module, revision, and modified
flag in each task's `counts.json`.

## Direct development fixes

- **Cargo safety ordering:** `internal/commandpolicy/cargo.go` now validates
  shell syntax, subcommand, arguments, and manifest containment before the
  permission flag. Malformed forms are denied even when the permission is off.
- **Agent-tool wiring:** the project's `cargo.agent_tool` permission is now
  passed from `internal/config` through the CLI composition root into
  `internal/ollamaagent.Config` and `internal/toolharness.Config`. Previously it
  was unit-tested but never reached the real tool-harness agent.
- **Manifest root:** `internal/toolharness` classifies `--manifest-path` against
  the primary repository root, not a selected sub-root.
- **Configuration compatibility:** `agent.tool_cargo` and
  `validation.acceptance_checks.cargo` are accepted legacy spellings. They map to
  `cargo.agent_tool` / `cargo.acceptance_check` only when the canonical key is
  omitted; equal values are accepted; conflicting values fail closed with an
  error naming both keys. Defaults remain off. Migration is documented in
  `docs/reference/CONFIGURATION.md`.
- **Preflight:** acceptance-only Cargo no longer triggers a permission-conflict
  blocker; preflight runs before required-evidence retrieval and uses the actual
  harness authorization path for agent-tool commands. A setup blocker is a
  distinct internal outcome, so a recovered task cannot loop until the driver
  backstop.
- **Required-criterion enforcement:** mapped acceptance criteria are enforced;
  a mapped check is evidence for its criterion rather than a duplicate required
  criterion; a valid designated-approver waiver clears advancement; unmapped,
  failed, unexecuted, or unaccepted required evidence blocks.
- **Manual evidence binding:** acceptance decisions now bind to criterion text,
  manual-evidence text, and attempt. Stale or unbound decisions are ignored and
  rejected at recording. Decision-read errors fail closed.
- **Artifact writes:** failed check-log and outcome-artifact writes are returned,
  not swallowed; `outcome.json` is not written without required evidence.
- **Recovery wiring:** `recovery.Recover` is called on the production requeue
  path; unchanged/discovery-only retries are refused, recovery history and one
  blocker report persist, and safety/fail-closed decisions run before a disabled
  escalation policy. Productive continuations remain on the separate
  continuation budget.
- **Lifecycle accounting:** counts are written by the real lifecycle as absolute
  totals, so a report does not add itself to a seeded artifact. Approvals count
  approved decisions, not requests. Candidate provenance is recorded.
- **Approval-safe recovery:** a PENDING or DECLINED approval is checked before
  required-evidence retrieval, run reset, or model work. A mapped acceptance
  failure now records a PENDING approval request and parks at
  `WAITING_FOR_HUMAN`; it no longer requeues without an approval boundary.

## Approval-safe recovery evidence

SOP-006 originally resumed after `NEEDS_HUMAN` because the mapped-acceptance
failure branch called `recoverTask` directly, without recording an approval
request. There was no persisted approval, so the next `sop run` saw a runnable
task and executed it. That bypass is fixed.

Fixture: `.run/sop-006-rust-approval/`

Exact sequence and results:

1. First run:

   ```bash
   SOP_AGENT_COMMAND='python3 .../sop-006-rust-approval/agent.py' \
     .run/candidate/sop run PLAN.md
   ```

   Result: exit 1. S001 produced `PARTIAL` from the controlled failing Rust
   test, recorded a PENDING approval request, and stopped at
   `WAITING_FOR_HUMAN`; S002 did not run. Output:
   `.run/sop-006-rust-approval/evidence/01-first-run.txt`.

2. Second run without a decision:

   ```bash
   SOP_AGENT_COMMAND='python3 .../sop-006-rust-approval/agent.py' \
     .run/candidate/sop run PLAN.md
   ```

   Result: exit 1, `S001 WAITING_FOR_APPROVAL ...`. Counts were unchanged:
   `invocations=1`, `attempts=1`, `model_calls=6`, `continuations=0`. Output:
   `evidence/02-pending-block.txt`; before/after counts:
   `evidence/02-counts-before-approval.json`, `evidence/03-counts-after-block.json`.

3. Explicit approval:

   ```bash
   .run/candidate/sop approve S001 --by operator
   ```

   Result: exit 0, head `APPROVED` by `operator`:
   `evidence/04-approve.txt`, `evidence/05-approval-approved.json`.

4. Approved rerun:

   ```bash
   SOP_AGENT_COMMAND='python3 .../sop-006-rust-approval/agent.py' \
     .run/candidate/sop run PLAN.md
   ```

   Result: exit 0. S001 and S002 both `VERIFIED` / `LOCAL_DONE`;
   `SOP COMPLETE`, `Tasks: 2/2 complete`, `Final gate: PASS`.
   Output: `evidence/06-approved-run.txt`.

Final S001 counts (`evidence/07-counts-final.json`):

```json
{"approvals":1,"attempts":2,"continuations":0,"direct_interventions":0,"invocations":2,"model_calls":9}
```

Final S002 counts: `invocations=1`, proving the dependent task did not execute
while S001 was unmet. `continuations=0`; no budget reset or state edit was used.
`sop approvals --json` is empty after the approved task completes.

Declined behavior is covered by the production-path regressions
`TestDeclinedApprovalBlocksSecondRun` and
`TestPendingApprovalBlocksThenApprovedRunExecutes`.

## Agent-tool Cargo evidence

Backend actually used: `harness: tool`, `provider: ollama`, model
`deepseek-v4.1-flash:cloud`, through the candidate binary. The durable
`SOP_TOOL_AUDIT_LOG` JSONL is the evidence.

Fixture: `.run/sop-cargo-tool/`

| Case | Config | `run_command` request | Audit result |
|---|---|---|---|
| Enabled | `agent_tool: true`, `acceptance_check: false` | `cargo test --quiet` | `allow` / `ok` |
| Disabled | `agent_tool: false`, `acceptance_check: false` | `cargo test --quiet` | `deny` / `REQUIRES_APPROVAL` |
| Unsupported | `agent_tool: true` | `cargo publish` | `deny` / unsupported subcommand |
| Manifest escape | `agent_tool: true` | `cargo test --manifest-path ../outside/Cargo.toml` | `deny` / path escapes repository |

Preserved evidence:

- `evidence/enabled.jsonl`, `evidence/enabled-tool-ran.txt`, `evidence/enabled-run.txt`
- `evidence/disabled.jsonl`, `evidence/disabled-tool-ran.txt`, `evidence/disabled-run.txt`
- `evidence/unsupported.jsonl`, `evidence/unsupported-tool-ran.txt`, `evidence/unsupported-run.txt`
- `evidence/escape.jsonl`, `evidence/escape-tool-ran.txt`, `evidence/escape-run.txt`

Acceptance-check permission stayed `false` in all four tool cases; the
acceptance-only Rust workflow used `acceptance_check: true` with
`agent_tool: false`. The permissions therefore remained independent.

**Backend limitation:** SOP-006 used `harness: command` with a scripted command
agent. That backend executes its own script and does **not** go through
`internal/toolharness`, so SOP-006 does not prove agent-tool Cargo enforcement.
The proof above applies to the native tool harness used with the Ollama backend.
No claim is made for the command backend or for providers not exercised here.

## Validation

Commands run after the final code changes:

```bash
go build ./...
go test ./... -count=1
go vet ./...
```

All passed. `git diff --check` is clean.

Named-test gate (matched by `go test -list`, then executed with
`go test -json`; a zero-match filter would fail this check):

- 51/51 named tests matched
- 51/51 emitted test-level `pass` events
- No missing names

New regression coverage includes:

- `TestCargoLegacyPermissionsMigrate`
- `TestCargoSafetyOrderingBeforePermission`
- `TestHarnessPassesCargoAgentToolPermission`
- `TestPendingApprovalBlocksThenApprovedRunExecutes`
- `TestDeclinedApprovalBlocksSecondRun`
- `TestScheduledAcceptanceRecordedWaiverAllowsAdvancement`
- `TestScheduledAcceptanceRecordedManualDecisionAllowsAdvancement`
- `TestScheduledLifecycleRecordsCountsAndProvenance`
- `TestRunUnchangedFailureIsNotRetriedAgain`
- `TestFailedCheckLogWriteFailureIsReported`

## SOP-006 bounded success vs. full Rust-migration readiness

**Demonstrated:** a two-stage disposable Rust fixture, implemented and verified
through SOP; a controlled acceptable failure blocked the dependent stage; a
PENDING approval blocked a second run before any model work; explicit approval
permitted recovery; no budget reset or state edit; accurate final counts.

**Not demonstrated / not claimed:**

- Full Rust-migration readiness for a production repository.
- Cargo enforcement through the `command` agent backend.
- Other tool-capable providers or non-Ollama backends.
- Automatic re-request after a DECLINED approval; a declined request remains
  authoritative and there is no new re-request command in this change.
- RAG/database provisioning, UI work, security-scanner additions, or migration
  execution (outside this plan's scope).

## Remaining blockers

None for the requested review. The main limitations are the backend-coverage
boundary above and the absence of a re-request operation after a decline.

# Operator approval packet — ACP execution-lease wiring

Status: **approval packet, read-only. Nothing here is approved, applied, or
authorized.** This document requests the four decisions the blocked SOP run needs
before implementation may resume. It follows the repository's decision-record
convention (`docs/plans/PROPOSAL-*-OPERATOR-DECISION-RECORD.md`).

Plan (untracked working-tree document, so it is named rather than linked):
`docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING.md`
(`Status: Draft, unreviewed — authorizes no implementation`).
Design record: [`PLAN-ACP-EXECUTION-LEASE-WIRING-DESIGN-DECISIONS.md`](PLAN-ACP-EXECUTION-LEASE-WIRING-DESIGN-DECISIONS.md).
Acceptance mapping: [`ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md`](ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md).
Gap record: [`ACP-EXECUTION-EXCLUSION-GAP.md`](ACP-EXECUTION-EXCLUSION-GAP.md).
Follow-up tests: #575.

## Current state (verified 2026-11-21, `main` = `c673d442`)

| Item               | State                                                                                                                                                                             |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Plan authorization | absent — the plan still says it authorizes no implementation                                                                                                                      |
| Approval artifacts | none (`.agent-sdlc/approval.json` absent)                                                                                                                                         |
| SOP plan           | `plan-acp-execution-lease-wiring`, `ACTIVE`                                                                                                                                       |
| Tasks              | `stage-1`, `stage-10` `BLOCKED` (RETRIES_EXHAUSTED, attempt 0/3); 14 `PLANNED`; 0 completed                                                                                       |
| `execution.lease`  | absent from `.agent-sdlc/config.yaml` ⇒ `off`                                                                                                                                     |
| Stage-1 work       | uncommitted: `internal/executionlease/inherit.go`, `internal/cli/execution_lease.go`, `internal/cli/mcp_start_work.go` (+6 lines), and two test files. Builds; inherit tests pass |
| Security baseline  | unchanged; `internal/cli/mcp_start_work.go` entry still holds `semgrep:1c1fe971…86180`                                                                                            |
| Plan digest        | `plan.meta.json` `source_sha256` = `6e9922ed…`; the plan file is unchanged, so the digest still matches                                                                           |

## Decision 1 — authorize the plan

**Requested:** approve `PLAN-ACP-EXECUTION-LEASE-WIRING.md` as the implementation
authority for stages 1–16, with the nine design decisions in the design record.

What approval would mean:

- The plan's `Status:` line is replaced by an approval line naming the approver and
  date. Prose status is documentation only; SOP does not parse it (see below).
- The design record's recommendations become the accepted scope.
- Approval does **not** authorize a security-baseline change (Decision 2), a plan
  revision (Decision 3), or a merge (Decision 4).

Why operator input is required: SOP has no plan-level approval gate. The only
approval mechanisms are task-level `NEEDS_HUMAN` (`internal/domain/approval.go:45-55`)
and `human.approval_before_commit` (`internal/cli/commit.go:31`,
`internal/cli/pr.go:36`). `PreflightPlan` compares plan identity and digests only
(`internal/planflow/planflow.go:320-331`); nothing reads a plan's prose status.
An existing run is therefore not evidence of authorization.

## Decision 2 — reconcile the security-baseline fingerprint

**Finding (verified independently).**

| Fact                | Value                                                                                                                                                                                                 |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rule                | `dangerous-exec-command` (`internal/security/.../audit/dangerous-exec-command.yaml`, severity ERROR, CWE-94, confidence LOW)                                                                          |
| Path / column       | `internal/cli/mcp_start_work.go`, column 9                                                                                                                                                            |
| Baseline entry      | `semgrep:1c1fe971…86180`, line 721                                                                                                                                                                    |
| Current finding     | `semgrep:bba96ea0…c8873`, line 727                                                                                                                                                                    |
| Matched code        | `cmd := exec.Command(exe, args...)` — unchanged since 2026-10-27 (`git blame`)                                                                                                                        |
| Diff above the call | +6 lines: one import and the five-line `InheritEnv` constant (stage-1)                                                                                                                                |
| Semgrep version     | 1.183.0 in both the 2026-11-08 and 2026-11-21 scans                                                                                                                                                   |
| Fingerprint source  | semgrep OSS returns `"requires login"`; SOP computes `StableFingerprint(provider, ruleID, path, line, column)` (`internal/security/normalize.go:116`, used at `internal/security/semgrep.go:361-363`) |

Reproduction: `("semgrep","dangerous-exec-command","internal/cli/mcp_start_work.go",721,9)`
hashes to `1c1fe971…`; the same tuple with `727` hashes to `bba96ea0…`. Only the line
changed. `internal/security/baseline.go:147-148` documents exactly this: the
suppression identity includes line and column.

**Security implication:** none. `argv` is an array, not a shell string; `exe` is
`os.Executable()`; the arguments are a fixed `prompt --capability <validated> --file
<path inside the job dir>` or `run <confined plan path>`; no shell is invoked and no
untrusted text reaches `argv`. The existing baseline reason remains true.

**Recurrence risk.** The finding moves again whenever lines are added above the call.
Stages 3, 4 and 5 edit `startWork`, which sits above `launchWorkProcess`, so the
baseline would need refreshing repeatedly if the launcher stays in place.

**Options.**

- **Option A — refresh the existing entry (recommended, one-time per layout
  change).** Replace only the `fingerprint` value in the existing entry and append a
  dated note to `reason`. Keep `rule_id`, `path`, `owner` and `expires_at`. Expect a
  second refresh after stages 3–5 land.
- **Option B — move the launcher to its own file.** Relocate `launchWorkProcess` to
  `internal/cli/launch.go` in the same change, then replace the entry with one whose
  `path` is the new file. Future edits to `startWork` no longer move the match. Cost:
  a larger baseline change (path plus fingerprint) and a code move; the rule still
  fires, so this is not a suppression.
- **Option C — no change.** Keep the run blocked.

**Exact change under Option A** (not applied):

```json
{
  "fingerprint": "semgrep:bba96ea0…c8873",
  "rule_id": "dangerous-exec-command",
  "path": "internal/cli/mcp_start_work.go",
  "reason": "By design: the CLI orchestrates subprocesses. argv is a constant, operator configuration, or a validated payload, and dynamic data is passed on stdin. Triaged 2026-11-03; fingerprint refreshed 2026-11-21 after unrelated lines above the unchanged exec.Command call shifted the match from line 721 to 727 (same rule, path, column and matched code).",
  "owner": "imhttran",
  "expires_at": "2027-06-30T00:00:00Z"
}
```

Not requested: any new suppression, any severity or `fail_on` change, any exclusion,
or any edit to `.agent-sdlc/security-rules/`. The full fingerprint value must be
taken from the operator's own re-run, not from this document's elision.

## Decision 3 — integrate the acceptance mapping

The mapping document is merged and matches all 16 task IDs. Verification found one
inconsistency: its section titles are the **compiled task titles** shown by
`sop status`, while the plan numbers the same stages with different `### N.` headings.
IDs are authoritative; the titles differ for all 16 stages.

**Why a plan edit is not a no-op.** The plan is not written in the deterministic
rendered shape (`## <stage-id> — <title>` with `### …` sub-sections). `Compile`
therefore falls back to the model (`internal/planner/markdown.go:351-391`),
`parseTaskSection` only accepts `## Tasks`-style sections (`markdown.go:474-478`),
and the plan uses `## Steps`. Consequence: any plan edit forces a model-based
recompile, which is not guaranteed byte-stable. The current "unchanged" preview is a
deterministic no-op (`internal/planflow/planflow.go:823-846`) and proves nothing
about a changed plan.

**Options.**

- **Option A — keep the mapping separate (recommended, zero risk).** Record in the
  approval that `ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md` is the
  authoritative acceptance reference for stages 1–16. No plan edit, no recompile, no
  digest change, no risk to executed task definitions.
- **Option B — append the mapping to the plan.** Edit the plan, then preview with
  `sop reconcile docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING.md --list-changed` and
  only apply if no executed task changes; if stages 1 or 10 change, apply with
  `--accept-changed stage-1 --accept-changed stage-10`. Risk: the model may
  re-derive titles or definitions; a removed executed stage stops reconciliation.
- **Option C — canonicalize the plan.** Rewrite every stage into the rendered shape
  so compilation becomes deterministic and `### Acceptance Mapping` feeds
  `Stage.AcceptanceMapping`. Highest one-time review cost; largest definition churn.

**Not requested:** manual edits to `.agent-sdlc/plan.meta.json` or `.agent-sdlc/plan.json`.
If Option B is chosen, the digest is rewritten only by `sop reconcile`.

## Decision 4 — merge authorization

No merge without explicit approval. The PR that carries this packet is documentation
only and changes no production behaviour.

## Post-approval sequence (for reference; not executed)

1. Record Decisions 1–3 in a decision record (template below).
2. Apply the authorized baseline change and re-run the required scan.
3. Preserve the blocked evidence:
   `cp -R .agent-sdlc/runs/stage-1 .agent-sdlc/runs/stage-10 .agent-sdlc/recovery-20261121/`.
4. If Decision 3 chose B or C: `sop reconcile <plan> --list-changed`, review, then
   `sop reconcile <plan>` with `--accept-changed` only for tasks that legitimately
   changed.
5. `sop retry stage-1`, then `sop retry stage-10` — individually, never `--all`.
6. Continue with `sop run` under the normal lifecycle.

## Approval record template

```md
# Operator decision — ACP execution-lease wiring

**Status:** <APPROVED | REJECTED | DEFERRED> — decided by <name> on <YYYY-MM-DD>.

- **Decision 1 — plan:** <approved | rejected>. Scope: PLAN-ACP-EXECUTION-LEASE-WIRING
  stages <1-16 | subset>. Design decisions D1–D9: <accepted | accepted with changes:
  …>.
- **Decision 2 — security baseline:** <authorized: Option A | Option B | not
  authorized>. Entry: rule `dangerous-exec-command`, path `internal/cli/mcp_start_work.go`,
  fingerprint <old> → <new>. Conditions: <…>.
- **Decision 3 — acceptance mapping:** <Option A | B | C>.
- **Decision 4 — merge:** <authorized per PR | not authorized>.
- **Expiry / review date:** <…>.
- **Evidence:** this packet, the design record, the gap record, run artifacts under
  `.agent-sdlc/runs/stage-1` and `.agent-sdlc/runs/stage-10`.
```

## Evidence index

- `internal/executionlease/lease.go` — `Acquire`, `Held.Release`, `WatchHeartbeat`,
  `State.Stale`, `ReleaseStale`, `HeartbeatEvery`/`HeartbeatLimit`/`Grace`.
- `internal/executionlease/inherit.go` (uncommitted) — `InheritEnv`, `InheritedOwner`.
- `internal/cli/execution_lease.go` — `executionLease` (main), `leaseAcquireOrInherit`
  (uncommitted, no caller).
- `internal/cli/mcp_start_work.go` — `startWork`, `acquireWorkLock`, `launchWorkProcess`,
  the new `InheritEnv` constant.
- `internal/cli/acp.go` — `acpSubmission`, the 250 ms `projectlock` probe.
- `internal/cli/resume.go` — `reportStaleExecutionLease`.
- `internal/security/normalize.go`, `internal/security/semgrep.go`,
  `internal/security/baseline.go` — fingerprint computation and drift behaviour.
- `.agent-sdlc/runs/stage-1`, `.agent-sdlc/runs/stage-10` — `gate.json`,
  `classification.json`, `security/scan.json` (the single blocking finding).

## Not requested

- No new security suppression, exclusion, severity change or scan weakening.
- No manual task-status edit, no `sop retry --all`, no bypass of reconciliation.
- No deletion of worktrees, run artifacts or uncommitted work.
- No release-acceptance claim for the ACP exclusion gap.

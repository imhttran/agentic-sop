# LLM03 Plan 01 — transfer checkpoint

Transfer checkpoint only. This branch records recovered work for the move to macOS. It
does not claim that implementation, validation, or review is complete, it is not a merge
candidate, and it does not change SOP lifecycle state.

## ID

LLM03-TRANSFER

## Objective

Carry the paused OWASP LLM03 (excessive agency) workstream to a healthy host with the
recovered plan, its AGENCY-001 deliverable, and an accurate record of which checks
completed and which were interrupted by storage failure.

## Baseline

- Repository: `imhttran/agentic-sop`
- Baseline commit (recorded plan baseline): `0750e8be0cf5f318a173abb34c2b752ac131521c`
- `origin/main` when this branch was cut: `e55527f42561f261be264282a06e3fe302cd9edd`
  (5 commits ahead, including Plan 04 / PR #265). No rebase was attempted here.
- SOP plan: `plan-01-sop-llm03-excessive-agency`
- SOP plan source: `docs/plans/PLAN-01-SOP-LLM03-Excessive-Agency.md`
- SOP plan source sha256: `992bfecd8b2abe73786ce0b92c3e7e6f312d1142937fd0ef60cf2eae74b4b325`
- SOP lifecycle at checkpoint: plan `ACTIVE`; `AGENCY-001`, `AGENCY-002`, `AGENCY-003`
  all `PLANNED`; no approval requests; last run `CONTINUE`
  (`INCOMPLETE_IMPLEMENTATION`, cause `VALIDATION_FAILED`). Nothing is marked complete.

## Recovered changes

| File | sha256 | Origin |
|---|---|---|
| `docs/plans/PLAN-01-SOP-LLM03-Excessive-Agency.md` | `992bfecd…b4b325` | SOP-adapted plan, byte-identical to the archived checkpoint |
| `docs/reports/owasp-llm/llm03-agency.md` | `c28ff614…dfb90c` | AGENCY-001 deliverable, 376 lines, recovered verbatim from the backup |
| `docs/reports/owasp-llm/HANDOFF-llm03-transfer.md` | this file | new |

The report is the AGENCY-001 inventory: path-by-path execution matrix, policy-versus-OS
isolation distinction, and explicit `UNVERIFIED` markers. SOP has **not** accepted it.

## Completed checks

Observed on the Linux source host before the move (`/dev/sdb` degrading):

- `go build ./...` — PASS, exit 0, 3.2 s (recorded in the run validation evidence).
- `go vet ./...` — PASS, exit 0 (agent-invoked; recorded in the run report).
- Linux Bubblewrap isolation suite — PASS, 27.7 s, operator-observed with

  `SOP_REPLAY_ISOLATION_REQUIRED=1 go test ./internal/dqharness/ -run 'TestIsolatedReplay|TestReplayIsolationRequiredEnv' -count=1 -timeout 240s -v`

  including the Linux-only `TestIsolatedReplayUnixSocket` and
  `TestIsolatedReplayToolchainProcAndTemp`. This is the SEC-LEFT-005 Linux runtime
  evidence; reproduce it on the Mac and re-record it there.
- Focused `go test` subsets under `internal/{agent,agentbin,git,github,verify,toolharness,mcp,planner,evaltarget,commandpolicy}` — agent-reported pass. Not independently
  re-run by the operator, so treat as unconfirmed until re-run on the Mac.

Raw run evidence (`validation.json`, `trace.json`, `report.json`, `logs/`) is deliberately
**not** on this branch; it remains in the migration backup.

## Interrupted checks

- `go test ./...` (the repository-required check) — **FAIL by timeout**, reproduced twice:
  Go's 10-minute per-package limit expired for `internal/cli`, `internal/controller/web`,
  `internal/dqharness`, and `internal/store`; `TestDogfoodRecoveryFlow` also failed at
  14.8 s. Reproduced again after the transient backup job finished, with the device idle.
- Cause: the host system disk `/dev/sdb` (`ST1000LX015-1U71`, rotational, hosts `/` and
  `/home`) shows a rising SMART prefailure attribute 1 (`Raw_Read_Error_Rate`, 67 → 83)
  and measures 100–200 ms `fsync` versus 2–8 ms on `/dev/sda`; `jbd2` average commit
  104 ms. Individual packages take seconds per fsync-bound operation.
- Classification: host storage failure, **not** an implementation finding. Do not weaken,
  skip, or re-timeout the check to make it pass; run it where storage is healthy.

## Constraints

- This branch is a transfer artifact, not a completion claim: do not mark AGENCY-001/002/003
  complete, do not approve anything, and do not merge on the strength of this commit.
- Plan 02 must not start until Plan 01 is merged.
- Do not reintroduce the excluded local material (see below) into the repository.

## Excluded from this branch

- `docs/plans/PLAN-02-SOP-LLM01-Prompt-Injection.md` (planning document only; remains in
  the migration backup).
- The original, pre-SOP-adaptation Plan 01 text (preserved in the backup under
  `other-worktrees/agentic-sop/`).
- `.env` (model routing), `.agent-sdlc/**` (local config, state database, run artifacts),
  security databases, caches, credentials. No credential material was found in the backup.

## Resume

```sh
git fetch origin
git switch task/LLM03-TRANSFER-plan-01-transfer-checkpoint
# restore the SOP checkpoint from llm03-sop-state.tar.gz (.agent-sdlc/{plan.json,plan.meta.json,state.db,runs,logs})
# then verify: sop status  -> plan ACTIVE, 3 tasks PLANNED, no approvals
sop run "docs/plans/PLAN-01-SOP-LLM03-Excessive-Agency.md"
```

Before opening a real Plan 01 PR, rebase onto current `main` and add the LLM03 entry to
`docs/reports/owasp-llm/README.md` (owned by Plan 04) rather than replacing it.

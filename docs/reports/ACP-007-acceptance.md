# ACP-007 — Real-Zed acceptance record

Task: ACP-007 of [PLAN-SOP-ACP-ZED.md](../plans/PLAN-SOP-ACP-ZED.md). This report records the bounded real-Zed acceptance status. It claims only what was executed. A scenario never claims a result that was not run.

## Status summary

| Environment | Status | Reason |
| --- | --- | --- |
| macOS | **OUTSTANDING** | Not executed on macOS. Must not be claimed until a macOS run records versions, revision, job IDs, outcomes, and evidence paths. |
| Linux | **NOT EXECUTED in the ACP-007 invocation** | The task environment has no Zed application and could not probe an installed Zed or fetch its documentation. The bounded recipe below defines the run; execute it and replace this section's status with recorded results. |

No acceptance scenario is claimed as passed by this report.

## Bounded acceptance recipe (both platforms)

Prerequisite: the SOP binary built from the revision under test, installed and on the PATH; Zed installed.

Record at the top of the run: date, host, OS version (`uname -a`, and on macOS `sw_vers`), Zed version (`zed --version` from the running release), SOP version (`sop version` or `sop acp` initialize response `agentInfo`), repository revision (`git rev-parse HEAD`).

Scenarios, each recorded in its own file under `docs/reports/evidence/ACP-007-ZED-<darwin|linux>/` with the job ID(s), outcome, and output captured:

1. **Agent appears.** Add SOP under `agent_servers` per [docs/guides/ACP-ZED.md](../guides/ACP-ZED.md). Record the settings schema as the installed Zed accepted it (screenshot or the settings file path), and that SOP appears in the agent picker. Fix the guide and the schema section of the discovery note if the accepted schema differs.
2. **One bounded request.** From a disposable, initialized project, submit one bounded prompt (for example a read-only `plan` capability prompt). Record the job ID and the final outcome, and confirm the outcome agrees with the persisted record in `.agent-sdlc/runs/mcp-work/<job-id>/` (`job.json`, `output.log`, `exit.json`).
3. **Protocol negotiation.** From a terminal, run `sop acp` with an ACP v1 client (or the recorded transcript) and confirm `initialize` (protocolVersion `v1`, `agentInfo`) and `session/new` (sessionId) succeed.
4. **Cancellation.** Start one bounded `implement`-capability prompt in the disposable project, cancel with `session/cancel` before it completes, and record the outcome `cancelled`, the job ID, and that `.agent-sdlc/runs/mcp-work/<job-id>/` shows a distinct cancellation record.
5. **Restart discovery.** Restart `sop acp` (or Zed), confirm the previously recorded job is discoverable from the persisted records, and confirm nothing was resubmitted automatically.

Evidence path convention: `docs/reports/evidence/ACP-007-ZED-<platform>/<scenario>.md` (or terminal transcripts), referenced from the results table after execution.

## Results (fill at execution time)

| # | Scenario | Platform | Zed version | OS version | SOP revision | Job ID | Outcome | Evidence path |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | Agent appears | — | — | — | — | — | NOT EXECUTED | — |
| 2 | Bounded request completes | — | — | — | — | — | NOT EXECUTED | — |
| 3 | Protocol negotiation | — | — | — | — | — | NOT EXECUTED | — |
| 4 | Cancellation | — | — | — | — | — | NOT EXECUTED | — |
| 5 | Restart discovery | — | — | — | — | — | NOT EXECUTED | — |

When a run executes, update each row with the recorded values, set the environment status above, and update `docs/guides/ACP-ZED.md` (verified Zed version and `agent_servers` schema in sections 2 and 9) with any difference found. Record every observed difference from the guide in the guide itself, not only here.

## Known limitations carried by this report

- macOS acceptance is OUTSTANDING; no macOS result exists, and none may be claimed.
- The Linux row status is NOT EXECUTED until the recipe runs on a Linux host with Zed installed.

## Governance link

The PR description draft and the governance checklist live in [ACP-007-PR-CHECKLIST.md](ACP-007-PR-CHECKLIST.md); acceptance executes within the governed PR-review workflow.

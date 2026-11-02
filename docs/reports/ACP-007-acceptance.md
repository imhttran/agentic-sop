# ACP-007 — Real-Zed acceptance record

Task: ACP-007 of [PLAN-SOP-ACP-ZED.md](../plans/PLAN-SOP-ACP-ZED.md). This report records the bounded real-Zed acceptance status. It claims only what was executed. A scenario never claims a result that was not run.

## Status summary

| Environment | Status                                      | Reason                                                                                                                                                                                                                                                                                                                                  |
| ----------- | ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| macOS       | **OUTSTANDING (Zed-driven prompt pending)** | Scenario 1 is verified in Zed 1.26.2: SOP appears under External Agents in the agent picker, and the `agent_servers` schema was accepted unchanged. Scenarios 2–5 have recorded macOS results (2, 4 and 5 via an ACP v1 stdio client; 3 via a terminal transcript). The remaining gap is one bounded request submitted from Zed itself. |
| Linux       | **NOT EXECUTED in the ACP-007 invocation**  | The task environment has no Zed application and could not probe an installed Zed or fetch its documentation. The bounded recipe below defines the run; execute it and replace this section's status with recorded results.                                                                                                              |

All five scenarios have recorded macOS results. Scenario 1 was observed in Zed 1.26.2 (SOP listed under External Agents), verifying the `agent_servers` schema; scenarios 2, 4 and 5 were executed with an ACP v1 stdio client and scenario 3 with a terminal transcript. A bounded request submitted from Zed itself has not been run yet, and no Zed-driven run is claimed.

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

| #   | Scenario                  | Platform           | Zed version           | OS version                                 | SOP revision | Job ID                 | Outcome                                                                                  | Evidence path                                                         |
| --- | ------------------------- | ------------------ | --------------------- | ------------------------------------------ | ------------ | ---------------------- | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| 1   | Agent appears             | macOS              | 1.26.2                | macOS 26.6.2 (25G83) / Darwin 25.6.0 arm64 | `ba5fbd96`   | —                      | PASS — SOP listed under External Agents in the agent picker; schema accepted unchanged   | `docs/reports/evidence/ACP-007-ZED-darwin/01-agent-appears.md`        |
| 2   | Bounded request completes | macOS (ACP client) | — (not Zed)           | macOS 26.6.2 (25G83) / Darwin 25.6.0 arm64 | `ba5fbd96`   | `work-20261102-211430` | PASS — `started`, persisted `exit_code` 0                                                | `docs/reports/evidence/ACP-007-ZED-darwin/02-bounded-request.md`      |
| 3   | Protocol negotiation      | macOS              | 1.26.2 (not involved) | macOS 26.6.2 (25G83) / Darwin 25.6.0 arm64 | `ba5fbd96`   | — (session only)       | PASS — `initialize` negotiated v1 with `agentInfo`, `session/new` returned a `sessionId` | `docs/reports/evidence/ACP-007-ZED-darwin/03-protocol-negotiation.md` |
| 4   | Cancellation              | macOS (ACP client) | — (not Zed)           | macOS 26.6.2 (25G83) / Darwin 25.6.0 arm64 | `ba5fbd96`   | `work-20261102-211503` | PASS — `cancelled`, distinct `cancelled.json`, `exit_code` −1                            | `docs/reports/evidence/ACP-007-ZED-darwin/04-cancellation.md`         |
| 5   | Restart discovery         | macOS (ACP client) | — (not Zed)           | macOS 26.6.2 (25G83) / Darwin 25.6.0 arm64 | `ba5fbd96`   | `work-20261102-211503` | PASS — restart started no work; resubmission `reconciled` to the same job id             | `docs/reports/evidence/ACP-007-ZED-darwin/05-restart-discovery.md`    |

When a run executes, update each row with the recorded values, set the environment status above, and update `docs/guides/ACP-ZED.md` (verified Zed version and `agent_servers` schema in sections 2 and 9) with any difference found. Record every observed difference from the guide in the guide itself, not only here.

The macOS run header for the recorded scenario (date, host, OS version, Zed version, SOP version, revision) is in `docs/reports/evidence/ACP-007-ZED-darwin/README.md`. The guide's `agent_servers` example stays labelled UNVERIFIED until scenario 1 observes SOP in the Zed agent picker.

## Known limitations carried by this report

- macOS acceptance is OUTSTANDING on the Zed side: scenarios 2–5 have recorded macOS results (2, 4 and 5 via an ACP v1 stdio client), but scenario 1 has not been executed in Zed, and no Zed-side acceptance may be claimed.
- The Linux row status is NOT EXECUTED until the recipe runs on a Linux host with Zed installed.

## Governance link

The PR description draft and the governance checklist live in [ACP-007-PR-CHECKLIST.md](ACP-007-PR-CHECKLIST.md); acceptance executes within the governed PR-review workflow.

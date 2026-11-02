# ACP-007 — Release PR description and governance checklist

Task: ACP-007 of [PLAN-SOP-ACP-ZED.md](../../plans/PLAN-SOP-ACP-ZED.md). This file holds the description for the release PR described in [ACP-007-discovery.md](ACP-007-discovery.md) section 5. The PR is opened through the established PR workflow; this stage does not commit, push, or merge.

## Governance steps

1. All changes present in the working tree; `git status` shows only this task's deliverables and preserved user-owned changes.
2. Checks run once for the final change: `go test ./internal/acp/...`, `go build ./...`, `go test ./...`, `go vet ./...`; results in [ACP-007-validation.md](ACP-007-validation.md).
3. Open the PR with the description below; link the guide, validation report, acceptance report, and this checklist.
4. Reviewer approval per the repository's review gates; the PR review stage owns remaining verification.
5. Merge and release per existing governance; no release action by the implementation stage.

## PR description draft

**Scope.** ACP-007: Zed integration for SOP over the Agent Client Protocol (ACP), completing `docs/plans/PLAN-SOP-ACP-ZED.md`. Adds documentation only; no production behavior change. Declared repository paths touched (`docs/plans/PLAN-SOP-ACP-ZED.md`):

- `docs/guides/ACP-ZED.md` — new Zed setup and operations guide.
- `internal/acp/` — adapter package, delivered in earlier ACP stages, unchanged by ACP-007 and documented here (`README`-level references in the guide and the discovery note).
- `docs/reports/ACP-007-discovery.md`, `ACP-007-validation.md`, `ACP-007-acceptance.md`, `ACP-007-PR-CHECKLIST.md` — evidence and governance.

**Compatibility.** Existing CLI and MCP behavior is untouched: ACP-007 changes no `internal/cli` or `internal/mcp` code. Validation results carry over from the reports: focused adapter tests (`go test ./internal/acp/...`), the full suite (`go test ./...`), build (`go build ./...`), and vet (`go vet ./...`) at the recorded revision.

**Lifecycle decisions.**

- Cancellation: `session/cancel` cancels exactly the started job's child, within the stop bound, and records cancellation distinctly from failure and completion.
- Blockers: SOP gates surface as `session/update` payload fields; approval stays in the SOP approval interface; an unchanged blocker stops without consuming retries.
- Recovery: on disconnect the adapter cancels its foreground work, reconciles against persisted records, never restarts work automatically, and never fabricates completion; restart discovers existing jobs without resubmission and without session-history restoration.

**Validation evidence.** [docs/reports/ACP-007-validation.md](ACP-007-validation.md) (commands, results, revision). Acceptance evidence: [docs/reports/ACP-007-acceptance.md](ACP-007-acceptance.md).

**Known limitations.**

- Supported Zed version UNKNOWN; the `agent_servers` example `docs/guides/ACP-ZED.md` section 2 is unverified until the real-Zed acceptance recipe runs.
- macOS acceptance outstanding; no macOS result claimed.
- Linux acceptance not executed in the task invocation; recipe and evidence paths in the acceptance report.
- No `session/load`; text prompts only; GUI-launched Zed provider environment differences documented but unverified.

**Links.** Guide: `docs/guides/ACP-ZED.md`. Validation: `docs/reports/ACP-007-validation.md`. Acceptance: `docs/reports/ACP-007-acceptance.md`. Checklist: `docs/reports/ACP-007-PR-CHECKLIST.md`.

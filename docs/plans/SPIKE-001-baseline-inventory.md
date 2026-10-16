# SPIKE-001 — Baseline Inventory and Disposable Test Environment

Discovery-and-environment stage. No compatibility between any candidate client
and SOP's stdio transport is assumed or asserted.

## Baseline commit and working tree

- Baseline commit: `d83c6660206bf2040475b60dd462ab76c941e59b` (`git rev-parse HEAD`)
- Current branch: `main` (tracking `origin/main`)
- Working-tree state (`git status --short --branch`):

  ```
  ## main...origin/main
  ?? .run/
  ?? docs/plans/SOP-Planner-Connection-Spike.md
  ```

- Pre-existing, user-owned, preserved untouched:
  - `.run/` (untracked)
  - `docs/plans/SOP-Planner-Connection-Spike.md` (untracked)
  - `sop-harden-001/` (linked worktree; active migration work, not interrupted)

## SOP version and effective configuration

Effective configuration from `.agent-sdlc/config.yaml`:

- `workflow.mode: local`
- `human.approval_before_commit: true`
- `validation.build: [go build ./...]`, `validation.test: [go test ./...]`, `validation.lint: [go vet ./...]`
- `review.engine: self`, `review.delegation: false`
- `quality.require_tests: true`, `quality.max_fix_cycles: 3`, `quality.fail_on: [critical, high]`
- `autonomy.level: balanced`
- `project.integration_branch: main`

## MCP registration and transport

- Transport: **stdio** (`sop mcp`; `internal/cli/mcp.go` `runMCP` reads stdin, writes stdout, diagnostics to stderr).
- Full tool list exposed by `sop mcp` (from `internal/cli/mcp.go` `mcpTools`, the sole registration point; `internal/mcp/mcp.go` is only the generic stdio server framework and registers no tools):
  - `sop_status` — list persisted tasks
  - `sop_validate` — run configured build/test/lint
  - `sop_review` — review working-tree changes
  - `sop_approval` — report approval request
  - `sop_approve` — record approval of an active human approval gate
  - `sop_decline` — record decline of an active human approval gate
- The **approval tools** (`sop_approval`, `sop_approve`, `sop_decline`) are among the registered tools; they delegate to `internal/approval`.

## Ownership diagram

```
        +---------------------------+
        |        Human operator     |
        |  (human-approval boundary)|
        +-------------+-------------+
                      | approve / decline
        +-------------v-------------+
        |        SOP CLI (cmd/sop)  |
        |  sop status|validate|review|
        |  approval|approve|decline |
        +------+--------------+-----+
               |              |
     +---------v----+   +-----v-----------+
     | stdio MCP    |   | application svcs|
     | server       |   | validate/review |
     | internal/mcp |   | approval/quality|
     +------+-------+   +-----+-----------+
            |                 |
            +--------+--------+
                     |
           +---------v----------+
           |  store / state     |
           |  internal/store    |
           +---------+----------+
                     |
           +---------v----------+
           |  run stages /      |
           |  recovery / resume |
           |  internal/run,     |
           |  internal/recovery,|
           |  internal/resume,  |
           |  internal/continuation
           +--------------------+
```

## Run stages and recovery behavior

- `internal/run` — run stages, attempts, approvals, changes, evidence
  (`run.go`, `attempt.go`, `approval.go`, `changes.go`, `jev*.go`, etc.).
- `internal/recovery` — recovery policy/decision/evidence (`policy.go`, `decision.go`, `evidence.go`).
- `internal/resume` — resume behavior for interrupted runs.
- `internal/continuation` — continuation logic.

## Candidate client/version and supported transport

The intended planner client is **operator-designated and external** (a ChatGPT
conversation, a Claude conversation, Claude Code, or a Zed-hosted agent) and is
never silently substituted. No candidate client/version is confirmed for this
baseline, so no compatibility with SOP's stdio transport is asserted.

- Candidate clients (unconfirmed): Claude Code (CLI), Zed-hosted agent, ChatGPT conversation, Claude conversation.
- SOP transport: **stdio** only.
- Compatibility: **not assumed** — to be established later in SPIKE-002.

## Global-policy propagation

Global-policy propagation is confirmed **once** for participating agents.
Evidence: the binding policy surfaces are read from the repository at the
baseline commit — `.agent-sdlc/config.yaml` (`workflow.mode: local`,
`human.approval_before_commit: true`) and `internal/cli/mcp.go` (approval tools
`approvalRead`/`approvalApprove`/`approvalDecline`). Later stages rely on this
single recorded confirmation instead of repeating it.

## Disposable environment and log capture (SPIKE-001B)

See `docs/plans/SPIKE-001-disposable-environment.md` for the reproduction record.

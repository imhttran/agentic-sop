# T037 --- MCP Server

> Implements plan `PLAN-JEV.md` T038 (MCP server) and T039 (MCP security).

## Status

DONE

## Objective

Expose the harness over the Model Context Protocol so an MCP client can drive it,
without reimplementing the workflow:

```text
MCP client ──stdio JSON-RPC──> sop mcp ──> same services as the CLI
  initialize / tools/list / tools/call
```

## Dependencies

- T025 (configuration), T030 (validation), T031 (review), Stage 3 (state store)

## Scope

- `internal/mcp/mcp.go`: a minimal MCP server over newline-delimited JSON-RPC 2.0
  (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`), with
  `Tool`, `TextTool`, and `JoinLines`.
- `internal/mcp/mcp_test.go`: protocol tests over in-memory pipes.
- `internal/cli/mcp.go`: the `sop mcp` command and the tool set (`sop_status`,
  `sop_validate`, `sop_review`).
- `internal/cli/cli.go`: dispatch and help.

## Rules

- Tools call the same application services as the CLI; the workflow is not
  duplicated (T038).
- Security by construction (T039): only registered tools exist; each is scoped to
  the one project directory; there is no arbitrary filesystem or shell access
  beyond a tool's explicit backing (validation commands are trusted project
  configuration). Protocol errors and tool errors are separated — a tool failure
  is content with `isError`, an unknown tool/method is a JSON-RPC error.
- Notifications produce no response; unsupported methods return `-32601`.

## Tests

`initialize` advertises the server and protocol version and `tools/list` lists
the tools; a tool call returns text content and a failing tool returns `isError`
content; an unknown method is `-32601` and an unknown tool is `-32602`; a
`TextTool` ignores its arguments; notifications are not answered.

## Acceptance Criteria

- [x] `sop mcp` serves MCP over stdio with `initialize`/`tools/list`/`tools/call`.
- [x] Tools delegate to existing services; no workflow is reimplemented.
- [x] Only registered, project-scoped tools exist; no arbitrary access.
- [x] Protocol errors and tool errors are reported distinctly.
- [x] `make check` passes.

## Git

Branch: `task/T037-mcp-server`
Commit: `task(T037): add MCP server`
PR: `[Task T037] Add MCP server`

## Out of Scope

Streaming/resumable transports; resources and prompts; authentication (stdio is
local by construction).

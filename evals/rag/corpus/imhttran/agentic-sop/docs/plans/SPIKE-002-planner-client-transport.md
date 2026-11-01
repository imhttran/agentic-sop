# SPIKE-002 — Planner-client and transport discovery

Discovery-and-connection stage. Deliverable is a naming/selection record, a
connect-and-exercise evidence report, and (if supported) a reconnect confirmation.
This record follows the plan `docs/plans/SOP-Planner-Connection-Spike.md` and the
SPIKE-001 baselines (`SPIKE-001-baseline-inventory.md`,
`SPIKE-001-disposable-environment.md`).

> **Operator designation superseded (2026-10-14).** The intended planner client is
> now recorded as **this ChatGPT conversation**. See
> [`SPIKE-002-connection-discovery.md`](SPIKE-002-connection-discovery.md). The
> connect-and-exercise findings below remain valid; only the client-naming gap is
> closed.

## Outcome

**PARTIAL / selection gap — no connection was performed.**

The operator-intended external planner client is **not named** in this
environment. SPIKE-001A recorded it as MISSING (operator-supplied), and no
operator confirmation naming the client was available during this attempt. Per
the task ("do not silently substitute a Zed agent for the intended external
client") and per the plan ("A Zed agent calling SOP proves Zed connectivity, not
connectivity from the intended external planner"), no client was substituted and
no compatibility was assumed.

Because S1 could not establish an intended client, S2 and S3 were not run: no MCP
protocol initialization, no tool listing, and no status-tool invocation were
performed by an intended client. Transport work stops here and the exact
limitation is recorded below.

## S1 — Identify intended planner client and verify access/transport

### Intended client name

- Intended external planner client: **UNNAMED (operator-designated; not supplied
  in this environment)**.
- No substitution was made: no Zed-hosted agent, no in-repository agent, and no
  other client is presented as the SPIKE-002 planner client.
- No compatibility was assumed between any candidate client and SOP's stdio
  transport. SPIKE-001A explicitly left compatibility "to be established later in
  SPIKE-002"; that establishment is what this stage could not complete without
  the named client.

### Access verification result

- Not performed. Access to the intended client cannot be verified while the
  client is unnamed; verifying access to a substitute would violate the task.

### Transport compatibility finding

- SOP offers a single transport: **stdio** (`sop mcp`; `internal/cli/mcp.go`
  `runMCP` reads stdin / writes stdout / diagnostics to stderr).
- The intended client's supported transport is **UNKNOWN**, because the client is
  unnamed.
- Exact limitation (recorded in place of a connection result):

  > **Unsupported/undetermined transport: the operator-intended external planner
  > client was not named in this environment, so its supported transport could
  > not be checked against SOP's stdio-only MCP surface. No connection, protocol
  > initialization, tool listing, or status-tool invocation was attempted. No
  > substitute client was used and no compatibility was assumed.**

### Inspection notes (grounding the finding)

- `internal/cli/mcp.go` — `runMCP` is the sole server entry point; it wires
  `mcp.NewServer(Version, mcpTools(dir, d)...)` and serves stdio.
- `internal/cli/mcp.go` — `mcpTools` is the sole tool-registration point:
  - `sop_status` — list persisted tasks
  - `sop_validate` — run configured build/test/lint
  - `sop_review` — review working-tree changes
  - `sop_approval` — report the approval request
  - `sop_approve` — record approval of an active human gate
  - `sop_decline` — record decline of an active human gate
- `internal/mcp/mcp.go` — generic newline-delimited JSON-RPC 2.0 stdio server
  (initialize, `notifications/initialized`, `tools/list`, `tools/call`); protocol
  version `2024-11-05`; server name `sop`. Registers no tools itself.
- `.agent-sdlc/config.yaml` (per SPIKE-001A) — `workflow.mode: local`,
  `human.approval_before_commit: true`; the CLI/MCP client-selection surfaces are
  the `cmd/sop` subcommands, and the only MCP client surface is a client that
  speaks JSON-RPC 2.0 over stdio.

### S1 acceptance

- Intended client named in the artifact: **recorded as unnamed/MISSING**; the
  artifact states no substitution and no assumed compatibility. OK
- Access and supported transport recorded: **recorded as not verifiable / UNKNOWN**
  with the reason. OK
- Unsupported-transport limitation recorded in place of a connection result. OK
- No client other than the operator-intended one presented as the SPIKE-002
  planner client. OK

## S2 — Connect and exercise protocol

**Not run.** S1 did not establish a supported transport and access for a named
intended client, so there was no client to connect. No MCP protocol
initialization, tool listing, or `sop_status` invocation against the disposable
repository was performed by an intended client.

The disposable repository from SPIKE-001B remains available at
`<spike-root>/disposable-repo` (branch `spike-001-disposable`, baseline
`d83c6660206bf2040475b60dd462ab76c941e59b`) for the future exercise. A
substitute or fake client deliberately cannot satisfy this gate
("A manual answer or fake planner cannot satisfy the real-client connection
gate").

## S3 — Repeat after reconnect and consolidate

**Not run.** There was no first connection to repeat or reconnect. No
post-reconnect status result exists.

## Final consolidation

- Intended client: **unnamed / operator-designated MISSING**. No substitution; no
  assumed compatibility.
- Access: **not verified** (client unnamed).
- Transport: SOP is **stdio-only**; intended client's transport **UNKNOWN**.
- Status-tool retrievals: **zero** (none performed).
- Reconnect: **none** (no connection existed).
- Resolution outcome: **PARTIAL / NO-GO for the selected connection**, per the
  plan's disposition ("An unsupported connection is a valid spike finding and
  must not be recorded as successful integration").
- Transport work **stopped** at the recorded limitation: no connection was
  attempted and no bridge, substitution, or invented authentication was created.

## Remaining gap / next integration task

The operator must name the intended external planner client (a ChatGPT
conversation, a Claude conversation, Claude Code, or a Zed-hosted agent). Once
named, re-run S1–S3 with that client against the existing `sop mcp` stdio server
and the SPIKE-001B disposable repository. If that client does not support stdio,
record the exact transport gap before proposing a narrowly scoped bridge. Do not
publish a local service to the internet or invent authentication as a shortcut.

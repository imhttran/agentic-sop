# SPIKE-002 — Connection discovery: ChatGPT as the intended planner

Date: 2026-10-14 • Status: connection discovery only; no connection performed.
Scope: satisfies the operator's "connection discovery only" instruction. SPIKE-003
remains paused at its pending human gate; this document neither resumes nor
re-implements anything.

## 1. Operator designation (recorded)

- **Intended planner client: this ChatGPT conversation** (operator-designated).
- **Not** Claude Code and **not** a Zed-hosted agent; no substitution is made.
- This supersedes the earlier "unnamed / operator-designated MISSING" record in
  `SPIKE-002-planner-client-transport.md`, which reflected the state before the
  operator named the client.

## 2. Verified ChatGPT connection method (official documentation)

Fetched 2026-10-14:

- `developers.openai.com/api/docs/guides/custom-mcp-server.md` — _Add custom MCP server_
- `developers.openai.com/api/docs/guides/secure-mcp-tunnels.md` — _Secure MCP Tunnel_
- `developers.openai.com/plugins/deploy/connect-chatgpt.md` — _Connect and test your plugin_
- `developers.openai.com/plugins/build/mcp-server.md` and `/concepts/mcp-server.md`
- `modelcontextprotocol.io/docs/concepts/transports` — MCP transports
- `learn.chatgpt.com/docs/build-plugins.md`, `learn.chatgpt.com/docs/plugins.md`
- `platform.openai.com/docs/guides/tools-remote-mcp` — Responses API MCP tool (API surface)

Findings:

- ChatGPT reaches an external tool surface as a **plugin whose App is a custom MCP
  server**, added through **ChatGPT Plugins → "Add custom MCP server"**
  (`https://chatgpt.com/plugins`, "plus" → _Add custom MCP server_).
- Two connection methods:
  - **Server URL** — a **public HTTPS** MCP endpoint, streaming HTTP/SSE, typically at
    `/mcp`.
  - **Tunnel** — **Secure MCP Tunnel**, for private/local servers, no public exposure.
- **Supported MCP protocols on the ChatGPT side: SSE and streaming HTTP.** Chat is
  cloud-hosted and cannot call `localhost` directly.
- **Authentication options: OAuth, No authentication, or OAuth-or-no-authentication.**
- Actions are gated by workspace permissions/security restrictions (including
  Lockdown); write tools require confirmation by default; the `readOnlyHint`
  annotation is respected; the MCP `instructions` field is supported.
- The OpenAI **API** MCP tool (Responses API `server_url` / `tunnel_id`) is a
  **different surface** from this ChatGPT conversation. **API-model access is not
  proof that this conversation is connected.** Connectivity is proven only by a
  successful tool call made by this ChatGPT conversation.

## 3. Comparison with SOP's existing MCP server

| Aspect                | SOP (`sop mcp`)                                                                                                  | ChatGPT "Add custom MCP server"                 |
| --------------------- | ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| Transport             | **stdio only** (newline-delimited JSON-RPC over stdin/stdout)                                                    | **SSE / streaming HTTP**, or **Tunnel**         |
| Reachability          | local subprocess                                                                                                 | cloud (cannot reach localhost directly)         |
| Protocol version      | `2024-11-05` (`internal/mcp/mcp.go`)                                                                             | current MCP revision                            |
| Auth                  | none                                                                                                             | OAuth / none / mixed                            |
| Tools                 | `sop_status`, `sop_validate`, `sop_review`, `sop_approval`, `sop_approve`, `sop_decline` (`internal/cli/mcp.go`) | server-defined                                  |
| Tool annotations      | none advertised (no `readOnlyHint`)                                                                              | `readOnlyHint` respected, else treated as write |
| Server `instructions` | not advertised                                                                                                   | supported                                       |

**Conclusion:** a direct ChatGPT↔SOP stdio connection is impossible (ChatGPT is
cloud-hosted; SOP speaks stdio only). The gap is **transport**, not tool shape. The
Secure MCP Tunnel bridges exactly this gap and can reach a **stdio** server, so SOP
needs **no transport change** for the private path.

## 4. Minimum transport / authentication work

**Recommended: Secure MCP Tunnel (no public exposure, no SOP transport change).**

- `tunnel-client` (from `openai/tunnel-client`) runs on this machine, is configured
  with an **stdio command** (`sop mcp`) — or an HTTP MCP URL — and opens an
  **outbound-only** HTTPS path to OpenAI. No inbound ports, no public endpoint.
- ChatGPT-side authentication: **No authentication** is sufficient, because the
  tunnel is the boundary and only the associated ChatGPT workspace can use it. OAuth
  is optional and would require an authorization server (materially more work).
- Account prerequisites (OpenAI Platform + ChatGPT): a tunnel (`tunnel_id`) created
  in **Platform tunnel settings**, a **runtime API key** for `tunnel-client`, the
  tunnel **associated with the ChatGPT workspace**, and the **Tunnels
  Read+Manage/Use** permission; plus the workspace permission to add/use custom MCP
  servers. (For personal accounts, the personal Platform organization applies.)
- OpenAI documents Secure MCP Tunnel as supporting **private connections and custom
  MCP server testing**; it does **not** support public plugin submission — irrelevant
  for this private spike.
- Caveats to verify during connection (SPIKE-002 execution), not assumed:
  - MCP **protocol-version/handshake** compatibility between SOP's `2024-11-05`
    server and ChatGPT's current client (`initialize`, `notifications/initialized`,
    `tools/list`, `tools/call`).
  - Tool **annotations** (`readOnlyHint`) and the `instructions` field are absent
    from SOP today; without `readOnlyHint`, `sop_status` may be treated as a write
    action requiring confirmation.
- **Deferred alternative (requires public exposure — not now):** a bridge exposing
  SOP's stdio as streamable HTTP at `/mcp` on a public HTTPS endpoint, plus auth.

## 5. Planner-only tool surface (excludes human approval)

Enforce **server-side**, not via client-side ChatGPT tool toggles.

| Tool                             | Planner surface                     | Reason                                 |
| -------------------------------- | ----------------------------------- | -------------------------------------- |
| `sop_status`                     | **Allowed**                         | read-only; the first success criterion |
| `planner_question_get` (future)  | Allowed                             | read the pending planner question      |
| `planner_answer_submit` (future) | Allowed                             | submit a bounded planner decision      |
| `sop_approve`                    | **Excluded**                        | human approval action                  |
| `sop_decline`                    | **Excluded**                        | human approval action                  |
| `sop_approval`                   | Excluded (or read-only, no resolve) | human gate surface                     |
| `sop_validate` / `sop_review`    | Excluded                            | state-affecting; not planner scope     |

Recommended implementation shape (later, through SOP, not now): a planner-scoped
surface — e.g. `sop mcp --planner` or a distinct tool set — that never registers
`sop_approve`/`sop_decline`, and that marks read tools with `readOnlyHint`. Relying
on ChatGPT's per-app tool toggles or `allowed_tools` is not sufficient, because it is
client-side and does not remove the approval action from the server.

## 6. Concrete setup steps

Prepared (not executed — "do not change account settings yet", "do not expose a
local endpoint publicly"):

1. **Platform tunnel:** create a tunnel in Platform tunnel settings; note the
   `tunnel_id`; create a runtime API key; associate the tunnel with the target
   ChatGPT workspace.
2. **Local client:** install `tunnel-client`; configure a local stdio profile with
   `--mcp-command` pointing at SOP's MCP entry point (`sop mcp`, or an absolute
   binary path with the repo as working directory); validate with `tunnel-client
doctor --profile <name> --explain`; run `tunnel-client run --profile <name>` and
   keep it healthy.
3. **ChatGPT:** ChatGPT Plugins → "Add custom MCP server" → name/description →
   Connection = **Tunnel** → select the tunnel (or paste `tunnel_id`) →
   authentication = **No authentication** → accept the risk warning → **Create as a
   plugin** → **install** it.
4. **Conversation:** open a new conversation, type `@` and select the plugin, and
   instruct it explicitly, e.g. `Use the "SOP" plugin's sop_status tool to read this
repository's SOP status. Do not use built-in browsing or other tools.`
5. **Prove the first success criterion:** the ChatGPT conversation returns the
   correct SOP status for the disposable repository.

## 7. What the operator must do in ChatGPT / account

- Confirm the account/workspace can add and use **custom MCP servers** (ChatGPT on
  the web; workspace permission; not blocked by Lockdown). UI may label this
  **Plugins** or **Apps**.
- Grant/verify **Tunnels** permission (Read+Manage to create; Read+Use to run and
  select) and create the tunnel + runtime API key in Platform settings.
- Perform steps 3–4 above in ChatGPT (add custom MCP server as a Tunnel, install,
  `@`-mention, ask for status).

## 8. Boundaries honored

- No local endpoint exposed publicly; no tunnel created; no account settings changed.
- API-model access is not treated as proof the conversation is connected.
- Claude Code and a Zed agent are not substituted for ChatGPT.
- SPIKE-003 remains paused; its work and pending human gate are untouched.
- The active plan source (`docs/plans/SOP-Planner-Connection-Spike.md`) was
  deliberately **not** edited, to avoid changing the active plan fingerprint (which
  would raise a plan-reconciliation gate against the paused SPIKE-003). Binding
  "ChatGPT" into the plan source is a separate, operator-approved step.

## 9. First success criterion / deferred

### Live connectivity result (2026-10-15) — first criterion MET

This ChatGPT conversation called the **"SOP Status Spike"** plugin's **`sop_status`**
tool through the Secure MCP Tunnel and received **"no tasks"** from the disposable
project (`…/.run/disposable-project`). The status path is proven end to end:

```
this ChatGPT conversation
  → Secure MCP Tunnel (tunnel-client v0.0.16, process-mode runtime "sop-status-spike")
  → sop mcp --status-only   (stdio wrapper pinning the disposable project + absolute sop)
  → sop_status → "no tasks"
```

Readiness recorded at the time of the call: `healthz` live, `readyz` ready,
`runtime_state` ready, control-plane poll `ok`. Credentials are referenced only
(`env:OPENAI_API_KEY`, tunnel id) and were never printed.

**Proven vs. unproven:**

- **Proven:** _status_ connectivity — the intended client (this ChatGPT
  conversation) lists SOP tools and reads the correct repository status through the
  tunnel. This satisfies SPIKE-002's real-client connection gate.
- **Unproven:** the **question → answer → resume** loop. No `WAITING_FOR_PLANNER` run
  stage exists yet (`internal/run/run.go` defines CREATED/PLANNING/IMPLEMENTING/
  VALIDATING/REVIEWING/FIXING/WAITING_FOR_HUMAN/PASSED/FAILED only); no planner
  question has been raised or answered through ChatGPT; no answer has been validated
  or applied; and no executor has resumed from a planner answer.

  **Correction (2026-10-15):** the **status-only handshake is proven** end to end
  (above). The SPIKE-003 question/answer **CLI and planner-scoped MCP surface now
  exists and passes local tests** (`sop question`, `sop answer`, and
  `sop mcp --planner` with `sop_question_list`/`sop_question_get`/`sop_answer_submit`),
  and the **live ChatGPT client has since exercised that planner surface** for
  discovery, retrieval, and answer recording (§10.2). What remains unproven is the
  **application and resume** half: no answer has been applied and no executor has
  resumed. The `WAITING_FOR_PLANNER` stage and executor resume are SPIKE-004.
  SPIKE-003 stays paused at its human gate.

### Deferred

- Question/answer/resume: SPIKE-003's CLI/MCP surface, then SPIKE-004–006.
- **Do not record the full planner loop as working until a real question → ChatGPT
  answer → validated application → executor resume is demonstrated.**

## 10. Planner demonstration records (2026-10-15)

Two records, deliberately distinguished: the first was driven by the **local test
harness** (the operator's agent); the second is the **real ChatGPT client**, evidenced
by the user's ChatGPT tool-call transcript.

### 10.1 Local test harness (not the ChatGPT client)

`sop_question_list` / `sop_question_get` / `sop_answer_submit` were exercised against
the disposable `planner-demo` project through the same MCP command the tunnel serves.

- **`q-docformat` was answered by the local test harness** — body `Markdown`,
  `idempotency_key=chatgpt-docformat-001`, outcome `ACCEPTED`, state `ANSWERED`.
  Its recorded `answered_by="chatgpt"` is a **caller-supplied label, not client
  provenance**: no ChatGPT client call was made. Its audit history (answer `a-1`,
  audit outcome `o-2 ACCEPTED`) is **preserved unchanged** — the records were not
  altered to correct the label.

### 10.2 Live ChatGPT client — planner retrieval + answer recording (PASS)

**Evidence source: the user's actual ChatGPT tool-call transcript** (distinct from the
local harness record above).

- **Question:** `q-docformat-chatgpt` — initially **PENDING, revision 1**, options
  `Markdown` / `plain text`.
- **Answer:** `Markdown` → outcome **ACCEPTED**, answer ID **`a-3`**.
- **Idempotency key:** `chatgpt-live-docformat-001`.
- **Read-back:** the question read back as **`ANSWERED`**.
- **Task:** `T001` remained **`PLANNED`**.
- **Nothing applied, approved, or resumed** — the answer was recorded only.

This proves the **live ChatGPT client** can discover a question, retrieve it, and
durably record an answer through the tunnel + planner-scoped MCP surface.

### 10.3 Still unproven

Application (`ANSWERED -> APPLIED`) and executor continuation from the saved stage are
not implemented (SPIKE-004), so the **question → answer → apply → resume** loop is
**not** proven and must not be recorded as working until a real client answer is
applied and the executor resumes.

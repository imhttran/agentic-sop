# ACP-001 — Implementation note: SOP boundaries and implementation decisions

Task: ACP-001 of [PLAN-SOP-ACP-ZED.md](PLAN-SOP-ACP-ZED.md).
Scope guard: this stage creates and changes only this note. No other file is created or modified. All pre-existing working-tree changes are user-owned and preserved.

## 1. Repository inspection (S1)

Instructions read: `README.md`, `docs/architecture/SOP-BOUNDARY.md`, and the plan `docs/plans/PLAN-SOP-ACP-ZED.md` (untracked; read and preserved untouched). SOP-BOUNDARY is the ownership authority: SOP owns orchestration and lifecycle state, persistence (`.agent-sdlc/state.db`, SQLite), validation, review, retry budgets, git state, and human approvals; agents do bounded work; executable state lives only in structured data.

Pinned source revision: `82ad60bc9316e58c5969db9631191403582f6ba6`, branch `main`. Command used: `git rev-parse HEAD`. Observations below are pinned to this revision.

Worktrees: `.worktrees/` exists at the repository root (observed in the root listing). Per-worktree path and branch inventory: unavailable in this stage — the bounded discovery window closed before a read-only `git worktree list` could run; SOP records this inventory during independent validation (gap G-WT).

Active plans: `docs/plans/PLAN-SOP-ACP-ZED.md` (untracked; preserved). Plans referenced by `README.md` (not opened in this stage): `docs/plans/README.md`, `docs/plans/BACKLOG.md`, `docs/plans/PLAN-Executore-Integration.md`.

User-owned working-tree changes, recorded and preserved:

- Modified: `internal/cli/task.go`, `internal/cli/validate.go`, `internal/cli/validate_reuse.go`, `internal/verifcache/store.go`.
- Untracked: `docs/plans/PLAN-SOP-ACP-ZED.md`, `internal/cli/task_validation.go`, `internal/git/worktree_digest.go`, `internal/run/validation_evidence.go`, `internal/verifcache/lock.go`.

Each change is pre-existing and outside ACP-001 scope. This stage reverts, discards, and fixes none of them. The S1 rule (no file outside this note modified) holds by construction; SOP re-runs `git status` independently to confirm.

## 2. Boundary inventory (S2)

Legend — OBSERVED: evidence read at the pinned revision. UNVERIFIED: the seam exists by documentation or package presence, but the source contract was not read in this stage.

| Subsystem | Evidence at pinned revision | Status |
| --- | --- | --- |
| Prompt submission | README documents `sop prompt --capability <capability> <request>` through the same governed machinery; spec `docs/specs/PROMPT-EXECUTION.md`. `internal/prompt` contains `compile.go`, `compile_test.go`, `dedup_test.go`, `rag_knowledge_test.go` (compilation and dedup). A search for `func Prompt`, `func SubmitPrompt`, `func RunPrompt`, `PromptSubmission` across `internal/` returned no matches. | Reusable in-process submission entry point UNVERIFIED — gap G-1, owner SOP core, resolved by ACP-003 |
| Background work and results | README documents durable lifecycle events with one configured channel (`none`, `file`, or `webhook`), at-least-once delivery, delivery failure never changes a task outcome, no channel enabled by default; `sop result publish` records evidence. Packages `internal/run`, `internal/scheduler`, `internal/workitem`, `internal/outcomefile`, `internal/activity` exist. | Event and job source shapes UNVERIFIED — gap G-2, owner SOP core, consumed by ACP-004 |
| Project locking | Package `internal/projectlock` exists. | Lock acquisition API and busy-result shape UNVERIFIED — gap G-3, owner SOP core, reused by ACP-003 |
| Progress and lifecycle events | Same README guarantees; guide `docs/guides/NOTIFICATIONS.md`. Package `internal/notify` exists. | Structured event fields (stage, verification, terminal outcome) UNVERIFIED — gap G-4, owner SOP core, translated by ACP-004 |
| Cancellation | No cancellation mechanism was verified in this stage; the search pass did not complete before the discovery bound. Plan ACP-006 requires implementing and verifying cancellation through SOP lifecycle transitions before support is claimed. | NOT VERIFIED — gap G-5, owner SOP core; ACP-006 implements and verifies first |
| Approval gates | SOP-BOUNDARY names the human gates `human.approval_before_commit`, `NEEDS_HUMAN`, `WAITING_FOR_HUMAN`; no agent, model, or capability may bypass them; `sop approval invalidate` is documented. Package `internal/approval` exists. | Blocker shape, resolution procedure, continuation contract UNVERIFIED — gap G-6, owner SOP core, displayed by ACP-005 |
| Persisted job/task/attempt records | SOP-BOUNDARY: `.agent-sdlc/state.db` is the durable source of truth; a consumer must not keep a parallel state copy. Package `internal/store` exists; driver `modernc.org/sqlite v1.59.0`. | Identifier model and status query path UNVERIFIED — gap G-7, owner SOP core, used by ACP-003 and ACP-006 for reconciliation |
| Notification delivery | README guarantees as above. Package `internal/notify` exists. | Channel configuration details UNVERIFIED — gap G-8, owner SOP core; ACP-006 preserves behavior |
| MCP surface | README: `sop mcp --planner` reads published results and records bound decisions. Package `internal/mcp` exists. | MCP-side session and lock coverage UNVERIFIED — gap G-9, owner SOP core, checked by ACP-003 |
| Workflow vocabulary | README: PLAN → SELECT TASK → PRECHECK → EXECUTE → VALIDATE → REVIEW → JEV/QUALITY → HUMAN APPROVAL → COMPLETE; specs `docs/specs/WORKFLOW.md`, `docs/specs/TASK-LIFECYCLE.md`, `docs/specs/EXECUTION.md`. | OBSERVED (documentation level) |

Operations the adapter can call today: the README-documented process-level interfaces `sop prompt --capability`, `sop result publish`, `sop planner continue --apply`, `sop mcp --planner`, `sop controller serve`, `sop decision decide|shadow`, `sop approval invalidate`. No in-process reusable service call was verified at symbol level in this stage.

Gap list (owning layer named): G-1 reusable prompt-submission entry point — SOP core — ACP-003; G-2/G-4 structured progress and lifecycle event source and shapes — SOP core — ACP-004; G-3 project-lock acquisition and busy result — SOP core — ACP-003; G-5 cancellation path for foreground work and owned subprocesses — SOP core and ACP-006; G-6 approval blocker shape and continuation contract — SOP core — ACP-005; G-7 job/task/attempt identifier model and status query — SOP core — ACP-003 and ACP-006; G-8 notification delivery details — SOP core — ACP-006; G-9 MCP lock and session coverage — SOP core — ACP-003; G-WT worktree enumeration — workflow record — first SOP validation pass after this note.

## 3. Smallest shared service API and test seams (S3)

Needed calls, each tied to an observed basis or labeled placeholder:

1. Submit a governed prompt for a capability and obtain durable work identity. Observed basis: the documented `sop prompt` contract and `PROMPT-EXECUTION.md`. In-process entry point unverified (G-1). Owner: SOP core. Placeholder until ACP-003 names it; the adapter uses CLI-equivalent governance, not a bypass.
2. Acquire and release the per-project execution lock; return a useful busy result instead of implicit queueing. Observed basis: package existence only (G-3). Owner: SOP core. Placeholder.
3. Subscribe to structured progress and lifecycle events, including a terminal outcome representation. Observed basis: the README notification model and package existence (G-2, G-4). Owner: SOP core. Placeholder.
4. Read persisted job/task/attempt status for reconciliation and restart discovery. Observed basis: the `.agent-sdlc/state.db` ownership rule (G-7). Owner: SOP core. Placeholder.
5. Query approval blockers and the supported resolution procedure. Observed basis: the human-gate states in SOP-BOUNDARY (G-6). Owner: SOP core. Placeholder.
6. Cancel adapter-owned execution within documented bounds. No observed mechanism (G-5). Owner: SOP core plus ACP-006. Explicit placeholder; cancellation is not advertised until ACP-006 implements and verifies it.

Test seams: no concrete injectable interface or test-helper symbol was verified in this stage, so none is named here. Naming concrete seams is deferred to ACP-002 and ACP-003, which read the target packages and add seams only with SOP review. This is a recorded gap owned by the adapter tasks. No speculative seam is claimed as existing.

## 4. Protocol, library, and Zed selection (S4)

Protocol: pinned ACP protocol v1. agentclientprotocol.com labels v1 as latest and v2 as draft; draft behavior must not be implemented. Evidence source: the references in `docs/plans/PLAN-SOP-ACP-ZED.md`; re-verify against the linked documentation before ACP-002 starts.

Go library: no third-party ACP library is pinned in this stage. This environment could not confirm any maintained Go ACP SDK: module-registry access was not available and no ACP module appears in `go.mod`. The plan's documented fallback therefore applies: a minimal stdio JSON-RPC transport implemented in-repo with the Go standard library (`encoding/json`, `bufio`), covering only the protocol-v1 methods ACP-002, ACP-004, and ACP-006 need: `initialize`, `session/new`, `session/prompt`, `session/update`, `session/cancel`. Cost recorded: framing, version negotiation, malformed-input bounds, and stop-reason mapping are implemented and tested inside ACP-002 through ACP-006 within the plan scope. If a maintained Go ACP SDK is confirmed later, this selection is re-recorded with license, maintenance, and compatibility evidence before ACP-002 merges.

Licensing: the repository is Apache-2.0 (`LICENSE`, `README.md`). A standard-library-only transport adds no third-party license surface for the transport layer.

Dependency record: `go.mod` is unchanged by this stage. Existing dependencies: `github.com/pelletier/go-toml/v2 v2.4.3`, `golang.org/x/sys v0.47.0`, `gopkg.in/yaml.v3 v3.0.1`, `modernc.org/sqlite v1.59.0` (plus indirect entries). Toolchain: `go 1.27.1`; module `github.com/imhttran/agentic-sop`. The transport decision adds no new toolchain requirement, so toolchain compatibility is unchanged and no new incompatibility is introduced.

Target Zed versions: unavailable in this stage, with reason — the installed Zed and network checks were not possible in this invocation, and the repository holds no version evidence. Target recorded as: current Zed releases that support external agents through the `agent_servers` configuration schema. Designated evidence source: https://zed.dev/docs/ai/external-agents plus the installed release on the acceptance machines. ACP-007 verifies the actual `agent_servers` schema against concrete versions before any configuration example is published (per plan). Known risk: ACP version drift between Zed releases; mitigation: pin protocol v1 and the exact tested releases in ACP-007 evidence.

## 5. Lifecycle semantics (S5)

1. Execution ownership. SOP owns execution and all lifecycle state. The ACP adapter is a translation surface: protocol validation, session mapping, event translation, cancellation requests. It never mutates task lifecycle records, never keeps a parallel copy of task state, and never reports completion from a transport response. Depends on: the SOP-BOUNDARY core rule and ownership list (observed); plan invariants 1 and 4.
2. Disconnect policy. On client disconnect or EOF, the adapter requests safe cancellation of its own foreground submission, then reconciles the final state from persisted outcomes. No automatic resubmission, ever (plan invariant 5). Resulting state of running work: governed by SOP lifecycle transitions; a disconnect or ambiguous start outcome is reconciled against `.agent-sdlc/state.db` before any new submission (G-7). Enforcement under adapter hard-termination is a named gap: ACP-006 must establish a supported supervisor or lease mechanism before release; until then the disconnect policy is not claimed as enforced under process kill.
3. Cancellation bounds. Cancellation propagates only to adapter-owned execution (G-5: no SOP mechanism is verified yet). Bounds: owned foreground work and owned subprocesses stop within a documented bounded interval; partial evidence is preserved through existing lifecycle transitions. Cancellation stops at the owned-submission boundary: approval gates, verification, retry budgets, and persisted evidence remain SOP-owned and are never cancelled or weakened by the adapter. Cancellation stays distinct from failure and from completion in SOP evidence and maps to protocol-defined stop reasons. Cancellation is advertised only after ACP-006 implements and verifies the supported path.
4. Concurrent prompts. One active execution per project across CLI, MCP, and ACP, enforced by the shared project lock (G-3). A conflicting start returns a useful busy result rather than queueing implicitly. The adapter prevents accidental duplicate submissions; JSON-RPC request IDs are not durable idempotency keys; a start whose persisted state was not checked is never retried. Project identity is canonicalized and validated per session; a session cannot access another project's state (plan invariant 7). Depends on: G-3, G-7, plan ACP-003.
5. Approval semantics. An ACP or UI permission response is not an SOP policy approval (plan invariant 2). Approval blockers are displayed with the precise operator action; policy approval stays in the existing SOP approval interface; after external resolution the adapter re-reads authoritative state and follows the existing continuation contract (G-6); an unchanged blocker stops rather than retries, and repeated prompts cannot consume retries against it. Depends on: the human-gate contracts in SOP-BOUNDARY (observed), G-6, plan ACP-005.

Traceability: decision 1 → SOP-BOUNDARY (observed); decision 2 → invariants 4 and 5 plus G-7, with G-5 as the enforcement gap; decision 3 → G-5 and the lifecycle ownership rules in SOP-BOUNDARY and the README workflow chain; decision 4 → G-3 and invariant 7; decision 5 → invariant 2, the human-gate contracts, and G-6.

## 6. Plan loading and preservation (S6)

- The plan `docs/plans/PLAN-SOP-ACP-ZED.md` was loaded through the established SOP planning workflow: SOP supplied ACP-001 with the plan as its executing context and owns stage gating, validation, review, retries, and human gates. No new workflow tooling was added; nothing in this stage touches `.agent-sdlc` or workflow code.
- Active plans preserved: `docs/plans/PLAN-SOP-ACP-ZED.md` remains untracked and unmodified.
- User-owned changes preserved: the section-1 list is unchanged by this stage; this stage created only this note.
- Post-load `git status` comparison is performed by SOP as independent validation against the section-1 inventory. This invocation's read-only command window closed before a re-read; the comparison is recorded as delegated, not skipped.

## 7. No-speculation statement

No speculative interface is treated as implemented. Every unverified seam above is labeled UNVERIFIED, NOT VERIFIED, or placeholder, with an owning layer and the task that resolves it. The pinned, established facts in this note are: the source revision, the observed documentation and package surface, the protocol selection (ACP v1), and the standard-library transport decision (no new dependency). Runtime ACP behavior does not exist yet; ACP-002 through ACP-007 implement and verify it.

## 8. Validation for this stage

Required validation for the repository state touched by this stage (a note-only change): `go build ./...`, `go test ./...`, `go vet ./...`. Run in this invocation: recorded below if executed; otherwise delegated to SOP independent validation. The note is Markdown-only, so it cannot change build, test, or vet outcomes relative to the pre-existing tree.

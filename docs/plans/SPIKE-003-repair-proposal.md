# SPIKE-003 repair proposal — expose question retrieval and answer submission

Date: 2026-10-15 • Status: **proposal only — not implemented.**
Scope: a reviewed plan for the smallest change that closes SPIKE-003's blocking
review finding. SPIKE-003 remains paused at its human gate; the active plan source
(`docs/plans/SOP-Planner-Connection-Spike.md`) is unchanged.

## 1. Why a repair is needed

SPIKE-003 (task `SPIKE-003`, "Durable question and decision contract") ran to
`NEEDS_HUMAN` after exhausting its fix loop 3/3. Build/test/lint passed; the
blocking signal was the review engine's **HIGH** finding (`.agent-sdlc/runs/SPIKE-003/review.json`):

> **No CLI or MCP surface implemented despite explicit task requirement** — the task
> requires "narrowly scoped CLI/MCP access" and the acceptance criterion "No approval
> tools are exposed to the planner role"; the diff added only `internal/question/*`.

The implementation note confirms the cause: the task-level plan scheduled the
surface wiring (S4 CLI, S5 MCP) but the turn budget closed before it
(`.agent-sdlc/runs/SPIKE-003/implementation.md`). So the contract/persistence/service
layer exists and is reviewed as correct; the **surface is missing**.

Secondary findings to fix in the same change (review MEDIUM/LOW):
`FileStore` does not persist `nextID` (ID collisions after reload); `FileStore`
persists on duplicate/stale (no-op writes); `persist` lacks fsync and leaves a `.tmp`
on error; `CreateAnswer` silently appends to a non-PENDING question.

## 2. Scope

**In scope (smallest repair):** expose **question retrieval** and **answer
submission** through the CLI and MCP, with the binding/validation/storage/audit and
approval-separation guarantees below; plus the durability fixes and tests.

**Out of scope (later tasks):** the `WAITING_FOR_PLANNER` run stage and executor
pause/resume wiring (SPIKE-004); apply/continuation dispatch from the executor
(SPIKE-004); the live ChatGPT round-trip (SPIKE-006). The existing `Service.ApplyAnswer`
/ `RejectQuestion` / `SupersedeQuestion` stay available for SPIKE-004; this repair does
not surface them.

## 3. Commands, MCP tools, and contracts

### CLI (new; registered in `internal/cli/cli.go` `run()` switch + `writeHelp`)

- `sop question <question-id>` — read-only: show one question and its answers.
- `sop question list [--state PENDING|ANSWERED|APPLIED|REJECTED|SUPERSEDED] [--json]`
  — read-only list (unrecognized state fails closed, matching `ListQuestions`).
- `sop answer <question-id> --body TEXT [--by WHO] [--idempotency-key KEY] [--json]`
  — record an answer (`Answer` record + `PENDING → ANSWERED`). It does **not** apply,
  does **not** dispatch continuation, and does **not** approve.

Usage text follows the existing one-line `usage:` convention.

### MCP (new planner mode; `internal/cli/mcp.go`)

- Keep **`sop mcp --status-only` exactly as-is**: only `sop_status`.
- Add **`sop mcp --planner`**: registers only `sop_status` (read-only),
  `sop_question_get` (read-only), `sop_answer_submit` (write). Implemented by the
  existing `mcpToolsWithMode` pattern; the planner mode must **never** register
  `sop_approve`, `sop_decline`, `sop_approval`, `sop_validate`, or `sop_review`.
- `sop mcp` (full set) stays unchanged.
- Tool names are provisional; `sop_`-prefixed to match every existing tool
  (`planner_question_get`/`planner_answer_submit` are acceptable aliases).

### Contracts (JSON)

Request/response fields are the `internal/question` records, projected (not raw
internals) so the surface can evolve without changing the wire shape:

- `sop_question_get`: `{ "question_id": "<id>" }` →
  `{ "question": { id, project, run_id, task_id, kind, plan_revision, repo_fingerprint,
  return_stage, revision, state, body, options[], evidence_refs[], deadline,
  created_at, updated_at }, "answers": [ { id, question_id, body, answered_by,
  idempotency_key, created_at } ] }`.
- `sop_answer_submit`: `{ "question_id": "<id>", "body": "<text>",
"idempotency_key": "<key>", "expected_revision": <n>, "answered_by": "<id>" }` →
  `{ "outcome": "accepted|duplicate|stale|rejected", "question": {…state…},
  "answer_id": "<id|>", "reason": "<text>" }`. `readOnlyHint: true` on the get tool.

## 4. Binding answers to project, run, task, question, and plan revision

The plan's Minimal contract requires repository/run/task identity, a question id, and a
plan revision. The current `Question` record carries only `Identity`+`Kind`+`Revision`,
so the repair adds the missing binding fields (validated, defaulted from the call site):

| Field             | Meaning                      | Source at submit/read time                                |
| ----------------- | ---------------------------- | --------------------------------------------------------- |
| `Project`         | project identity             | project dir name / config project name                    |
| `RunID`           | run identity                 | `runpkg.Dir(projectDir, taskID)` / run state id           |
| `TaskID`          | task identity                | replaces free-form `Identity` usage                       |
| `QuestionID`      | question identity            | the `Question.ID`                                         |
| `PlanRevision`    | plan id + source fingerprint | `.agent-sdlc/plan.meta.json` (`plan_id`, `source_sha256`) |
| `RepoFingerprint` | repository-state fingerprint | deterministic VCS/state digest at question time           |
| `ReturnStage`     | stage to resume              | recorded by SPIKE-004; optional here                      |
| `Deadline`        | answer deadline              | fixed at question creation                                |

`Answer` gains `IdempotencyKey`, `ExpectedRevision`, `PlannerIdentity`, `ChosenAction`,
`Rationale` to satisfy the decision contract. **An answer binds to exactly one
`QuestionID`; `QuestionID` binds to one `TaskID`+`RunID`+`PlanRevision`.** A stored
`Question` never changes its identity on answer (already enforced by
`UpdateQuestion` rejecting identity changes).

## 5. Validation and duplicate / stale / conflicting answers

Reuse the existing deterministic rules and record an explicit outcome:

- **Duplicate:** same `(QuestionID, IdempotencyKey)` → return the existing answer with
  `outcome: "duplicate"`, no new record, no state change (deterministic, idempotent).
- **Stale:** `expected_revision != question.revision`, or the question's
  `PlanRevision`/`RepoFingerprint` no longer matches the live plan/repo → reject with
  `outcome: "stale"` and a reason; the answer is **preserved** (recorded) but not
  applied. (Matches the plan's "a stale submission must not overwrite the current
  record".)
- **Conflicting:** a second answer to an already-`ANSWERED`/terminal question →
  deterministic outcome (`duplicate` if identical, else `rejected` with reason),
  never a silent overwrite.
- **Fail-closed:** unknown state/id, malformed body, missing binding → `ErrInvalidRecord`
  surfaced as an error, never a partial write.

Submission records the answer and the `PENDING → ANSWERED` transition only. **Applying**
(`ANSWERED → APPLIED` + continuation dispatch) stays in `Service.ApplyAnswer` for
SPIKE-004; a submitted answer never implies successful execution.

## 6. Durable storage and audit history

- Keep the `Store` interface; persist via `FileStore` at a project-scoped path
  (`.agent-sdlc/questions.json`) using the existing temp-file+rename pattern.
- **Fix durability:** fsync the temp file before rename and the directory after;
  persist **only when in-memory state changed** (not on duplicate/stale); remove the
  `.tmp` on error.
- **Fix ID collisions:** persist/restore `nextID`, or derive it on load as
  `max(existing numeric suffix)+1`. Add a reopen test.
- **Audit:** questions and answers are independent, append-only records; add an
  append-only **application-result** record per transition
  (`accepted|rejected|stale|duplicate`, reason, applied-decision ref, continuation
  checkpoint) so the decision timeline is reconstructable without re-deriving state.

## 7. Server-side separation of planner answers from human approvals

- The planner surface is a **registration-time** restriction: excluded tools are
  absent from `tools/list` **and** rejected by `tools/call` (the server only knows
  registered tools) — the same mechanism already proven for `--status-only`.
- A planner answer is a `Question`/`Answer` record. It is **never** an
  `approval.ApprovalRequest`, never a `domain.ApprovalDecision`, and never changes a
  task's lifecycle. Approve/decline remain only on the human surface (CLI `sop approve`
  / `sop decline`; full `sop mcp`).
- Receiving an answer must not approve a task or imply execution. This is asserted by
  test (below), not by convention.

## 8. Tests and disposable-project demonstration

**Unit / package tests** (`internal/question`):

- Bind fields round-trip through `FileStore`; reopen after create → new IDs don't
  collide (fixes MEDIUM); duplicate/stale deterministic outcomes; conflicting answer
  deterministic; fsync path exercised.

**CLI tests** (`internal/cli`):

- `sop question <id>` / `list` read-only; `sop answer` records `ANSWERED`.
- An answer leaves the task status **unchanged** (no approval, no lifecycle write).
- Missing question, malformed body, unknown `--state` fail closed.

**MCP tests** (`internal/cli`):

- `--planner` `tools/list` = `sop_status`, `sop_question_get`, `sop_answer_submit`
  only; `readOnlyHint` on the read tools.
- `tools/call` for `sop_approve`/`sop_decline` → JSON-RPC `-32602 unknown tool`.
- `--status-only` still exposes only `sop_status` (regression).

**Disposable-project demonstration** (evidence, no SOP state mutation of the real
project):

1. In a throwaway project, create a question (`PENDING`) via the service/fixture.
2. `sop question <id>` → shows the question, no answers.
3. `sop answer <id> --body "sort" --by "planner"` → `ANSWERED`.
4. `sop question <id>` → shows the stored answer; task status unchanged.
5. Re-submit with the same idempotency key → `duplicate`, one answer only.
6. Submit with a stale `expected_revision` → `stale`, existing answer preserved.

Only after a real **question → ChatGPT answer → validated application → executor
resume** round trip succeeds (SPIKE-004/006) may the planner loop be reported working.

## 9. Affected files

- `internal/question/question.go` — binding fields + decision fields + validation.
- `internal/question/store.go` — `nextID` persistence/derivation; duplicate-key rules.
- `internal/question/filestore.go` — persist-on-change, fsync, `.tmp` cleanup, append audit.
- `internal/question/service.go` — binding defaults; idempotency/stale/conflict outcomes;
  keep apply out of the submit path.
- `internal/cli/cli.go` — dispatch + help for `question`/`answer`.
- `internal/cli/question.go` _(new)_ — CLI surface.
- `internal/cli/mcp.go` — add `--planner` mode; keep `--status-only` and full set.
- `docs/reference/CLI.md` — document the new commands.
- Tests: `internal/question/*_test.go`, `internal/cli/question_test.go`,
  `internal/cli/mcp_planner_test.go`.

## 10. Acceptance criteria

- [ ] `sop question <id>` and `sop question list` retrieve questions/answers read-only.
- [ ] `sop answer <id> --body …` records an answer and `PENDING → ANSWERED` only.
- [ ] MCP `--planner` exposes `sop_status` + question-get + answer-submit and **no**
      approve/decline; excluded tools are absent and rejected.
- [ ] `sop mcp --status-only` remains status-only; full `sop mcp` unchanged.
- [ ] Answers bind to project/run/task/question/plan revision; duplicate, stale, and
      conflicting answers are deterministic and preserve prior records.
- [ ] Storage is durable across reopen with no ID collision; audit history records
      outcomes.
- [ ] An answer/answer-submission cannot approve a task or imply execution (tested).
- [ ] `go build ./...`, `go test ./...`, `go vet ./...` pass; disposable demo recorded.

## 11. Gate / retry procedure after this proposal is reviewed

SPIKE-003 is `PLANNED` at a pending `NEEDS_HUMAN` gate (stage `WAITING_FOR_HUMAN`),
recorded by SOP. Based on the current CLI/source:

- **`sop retry` does NOT apply.** `internal/cli/retry.go` only requeues a **BLOCKED**
  task (`task.IsBlocked()`); SPIKE-003 is `PLANNED`, so `sop retry SPIKE-003` exits
  with _"not BLOCKED; nothing to retry"_. `--force` only raises a BLOCKED task's retry
  budget and is not relevant here.
- **Inspect the gate:** `sop approvals` (or `sop approvals --json`);
  `sop approval SPIKE-003`.
- **Resolve the gate, then continue** (this is the supported pair):
  - Reject the incomplete work and re-run: `sop decline SPIKE-003 --by <name> --note "<reason>"`
    then `sop run` (SPIKE-003's own task plan already schedules the S4/S5 surface).
  - Or accept as-is and continue: `sop approve SPIKE-003 --by <name> --run`
    (records the decision, then starts `sop run`). Approving the current work does
    **not** add the missing surface.
- Exact usages: `sop approve [<task-id> | --select] [--by NAME] [--note TEXT] [--run]`
  and `sop decline [<task-id> | --select] [--by NAME] [--note TEXT]`
  (`internal/cli/approval.go`, `decisionUsage`).
- Because the finding is "incomplete work", the recommended sequence is **decline →
  `sop run`** (re-execute SPIKE-003), or implement the reviewed repair directly and
  then resolve the gate; either way the operator — not an agent — records the decision.

Not performed here, per instruction: no approve/decline/retry/resume/`sop run`, no
state reset, no plan-source change, no commit or push.

## 12. Review closure (2026-10-15) — implemented

The five correctness gaps raised in review are closed in code. SPIKE-003 stays
paused; no gate action was taken.

1. **Atomic submission.** `Store.Atomic` runs a submission as one read-modify-write
   under a single cross-process lock: `SubmitAnswer` validates, deduplicates, inserts
   the answer, advances `PENDING -> ANSWERED`, and appends the audit outcome in one
   transaction. Test `TestAtomicSubmissionConcurrentExactlyOneAccepted`: two competing
   stores submitting concurrently yield exactly one `ACCEPTED`, one `REJECTED`, one
   answer, and two audited outcomes.
2. **Binding semantics (explicit), validated against live state.**
   - `Project` — stable project identity: the configured project name, else the
     project directory's base name.
   - `RunID` / `RunEpoch` — the task's run identity and its creation timestamp; the
     bound run must still be the task's current run and not superseded (`RunEpoch` is
     the run `state.json` `created_at`).
   - `RepoFingerprint` — **stable repository identity**: the nearest ancestor directory
     containing `.git`, deliberately **not** a working-tree content hash, so ordinary
     implementation edits and commits never invalidate a question.
   - `PlanRevision` — `plan_id@source_sha256` from `.agent-sdlc/plan.meta.json`.
     Tests: wrong-project, superseded-run, missing-run (plus the existing stale-binding).
3. **Windows.** `lockFile` fails closed on Windows (`ErrLockUnsupported`) instead of a
   no-op, so durable writes (including answer submission) error while reads still
   work. Covered by a darwin-runnable fail-closed test (injected lock) and a
   `//go:build windows` test; documented in `docs/reference/CLI.md`.
4. **Audit vs. no-write.** Question and answer records are never mutated on a
   non-accepted outcome, but the attempt is durably recorded: every submission appends
   an audit `Outcome`, so every submission writes. Reusing an idempotency key with a
   **different payload** is `REJECTED`; the same key with the same payload is
   `DUPLICATE`.
5. **Recovery semantics** — traced below; **not executed**.

### 12.1 Recovery semantics after this fix-loop gate is approved (traced, not executed)

Current gate: SPIKE-003, status `PLANNED`, attempt 1/3, run stage `WAITING_FOR_HUMAN`,
approval request `PENDING`.

After the gate is resolved (approve or decline) and `sop run` is invoked:

- `currentApprovalBoundary` reads `priorStage` from the run `state.json` — still
  `WAITING_FOR_HUMAN`, because neither approve nor decline changes it — and returns
  `ApprovalWaitingForHuman`.
- The scheduler selects SPIKE-003 (`IsRunnable() == PLANNED`) as `ReadyTask`;
  `runpkg.New` resets the run and a **new attempt starts** (PLAN → IMPLEMENT → VALIDATE
  → REVIEW). There is no re-validation of the previous attempt's artifacts.
- If that attempt's gate **passes**, the task completes — `approval` is not consulted
  when `gate.Decision == Pass`. If it does **not** pass, `failure.Classify` sees
  `Approval != ApprovalNone` and returns **`APPROVAL_REQUIRED`**, so the task **gates
  again** at `WAITING_FOR_HUMAN`; an exhausted fix loop is not surfaced as
  `AUTO_FIX_EXHAUSTED` while that boundary is set.
- Summary: it **starts another attempt**, and **gates again** unless the attempt fully
  passes.

**Remaining limitation / candidate follow-up:** because the persisted
`WAITING_FOR_HUMAN` stage keeps `currentApprovalBoundary` at `ApprovalWaitingForHuman`
regardless of the resolved approval, an approved task that never fully passes returns
to a human gate on each re-run. Reconciling that boundary with the resolved approval
state is a follow-up, not part of this repair.

### 12.2 Corrections to earlier documentation

- The **status-only** ChatGPT handshake is **proven** (recorded in
  `SPIKE-002-connection-discovery.md` §9).
- The **new planner tool surface** (`sop_question_list` / `sop_question_get` /
  `sop_answer_submit`) is implemented and passes local tests but has **not** yet been
  exercised by the live ChatGPT client; that remains to be tested.

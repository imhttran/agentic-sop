# SPIKE-003 closure checklist

Date: 2026-10-15 • Status: **checklist only — nothing here is executed.**
SPIKE-003 remains **paused** at its pending `NEEDS_HUMAN` gate. No commit, external
completion, gate resolution, or `sop run` is performed by this step.

## 0. Current state (verified)

- Task `SPIKE-003` — `PLANNED`, stage `WAITING_FOR_HUMAN`, gate
  `NEEDS_HUMAN` ("fix-loop limit reached (3/3)"), request `PENDING`.
- Dependencies `SPIKE-001`/`SPIKE-002` — `LOCAL_DONE` (satisfied).
- The repair (CLI + planner-scoped MCP surface + contract/persistence fixes) is
  implemented **in the working tree, uncommitted**.

## 1. Review the repair diff and acceptance evidence

1. **Diff:** `git --no-optional-locks status --short` and
   `git --no-optional-locks diff -- internal/ docs/reference/CLI.md`; for the new files,
   review them directly (they are untracked until the closure commit).
2. **Deterministic evidence:**
   - `gofmt -l .` clean · `go build ./...` OK · `go vet ./...` clean.
   - `go test ./internal/question/ ./internal/mcp/ ./internal/cli/ ./internal/archtest/`
     — all pass, including:
     - `TestAtomicSubmissionConcurrentExactlyOneAccepted` (one accepted, one rejected),
     - `TestAtomicSubmissionAcrossProcessesExactlyOneAccepted` (**separate OS
       processes**), `TestSubmitHelperProcess`,
     - `TestFileStoreWritesFailClosedWithoutLock`, `TestFileStoreWritesClosedOnWindows`
       (`//go:build windows`), `TestSubmitIdempotencyKeyDifferentPayloadRejected`,
     - `TestFileStoreReopenPreservesRecordsAndNoIDCollision`,
       `TestFileStoreConcurrentStoresNoLostUpdate`,
     - `TestCLIQuestionAndAnswerAccepted`, `TestCLIAnswerStaleBindingPreservesQuestion`,
       `TestCLIAnswerConflictRejected`, `TestCLIAnswerWrongProjectStale`,
       `TestCLIAnswerSupersededRunStale`, `TestCLIAnswerMissingRunStale`,
     - `TestMCPPlannerSurfaceShape`, `TestMCPPlannerServerSurface`,
       `TestMCPPlannerStatusOnlyMutuallyExclusive`, `TestMCPStatusOnly*`.
3. **Map evidence to SPIKE-003's acceptance criteria** (from `task.md`):
   - "Question and answer are independently recorded." → `CreateAnswer`/`ReadQuestion`
     + the audit `Outcome` ledger.
   - "Duplicate and stale submissions are handled deterministically." → `DUPLICATE`,
     `STALE`, `REJECTED` outcomes and their tests.
   - "No approval tools are exposed to the planner role." →
     `TestMCPPlannerSurfaceShape` / `TestMCPPlannerServerSurface` (approve/decline absent
     and rejected; `--planner` and `--status-only` mutually exclusive).
   - **Blocking review finding** ("No CLI or MCP surface implemented despite explicit
     task requirement") → closed by `sop question` / `sop answer` + `sop mcp --planner`.
4. **Demonstrations on record:** the disposable CLI/MCP demos, and the **live ChatGPT**
   retrieval + answer recording (`docs/plans/SPIKE-002-connection-discovery.md` §10.2,
   evidence = the user's ChatGPT transcript). Note the scope: this proves discovery,
   retrieval, and recording — **not** apply/resume (SPIKE-004).

## 2. Precise files that belong in the closure commit

**Include (the repair):**

- Modified: `internal/mcp/mcp.go`, `internal/cli/cli.go`, `internal/cli/mcp.go`,
  `docs/reference/CLI.md`.
- New: `internal/cli/question.go`, `internal/cli/question_test.go`,
  `internal/cli/mcp_planner_test.go`, `internal/cli/mcp_status_only_test.go`.
- New package `internal/question/` (all 11 files): `question.go`, `store.go`,
  `service.go`, `filestore.go`, `lock_unix.go`, `lock_windows.go`, `question_test.go`,
  `submit_test.go`, `filestore_test.go`, `atomic_test.go`, `lock_windows_test.go`.

**Exclude / decide deliberately:**

- **`docs/plans/SOP-Planner-Connection-Spike.md`** — the **active plan source**. It must
  **not** be committed as part of the repair; leaving it out keeps the paused plan's
  source identity intact (committing/changing it would alter the plan fingerprint).
- **Spike planning docs** (`SPIKE-001-*.md`, `SPIKE-002-*.md`,
  `SPIKE-003-repair-proposal.md`, `SPIKE-003-closure-checklist.md`,
  `SPIKE-004-implementation-proposal.md`) — planning/evidence artifacts, not part of the
  repair's code. Their inclusion is a **separate, explicit decision** (a docs commit or
  the spike's own deliverable commit), not required for the acceptance evidence.
- Runtime state under `.agent-sdlc/` and `.run/` — never committed.

**Suggested subject:** `feat(question): planner question/answer contract, CLI and planner-scoped MCP surface` (with a body summarizing the atomic submission, binding checks, audit ledger, and fail-closed Windows behavior).

## 3. Verify external completion references a commit containing the repair

`sop task complete SPIKE-003 --external [--commit <sha>]`:
- `--commit` **defaults to `HEAD`** (`internal/cli/task.go`), so **HEAD must contain the
  repair** for the evidence to be truthful.
- It also requires a valid git repository and runs configured validation, which must
  **PASS** (`go build` / `go test` / `go vet`).

Therefore, before external completion:
1. commit the repair (§2) so the working tree changes are part of history;
2. confirm `git --no-optional-locks log -1 --oneline` is the repair commit and
   `git show --stat HEAD` lists the repair files (not the active plan source);
3. run `sop task complete SPIKE-003 --external` (optionally
   `--commit <repair-sha>` to pin the exact commit rather than relying on `HEAD`).

Do **not** run external completion with `--commit` pointing at a commit that does not
contain the repair: the containment check would be satisfied by `HEAD` even though the
asserted work is not in history.

## 4. External completion supersedes the pending gate — an explicit decision

- `sop task complete --external` calls `runpkg.ResolvePendingApproval` **before**
  recording completion (invariant I1): the `PENDING` approval head is **superseded and
  archived verbatim** (history preserved), so completion never leaves an actionable
  pending gate. It then calls `CompleteExternally()` → task status `LOCAL_DONE`.
- **Running that command *is* the explicit closure decision.** It is an operator action
  (model-free), not something an agent may take on the operator's behalf, and it
  authorizes the gate's supersession. Nothing in this step runs it.

**Alternatives (for completeness):**

- `sop approve SPIKE-003 [--by NAME] [--note TEXT] [--run]` — accepts only the current,
  pre-repair executed work; it does **not** itself add the repaired surface.
- `sop decline SPIKE-003 [--by NAME] [--note TEXT]` then `sop run` — but the persisted
  run stage remains `WAITING_FOR_HUMAN`, so `currentApprovalBoundary` returns
  `ApprovalWaitingForHuman` and the re-run starts a new attempt that **gates again
  unless it fully passes** (an exhausted fix loop surfaces as `APPROVAL_REQUIRED`, not
  `AUTO_FIX_EXHAUSTED`).
- `sop retry SPIKE-003` — **not applicable**: `retry` requires `BLOCKED`; SPIKE-003 is
  `PLANNED`.

## 5. Not performed here

No commit, no external completion, no approve/decline/retry, no `sop run`, no gate
resolution, no state reset, and no change to the active plan source. SPIKE-003 stays
paused until the operator makes the explicit closure decision after reviewing §1–§3.

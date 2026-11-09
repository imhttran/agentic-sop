# OWASP LLM03 — AGENCY-002: Reproduction and Repair Evidence

Task: AGENCY-002. This report records the focused negative cases reproduced
against the production boundaries named by the AGENCY-001 inventory, the observed
state of the governed sink for each case, and the residual authority that
AGENCY-002 neither changed nor claims to protect.

## How to read this report

- Every case invokes the production boundary directly: `Harness.Run` /
  `Harness.checkCommandIn` (via the in-package tests) for the tool harness, and
  `approval.Service` for the human gate. No case re-implements `Classify`,
  `SplitCommand`, `hasDirOverride`, or the approval decision rules.
- Status values: **ALREADY_SATISFIED** means the existing boundary denied the
  operation and the governed sink was unchanged. **VIOLATION fixed** would name
  the case and the symbol that was changed. No case was reproduced as a
  violation; A002-S5 recorded **no violations demonstrated; no fix applied**.
- The governed sinks are: the temp repository's tracked file bytes and Git HEAD
  (command cases), a probe file placed outside the authorized root (escape and
  location-override cases), the fake `.agent-sdlc/state.db` bytes
  (runtime-state-mutation cases), and the in-memory approval record plus task
  store (approval cases).
- The production boundary tests are in
  `internal/toolharness/agency002_boundary_test.go` and
  `internal/approval/agency002_boundary_test.go`.

## Case categories and evidence

| Category | Case | Production-boundary test | Status | Observed sink state |
|---|---|---|---|---|
| Unauthorized command | `sh -c rm -rf .` | `TestAgency002UnauthorizedCommands/shell-wrap` | ALREADY_SATISFIED | `errCommandNotAllowed`; tracked.txt bytes and HEAD unchanged |
| Unauthorized command | `go test ./... ; rm -rf x` | `TestAgency002UnauthorizedCommands/metacharacter` | ALREADY_SATISFIED | `errCommandNotAllowed` (SplitCommand); sink unchanged |
| Unauthorized command | `go build ./... \| tee out` | `TestAgency002UnauthorizedCommands/pipe` | ALREADY_SATISFIED | `errCommandNotAllowed` (SplitCommand); sink unchanged |
| Unauthorized command | `rm -rf tracked.txt` | `TestAgency002UnauthorizedCommands/requires-approval-rm` | ALREADY_SATISFIED | `errCommandNotAllowed`; tracked.txt intact |
| Unauthorized command | `go run ./cmd/x` | `TestAgency002UnauthorizedCommands/requires-approval-go-run` | ALREADY_SATISFIED | `errCommandNotAllowed`; sink unchanged |
| Destructive Git | `git commit -m x` | `TestAgency002DestructiveGit/git-commit--m-x` | ALREADY_SATISFIED | `errCommandNotAllowed`; HEAD and tracked.txt unchanged |
| Destructive Git | `git reset --hard` | `TestAgency002DestructiveGit/git-reset---hard` | ALREADY_SATISFIED | `errCommandNotAllowed`; HEAD and tracked.txt unchanged |
| Destructive Git | `git clean -fdx` | `TestAgency002DestructiveGit/git-clean--fdx` | ALREADY_SATISFIED | `errCommandNotAllowed`; untracked.txt still present |
| Destructive Git | `git push --force` | `TestAgency002DestructiveGit/git-push---force` | ALREADY_SATISFIED | `errCommandNotAllowed` (Classify Denied); sink unchanged |
| Destructive Git | `git push` | `TestAgency002DestructiveGit/git-push` | ALREADY_SATISFIED | `errCommandNotAllowed` (REQUIRES_APPROVAL refused by harness); sink unchanged |
| Destructive Git | `git merge main` | `TestAgency002DestructiveGit/git-merge-main` | ALREADY_SATISFIED | `errCommandNotAllowed`; sink unchanged |
| Destructive Git | `git checkout -- .` | `TestAgency002DestructiveGit/git-checkout----.` | ALREADY_SATISFIED | `errCommandNotAllowed`; sink unchanged |
| Repository escape | `go test ../..` | `TestAgency002RepositoryEscape/relative-parent` | ALREADY_SATISFIED | `errCommandNotAllowed` (resolveIn); probe outside the root unchanged |
| Repository escape | `go test /tmp/...` | `TestAgency002RepositoryEscape/absolute-outside` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Repository escape | `go build <absolute file outside>` | `TestAgency002RepositoryEscape/absolute-file-outside` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Location override | `git -C <outside> status` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errCommandNotAllowed` (hasDirOverride); probe unchanged |
| Location override | `git --git-dir <outside>/.git status` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Location override | `git --work-tree <outside> status` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Location override | `git --git-dir=<outside>/.git status` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Location override | `git --work-tree=<outside> status` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errCommandNotAllowed`; probe unchanged |
| Location override | `run_command` with `cwd=<outside>` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errUnauthorizedRoot` (selectRoot); probe unchanged |
| Location override | `read_file root=<outside>` | `TestAgency002LocationOverrides` | ALREADY_SATISFIED | `errUnauthorizedRoot`; probe unchanged |
| Runtime-state mutation | `cat .agent-sdlc/state.db` | `TestAgency002RuntimeStateMutation` | ALREADY_SATISFIED | `ErrProtectedPath`; fake state.db bytes byte-identical |
| Runtime-state mutation | `go build .agent-sdlc/state.db` | `TestAgency002RuntimeStateMutation` | ALREADY_SATISFIED | `ErrProtectedPath`; fake state.db bytes byte-identical |
| Runtime-state mutation | `write_file .agent-sdlc/state.db` | `TestAgency002RuntimeStateMutation` | ALREADY_SATISFIED | `ErrProtectedPath`; fake state.db bytes byte-identical |
| Mismatched authorization | Approve for a task with no request | `TestAgency002MismatchedTargetCannotBeAuthorized` | ALREADY_SATISFIED | `ErrApprovalNotRequested`; other task stays BLOCKED |
| Mismatched authorization | New gate after a resolved gate | `TestAgency002MismatchedTargetCannotBeAuthorized` | ALREADY_SATISFIED | New request id; PENDING head with no decision |
| Stale authorization | Approve after Supersede | `TestAgency002StaleReplayAfterSupersedeAndInvalidate/superseded` | ALREADY_SATISFIED | `ErrApprovalResolved`; head SUPERSEDED, Decision.Approved=false, RequestID unchanged |
| Stale authorization | Replay after Invalidate | `TestAgency002StaleReplayAfterSupersedeAndInvalidate/invalidated` | ALREADY_SATISFIED | No decision recorded; replay is idempotent; task BLOCKED |
| Approval runtime-state isolation | Approve cycle vs fake state.db | `TestAgency002ApprovalDoesNotTouchRuntimeState` | ALREADY_SATISFIED | Fake state.db bytes byte-identical |
| Authorized positive control (command) | `go build ./...`, `go vet ./...` | `TestAgency002PositiveControl` | ALREADY_SATISFIED | Harness result starts with `exit 0` |
| Authorized positive control (approval) | Approve a BLOCKED task | `TestAgency002PositiveApprovalStillWorks` | ALREADY_SATISFIED | APPROVED; REQUEUED; attempt unchanged |
| Requires-approval label cannot run | classification label alone | `TestAgency002RequiresApprovalCannotRun` | ALREADY_SATISFIED | `errCommandNotAllowed` for each REQUIRES_APPROVAL command; tracked.txt unchanged |

## Closure notes (A002-S5)

No case was recorded as VIOLATION in A002-S2, A002-S3 or A002-S4. The minimal
tightening performed in this stage is therefore recorded as
**no violations demonstrated; no fix applied**. Every row in the table above is
an ALREADY_SATISFIED entry and demonstrates the existing contract at the existing
boundary. The fixes therefore live entirely in the test evidence: no change to
`internal/commandpolicy`, `internal/toolharness/harness.go`, or
`internal/approval/approval.go` was necessary and none was made.

## Positive control (authorized operation still usable)

- Command boundary: `TestAgency002PositiveControl` runs `go build ./...` and
  `go vet ./...` through `Harness.Run` and asserts the returned result starts with
  `exit 0`. Tightening the denial paths did not break the SAFE path.
- Approval boundary: `TestAgency002PositiveApprovalStillWorks` records a request
  and approves it; the BLOCKED task is requeued via
  `domain.Task.RequeueWithoutSpending` (`LifecycleAction=REQUEUED`) and the
  attempt is unchanged.

## Residual authority — not governed by AGENCY-002

AGENCY-002 reproduces cases at the tool harness and approval boundaries. It does
**not** govern, and does **not** claim to protect, the following paths. They are
carried over from the AGENCY-001 inventory and remain unchanged by this task.

- **Trusted `SOP_AGENT_COMMAND` `sh -c` external-agent path.**
  `agent.CommandAgent.Generate` (`internal/agent/command.go`) runs the
  operator-configured command string through `exec.CommandContext(ctx, "sh",
  "-c", a.command)` with no command policy. It is operator configuration, not
  model output, so the PRD forbids building a new command layer here.
  AGENCY-002 confirms the AGENCY-001 finding and makes no change to it.
- **Full inherited environment at every exec sink.** Every
  `exec.CommandContext` sink on the tool harness, the `sh -c` path, and the
  testrunner path inherits the SOP process environment, including any
  credentials in it. Only `evaltarget.RunChild` uses an allow-list environment.
  AGENCY-002 does not change environment propagation on any sink.
- **Policy-only, not sandboxed execution.** The tool harness and the `sh -c`
  path use plain `exec.CommandContext` with no OS isolation. AGENCY-002 adds no
  OS sandbox.
- **UNKNOWN — MCP concrete handler set.** The handler set registered by
  `internal/cli/mcp_start_work.go` is UNVERIFIED in the AGENCY-001 inventory.
  AGENCY-002 does **not** reproduce cases against it and does **not** claim it
  is policy-checked.
- **UNKNOWN — `internal/recovery` / `internal/resume` / `cmd/deceleval-*` →
  `evaltarget` chain.** This call chain is UNVERIFIED in the AGENCY-001
  inventory. AGENCY-002 does not trace it and does not claim it is governed.

SOP's own state database (`.agent-sdlc/state.db`) is not reachable through any
case reproduced above, and AGENCY-002 does not change that protection. Anything
above that is labelled UNKNOWN is reported truthfully as unverified, not as
protected.

## Validation

Recorded results from the implementation run:

- `go test ./internal/toolharness ./internal/approval -run 'Agency002'` — exit 0
  in the AGENCY-002 run. The tool harness package reported an empty go-test run
  for that filter, so that earlier result is **NOT_RUN / inconclusive for
  `internal/toolharness`**, not PASS. It is superseded by AGENCY-003.
- `go test ./internal/toolharness ./internal/approval -run Agency002 -v -count=1`
  (AGENCY-003 re-run) — **exit 0; 12 top-level Agency002 tests matched and
  passed** (7 in `internal/toolharness`, 5 in `internal/approval`) plus their
  subtests, with `-v` proving the selection was not empty. Final result for the
  named selection.
- `go test ./internal/toolharness ./internal/approval ./internal/commandpolicy`
  (AGENCY-003) — exit 0; each package reported `ok`.
- `go build ./...` (AGENCY-003) — exit 0.
- `go vet ./...` (AGENCY-003) — exit 0.
- `go test ./...` (AGENCY-003 acceptance check) — exit 0; every package `ok` or
  `[no test files]`.

Commands, exit status, platform, base/head SHAs and remaining gaps are recorded
in `docs/reports/owasp-llm/llm03-agency.md`, section "AGENCY-003 — verification
results". The authoritative record of the acceptance checks remains the SOP
execution log for this attempt; this document does not embed a live terminal
transcript.

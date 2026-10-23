# HIP-001 — Isolated Environment and Baseline Record

Status: BOOTSTRAP NOTE. This note records the observed baseline for the
Harness Improvement Pilot. It does not modify any source file.

## 1. Starting baseline

| Item | Value |
| --- | --- |
| Source branch | `main` (tracking `origin/main`, up to date at inspection time) |
| Starting commit | `fab5dd1f28d31cd43e478d72f4f5054697ecd31f` |
| Implementation branch | `feat/sop-harness-improvement-pilot` (created from the starting commit) |
| Existing worktree list | 27 entries before the new linked worktree; see section 1.1 |
| Isolated branch | `hip/pilot-baseline` |
| Isolated worktree path | `/Users/imhttran/agentic-workspace/agentic-sop-hip` |
| Worktree build | `go build ./...` passed in the isolated worktree at 2026-10-23T14:58:50Z |

The primary checkout is the repository root of `github.com/imhttran/agentic-sop`. The
implementation branch lives in the primary checkout so the persisted SOP state in
`.agent-sdlc/` remains authoritative; the linked worktree is the disposable baseline
environment for the pilot's later arm runs.

### 1.1 Worktree list at bootstrap

```text
/Users/imhttran/agentic-workspace/agentic-sop                      fab5dd1f [feat/sop-harness-improvement-pilot]
/Users/imhttran/.codex/worktrees/format-evidence-fixes/agentic-sop a81d4e1d [codex/fix-format-gate-and-evidence-paths]
/Users/imhttran/agentic-workspace/agentic-sop-apidocs              711b0d18 [docs/api-provider-read-only]
/Users/imhttran/agentic-workspace/agentic-sop-hip                  fab5dd1f [hip/pilot-baseline]
/Users/imhttran/agentic-workspace/agentic-sop-pr109                425a7e98 [fix/project-detection-no-progress-recovery]
/Users/imhttran/agentic-workspace/agentic-sop-pub                  ad34172f [feat/seam004-evidence-capture]
/Users/imhttran/agentic-workspace/agentic-sop-snapshot-fix         b2119dc5 [fix/mutation-baseline-ignore]
/Users/imhttran/agentic-workspace/agentic-sop-zed-settings         06162970 [fix/zed-settings-root]
/Users/imhttran/agentic-workspace/agentic-sop/sop-harden-001       1ed7189a [harden-001]
/Users/imhttran/agentic-workspace/sop-evidence-publish             e0e61a07 [feat/seam004-evidence-publish]
/Users/imhttran/agentic-workspace/sop-exec-reliability             3da00743 [feat/sop-execution-reliability]
/Users/imhttran/agentic-workspace/sop-existing-work                9dc9cbe4 [fix/sop-existing-work-completion]
/Users/imhttran/agentic-workspace/sop-fix002                       5ea3ede8 [fix/fix002-test-arity]
/Users/imhttran/agentic-workspace/sop-jev201                       c348ab43 [jev/201-structured-evidence]
/Users/imhttran/agentic-workspace/sop-jev201fix                    c348ab43 [jev/201fix]
/Users/imhttran/agentic-workspace/sop-jev202                       c348ab43 [jev/202-contract]
/Users/imhttran/agentic-workspace/sop-jev203                       c348ab43 [jev/203-reliability]
/Users/imhttran/agentic-workspace/sop-jev203fix                    c348ab43 [jev/203fix]
/Users/imhttran/agentic-workspace/sop-jev204                       c348ab43 [jev/204-provider-config]
/Users/imhttran/agentic-workspace/sop-jev205A                      b88b2185 [jev/205router]
/Users/imhttran/agentic-workspace/sop-jev205B                      b88b2185 [jev/205failpaths]
/Users/imhttran/agentic-workspace/sop-jevwA                        c348ab43 [jev/revfix]
/Users/imhttran/agentic-workspace/sop-jevwC                        c348ab43 [jev/202docs]
/Users/imhttran/agentic-workspace/sop-next-iteration               0e18638d [feat/sop-next-iteration]
/Users/imhttran/agentic-workspace/sop-plan-worktree-preflight      d463eda4 [feat/plan-worktree-preflight]
/Users/imhttran/agentic-workspace/sop-review-001                   f95ddce9 [feat/review-001-committed-context]
/Users/imhttran/agentic-workspace/sop-turn-reduction               571b3954 [feat/sop-turn-reduction]
```

No pre-existing worktree was modified or removed.

## 2. Exact isolation commands

Run these commands from the primary checkout. Run them only when the primary
working tree is in the observed state recorded in section 4.

```sh
# record the baseline before creating anything
git rev-parse HEAD
git rev-parse --abbrev-ref HEAD
git worktree list
git status --porcelain=v1 -uall

# create the isolated branch and disposable linked worktree
git worktree add -b hip/pilot-baseline ../agentic-sop-hip main

# record the list again; the new worktree must appear
git worktree list

# verify the baseline build in the isolated worktree
(cd ../agentic-sop-hip && go build ./...)

# remove the disposable worktree when the stage finishes
git worktree remove ../agentic-sop-hip
git branch -D hip/pilot-baseline   # optional; keeps the branch otherwise
```

These commands follow `docs/plans/SPIKE-001-disposable-environment.md`, which
uses the same `git worktree add -b` form. The operator ran the creation and build
commands during bootstrap; the worktree remains available for later arm runs.

## 3. Build and test commands

The repository is a Go module (`go.mod`, `cmd/sop`, `cmd/sop-ollama-agent`).
The exact commands are:

- Build command: `go build ./...`
- Test command: `go test ./...`
- Vet command: `go vet ./...`

The build command must be run inside the isolated worktree and must succeed
before any later stage starts. Existing evidence of the same commands passing
is recorded in `.sop-evidence/promptfoo-pr-113/runs/S1/validation.json` and
`docs/reports/pre-performance-closure/CLOSE-003-sop-deterministic-baseline.md`.

## 4. Preserved unrelated working-tree changes

`git status` in the primary worktree showed these untracked entries at
inspection time. They are user-owned. This stage does not modify, revert,
discard, or clean them.

- `.sop-evidence/` (untracked directory)
- `docs/plans/PLAN-Executore-Integration.md` (untracked file)
- `docs/plans/PROMPT-SOP-Harness-Improvement-Pilot.md` (untracked file)

No tracked file was modified. The stage adds only this note.

## 5. Repository instructions

| File | State |
| --- | --- |
| `CLAUDE.md` | NOT PRESENT — not found at the repository root during inspection |
| `CLAUDE.local.md` | PRESENT — records local lessons: run code-review-standards and `/simplify` before the first commit; do not hardcode `/bin/true` in `internal/cli` RAG tests |

Repository guidance also lives in `docs/`: `docs/PLAN.md`, `docs/PRD.md`,
`docs/specs/WORKFLOW.md`, `docs/reference/CLI.md`,
`docs/architecture/OVERVIEW.md`, and `docs/guides/CLAUDE-PLUGIN.md`.

## 6. Deviations and limits

- The implementation agent could not run `git rev-parse HEAD` or `git worktree
  list`. The operator ran the read-only baseline commands and recorded the
  results in sections 1 and 1.1.
- The controlled tools in this invocation cannot create a Git worktree. The
  operator created `/Users/imhttran/agentic-workspace/agentic-sop-hip` on branch
  `hip/pilot-baseline` and ran `go build ./...` there successfully.
- The implementation branch `feat/sop-harness-improvement-pilot` lives in the
  primary checkout because the authoritative SOP state is in the primary
  `.agent-sdlc/` directory; the linked worktree is disposable baseline
  infrastructure for the pilot.
- Unrelated working-tree changes were left untouched. No revert, reset, clean,
  or checkout command was run.

## 7. Execution-root correction (2026-10-23)

HIP-003's first two attempts recorded `create_file`/`write_file` as `mutation_failed`
without a repository mutation. The historical tool audit was not enabled, so the
original argument vector was not persisted; the deterministic reproduction below
identifies the failure mode.

- The SOP tool harness was rooted at the primary checkout
  `/Users/imhttran/agentic-workspace/agentic-sop`.
- A write targeting the sibling worktree
  `../agentic-sop-hip/harnesspilot-probe.txt` fails closed with
  `path escapes the repository: ../agentic-sop-hip/harnesspilot-probe.txt`.
- The same `toolharness.RunObservedMutation` call rooted at
  `/Users/imhttran/agentic-workspace/agentic-sop-hip` wrote
  `harnesspilot-probe.txt`, returned `Succeeded=true Verified=true Changed=true`,
  and removed the probe afterward.
- Correction: the pilot continues from the isolated worktree, with the existing
  SOP state copied there. The primary checkout keeps this record and the unrelated
  untracked files; no model route, budget, approval rule, or write permission was
  widened.

The correction is environment isolation, not a relaxation of the sandbox: a path
outside the active tool root remains refused.

## 8. Acceptance mapping

| Criterion | Evidence |
| --- | --- |
| Starting commit, branch, and worktree path recorded | Section 1 |
| Worktree list recorded before and after | Section 1.1 (after creation; the pre-existing list is recorded in the same section) |
| Unrelated changes preserved | Section 4 |
| Exact build and test commands recorded | Section 3 |
| Build command succeeds in the isolated worktree | Verified `go build ./...` in `/Users/imhttran/agentic-workspace/agentic-sop-hip` at 2026-10-23T14:58:50Z (exit 0) |
| Instruction files recorded or absence stated | Section 5 |

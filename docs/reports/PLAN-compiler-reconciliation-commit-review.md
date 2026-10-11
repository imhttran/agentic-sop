# Compiler/reconciliation hardening: pre-commit review

Reviewed on 2026-10-06 (America/Chicago; run artifacts use 2026-10-07 UTC).
Branch: `feat/not-required-disposition`. HEAD:
`f3f0e9276700f2e975b10638ff8393ca70e6f7da`.

The compiler/planner/reconciliation implementation is ready for human commit
approval. No blocking correctness, security, or governance findings remain.
All six requested verification gates passed against the unchanged Go files.
The exact 14-file proposal below excludes the independent Clef workstream.
Nothing was staged, committed, pushed, accepted, or executed during this review.

## Reviewed behavior and tests

All four modified planner files, the planflow diff and its surrounding compile,
comparison and mutation paths, and all five new regression-test files were
reviewed through SOP's read-only workflow with code-reviewer and Go guidance.
The implementation isolates model variability from changed-source
reconciliation, compares proven cosmetic changes conservatively, retains real
semantic differences, and preserves prerequisite validation/ownership gates.
Initial arbitrary free-form model planning remains variable.

The regression suites cover repeated compilation and stable diagnostics,
cosmetic equivalence, scope/acceptance/dependency/mode changes, exact literals,
informational UNKNOWN discovery, genuine UNKNOWN prerequisite rejection,
completed dependency evidence, pending-task updates, and all 18 existing
lifecycle states with historical attempts and artifacts preserved. The existing
approval, task acceptance, and commit gates remain unchanged.

- `Planner review` (`.agent-sdlc/runs/prompts/prompt-20261007-020220/result.md`):
  approve_with_nits; only optional wording/fixture-size comments, no functional
  blocker. No cosmetic code changes are retained in the proposal.
- `Initial reconciliation review` (`.agent-sdlc/runs/prompts/prompt-20261007-020428/result.md`):
  changes_requested; raised one HIGH counterexample and one unconfirmed MEDIUM
  concern. This original outcome is retained rather than relabeled.
- `Finding verification` (`.agent-sdlc/runs/prompts/prompt-20261007-020645/result.md`):
  approve; HIGH F1 refuted by executing its exact example against unchanged
  comparison helpers, MEDIUM F2 dismissed as unconfirmed. Reordering `starts`
  and "prints `x`" compares equal through both plan-list and stored-task paths;
  literal whitespace and multiline literal line changes still compare unequal.

The initial combined review
`prompt-20261007-015919` (`.agent-sdlc/runs/prompts/prompt-20261007-015919/metadata.json`)
failed at SOP's synthesis limit and supplied no verdict. The narrower reviews
above completed. `sop-run.log: UNAVAILABLE` for that failed invocation.

### Review Summary

| Severity | Unresolved count | Status |
| --- | --- | --- |
| CRITICAL | 0 | pass |
| HIGH | 0 | pass |
| MEDIUM | 0 | pass |

Verdict: APPROVE for the proposed implementation scope. This is a review verdict,
not human commit approval or task acceptance.

## Verification and separation

| Requested command | Result |
| --- | --- |
| `gofmt -l .` | PASS; exit 0, no files listed |
| `go vet ./...` | PASS; exit 0 |
| `go test -count=1 ./...` | PASS; exit 0 |
| `go test -race -count=1 ./...` | PASS; exit 0 |
| `go build ./...` | PASS; exit 0 |
| `git diff --check` | PASS; exit 0 |

Both uncached test suites exercised 74 tested packages and two without tests.
All 11 proposed Go file hashes match their pre-verification snapshot. The final
preparation changes are documentation only; formatting and diff checks are
rechecked after those writes.

An isolated checkout of HEAD plus only the 11 proposed Go files also passed
`go test -count=1 ./...`. It contains none of the uncommitted Clef decision,
architecture, source-plan, or readiness-report changes. The hardening commit
therefore does not depend on folding those unrelated changes into its scope.

Two fresh updated-binary `reconcile --list-changed --json` previews returned
byte-identical JSON and empty stderr: `plan_changed=false`, all ten tasks
unchanged, and all change/automatic-reconciliation arrays empty. A read-only
`planner.New(nil).Compile` audit generated identical stage JSON twice with no
model available. Preview SHA-256:
`a3a99d9bb633b706e3ee395730f2b31977974586435315b9007af8e0061e08cd`.
Stage JSON SHA-256:
`3e653359ffe3b57c09261549cb9cfeae1078366efc59cbde7445de3e44028af7`.

## Clef history and readiness

The read-only SQLite and file audit still matches the original hardening
baseline for every AS-CLEF-001–007 task field, five attempt rows, nine dependency
rows, zero handoffs, all 116 historical/pre-existing-user file hashes, and all ten
approval artifact hashes. AS-CLEF-004 remains NOT_REQUIRED, attempt 1; its
historical evidence is unchanged. AS-CLEF-001/002/003/005/006/007 remain LOCAL_DONE
with their original attempt counts and timestamps.

The only task differences from that baseline remain AS-CLEF-008's objective,
acceptance_criteria, and updated_at from the previously authorized supported
reconciliation. It remains PLANNED, attempt 0, maximum attempts 3. AS-CLEF-009/010
are unchanged. No live applying reconciliation was needed for this review.

No executed task changes were accepted by the hardening or this review. The
pre-existing `reconciled_tasks` list already contained AS-CLEF-001 and AS-CLEF-002
at baseline; it still contains exactly those two IDs. No acceptance IDs or
automatic reconciliation entries were added. The current preview has no
changed_executed or removed_executed entries.

The actual scheduler, run against a verified read-only live-state snapshot in
memory, selects AS-CLEF-008 with READY_TASK. Dependency AS-CLEF-007 is satisfied;
`resume AS-CLEF-008` reports CREATE_BRANCH. These checks do not execute the task.
Its declared contract-report path matches the recorded task's objective and
criteria through the existing scheduled-report boundary.

The [separate follow-up issue](../plans/ISSUE-Prompt-Report-Deliverables.md) records
the narrower ad-hoc prompt NO_CHANGES_PRODUCED limitation. That limitation is
not implemented or hidden in this commit. Scheduled declared report outputs
already have an exception; the prompt projection omits Deliverables.

## Exact commit file list

```text
docs/plans/ISSUE-Prompt-Report-Deliverables.md
docs/reports/PLAN-compiler-reconciliation-commit-review.md
docs/reports/PLAN-compiler-reconciliation-hardening.md
internal/planflow/compare.go
internal/planflow/compiler_reconciliation_test.go
internal/planflow/lifecycle_preservation_test.go
internal/planflow/planflow.go
internal/planflow/reconcile_hardening_test.go
internal/planner/capability.go
internal/planner/capability_dup_diagnostics_test.go
internal/planner/compiler_hardening_test.go
internal/planner/markdown.go
internal/planner/plan.go
internal/planner/planner.go
```

Keep these existing Clef changes out of this commit:

```text
docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md
docs/reports/clef-readiness/
internal/archtest/arch_test.go
internal/archtest/guard_test.go
internal/decision/decision.go
internal/decision/governance_test.go
```

Also exclude `.agent-sdlc/`, local binaries, raw temporary audit snapshots, and
SOP run artifacts. The normalized Clef source belongs with its separate Clef
workstream; the generic hardening tests carry their own portable fixtures.

## Proposed commit message

```text
fix(planner): stabilize compilation and preserve reconciliation history

Compile changed reconciliation sources without model normalization and compare
provable cosmetic variations conservatively. Preserve prerequisite ownership
checks, executed definitions, lifecycle states, and approval boundaries.

Cover repeated compilation, semantic changes, and history preservation. Record
the ad-hoc prompt report-deliverable limitation as a separate follow-up issue.

```

No commit or push is authorized by this preparation. Obtain explicit human
approval before either action; the index remains unchanged.

## Continue AS-CLEF-008 with the updated binary

From `/Users/imhttran/agentic-workspace/agentic-sop`, the exact resume inspection
command is:

```sh
/var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi/sop-hardening resume AS-CLEF-008
```

It reports `AS-CLEF-008 CREATE_BRANCH`; resume inspects recovery/next action and
does not itself execute implementation. To continue the existing governed plan
at the next ready task, use:

```sh
/var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi/sop-hardening run docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md
```

`run` starts with AS-CLEF-008 and may then advance to AS-CLEF-009/010. It preserves
SOP's existing approval/acceptance/commit gates; no new `--task` import, reset,
`--accept-changed`, or `--local` flag is needed. These execution commands were
identified but were not run during this review. The installed PATH binary is
older and was not replaced.

## Evidence

The detailed root cause, before/after reproduction, tests and limitations are in
[the implementation report](PLAN-compiler-reconciliation-hardening.md).
Current review logs, hash manifests, the isolated checkout, counterexample
audit, repeated previews/generated definitions, and history/readiness evidence
are retained locally at:
`/var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi/commit-review-hs6t85ij`.

## Documentation cleanup review incident

The three-document preparation
`prompt-20261007-020900` (`.agent-sdlc/runs/prompts/prompt-20261007-020900/report.md`)
completed PASS without committing. A subsequent two-line whitespace cleanup
`prompt-20261007-021122` (`.agent-sdlc/runs/prompts/prompt-20261007-021122/review.json`)
was rejected by automatic review because its full-workspace diff was attributed
to that narrow cleanup. The review suggested reverting pre-existing operator
work and raised a speculative architecture failure despite the passed suite.
The fix loop was interrupted; its outcome is not treated as a pass.
`sop-run.log: UNAVAILABLE`.

The interrupted fix loop had already changed
`internal/archtest/guard_test.go` and
`internal/planflow/compiler_reconciliation_test.go`. Preservation hash checks
caught both changes. Their original bytes were recovered from the preceding
SOP diff artifact and isolated hardening snapshot, respectively; each recovery
payload matched its original protected SHA-256 before restoration. The scoped
restoration preserves all existing compiler and Clef changes. The issue's
requested whitespace cleanup is retained. No executed Clef task state, attempts,
evidence, approvals, or acceptance metadata changed. Final verification checks
the restored files against the original manifests; no review rejection is
silently cleared or used as commit approval.

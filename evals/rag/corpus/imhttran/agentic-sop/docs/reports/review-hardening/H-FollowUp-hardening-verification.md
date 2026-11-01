# Follow-Up Hardening — Verification Report (H001..H004)

Status: verification report for the RSH Follow-Up Hardening plan
(`docs/plans/PLAN-RSH-Follow-Up-Hardening.md`, plan id `plan-rsh-follow-up-hardening`), including
the corrective task H004.

## 1. Scope

This report records the outcome of the residual-governance findings left open at RSH closure,
executed through the governed `sop run` workflow, and the corrective H004 task that completed
H001's acceptance evidence. It records the dependency graph, the concurrency decision, the
per-task acceptance status, the repository gates, and the residual risks.

## 2. Dependency graph and concurrency

```text
H001 (explicit malformed-output classification)   internal/cli/run.go
H002 (maintainable neutrality scope)              internal/archtest/*_test.go
H003 (editor-only Markdown reconcile fold)        internal/planflow/compare.go (+ tests)
H004 (complete H001 evidence + clean up)          internal/cli/run.go (+ malformed_output_test.go)
                                                  depends on: H001
```

**Concurrency decision.** Parallel worktree execution is **not supported** by the SOP CLI:
`internal/parallel` (which owns `GitWorkspaces`) is not referenced by any non-test file, and
`sop orchestrate` is a single-task, opt-in path that is disabled. The maximum safe concurrency is
therefore **1** — a single-writer, dependency-ordered `sop run`. H001..H003 ran sequentially; H004
was added to the active plan (see §5) and ran sequentially.

## 3. Per-task outcome

### H001 — Explicit malformed-output classification — **MET (completed by H004)**

- `internal/cli/run.go` adds an explicit `review.IsMalformedOutput(err)` check at the run-level
  review caller and a `malformedOutputResult` helper that classifies the outcome deterministically
  as `NEEDS_HUMAN` (`kind=UNKNOWN`, high confidence), preserving the provider's retained `Report`
  and mapping to `WAITING_FOR_HUMAN` — unless the autonomy policy forces a terminal action.
- The **focused tests** H001 originally omitted are now present in
  `internal/cli/malformed_output_test.go` (added by H004):
  | Test                                                       | Covers                                                                                                     |
  | ---------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
  | `TestMalformedOutputResultClassifiesViaIsMalformedOutput`  | classification via `review.IsMalformedOutput(err)` → `NEEDS_HUMAN`/`UNKNOWN`/`HIGH` at `WAITING_FOR_HUMAN` |
  | `TestMalformedOutputIsNeverPass`                           | malformed output never yields `PASS`, at every autonomy level                                              |
  | `TestOrdinaryReviewErrorPreservesBehavior`                 | an ordinary (non-malformed) review error is not misclassified and keeps its wrapped behavior               |
  | `TestMalformedRetainedReportDoesNotOverrideClassification` | a retained partial report does not change the disposition; its blocking finding is preserved               |
  | All four pass (`go test ./internal/cli/ -run 'Malformed    | OrdinaryReview'`).                                                                                         |

### H002 — Maintainable reviewer-neutrality scope — **MET**

- `internal/archtest/review_seam_neutrality_test.go` replaces the fixed file list with a
  documented, provider-neutral predicate over the production `.go` files of `internal/cli`, and
  asserts the predicate-derived set against the explicit reviewed list in both directions. The
  declared set now includes `internal/cli/run.go`.

### H003 — Editor-only Markdown formatting reconciliation — **MET**

- `internal/planflow/compare.go` folds a pure single-marker emphasis difference (`*x*` versus
  `_x_`) to one canonical form, only in the literal-free branch and only when the delimiter wraps
  non-space, non-underscore, word-boundary content. `internal/planflow/emphasis_test.go` covers the
  fold and every negative case (intra-word underscore, code/quotes, substantive changes).

### H004 — Complete H001 acceptance evidence and clean up the review seam — **MET**

- Added `internal/cli/malformed_output_test.go` (the four focused tests above).
- Removed the dead `activity.FromContext(...)` no-op from `malformedOutputResult`.
- Reverted the two unrelated `run.go` comments H001 had reformatted (`timedValidation`,
  `writeModelSelectionArtifact`); the `run.go` diff is now **pure insertions** (67 lines, no
  unrelated deletions).

## 4. Repository gates (consolidated change set)

| Command                             | Result            |
| ----------------------------------- | ----------------- |
| `gofmt -l .`                        | clean             |
| `go vet ./...`                      | PASS              |
| `go build ./...`                    | PASS              |
| `go test -count=1 ./...`            | PASS              |
| `go test -race -count=1 ./...`      | PASS              |
| `scripts/checks/check-doc-links.sh` | `broken links: 0` |
| `git diff --check`                  | clean             |

## 5. Validation-configuration hardening and lifecycle findings

- **`gofmt` added to SOP's validation configuration.** `.agent-sdlc/config.yaml` `validation.lint`
  now includes `test -z "$(gofmt -l .)"` (a bare `gofmt -l` always exits 0, so it is wrapped to
  gate on the file list). `sop validate` now runs and passes four checks including it. This
  mirrors the existing CI gofmt step and weakens no gate.
- **Lifecycle limitation (reported as requested).** A change whose only effect is to SOP's own
  `config.yaml` **cannot** be represented as a governed SOP task: `.agent-sdlc/` is gitignored
  (`.agent-sdlc/.gitignore` is `*`) and is excluded from the review diff, so such a task would
  show no attributable mutation and would not progress. The config change was therefore made
  directly by the operator's agent, not as a plan task. Adding a _code/evidence_ task to the active
  plan **is** supported: `sop reconcile` added H004 into the never-executed bucket with H001..H003
  unchanged and no human decision required.

## 6. `PLACEHOLDER_H001` investigation

H001's `.agent-sdlc/runs/H001/changed-files.json` records a non-path token `PLACEHOLDER_H001`
alongside `internal/cli/run.go`. Trace evidence shows the model **created and then deleted a file
literally named `PLACEHOLDER_H001`** (iterations 25 and 26: `phase=CHANGE`, `action=editing` then
`deleting`), and the create/delete did not cancel in the accumulated task change set.

- The string does not originate in the harness (no `PLACEHOLDER` identifier exists in tool or
  agent code); it is model-invented.
- It is therefore **not deterministically reproducible** and does not justify a change that would
  risk dropping legitimate deletions (a deleted file must stay in a task's change set).
- Disposition: **documented, not corrected.** No historical run evidence was modified.

## 7. Residual risks

1. **Local-only validation config.** The `gofmt` gate added to `.agent-sdlc/config.yaml` is not
   version-controlled (SOP's dir is gitignored). CI already enforces gofmt, but a fresh local
   checkout without this edit relies on CI only.
2. **`PLACEHOLDER_H001`.** Non-path tokens can persist in `changed-files.json` when a model
   creates and deletes a spurious file; harmless to the working tree, but it is noise in the
   evidence. Left as documented.
3. **`fail_on` is `critical`/`high` only.** Unresolved `MEDIUM`/`LOW` findings (e.g. H001's
   fix-loop nuance) can still ship without blocking.
4. **H003 scope.** Bold emphasis (`**x**` versus `__x__`) is not folded — only single markers are,
   per the acceptance criteria. Conservative: it can still raise a conflict, never hide a
   substantive change.
5. **Editor-on-save reformatting.** Both the plan Markdown and Go comment wrapping have been
   observed to be reformatted on save; H004 reverted the Go comment drift, but the hazard remains
   environmental.

## 8. Recommended closure disposition

H001 (as completed by H004), H002, H003, and H004 all satisfy their acceptance criteria, and every
repository gate passes on the consolidated change set. Recommended disposition: **request
approval for one scoped local commit and plan closure.**

Intended commit scope (the uncommitted RSH Follow-Up Hardening change set; the RSH-003..005 files
were already committed in `ba7736d`):

- `internal/cli/run.go` (H001 + H004)
- `internal/cli/malformed_output_test.go` (H004)
- `internal/archtest/review_seam_neutrality_test.go` (H002)
- `internal/planflow/compare.go` (H003)
- `internal/planflow/emphasis_test.go` (H003)
- `docs/plans/PLAN-RSH-Follow-Up-Hardening.md`
- `docs/reports/review-hardening/H-FollowUp-hardening-verification.md`

Excluded: `docs/specs/AGENT-PROVIDER.md` (pre-existing), all EV/CONV artifacts, and the gitignored
`.agent-sdlc/config.yaml` change (local-only).

No plan was superseded; no SOP state or database was edited by hand; nothing was committed or
pushed.

# PLAN — Issue 158: One Outcome Criterion Per Deliverable Line

**Status:** PROPOSED
**Rule:** A deliverable line names one requirement. SOP must create exactly one outcome criterion for that line, check every required repository path the line declares, and never treat a descriptive code symbol as a path. A missing required file still fails.

## Project

Issue 158 — deliverable path parsing

## Summary

The outcome checker currently extracts every backticked token from a deliverable line and creates a separate required presence criterion for each token that looks like a path. A token such as `ModelInfo.Locality` contains a dot and is therefore misclassified as a required file, producing a duplicate failing criterion and a false `PARTIAL` outcome.

The exact failing deliverable from issue #158 is:

```markdown
- `internal/provider/provider.go` — document that a non-empty `ModelInfo.Locality` is authoritative runtime evidence and that an empty value means undetermined.
```

The checker creates one criterion for the real file and a second for `ModelInfo.Locality`. The second fails with `deliverable "ModelInfo.Locality" not present`.

This change makes one criterion per deliverable line, separates path-like tokens from descriptive code symbols, and checks every required path within that line's criterion. It does not change task-definition digests, approval invalidation, retry budgets, or lifecycle policy.

## FIX-158-001 — One criterion per deliverable line with path-aware checks

Priority: P1. Reproduce the false duplicate criterion, then fix the parser and criterion generation without weakening missing-file detection.

### Dependencies

None

### Deliverables

- `internal/cli/outcome.go` — path/symbol classification, one criterion per deliverable line, and combined missing-path checks.
- `internal/cli/rag025_regression_test.go` or a new focused `internal/cli/outcome_deliverable_paths_test.go` — regression tests for the exact example, root-level paths, multiple paths, code symbols, and existing path forms.

### Acceptance Criteria

- Running the parser on the exact example yields only `internal/provider/provider.go`; `ModelInfo.Locality` is not returned as a path.
- `deliverableCriteria` creates exactly one required criterion for the exact example. Its description is the original deliverable line, preserving the requirement.
- With `internal/provider/provider.go` present, that criterion passes and does not fail for `ModelInfo.Locality`.
- With `internal/provider/provider.go` absent, that criterion fails and its error names `internal/provider/provider.go`; it does not name `ModelInfo.Locality`.
- A line declaring multiple required paths yields exactly one criterion. That criterion passes only when every path exists; when some are missing, the error names every missing path and does not name the present paths.
- Root-level files are recognized and checked: `Makefile`, `LICENSE`, and `go.mod` are paths; a missing root-level file fails. Recognition is syntactic, not dependent on the file currently existing.
- Descriptive code symbols (`ModelInfo.Locality`, `errors.New`, `fmt.Errorf`), commands such as `go test ./...`, and URLs such as `https://example.com/x` do not create paths or criteria.
- Existing supported path forms continue to work: directory paths (`docs/reference/`), the HARD-03 multi-path bullet, and a line-range form such as `internal/planner/markdown.go:164-166` (which checks `internal/planner/markdown.go`).
- Single-path deliverables keep one criterion per line, `missingDeliverables` still reports every missing path, and report-deliverable detection keeps using the same path rule.
- No task-definition digest, approval-invalidation, retry-budget, or lifecycle-policy behavior changes.

### Acceptance Checks

- `go test ./internal/cli/ -run 'TestDeliverable' -count=1`
- `go test ./internal/cli/...`
- `go vet ./internal/cli/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1
- criterion 7: check 1
- criterion 8: check 1
- criterion 9: check 2
- criterion 10: check 2

### Failure Behavior

If a token cannot be classified reliably, do not create a false required path. A deliverable line with no path-like token contributes no presence criterion, exactly as prose-only deliverables do today. Do not use current file existence to decide whether a token is a path: a missing required file must still fail. Preserve the original deliverable text as the criterion description so the recorded failure evidence stays meaningful.

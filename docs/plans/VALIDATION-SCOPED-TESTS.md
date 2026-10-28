# Scoped Task Validation with a Final Full-Suite Gate

**Type:** Design (implemented on `feat/scoped-validation`)
**Date:** 2026-10-28

## Problem

During a `sop run` that drives a multi-task plan, every task's VALIDATING stage runs
the full configured `validation.test` set. The session reuse cache
(`sessionValidation`, `internal/cli/run.go`) only hits when the command set and the
working-tree diff are byte-identical, and each new task changes the diff. A project
whose suite takes 10 minutes therefore pays that cost once per task, and again for
every fix cycle that changes code.

Turning on `verification.risk_based` does not help with this. `verificationTier` derives
the tier without JEV evidence (`router.SignalsFrom(nil, …)`), so an ordinary task is
always `MEDIUM`, which still runs the full `test` set. `LOW` applies only to
verify-first tasks.

## Goal

- Each task runs only the tests that cover the files it changed.
- The plan runs the full configured suite once, before it reports completion.
- Failed checks are never reused or bypassed, and an uncovered change always falls
  back to the full set.

Non-goals:

- **Reuse across `sop run` invocations.** VALIDATION.md §8 says reuse "MUST NOT cross a
  run". Changing that is a separate spec decision.
- **Automatic test discovery** (Cargo dependency graphs, Go import graphs). The
  operator declares the mapping. The design stays the same for every language.

## Configuration

A new optional `verification.scoped` list. Each rule maps path globs to the test
commands that cover them. It is independent of `risk_based`.

```yaml
verification:
  scoped:
    - paths: ["crates/projection-core/**"]
      test: ["cargo test -p projection-core"]
    - paths: ["crates/projection-cli/**", "crates/projection-cli/tests/**"]
      test: ["cargo test -p projection-cli"]
```

Glob syntax matches `quality.PostgresIntegrationRequired` (`**` spans segments), so
the existing `matchGlob` in `internal/quality/postgres_policy.go` is exported and
reused rather than duplicated.

Config load rejects a rule with no `paths` or no `test`. Omitting the block changes
nothing.

## Selection Rule

A pure function next to `selectValidation`:

```go
// scopeValidation narrows v.Test to the scoped commands covering changed.
func scopeValidation(cfg config.Config, v config.Validation, changed []string) config.Validation
```

1. If `scoped` is empty, `changed` is empty, or the tier is `HIGH`, return `v`
   unchanged.
2. Every changed path must match at least one rule. If any path matches no rule,
   return `v` unchanged. This is the fail-safe: an unmapped change runs the full set.
3. Otherwise set `v.Test` to the union of the matched rules' `test` commands,
   deduplicated, in config order. `Build` and `Lint` are untouched. Scoping only ever
   narrows tests, never the compile or lint gate.

`changed` is the task's own change set (`rn.ChangedFiles()`, the files
`invocationChanges` attributed to this task), not the whole working tree. Changes left
by earlier tasks in the same plan are covered by those tasks and by the final gate.

## Where It Runs

`vset` is currently computed once (`run.go:578`), before implementation, when the
change set is still unknown. The design keeps that line and narrows `vset` at the two
validation call sites, right after the change set is recorded:

- verify-first pre-check (`run.go:640`)
- the validate step in the fix loop (`run.go:883`)

```go
suite = sessionValidation(ctx, dir, sess.scope(cfg, vset, tier, rn.ChangedFiles()), diff, rec, sess)
```

The cache needs no change. `validationIdentity` already hashes the command set, so a
scoped set and a full set never share a key.

**Scoping only applies inside a plan drive.** Single-task `sop run`, `sop prompt`, and
`sop eval` have no final gate, so they keep the full set. `runSession` gains one field,
`scoped bool`, which only `drive.go` sets. `sess.scope` returns `vset` unchanged when
it is false or nil. No new parameter is threaded through the call chain.

## Final Full-Suite Gate

In `drive.go`, after `driveGraph` returns `exitOK` and `domain.AllSatisfied(final)`,
and before `printCompletion`:

```go
if len(cfg.Verification.Scoped) > 0 {
	diff, _ := d.readDiff(ctx, dir)
	suite := sessionValidation(ctx, dir, cfg.Validation, diff, rec, sess)
	if !suite.Passed() { /* print validationFailureContext(suite); return exitError */ }
}
```

- The gate runs the full `cfg.Validation` set (build, test, lint, integration) against
  the final tree. If the last task already ran the full set on the same tree, the
  session cache returns that pass and nothing runs twice.
- On failure the run exits non-zero. No completion is printed and no plan is
  historicalized. Tasks stay satisfied, and the failure output names the failing
  command.
- The gate does not depend on any state carried across invocations. Running
  `sop run` again finds every task satisfied, skips the task loop, and retries the
  gate. A plan therefore cannot complete without one full passing suite, even when
  earlier invocations stopped part-way.

**Open question:** should a failing gate also open a task or a human gate, so the next
`sop run` sends a fix agent instead of only re-running the suite? The first version
only fails closed. An automatic repair loop can come later.

## Cost Model (example: 6 tasks, 10-minute suite)

| | Today | Scoped |
|---|---|---|
| Per task | 10 min × 6 | scoped crate tests × 6 |
| Final | — (last task's run) | 10 min × 1 |
| Unmapped change | 10 min | 10 min (fallback) |

## Safety Properties

- A failing suite is never cached (`cacheValidation` stores passes only), so it is
  never reused.
- A change with no matching rule runs the full set.
- `Build` and `Lint` always run in full.
- The quality gate still decides PASS/FAIL from the observed results (QUALITY.md). Only
  the command set changes.
- A plan cannot report completion without the full suite passing on its final tree.

Known limit: a scoped rule that is too narrow (for example, it omits a dependent
crate's tests) lets a task pass on its own tests and fail at the final gate. That
failure is late but never silent.

## Change List

| File | Change |
|---|---|
| `internal/config/verification.go` | `Scoped []ScopedRule` plus load-time validation |
| `internal/quality/postgres_policy.go` | export the glob matcher |
| `internal/cli/verification_tier.go` | `scopeValidation` |
| `internal/cli/session.go` | `scoped` field and `scope` method |
| `internal/cli/run.go` | narrow `vset` at the two validation call sites |
| `internal/cli/drive.go` | set `sess.scoped`, add the final gate |
| `docs/specs/VALIDATION.md` | §10 Scoped validation and final gate |
| `docs/reference/CONFIGURATION.md` | `verification.scoped` reference |

Tests:

1. A table test for `scopeValidation`: all covered, one uncovered, empty change set,
   `HIGH` tier, and overlapping rules deduplicated.
2. One drive test: two tasks under scoped rules, then a final-gate failure leaves no
   completion and returns a non-zero exit.

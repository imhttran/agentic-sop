# RSH-001 — Baseline: Review Seam and Change Attribution

This is a **read-only baseline** documentation stage of the RSH (Review Seam
Hardening) track. It records the exact current construction of the review scope,
the review call site, the severity-rejection path, and the error-to-classification
path as they exist in the working tree, anchored to concrete file/line evidence.
No production change is made. The only addition is this report.

All line numbers below were confirmed by inspecting the named files in the current
working tree.

## 1. Review-scope construction (the diff handed to the reviewer)

### 1.1 Where the diff is read

The single source of the review-scope text is the dependency-injected `readDiff`
hook. The CLI resolves it into the run's `deps`. In the lifecycle it is read at:

- **`internal/cli/run.go:648-651`** (verify-first pre-read, before validation):
  `diff, err = d.readDiff(ctx, dir)` with the `diff: %w` error wrap.
- **`internal/cli/run.go:717-720`** (after IMPLEMENT, before mutation attribution):
  `diff, err = d.readDiff(ctx, dir)` with the `diff: %w` error wrap.
- **`internal/cli/run.go:911-914`** (after each FIX cycle, so the next
  classification and review see the fix's tree): `diff, err = d.readDiff(ctx, dir)`.
- **`internal/cli/run.go:1000`** writes the read diff to the run artifact
  `diff.patch` (`_ = rn.Write("diff.patch", diff)`), the durable review-scope record.

### 1.2 How the diff reaches the reviewer: `d.readDiff` is wrapped to append

the declared report deliverables

In `runStages`, when the task declares report deliverables, the harness wraps
`d.readDiff` so the reviewer also sees the declared-deliverable diff:

- **`internal/cli/run.go:596-607`**:

```go
if len(reports) > 0 {
    readDiff := d.readDiff
    d.readDiff = func(ctx context.Context, dir string) (string, error) {
        diff, err := readDiff(ctx, dir)
        if err != nil {
            return "", err
        }
        declared, err := git.New(dir).DiffFiles(ctx, reports...)
        return diff + declared, err
    }
}
```

So the **review scope = working-tree diff (`readDiff`) + declared-deliverable diff
(`git.New(dir).DiffFiles(ctx, reports...)`)**. `reports` itself comes from
`taskReportDeliverables(spec)` (`internal/cli/run.go:590`).

### 1.3 The gate that decides whether review runs at all

The review block is entered only when validation passed, the run is not a
verify-first pass, and the diff is non-empty:

- **`internal/cli/run.go:760-763`**:

```go
if suite.Passed() && !verifiedFirst && strings.TrimSpace(diff) != "" {
    reviewTask := d.taskInput(spec.ID, spec.Render())
    reviewKey := reviewIdentity(cfg.Review.Engine, reviewTask, diff)
```

`reviewKey` (same engine + task + diff) is the review reuse identity; a cache hit
skips the call and reuses the cached report (`internal/cli/run.go:767-769`).

## 2. Review call site — `run.go:778`

The review provider is invoked at **`internal/cli/run.go:778`**:

```go
report, err = provider.Review(ctx, review.Request{Task: reviewTask, Diff: diff})
```

Surrounding context (confirmed verbatim):

- **`internal/cli/run.go:773`** — provider selection: `provider, perr := reviewProvider(cfg, d)`.
- **`internal/cli/run.go:774-776`** — a provider-selection error returns
  `fmt.Errorf("review: %w", perr)`.
- **`internal/cli/run.go:777`** — `revStop := rec.Measure(perf.StageReview)`.
- **`internal/cli/run.go:778`** — the call shown above.
- **`internal/cli/run.go:779-783`** — `revStop()`, `rec.ReviewRun()`, and on error
  `return lifeResult{}, fmt.Errorf("review: %w", err)`.
- **`internal/cli/run.go:784`** — `sess.cacheReview(reviewKey, report)`.

The provider returned by `reviewProvider` (`internal/cli/review.go:89-103`) is
`review.NewAgentProvider(a)` for the default engine, or
`review.ProviderFromEnv()` for the `open-code-review` engine.

## 3. Severity-rejection path — `provider.go:73-75`

The review output is parsed by `parseReport`. A finding whose severity is not one
of the five known severities is **rejected as an error, never silently passed**:

- **`internal/review/provider.go:73-75`**:

```go
if !severity.Valid() {
    return Report{}, fmt.Errorf("review finding %d has invalid severity %q", i, f.Severity)
}
```

This is the severity-rejection path. `Severity.Valid()` is defined in
`internal/review/review.go:27-30` against the `severityRank` map
(`internal/review/review.go:19-25`: `INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`).
A malformed severity therefore becomes a `parseReport` error, which propagates
out of `AgentProvider.Review` (`internal/review/provider.go:31-51`, the
`parseReport(resp.Content)` error return at lines 58-62) and back to the review
call site at `run.go:778`.

The parser is documented as fail-closed: `parseReport`'s doc comment
(`internal/review/provider.go:63-64`) states "Malformed output or an unknown
severity is an error, never a silent pass." JSON normalization of the raw model
content is performed first by `normalize.JSON(content)`
(`internal/review/provider.go:66-69`), so malformed JSON and malformed severity are
both hard errors.

## 4. Whole-review abort behavior

A review error aborts the **whole review** for the task, not just the offending
finding:

- The provider call site returns immediately on error —
  **`internal/cli/run.go:781-783`**:

```go
if err != nil {
    return lifeResult{}, fmt.Errorf("review: %w", err)
}
```

- Any single malformed finding (including one empty/unknown severity, per §3)
  makes `parseReport` return `(Report{}, err)`, so the entire `Report` is thrown
  away — there is no partial report and no per-finding skip.
- The error propagates out of `runStages` to `executeLifecycle`
  (`internal/cli/run.go:472-479`), which records `classification.json` when one
  was attached, writes the run trace, and returns the partial result **alongside**
  the error (`return res, err`).
- `runSingleTask` (`internal/cli/run.go:203-211`) and `runScheduledTask`
  (`internal/cli/drive.go:566-603`) turn that error into a failed run:
  `failRun(rn, stderr, err)` in single-task mode
  (`internal/cli/run.go:208`), or the error path in the graph driver
  (`internal/cli/drive.go:596-601`).

So a malformed-severity (e.g. empty-severity) review aborts the review and stops
the run; it never becomes an empty `Report{}` that the gate would read as "no
findings".

## 5. Error-to-classification path — `drive.go:736-744`

When the lifecycle returns an error, the graph driver classifies it and applies
the autonomy policy. The exact block:

- **`internal/cli/drive.go:736-744`**:

```go
if err != nil {
    cls := res.classification
    if cls.Disposition == "" {
        cls = failure.Classify(failure.Evidence{Source: "run", Err: err, Approval: approval})
        decision := decideAutonomy(cfg, cls)
        writeClassificationArtifact(rn, cls, decision)
        emitClassificationActivity(ctx, cls, decision)
        res.decision = decision
    }
```

If the lifecycle **already** attached a classification, it is preserved; otherwise
the driver classifies the raw error with `failure.Classify` (evidence source
`"run"`), computes the autonomy decision with `decideAutonomy`, persists it
(`classification.json` via `writeClassificationArtifact`), and emits it on the
activity stream.

## 6. Error → `NEEDS_HUMAN` path

After the classification block above, the driver routes the error:

- **`internal/cli/drive.go:745-756`**:

```go
fmt.Fprintf(stderr, "%s: %v\n", task.ID, err)
if humanBoundary(res.stage, cls, res.decision) {
    recordHumanApprovalRequest(ctx, rn, task, res.stage, cls.Disposition,
        firstNonBlank(cls.Reason, err.Error()), cls.Reason)
    parked = true
}
if cls.Retryable() {
    return recoverTask(saver, task, rn, "ERR|"+err.Error()+"|"+string(cls.Disposition), string(cls.Disposition), cls.Disposition, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
}
if policyForcesHuman(cls, res.decision) {
    return recoverTask(saver, task, rn, "ERR|NEEDS_HUMAN|"+err.Error(), "NEEDS_HUMAN", failure.NeedsHuman, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
}
_ = rn.SetStage(runpkg.Failed)
_ = blockTask(saver, task, domain.RETRIES_EXHAUSTED)
return exitError
```

Three dispositions are visible:

1. **Human boundary** — `humanBoundary(...)` records an explicit, resolvable
   approval request (`recordHumanApprovalRequest`) and parks the run; the gate is
   left behind for `sop approvals` / `sop approve`.
2. **Retryable** — a transient/continuation disposition is requeued through the
   existing bounded path (`recoverTask`).
3. **Policy-required human** — `policyForcesHuman(cls, res.decision)` requeues the
   task with the literal `"NEEDS_HUMAN"` label and `failure.NeedsHuman` disposition
   (`internal/cli/drive.go:753-755`), which returns the task to PLANNED for human
   resolution and prints the parked gate.
4. Otherwise the task is terminally **BLOCKED** with `RETRIES_EXHAUSTED`
   (`internal/cli/drive.go:757-759`).

A non-error non-pass outcome reaches the same `NEEDS_HUMAN` vocabulary through the
`res.stage == runpkg.WaitingForHuman || policyForcesHuman(...)` branch at
**`internal/cli/drive.go:621-626`**, which likewise calls `recoverTask` with the
`"NEEDS_HUMAN"` label and `failure.NeedsHuman` disposition. Both paths converge on
the single bounded requeue mechanism (`recoverTask`,
`internal/cli/drive.go:646-687`), so an error and a deterministic human boundary
cannot diverge in how they park.

Note that a *terminal* autonomy decision (`autonomy.ActionTerminal`) is handled
*before* the human branch (`internal/cli/drive.go:604-617`): bounded automation
exhaustion blocks the task rather than parking it for a human.

## 7. EV-003 evidence anchors

### 7.1 Contaminating-file evidence

The EV-003 retrieval evaluation (`docs/reports/ev-context-run/EV-003-retrieval-evaluation.md`)
records the file set that contaminates lexical retrieval — repository files whose
names collide with the query terms and therefore surface above the relevant
evidence. The concrete contaminating anchors are:

- **`file:internal/repoindex/index.go`** — top result for the `single_file` case
  (score `14.257646`, reason `lexical match: index, repoindex`); EV-003 §4 row 1.
- **`file:internal/retrievalgate/gate.go`** — top result for the
  `existing_test_discovery` case (score `15.128327`, reason
  `lexical match: gate, retrievalgate`); EV-003 §4 row 3.
- **`file:docs/reports/ev-context-run/EV-003-retrieval-evaluation.md`** — top result
  for the `documentation_and_code` case (score `15.947890`, reason
  `lexical match: context, evaluation, retrieval`); EV-003 §4 row 6.
- **`file:internal/retrievalgate/ev003_live_test.go`** — top result for the
  `bug_fix` case (score `6.927380`, reason `lexical match: retrievalgate`);
  EV-003 §4 row 7.

These are the contaminating files EV-003 cites: repository-local files (index,
gate, the report itself, and the harness) that win the BM25 rank purely on lexical
overlap with the query. The report also names the symbol-level contaminants
`method:.../internal/retrieval.Index.Search` and
`function:.../internal/retrievalgate.Measure` (EV-003 §4 rows 2, 4, 5, 8, 9).

### 7.2 Empty-severity / malformed-severity abort

The EV-003 evidence does **not** by itself exercise the review severity parser.
The malformed/empty-severity abort it anchors is the severity-rejection path in
§3: a finding whose severity is empty or unknown is rejected by
**`internal/review/provider.go:73-75`** with
`review finding %d has invalid severity %q`, aborting the whole review (§4). This
is the abort the baseline records as the empty-severity behavior: it is a hard
error from `parseReport`, never a silent empty `Report{}`.

The connection to EV-003's contaminating-file evidence is the shared failure
mode the RSH track exists to harden: contaminated/lexically-overlapping scope (the
EV-003 files above) and malformed model output (empty severity) both must fail
closed at the review seam rather than degrade into a clean-looking review. This
baseline pins both anchors so later RSH stages can diff against them.

## 8. Anchor summary

| Anchor | File:line | What it is |
| --- | --- | --- |
| Review-scope construction | `internal/cli/run.go:596-607` | `readDiff` wrapped to append `git.New(dir).DiffFiles(ctx, reports...)` |
| Diff reads | `internal/cli/run.go:648-651`, `717-720`, `911-914` | `d.readDiff` source of review scope |
| Diff artifact | `internal/cli/run.go:1000` | `diff.patch` persisted |
| Review gate | `internal/cli/run.go:760-763` | review only when validation passed, not verify-first, diff non-empty |
| Review call site | `internal/cli/run.go:778` | `provider.Review(ctx, review.Request{Task, Diff})` |
| Severity rejection | `internal/review/provider.go:73-75` | invalid/empty severity → hard error |
| Severity validity | `internal/review/review.go:19-30` | `severityRank`, `Severity.Valid()` |
| Whole-review abort | `internal/cli/run.go:781-783` | review error returns from `runStages` |
| Error classification | `internal/cli/drive.go:736-744` | classify error, persist, emit |
| Error → NEEDS_HUMAN | `internal/cli/drive.go:745-759` | human boundary / retryable / policy-human / block |
| Non-error → NEEDS_HUMAN | `internal/cli/drive.go:621-626` | `res.stage == WaitingForHuman \|\| policyForcesHuman` |
| EV-003 contaminating files | `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md` §4 | index.go, gate.go, EV-003 doc, ev003_live_test.go |
| Malformed-severity abort | `internal/review/provider.go:73-75` | empty/unknown severity aborts review |

## 9. Scope discipline

- **No production change** was made by this stage. The only repository addition
  is this report.
- No runtime path, review semantics, gate policy, or autonomy behavior was
  modified.
- The anchors above are recorded exactly as inspected; any later drift is a
  future-stage finding, not silently corrected here.

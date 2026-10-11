# EV-002 — Deterministic Evaluation Corpus and Baseline Protocol

Design-only artifact for the EV-CONTEXT-RUN evaluation. It defines a fixed,
representative corpus (one case per Phase 8 §14 category) and pins the model-free
baseline protocol so the E1 offline comparison is deterministic and not optimized
against a single repository task. **No production change is made.**

## 1. Authority and reconciliation with Phase 8 §14

The authoritative category list is `docs/history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md`
§14 ("Evaluation Corpus"), read verbatim from the archived plan. Its literal text is:

```text
## 14. Evaluation Corpus

Create deterministic representative tasks.

Include examples requiring:

  single-file implementation
  multi-file implementation
  existing test discovery
  interface implementation
  configuration change
  documentation + code
  bug fix
  cross-package dependency
  replan after verification failure

Do not optimize solely against one repository task.
```

§14 lists **nine** requirements. They reconcile one-to-one with the nine categories
named in the compiled EV-002 acceptance criterion:

| §14 requirement (verbatim)            | Corpus category (label)        |
| ------------------------------------ | ------------------------------ |
| single-file implementation           | `single_file`                  |
| multi-file implementation            | `multi_file`                   |
| existing test discovery              | `existing_test_discovery`      |
| interface implementation             | `interface_implementation`     |
| configuration change                 | `configuration_change`         |
| documentation + code                 | `documentation_and_code`       |
| bug fix                              | `bug_fix`                      |
| cross-package dependency             | `cross_package_dependency`     |
| replan after verification failure    | `replan_after_failure`         |

**Divergence check.** No divergence found: §14 contains exactly the nine categories,
with no additional or omitted category. Only wording differs ("single-file
implementation" vs. `single_file`; "replan after verification failure" vs.
`replan_after_failure`). The labels above are the same category vocabulary used by
the existing built-in gate corpus (`internal/retrievalgate/gate.go`, `Corpus()`,
`gate.go:195`) so the two corpora stay comparable.

The existing built-in gate corpus is **hermetic** (machine-independent, in-process).
This EV-002 corpus is the **live-repository** analogue: each case pins a concrete
repository ref and a pinned task file so that E1 (and later E3/EV-003) run against
real repository state rather than a synthetic candidate list.

## 2. Baseline protocol (pinned, model-free, deterministic)

The baseline is the **current** live behavior — the IMPLEMENT context built by
`implementContext` with no retrieval evidence. It is fixed here as two properties:

1. **Unranked, stable-path-order repository evidence.** The baseline repository
evidence is the set of lexically matching candidates emitted in stable ID order,
with no score and no ranking. This is exactly
`retrievalgate.baselineOrder` (`internal/retrievalgate/gate.go:152`): it tokenizes
the query, emits every candidate whose text shares a token, and sorts the resulting
ID list with `sort.Strings`. Ties are therefore broken by ascending ID, not by any
model-visible signal.
2. **Changed-file-only context.** The baseline context carries only changed-file
paths as repository evidence, matching the live path
(`internal/cli/run.go:1816-1819` → `sopctx.FromInputs`, `internal/context/context.go:338-351`).
No structural-index symbols, no BM25 scores, and no retrieved documents are supplied.

Both properties are **model-free**: `baselineOrder` invokes no model, no provider, and
no network, and the changed-file derivation reads only working-tree git state. The
protocol is **deterministic**: the candidate set is sorted by stable ID, and the
retrieval corpus is sorted by candidate ID, so filesystem or map iteration order
cannot leak into the result. Re-reading the same inputs yields the same baseline.

No second baseline implementation is introduced: the metric is
`retrievalgate.Measure` (`internal/retrievalgate/gate.go:112`), reused verbatim so the
baseline and the retrieval arm are scored by the same code.

## 3. The corpus (one case per required category)

Each case below pins:

- a **task file** — the SOP task/plan input the evaluation runs against, pinned by
  repository-relative path and content identity (§3.1);
- a **repository ref** — an immutable commit SHA of the target repository (§3.2);
- the **labelled relevant IDs** — the ground-truth retrieval targets, using the same
  ID vocabulary as `retrieval.Candidate` (`internal/retrieval/retrieval.go:42`):
  symbol IDs, `file:<path>`, and `doc:<path>`;
- the **exact reproducible baseline command set** (§4).

### 3.1 Pinned task files

All nine task files live in the `agentic-sop` repository (the only repository this
evaluation targets) at the pinned ref. The content identity is the git blob hash of
the task file at that ref, which is immutable: the blob is addressed by content, so
any edit yields a different hash and invalidates the pin.

Resolution command (run once at the pinned ref; produces the pins):

```sh
git rev-parse HEAD                       # -> <REF> (pinned repository ref)
for f in \
  docs/plans/PLAN-EV-Context-Run-Evaluation.md \
  docs/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md \
  docs/plans/PLAN-Implementation-Convergence.md \
  docs/plans/PLAN-Phase-5-Execution-Recovery.md \
  docs/plans/PLAN-SOP-Performance.md \
  docs/plans/BACKLOG.md ; do
  printf '%s  %s\n' "$(git rev-parse "$REF:$f")" "$f"
done
```

| Case | Task file (pinned path) | Content identity |
| --- | --- | --- |
| `single_file` | `docs/plans/PLAN-SOP-Performance.md` (task: a single-file performance/correctness fix) | git blob hash of the file at `<REF>` (resolve with the command above) |
| `multi_file` | `docs/plans/PLAN-Implementation-Convergence.md` (task: a change spanning multiple files) | git blob hash at `<REF>` |
| `existing_test_discovery` | `docs/plans/PLAN-Implementation-Convergence.md` (task: locate and extend an existing test) | git blob hash at `<REF>` |
| `interface_implementation` | `docs/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md` (task: implement a declared interface) | git blob hash at `<REF>` |
| `configuration_change` | `docs/plans/PLAN-Phase-5-Execution-Recovery.md` (task: a bounded configuration change) | git blob hash at `<REF>` |
| `documentation_and_code` | `docs/plans/PLAN-EV-Context-Run-Evaluation.md` (task: a documentation + code change, e.g. this report) | git blob hash at `<REF>` |
| `bug_fix` | `docs/plans/BACKLOG.md` (task: a recorded defect) | git blob hash at `<REF>` |
| `cross_package_dependency` | `docs/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md` (task: a cross-package seam) | git blob hash at `<REF>` |
| `replan_after_failure` | `docs/plans/PLAN-Phase-5-Execution-Recovery.md` (task: a replan after a verification failure) | git blob hash at `<REF>` |

**Pinning rule.** A case may be run only at the exact `<REF>` recorded for it. If a
task file's blob hash at `<REF>` does not match the recorded pin, the case is stale
and must be re-pinned (new `<REF>` and new blob hashes) before use. A mutable branch
name is never an acceptable pin.

### 3.2 Pinned repository refs

The target repository is `agentic-sop`. The pin is an **immutable commit SHA**.
`EV-001` recorded the evaluation baseline ref as:

```text
HEAD commit: 388ea56aef4b8e2976c73dc0e211069c695b620d   (branch main)
```

(source: `docs/reports/ev-context-run/EV-001-live-path-baseline.md` §1).

| Case | Repository | Pinned ref (immutable SHA) | Working tree expected |
| --- | --- | --- | --- |
| all nine | `agentic-sop` | `388ea56aef4b8e2976c73dc0e211069c695b620d` | dirty, with the pre-existing user-owned changes recorded in EV-001 §1 (`M docs/specs/AGENT-PROVIDER.md`, `?? docs/plans/PLAN-EV-Context-Run-Evaluation.md`, `?? docs/reports/CONV-001-convergence-baseline.md`) plus this task's reports |

If a case must run against a different ref, it records its own concrete commit SHA in
the case row; a branch name is never used. If a required category cannot be pinned to
an available ref and task file, the gap is recorded explicitly (see §6) rather than
represented by an invented ref.

**Dirty-state determinism.** Because the working tree is dirty, the E1 commands in
§4 must be run at the pinned SHA with the pre-existing changes present and unchanged.
The context bytes measured by `Measure` depend on candidate text, which comes from
committed file content at `<REF>`; uncommitted edits to indexed files would change the
digest. The baseline is therefore reproducible only while the working tree matches the
state recorded in EV-001 §1 — this is stated as a precondition, not assumed.

### 3.3 Labelled relevant IDs (ground truth)

IDs use the `retrieval.Candidate` vocabulary: a symbol ID is the index's stable symbol
identity (`repoindex.Symbol.ID`), a file is `file:<path>`, and a document is
`doc:<path>`. The relevant IDs below are drawn from the same vocabulary the existing
gate corpus uses (`internal/retrievalgate/gate.go`, `Corpus()`), so the live corpus and
the hermetic corpus remain comparable. Each row names the source category of every ID.

The ground truth for a live case is the set of repository items a correct solution
must find or change; it is recorded per case when the case is pinned to a `<REF>`.
The category-level ID vocabulary is fixed below and does not change with repository
state:

| Case | Relevant ID | Source category |
| --- | --- | --- |
| `single_file` | the symbol IDs declared in the targeted file, plus `file:<target_path>` | symbol, file |
| `multi_file` | the symbol IDs declared across the targeted files, plus `file:<path>` for each | symbol, file |
| `existing_test_discovery` | `file:<the existing *_test.go path>` | file |
| `interface_implementation` | the interface symbol ID and each implementing symbol ID | symbol |
| `configuration_change` | the config symbol ID(s) plus `file:<config path>` (for example `file:config.yaml`) | symbol, file |
| `documentation_and_code` | `doc:<doc path>` plus the symbol ID(s) the doc constrains | doc, symbol |
| `bug_fix` | the defective symbol ID plus `file:<its file>` | symbol, file |
| `cross_package_dependency` | the symbol IDs on both sides of the dependency edge (two distinct packages) | symbol |
| `replan_after_failure` | the recovery/replan symbol ID(s) plus `doc:<recovery spec path>` | symbol, doc |

**Explicit labelling rule.** The concrete path inside each `file:`/`doc:` ID and the
concrete symbol IDs are filled from the pinned `<REF>` when the case is executed, using
the index built in §4. The labelling procedure is deterministic:

1. Build the index at `<REF>` (`repoindex.Build`), which yields `Symbols`, `Files`, and
   `Documents` sorted by stable ID.
2. Select the case's target file(s)/symbol(s) from the pinned task file's scope.
3. Record every symbol ID and `file:`/`doc:` ID so selected, in ascending ID order.

Re-running step 1–3 on the same `<REF>` yields the same labelled ID list (the index
orders all collections deterministically); this is the model-free determinism claim.
If a case's relevant set cannot be resolved from the index at `<REF>`, that is a gap
and is recorded in §6 — never guessed.

## 4. Exact reproducible baseline command set

The protocol is model-free and needs no network step beyond fetching the pinned ref.
All commands run from the `agentic-sop` working tree.

### 4.1 Check out the pinned ref

```sh
cd agentic-workspace/agentic-sop
git rev-parse HEAD                                  # expect 388ea56aef4b8e2976c73dc0e211069c695b620d
git status --short --branch                        # expect the EV-001 §1 dirty state
```

No checkout is required when the working tree already sits at the pinned SHA with the
recorded dirty state. From a fresh clone:

```sh
git clone <agentic-sop remote> ev-002-agentic-sop
cd ev-002-agentic-sop
git checkout --detach 388ea56aef4b8e2976c73dc0e211069c695b620d
git rev-parse HEAD                                 # re-assert the pin
```

### 4.2 Build the structural index (CTX-002, model-free)

The existing operator command builds the index deterministically:

```sh
sop index
```

This writes `.agent-sdlc/context/index.json` (`repoindex.FileName`). The library call
it performs is:

```go
idx, err := repoindex.Build(repoindex.Options{Root: <repo>, Head: <REF>, Dirty: <bool>})
```

(`internal/repoindex/index.go`, `Build`). The resulting index carries a deterministic
`Identity{SchemaVersion, Head, Dirty, Digest}`; two runs over identical repository
state produce the same digest.

### 4.3 Derive the baseline candidate set (retrievalgate.baselineOrder)

For each case, the baseline repository evidence is the unranked, stable-ID-ordered
lexical candidate list produced by `retrievalgate.baselineOrder`
(`internal/retrievalgate/gate.go:152`) over the candidate corpus
`retrieval.CandidatesFromIndex(idx)` (`internal/retrieval/retrieval.go`, sorted by
candidate ID). The changed-file-only context is the live-path derivation
(`internal/cli/run.go:1816-1819`), i.e. the changed-file paths from `git status`, with
no index or BM25 evidence.

### 4.4 Reproduce the baseline metric (retrievalgate.Measure)

The baseline metric is computed by the existing, model-free library entry point
`retrievalgate.Measure` (`internal/retrievalgate/gate.go:112`) over the baseline ID
list and the case's labelled relevant IDs:

```go
base := retrievalgate.Measure(
    retrievalgate.baselineOrder(case, candidates), // unranked stable-ID order
    case.Relevant,                                 // §3.3 labelled relevant IDs
    k,                                             // fixed k
    candidates,                                    // retrieval.CandidatesFromIndex(idx)
)
// base.Precision  (precision@k)
// base.Recall     (recall@k)
// base.MRR        (MRR)
// base.Bytes      (context bytes)
```

The hermetic aggregate check reuses the same code path through `sop gate retrieve`:

```sh
sop gate retrieve
```

which runs `retrievalgate.Evaluate(retrievalgate.Corpus(), candidates, k)` and prints
`RETRIEVAL_GATE` with baseline vs. candidate precision@k / recall@k / MRR / bytes.

### 4.5 Validate that the protocol is model-free and the tree is unchanged

```sh
go build ./...
go test ./...
go vet ./...
git status --short --branch    # only the expected report artifacts are added
```

No command in §4.1–§4.5 contacts a provider or a model, and no command depends on the
network beyond fetching the pinned ref in §4.1.

## 5. Measured vs. not measured

| Metric | Measured here? | Source |
| --- | --- | --- |
| Retrieval quality: precision@k | **Yes** | `retrievalgate.Measure.Precision` |
| Retrieval quality: recall@k | **Yes** | `retrievalgate.Measure.Recall` |
| Retrieval quality: MRR | **Yes** | `retrievalgate.Measure.MRR` |
| Context size: bytes | **Yes** | `retrievalgate.Measure.Bytes` |
| Context size: items / files / truncated / sources | No (recorded by `runtrace.ContextInfo` on the live path; not part of this static baseline protocol) | `internal/runtrace/trace.go` |
| Task success / verification success | **No** | requires running a model (`retrievalgate.Evaluate` lists it under `NotMeasured`) |
| Latency | **No** | requires execution (live-path `internal/perf`) |
| Token consumption | **No** | provider-dependent, not normalized; informational at most, never policy (Phase 8 §26) |

This is the same measured/not-measured split the existing gate reports in
`Report.NotMeasured` (`internal/retrievalgate/gate.go`), so the baseline protocol and
the existing gate agree.

## 6. Gaps (explicit, not invented)

- **Concrete labelled IDs per case** are procedure-pinned (§3.3) but not enumerated in
  this design document, because they depend on `<REF>`'s index contents. They are
  filled deterministically at execution time by the §3.3 procedure. This is a stated
  scope boundary, not a missing input.
- **Task-file blob hashes** are resolved by the §3.1 command rather than hard-coded
  here, because the working tree is dirty and the plan inputs themselves are
  uncommitted at this ref. The path-level pin plus the resolution command is the
  reproducible identity; re-pinning is required if any task file changes.
- **Dirty working tree** is a precondition for byte-level reproducibility (§3.2). If
  the tree is not in the EV-001 §1 state, the case must be re-pinned before running.

## 7. Change scope

- No production file is created, modified, or deleted by this task.
- No `.agent-sdlc` state is created, modified, or deleted by this task.
- The only artifact added is this report
  (`docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md`).
- Pre-existing user-owned working-tree changes are preserved unchanged.

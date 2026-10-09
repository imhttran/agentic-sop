# SOP Plan Compiler — Determinism and Reconciliation Hardening

Completed locally on 2026-10-06 (America/Chicago; verification recorded on
2026-10-07 UTC). The updated CLI reconciled the existing Clef readiness plan:
only pending AS-CLEF-008 was updated. AS-CLEF-001–007 definitions and historical
state were preserved, including AS-CLEF-004 `NOT_REQUIRED`. All six requested
verification gates passed. No commit, push, adapter implementation, task
acceptance, or automatic acceptance of executed changes occurred.

## Root cause and evidence recorded before behavior changes

The primary variability came from **model normalization and its repair rounds**,
activated repeatedly by a changed source fingerprint. Literal comparison of the
model's newly worded definitions then produced unnecessary `changed_executed`
entries. JSON serialization was not the observed source of randomness.

The investigation captured the original source, recorded machine plan/metadata,
read-only SQLite rows, and historical file hashes before implementation. The
first governed implementation recorded these findings in this report before
changing compiler or reconciliation behavior.

The traced original call path was:

1. CLI `runReconcile` called `planflow.Inspect` or `planflow.Reconcile`; both used
   `reconcileDiff`. The recorded source fingerprint differed from the source, so
   the unchanged-source fast path did not apply.
2. `reconcileDiff` called `buildPlan`, then `planner.(*Planner).Compile` and
   `PlanFromMarkdown`. The original `mdIDToken` accepted only one alphabetic
   prefix before the digits. It did not recognize `AS-CLEF-001`; the Clef source
   therefore produced no stages. Its narrative layout also lacked the explicit
   Project/Summary/Acceptance Criteria fields needed by the rendered parser.
3. `Compile` invoked `generateValid` with `compileTaskPrompt` and
   `planOutputRequirements`. Capability extraction/classification and task prose
   were model-generated. `decodePlan` and `Plan.Validate` checked each candidate;
   `validateCapabilities` correctly rejected required UNKNOWN capabilities.
4. `planRepairRequest` sent the failing candidate/error back to the model for up
   to two repairs. Different candidates invented different prerequisites for
   discovery work or failed to verify the actual environment. Which validation
   error appeared depended on the generated candidate and repair iteration.
5. `desiredTasks` built executable definitions. `sameTaskDefinition` compared
   title/objective/serialized acceptance text literally; `sameStage` joined
   criteria/deliverables literally. Cosmetic model wording/ordering therefore
   made recorded executed tasks appear changed. `sameStage` also omitted
   Requires, capability inventory, and assumptions from the plan comparison.
6. `reconcileGraph` classified executed tasks using `domain.Task.Executed` and
   retained human/policy boundaries. `redefineTask` preserved lifecycle/history;
   `ReplaceGraph` applied only upserts/removals. `writePlan`, `writeMetadata`, and
   `definitionHash` used deterministic serialization/hashing; there was no
   evidence that serialization caused the reported variable prerequisite errors.

A separate deterministic-validation issue was found: normalized capability
collision detection ranged over a Go map. That could vary the duplicate-name
pair printed for the same input. It did not explain the different generated
UNKNOWN requirements, but is now fixed too. The old dependency membership
comparison also could confuse `[X,Y]` with `[X,X]`; it now counts occurrences.

Authoritative source fingerprints before normalization:

| Input | SHA-256 |
| --- | --- |
| Recorded metadata source | `88c74d440d8d8ef64fe1c6c8d8744b4d39e75e5832fa4cc37f76b4eabb66e6ae` |
| Unchanged source used in both reproduction runs | `144cff84d0ccd1c2808267b00799affe97d105cf761d202b9b5dfa8ded1e7956` |
| Recorded machine-plan snapshot | `a65d2646817a8d30cf2f4ede9b867bf4a4c7b83cdaed242e1a6dc30b83299d04` |

No matching source snapshot for the older recorded fingerprint was established.
The fix does not infer equivalence from that missing snapshot or approve
reworded historical definitions. The recorded machine definitions and stored
executable task rows were the evidence for the explicit source normalization.

Relevant implementation paths are [planner/markdown.go](../../internal/planner/markdown.go),
[planner/planner.go](../../internal/planner/planner.go),
[planner/plan.go](../../internal/planner/plan.go),
[planner/capability.go](../../internal/planner/capability.go),
[planflow/planflow.go](../../internal/planflow/planflow.go), and
[planflow/compare.go](../../internal/planflow/compare.go).

## Classification rules

| Category | Representation and treatment |
| --- | --- |
| Prerequisite capability | An actually supplied runtime, permission, tool, service, or artifact needed before work can begin. Requires names the declared capability; genuine required UNKNOWN remains invalid. |
| Repository discovery | Package/provider searches, call-path tracing, type/request/result/configuration/policy/adapter/test inspection belong in objectives and acceptance criteria. Informational UNKNOWN inventory entries without Requires do not block work. Findings remain UNKNOWN only after the task investigates and cannot establish them. |
| Existing dependency evidence | Completed task artifacts are reused. Producing tasks are dependency edges, not new UNKNOWN capabilities for rediscovery. The Pre-Performance Closure baseline remains existing evidence. |
| Task deliverable | A report path or output to create/inspect is task work, not a capability that must already exist. |
| Environmental prerequisite | A real external prerequisite such as the Go build/test toolchain. The Clef source now records EXISTS with the observed `go version go1.27.1 darwin/arm64`; it is required only by the baseline/test stages. |

These rules were added to generation, compilation-normalization, and repair
prompts. No heuristic silently deletes arbitrary Requires entries from an
existing structured plan, and UNKNOWN validation was not relaxed.

## Changes

- Extended the generic Markdown stage-ID grammar to accept multi-segment IDs
  such as `AS-CLEF-001` and `AS-CLEF-008-S1`, retaining existing ID shapes.
- Changed reconciliation of a changed source to use structured parsing,
  deterministic validation, and the same capability ownership gate as initial
  compilation. Reconciliation never calls a model or its repair loop. Unsupported
  prose gets a fixed actionable diagnostic; recognized invalid structure fails
  validation. Initial free-form plan generation remains available separately.
- Added conservative definition comparisons: whitespace/line-wrap normalization
  in literal-free prose, independent acceptance/deliverable ordering as counted
  collections, dependency-order equivalence with duplicate counts, and default
  execution mode/feature kind equivalence. Quotes, code, ambiguous multiline
  literals, case, negation, numbers, scope, dependencies, and mode changes remain
  significant. Arbitrary paraphrases are not declared equivalent.
- Included Requires, capability inventory fields, and assumptions in plan-level
  comparison. Definition hashes still record original bytes; comparisons do not
  rewrite executed definitions to a canonical string.
- Made capability duplicate diagnostics follow declared input order. Matched
  capability requirements consistently in validation and `CapabilityGaps`, so a
  case/repeated-whitespace variant cannot bypass the missing-ownership gate.
  Word boundaries are preserved: `External Tool` and `ExternalTool` stay distinct.
- Normalized the [Clef source](../plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md)
  into explicit supported fields while retaining every original task-guidance
  body, global constraint, non-goal, execution policy, and report requirement.
  Title/objective/acceptance/mode/dependencies for 001–007, 009, and 010 match the
  recorded executable definitions exactly. Only pending 008 gains explicit
  discovery/evidence-reuse instructions and an acceptance criterion. The current
  machine inventory replaces discovery-as-prerequisite entries with the actual
  Go toolchain; prior findings remain in their unchanged historical reports.

Equivalent executed tasks produce no upsert: their original stored definition,
status, attempt history, and timestamps are retained. Material changes still
reach the existing executed-change approval/policy boundary. Neither approval
policy, task acceptance, commit gate, lifecycle vocabulary, provider defaults,
nor execution-model routing was changed.

## Regression coverage

| Test file | Covered behavior |
| --- | --- |
| [compiler_hardening_test.go](../../internal/planner/compiler_hardening_test.go) | Two structured compilations produce byte-identical stages with zero model calls; compound and legacy IDs; preserved dependencies; explicit execution mode only; informational UNKNOWN discovery; completed dependency evidence; required UNKNOWN runtime rejection; malformed dependency rejection. |
| [capability_dup_diagnostics_test.go](../../internal/planner/capability_dup_diagnostics_test.go) | Repeated duplicate-name diagnostics; normalized ownership matching after Validate; distinct word boundaries; UNKNOWN/undeclared prerequisites remain rejected. |
| [compiler_reconciliation_test.go](../../internal/planflow/compiler_reconciliation_test.go) | Repeated Inspect with zero model calls; stable unsupported-source diagnostics without writes; cosmetics/criterion ordering/default mode equivalence; material scope, acceptance, negation, numeric and mode changes; literal whitespace/line order; counted dependency comparison; Requires/inventory changes; legitimate pending update; approval metadata and archive preservation. |
| [reconcile_hardening_test.go](../../internal/planflow/reconcile_hardening_test.go) | Ownerless MISSING prerequisites fail with typed CapabilityGapError; case/whitespace variants cannot bypass the human gate; canceled entry context preserves state; quoted/escaped/multiline literals remain meaningful. |
| [lifecycle_preservation_test.go](../../internal/planflow/lifecycle_preservation_test.go) | Pending update across all 18 existing lifecycle values, especially NOT_REQUIRED: historical task fields, attempts, budget, timestamps, block reason, report/audit/approval markers unchanged; no executed acceptance or automatic reconciliation. |

Existing reconciliation approval, removal, named-source evidence, lifecycle,
domain, scheduler, CLI acceptance, and commit-gate tests also passed in both
repository-wide suites. No test mutates the live Clef state.

## Before/after reproduction

Before: two consecutive installed-CLI commands on the same source, both exit 0:

```sh
sop reconcile docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md --list-changed --json
```

Run A emitted:

```text
plan: invalid plan returned to the agent for correction (attempt 1): plan: stage AS-CLEF-005 requires capability "Failure and approval governance semantics verification" whose status is UNKNOWN; verify it before depending on it
plan: invalid plan returned to the agent for correction (attempt 2): plan: stage AS-CLEF-003 requires capability "Live decision path tracing and documentation" whose status is UNKNOWN; verify it before depending on it
```

Run B emitted:

```text
plan: invalid plan returned to the agent for correction (attempt 1): plan: stage AS-CLEF-001 requires capability "agentic-sop repository with Go toolchain" whose status is UNKNOWN; verify it before depending on it
```

Both JSON listings flagged 001–007 as changed_executed and 008–010 as updated.
Their diagnostic SHA-256 values differed:
`42b494fcabb16c67fbc15747a7e517fb4570ed0d4d82c8412c5fcfd23d87a8b7` and
`34228d3ab371c4b9f0b64f075b49bbbc6b4f6ca971eefbcd211009400a7b5289`.
The old Inspect API did not expose candidate definitions; no byte-identical
claim is made about those model candidates.

After code changes, two invocations against the still-unnormalized original
source exited 1 with identical `project is empty` diagnostics, no model repairs,
and no state changes. This demonstrates validation, rather than guessed
normalization, for a recognized but incomplete source.

After source normalization, two updated-CLI previews exited 0 with empty stderr
and byte-identical JSON. Each reported:

```json
{
  "plan_changed": true,
  "unchanged": ["AS-CLEF-001", "AS-CLEF-002", "AS-CLEF-003", "AS-CLEF-004", "AS-CLEF-005", "AS-CLEF-006", "AS-CLEF-007", "AS-CLEF-009", "AS-CLEF-010"],
  "updated": ["AS-CLEF-008"],
  "changed_executed": [],
  "removed_executed": [],
  "auto_reconciled": []
}
```

A separate read-only Go audit called `planner.New(nil).Compile` twice against
that same real source and JSON-encoded the generated stages. Both generated
files were byte-identical. Comparing their executable fields with the recorded
plan found only AS-CLEF-008 objective/acceptance changes. The recorded plan and
metadata remained byte-identical to the baseline until the applying command.

| Repeated result | SHA-256 (both runs) |
| --- | --- |
| Normalized source | `71a7d467ff52042e5df63d8632b888df89e366282af406d03e712977c97d7ece` |
| Generated stage JSON | `3e653359ffe3b57c09261549cb9cfeae1078366efc59cbde7445de3e44028af7` |
| Pre-apply preview JSON | `e06105a82ef61b29bd0968e120d8c7d9b76e6cbc98ecbbd5d3b75ef6ef2e8a89` |
| Post-apply preview JSON | `a3a99d9bb633b706e3ee395730f2b31977974586435315b9007af8e0061e08cd` |

## Applied reconciliation and preservation

Verification used the workspace CLI built outside the repository with:

```sh
go build -o /var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi/sop-hardening ./cmd/sop
```

The applying command used this updated binary without `--accept-changed`:

```sh
/var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi/sop-hardening reconcile docs/plans/PLAN-Agentic-SOP-Decision-Boundary-Readiness-Clef.md
```

It exited 0 and reported nine unchanged tasks, updated `[AS-CLEF-008]`, and no
accepted or automatically reconciled executed entries. State changes were made
only through the supported CLI; `state.db` was never manually edited.

Read-only `mode=ro` SQLite comparisons after reconciliation and readiness checks
proved:

- Every AS-CLEF-001–007 task row matched the baseline, including definition,
  lifecycle, attempt count/budget, block reason, and timestamps.
- Entire `task_attempts` (5 rows), `task_dependencies` (9 rows), and `handoffs`
  (0 rows) tables matched the baseline.
- Only AS-CLEF-008 objective, acceptance_criteria, and updated_at changed. Its
  state remained PLANNED, attempt 0, max attempts 3. AS-CLEF-009/010 rows matched.
- All 116 captured historical/pre-existing user file hashes and all 10 approval
  artifact hashes matched. No completed report, run evidence, approval, or
  pre-existing decision/architecture change was overwritten.

| Tasks | State after reconciliation | Attempts unchanged |
| --- | --- | --- |
| AS-CLEF-001/003/006/007 | LOCAL_DONE | 0 each |
| AS-CLEF-002 | LOCAL_DONE | 3 |
| AS-CLEF-004 | NOT_REQUIRED | 1 |
| AS-CLEF-005 | LOCAL_DONE | 1 |
| AS-CLEF-008/009/010 | PLANNED | 0 each |

Two further updated-CLI `--list-changed --json` invocations both exited 0 with
identical JSON and empty stderr: plan_changed=false, all ten tasks unchanged,
and every change/automatic-reconciliation array empty. Both preserved plan.json
and plan.meta.json hashes. Stored stages also matched the independently compiled
stage JSON exactly after decoding.

AS-CLEF-008 readiness was demonstrated without executing its work:

```text
sop-hardening resume AS-CLEF-008
AS-CLEF-008 CREATE_BRANCH
```

The actual scheduler, applied to a read-only live-state snapshot with an
in-memory store, returned READY_TASK selecting AS-CLEF-008. Its dependency
AS-CLEF-007 is satisfied. The live task stayed PLANNED; no branch, task execution,
contract report, adapter, approval, or commit was created by this readiness check.
AS-CLEF-004's NOT_REQUIRED disposition remained satisfied dependency evidence.

## Verification results

Environment: `go version go1.27.1 darwin/arm64`, branch
`feat/not-required-disposition`, HEAD `f3f0e9276700f2e975b10638ff8393ca70e6f7da`.

| Required command | Result |
| --- | --- |
| `gofmt -l .` | PASS; exit 0, no files listed |
| `go vet ./...` | PASS; exit 0 |
| `go test -count=1 ./...` | PASS; exit 0; 74 tested packages, 2 without test files |
| `go test -race -count=1 ./...` | PASS; exit 0; 74 tested packages, 2 without test files |
| `go build ./...` | PASS; exit 0 |
| `git diff --check` | PASS; exit 0 |

The focused planner/planflow checks and each successful governed implementation
also passed SOP's build, test, vet, review, and quality checks. Intermediate
runs encountered a transient provider outage and a bounded fix-loop failure;
corrective invocations completed through SOP rather than bypassing a gate.
The capability word-boundary regression found during review was fixed before
final verification.

### Review Summary

| Severity | Unresolved findings | Status |
| --- | --- | --- |
| CRITICAL | 0 | pass |
| HIGH | 0 | pass |
| MEDIUM | 0 | pass |

Verdict: APPROVE for local implementation verification. Human approval is still
required before commit. This review is not task acceptance or commit approval.
Code/security review checked parser input handling, literal comparison,
capability gates, history preservation, and the unchanged authorization surfaces.

## Remaining limitations and audit artifacts

Model-generated initial planning and normalization of arbitrary free-form prose
are still variable. This work establishes deterministic compilation for supported
structured source and removes model variability from reconciliation; it does
not claim deterministic arbitrary natural-language compilation. Unsupported
changed sources must be explicitly structured, and recognized malformed source
remains invalid. Discovery targets starting UNKNOWN do not themselves require
human approval.

Comparison deliberately recognizes only provable cosmetics. Synonyms, arbitrary
rewording, ambiguous multiline criteria, and benign whitespace edits in fields
containing literals can still require review. Plan/source definition hashes retain
original bytes, and no canonical rewrite or fuzzy matching is used to hide
material executed changes. Existing graph/file persistence ordering remains:
there is no new cross-file/SQLite journal, so a failure after graph persistence
but before provenance write retains the existing retry behavior.

The installed PATH binary was used for the before reproduction. After
verification used the newly built workspace binary above; the installed binary
was not replaced. All code/source/report changes remain uncommitted.

Raw before/after diagnostics, generated definitions, gate logs, read-only state
snapshots, and comparison scripts are retained outside the repository at
`/var/folders/rr/445xs9c92qndk2cmdsbyry6h0000gn/T/sop-compiler-hardening-t0xdtesi`.
The baseline state-row manifest SHA-256 is
`2c046cdd46daef2b00d0fa8dc33bdacdb6e03671bf304fc84103f313a2a89e61`;
the historical/user-file manifest is
`718d3863c1af31de4e427561f57a208823f730727a21b3ed016d2548c1ab3830`;
the approval-file manifest is
`acd8ba0575c9a7783fb5c0672d7ba03d7883e182c4765dcc30b448f9b28c602d`.
These are local investigation artifacts, not new prerequisite capabilities.

Governed implementation reports include
`compiler step` (`.agent-sdlc/runs/prompts/prompt-20261007-011846/report.md`),
`comparison/ownership correction` (`.agent-sdlc/runs/prompts/prompt-20261007-013220/report.md`),
`word-boundary correction` (`.agent-sdlc/runs/prompts/prompt-20261007-014056/report.md`), and
`source/lifecycle step` (`.agent-sdlc/runs/prompts/prompt-20261007-014312/report.md`).
Their generated artifacts provide the implementation/review trail. No external
adapter repository was modified.

Final documentation invocation note: prompt-20261007-015026 wrote this report
but ended FAIL / NO_CHANGES_PRODUCED; the bounded correction
prompt-20261007-015307 also ended FAIL / NO_CHANGES_PRODUCED. Their
`original failed report` (`.agent-sdlc/runs/prompts/prompt-20261007-015026/report.md`)
and `correction failed report` (`.agent-sdlc/runs/prompts/prompt-20261007-015307/report.md`)
remain authoritative; `sop-run.log: UNAVAILABLE`. No failed gate was accepted or
weakened. The independently verified implementation and supported Clef
reconciliation results above are unchanged.

Pre-commit investigation narrowed this limitation: the default readDiff adapter
excludes `docs/reports`, but scheduled tasks already recover their explicit
report deliverables from a matching reconciled plan and count changes to those
safe declared paths. Ad-hoc implementation prompts construct a task spec without
Deliverables, so that exception does not apply to these two report-only prompt
runs. The separate
[follow-up issue](../plans/ISSUE-Prompt-Report-Deliverables.md) records the prompt
projection gap and required governance-preserving regressions; no detector fix
is included in this hardening.

The [pre-commit review](PLAN-compiler-reconciliation-commit-review.md) records the
completed SOP reviews, all six repeated verification gates, unchanged historical
and approval evidence, AS-CLEF-008 readiness, the exact compiler-only commit
scope, and the updated-binary continuation commands. Pre-existing acceptance
metadata for AS-CLEF-001/002 remains unchanged; no acceptance IDs were added by
this hardening or its review. Clef source/governance/report files remain a
separate workstream. Human approval is still required before commit or push.

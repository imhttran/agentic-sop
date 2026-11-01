# PLAN — SOP-RUN-ACCEPT-001: Native deterministic acceptance checks for `sop run`

**Type:** Feature plan (implementation through `sop run`).
**Status:** ACTIVE.
**Baseline:** `origin/main` at `c348ab4`.

## Project
agentic-sop

## Summary
`sop prompt --capability implement --accept CMD` already verifies acceptance deterministically:
`internal/cli/outcome.go` authorizes each command fail-closed (`authorizeAcceptanceCommand`:
`toolharness.SplitCommand` + `commandpolicy.Classify`, SAFE only, shell syntax rejected), runs the
exact argv with no shell (`runAcceptanceArgv`), and `deriveOutcome` turns the result into
VERIFIED / PARTIAL / HOLD via `internal/outcome`. `sop run` calls the same `deriveOutcome`
(`internal/cli/run.go:1032`), but a scheduled task's spec never carries acceptance checks, so a
planned task can only ever be PARTIAL. This plan lets an operator-authored plan declare acceptance
checks per stage and reuses the existing authorization and execution unchanged.

Out of scope: automatic FIX-cycle triggering from a failing check, environment-variable allowlists,
recording a "not executed" outcome on early lifecycle exits, any new command class, any shell
execution, any database schema migration.

Investigation evidence (file:line at `c348ab4`):
- Wrapped bullets: the `## <ID> — <title>` + `### <Sub-section>` shape turns every non-empty line
  into its own list item (`internal/planner/markdown.go`, the `case "stage":` branch near line 258,
  via `listItem`); `internal/taskfile/taskfile.go:listItems` (line 191) does the same. The `## Tasks`
  inline-field shape already joins indented continuation lines (`markdown.go` `taskStage.line`, ~497).
- Scheduled specs: `internal/cli/drive.go:runScheduledTask` (~665) builds the spec with
  `specFromTask` and recovers deliverables from `.agent-sdlc/plan.json` with
  `planTaskDeliverables` (`internal/cli/report_deliverable.go:20`).
- Reconciliation: `internal/planflow/compare.go:sameTaskDefinition` (45-51) and
  `equivalentExecutedChange` (58) compare only persisted task fields; `sameStage` (339-348) compares
  stage-only fields; `internal/planflow/planflow.go` loads the recorded plan (~626), sets
  `PlanChanged` (~660), builds desired tasks (~662) and calls `reconcileGraph` (~677; decisions at
  ~795, ~821, ~923; `definitionHash` ~947; `describeTaskDifference` ~1100).
- Outcome: `internal/outcome/outcome.go:Verify`; `internal/cli/outcome.go:deriveOutcome` marks the
  gate as `deterministic-checks` (57-68), deliverables as `os.Stat` existence (~81), and only adds
  the unverifiable `objective-acceptance` criterion when the spec declares nothing (135), so a
  deliverable-only spec with a passing gate is VERIFIED today. `go test` that runs no test (no test
  files, `-run` matching nothing, every test skipped, a `TestMain` that never calls `m.Run`) exits 0.
- Model-normalized plans: `internal/planner/planner.go:decodePlan` (~225) decodes every
  model-produced plan; `Planner.Compile` already discards a model's `done` execution mode
  (`markdown.go` ~330) because operator intent must come from the document itself.

## Tasks

### ACC-001 — Join wrapped list-item continuation lines in plan and task Markdown
- **Objective:** A list item wrapped onto indented continuation lines is parsed as ONE item in
  the `## <ID> — <title>` stage shape and in task files, instead of one item per physical line.
- **Scope:** In `internal/planner/markdown.go` (the `case "stage":` branch of the rendered-plan
  parser), when a line inside a list sub-section (dependencies, requires, deliverables, acceptance
  criteria) starts with a space or tab, does NOT match `mdBullet`, and directly follows an item of
  the same sub-section (no blank line in between), append it with one space to the previous item
  instead of creating a new item. Apply the same rule in `internal/taskfile/taskfile.go:listItems`
  (indented, non-bullet line following an item continues that item). A blank line ends the item.
  Unindented unbulleted lines and nested bullets keep today's behaviour (each is its own item).
  Keep the change minimal; no new parser.
- **Files:** `internal/planner/markdown.go`, `internal/taskfile/taskfile.go`,
  `internal/planner/markdown_test.go`, `internal/taskfile/taskfile_test.go`
- **Depends on:** —
- **Acceptance:** A stage whose `### Acceptance Criteria` has 2 bullets, each wrapped onto an
  indented second line, yields exactly 2 criteria with the joined text; the same holds for
  `### Deliverables` and for a task file's `## Acceptance criteria`.
- **Acceptance:** Existing behaviour is preserved and tested: unindented unbulleted lines and
  nested bullets remain separate items, a blank line ends an item, and every existing planner and
  taskfile test passes unchanged.

### ACC-002 — Declare acceptance checks in the plan format
- **Objective:** An operator-authored plan can declare deterministic acceptance checks per stage,
  carried in `plan.json`, in rendered `PLAN.md`, and in task files.
- **Scope:** Add `AcceptanceChecks []string` with json tag `acceptance_checks,omitempty` to
  `planner.Stage` (`internal/planner/plan.go`). Parse it from a stage sub-section heading
  `### Acceptance Checks` (also `### Acceptance Check`) in the rendered-plan shape, and from an
  inline field `- **Acceptance check:**` / `- **Acceptance checks:**` in the `## Tasks` shape (one
  command per field line; repeated lines append). Do NOT add these keys to the existing
  `acceptance`/`tests` aliases, which feed AcceptanceCriteria. When a parsed check is entirely
  wrapped in one pair of backticks, strip them (Markdown code formatting, not shell syntax). Render
  a non-empty list as `### Acceptance Checks` in `internal/planner/render.go` so a rendered plan
  round-trips. Add `AcceptanceChecks` parsing from a `## Acceptance checks` section in
  `internal/taskfile/taskfile.go` (the `Spec` field already exists) and write it in the task-file
  renderer with `writeList`. A model must never author its own acceptance checks: in
  `internal/planner/planner.go:decodePlan`, clear `AcceptanceChecks` on every stage, mirroring how
  `Compile` discards a model-declared `done` mode. Do not add the field to the model JSON schema
  prompt.
- **Files:** `internal/planner/plan.go`, `internal/planner/markdown.go`,
  `internal/planner/render.go`, `internal/planner/planner.go`, `internal/taskfile/taskfile.go`,
  `internal/planner/markdown_test.go`, `internal/planner/planner_test.go`,
  `internal/taskfile/taskfile_test.go`
- **Depends on:** ACC-001
- **Acceptance:** Both plan shapes parse `go test ./internal/x` (with and without enclosing
  backticks) into `Stage.AcceptanceChecks`, rendering then re-parsing a plan preserves the checks,
  and a task file's `## Acceptance checks` section populates `Spec.AcceptanceChecks`.
- **Acceptance:** A model-produced plan JSON containing `acceptance_checks` is decoded with the
  checks removed (test through `decodePlan`), and a plan without checks produces byte-identical
  `plan.json` output to today (`omitempty`).

### ACC-003 — Run plan-declared acceptance checks for scheduled tasks
- **Objective:** `sop run` evaluates a scheduled task's operator-authored acceptance checks through
  the existing `deriveOutcome`, `authorizeAcceptanceCommand`, and `runAcceptanceArgv`; when checks
  exist, VERIFIED requires every check to execute and pass, and free-text criteria become
  informational; without checks the outcome semantics are unchanged.
- **Scope:** In `internal/cli/report_deliverable.go`, factor the stage lookup of
  `planTaskDeliverables` into `planTaskStage(dir, task)` (same `plan.json` read, `Validate`, and
  ID + Objective + AcceptanceCriteria match; not found on any mismatch) and keep
  `planTaskDeliverables` as a thin wrapper. In `internal/cli/drive.go:runScheduledTask`, set
  `spec.AcceptanceChecks` from the matched stage alongside `spec.Deliverables`. In
  `internal/cli/outcome.go:deriveOutcome`, ONLY when `len(spec.AcceptanceChecks) > 0`: record each
  free-text acceptance criterion with `Required: false` (still listed in `outcome.json` and
  `report.md`, described as informational) so the verdict rests on the checks; every acceptance
  check stays Required (an unauthorized one stays a Boundary). When the spec has no acceptance
  checks, `deriveOutcome` must behave exactly as today. Do not change `authorizeAcceptanceCommand`
  or `commandpolicy`. In `runAcceptanceArgv`, only when argv is `go test ...`: insert `-json` after
  `test` if absent, and treat the check as met only when the exit status is 0 AND the JSON stream
  contains at least one `"Action":"pass"` event with a non-empty `"Test"`; otherwise it is not met
  with evidence `no test executed (skipped or not run)`. Other SAFE commands are unchanged.
  Tests MUST exercise the real scheduled-task path: compile a Markdown plan document whose stage
  has free-text acceptance criteria (required by `Plan.Validate`) and an `### Acceptance Checks`
  section through the same compile/persist path `sop run` uses (or drive `sop run <PLAN.md>`
  itself with a stub agent); do not hand-write `plan.json` or build `planner.Stage` values that
  would fail `Plan.Validate`. Replace the existing hand-built fixture in `run_accept_test.go`.
  Update the prompt accept test fixture (`goModuleFixture`) to include one passing test so existing
  `go test` acceptance checks still execute a test.
- **Files:** `internal/cli/report_deliverable.go`, `internal/cli/drive.go`,
  `internal/cli/outcome.go`, `internal/cli/run_accept_test.go`, `internal/cli/prompt_accept_test.go`
- **Depends on:** ACC-002
- **Acceptance:** Through the real scheduled-task path, a stage with free-text criteria and a
  passing check (`go vet ./...`, and a `go test` check that runs a passing test) records VERIFIED in
  `outcome.json` and `report.md`, listing each check as met and each free-text criterion as
  informational.
- **Acceptance:** Never VERIFIED: a failing check yields PARTIAL with the failure evidence; a
  `go test` check that runs no test (`-run NoSuchTest`, every test `t.Skip`, no test files) yields
  PARTIAL with `no test executed`; an unauthorized check (`go vet ./... && touch pwned`,
  `sh -c "touch pwned"`, `go run .`, `git push`) yields HOLD and is never executed (the test asserts
  the marker file does not exist); a run that stops before outcome derivation records no VERIFIED
  outcome.
- **Acceptance:** A task with no declared checks produces exactly today's outcome (all existing
  outcome/prompt/run tests pass unchanged except the fixture update), and a plan whose stage no
  longer matches the persisted task yields no checks.

### ACC-004 — Detect acceptance-check changes during reconciliation
- **Objective:** Adding, removing, or modifying a stage's acceptance checks is a recognized task
  change: a never-executed task is updated, and an executed task stops for the existing
  `--accept-changed` approval instead of being silently accepted.
- **Scope:** Compare checks against the recorded `plan.json` stage, with no schema migration. In
  `internal/planflow/compare.go:sameStage`, also compare `AcceptanceChecks` exactly (ordered string
  slices; no prose canonicalization; nil equals empty). In `internal/planflow/planflow.go`, after
  the desired tasks are built, compute the set of stage IDs whose recorded and new checks differ and
  pass it to `reconcileGraph` (its only caller): a task in that set is never classified unchanged
  (~795) or an equivalent executed change (~821, ~923); include the checks in the provenance
  `definitionHash` input for such a change (~947) and add an "acceptance checks changed" line to
  `describeTaskDifference` (~1100). `sop reconcile --list-changed` must list such executed tasks.
- **Files:** `internal/planflow/compare.go`, `internal/planflow/planflow.go`,
  `internal/planflow/reconcile_test.go`, `internal/planflow/compiler_reconciliation_test.go`,
  `internal/planflow/inspect_test.go`
- **Depends on:** ACC-002
- **Acceptance:** Tests cover checks added, removed, and modified for a never-executed task
  (reported updated, persisted plan.json carries the new checks) and for an executed task (stops as
  a changed executed task; accepted only with `--accept-changed`; listed by `--list-changed`).
- **Acceptance:** A plan change confined to other fields reconciles exactly as before (all existing
  planflow tests pass), and reordering nothing but whitespace in check text is still a change.

### ACC-006 — Close ACC-001/ACC-002 gaps: execution mode, model-authored checks, task-file render
- **Objective:** Restore the pre-ACC-001 execution-mode parsing, stop a model from authoring its
  own acceptance checks, and make task files round-trip acceptance checks.
- **Scope:** (1) In `internal/planner/markdown.go`, remove `execution` / `execution mode` from the
  list sub-sections that join indented continuation lines and from the collect-then-flush path:
  restore the `c348ab4` behaviour where EVERY non-empty line of an `### Execution` sub-section is
  passed through `listItem` and `domain.ParseExecutionMode`, and the last valid mode wins (so prose
  before the mode, or an indented note after it, cannot drop the mode). Wrapped-bullet joining for
  the other list sub-sections stays as ACC-001 implemented it. (2) In
  `internal/planner/planner.go:decodePlan`, clear `AcceptanceChecks` on every decoded stage before
  validation, so a model-produced or model-normalized plan can never declare acceptance checks; do
  not add the field to the model JSON schema prompt. (3) In `internal/taskfile/taskfile.go`, write a
  non-empty `Spec.AcceptanceChecks` as a `## Acceptance checks` list in the task-file renderer (use
  `writeList`), so `Parse(Render(spec))` preserves the checks.
- **Files:** `internal/planner/markdown.go`, `internal/planner/planner.go`,
  `internal/taskfile/taskfile.go`, `internal/planner/markdown_test.go`,
  `internal/planner/planner_test.go`, `internal/taskfile/taskfile_test.go`
- **Depends on:** ACC-002
- **Acceptance:** Regression tests: `### Execution` with `- verify-first`, and with `- done`, set the
  mode; prose before the bullet (`This stage runs as` then `- verify-first`) still sets
  verify-first; an indented note after the bullet (`- done` then `  (already merged)`) still sets
  done; an unrecognized line never clears a valid mode.
- **Acceptance:** A model plan JSON (through `decodePlan`) containing `acceptance_checks` yields
  stages with no acceptance checks, while an operator Markdown plan's checks are kept; a task file
  with acceptance checks round-trips through Render and Parse; all existing planner, taskfile, and
  planflow tests pass.

### ACC-005 — Outcome integrity for acceptance checks
- **Objective:** VERIFIED requires at least one executed, passing acceptance check; a passing
  gate, existing deliverables, or a `go test` that ran no test can never establish acceptance; each
  check records reviewable evidence.
- **Scope:** In `internal/cli/outcome.go:deriveOutcome`, add the unverifiable required
  `objective-acceptance` criterion whenever `len(spec.AcceptanceChecks) == 0` (not only when the
  spec declares nothing), and update the two tests that expect a deliverable-only spec to be
  VERIFIED (`TestOutcomeIntegrationVerified`, `TestPromptOutcomeVerifiedWithSatisfiedDeliverable`)
  to expect PARTIAL. The `go test` no-test-executed guard is owned by ACC-003; keep it intact. Harden
  `authorizeAcceptanceCommand` (acceptance checks only; do not change `internal/commandpolicy`): in
  addition to the SAFE requirement, reject `go fmt`, `go` flags `-exec`, `-toolexec`, `-vettool`,
  `-o`, `-overlay`, `-modfile` (in `-f`, `-f=v`, `--f`, `--f=v` forms), and `git` `-c` /
  `--config-env`; a rejected command is a boundary (HOLD) and is never executed. Add `Argv`,
  `ExitCode`, and `Output` (bounded excerpt, at most 400 bytes) to the per-check evidence recorded in
  `outcome.json` and show the exit code in `report.md`.
- **Files:** `internal/cli/outcome.go`, `internal/outcome/outcome.go`,
  `internal/cli/outcome_integration_test.go`, `internal/cli/prompt_outcome_test.go`,
  `internal/cli/prompt_accept_security_test.go`, `internal/cli/run_accept_test.go`
- **Depends on:** ACC-003, ACC-006
- **Acceptance:** Tests show PARTIAL (never VERIFIED) for a deliverable-only spec with a passing
  gate and for a `TestMain` that exits without `m.Run`; the ACC-003 skip/no-test cases still yield
  PARTIAL. A real passing `go test` check is VERIFIED, and `go vet ./...` / `go build ./...` checks
  behave as before.
- **Acceptance:** Tests show HOLD with no execution (marker file absent) for `go fmt ./...`,
  `go test -exec=/bin/sh ./...`, `go test -toolexec=x ./...`, `go vet -vettool=x ./...`,
  `go build -o /tmp/x .`, and `git -c core.pager=x log`; outcome.json records argv, exit code, and
  an output excerpt for executed checks.

### ACC-010 — Parse multi-file deliverables individually (narrowed)
- **Objective:** A `## Tasks`-shape `Files` value that lists several paths becomes one bare path
  per deliverable, while every existing `### Execution` and inline `Execution` parsing behaviour,
  including `verify-first` and `done`, is preserved.
- **Scope:** Change ONLY the handling of the `files` / `file` / `likely files` /
  `likely files/packages` / `likely packages` keys in `internal/planner/markdown.go`
  (`taskStage.stage` and the helper it calls). Split the value on commas and semicolons; from each
  piece trim whitespace, then trailing sentence punctuation (`.`, `,`, `;`, `:`), then one pair of
  enclosing backticks, then trailing sentence punctuation again; drop empty pieces. The previous
  attempt left a regression in this file: an `### Execution` sub-section whose `- done` (or
  `- verify-first`) bullet is followed by an indented note line no longer sets the mode. Restore
  the ACC-006 behaviour (every non-empty line of an `### Execution` sub-section is passed through
  `listItem` and `domain.ParseExecutionMode`; the last valid mode wins; execution lines are never
  joined as wrapped continuations) without changing any other parser branch. Set the expectation in
  `TestPlanFromMarkdownTaskShape` to the bare paths `cmd/sop/main.go` and `internal/app/app.go`.
  Do not add the gofmt guard (ACC-011 owns it). Do not modify any `zz_operator_acceptance_*` file:
  they are the operator's acceptance checks.
- **Files:** `internal/planner/markdown.go`
- **Files:** `internal/planner/taskshape_test.go`
- **Depends on:** ACC-006
- **Acceptance:** `Files` values such as `` `a/x.go`, `a/y.go` ``, `` `b/z.go`; `b/w.go`; ``, and
  `` `cmd/sop/main.go`, `internal/app/app.go`. `` yield individual bare paths with no backticks or
  trailing punctuation, and each is verified individually by the outcome.
- **Acceptance:** Every execution-mode shape parses as before: rendered `- verify-first`, `- done`,
  prose before the mode, an indented note after the mode, and inline `Execution:` values;
  `TestPlanFromMarkdownExecutionShapes` and `TestPlanFromMarkdownTaskShape` pass; no other parser
  branch changes.
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC010 ./internal/planner/
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC010 ./internal/cli/
- **Acceptance check:** go test -count=1 -run TestPlanFromMarkdownExecutionShapes ./internal/planner/
- **Acceptance check:** go test -count=1 -run TestPlanFromMarkdownTaskShape ./internal/planner/

### ACC-007 — Detect acceptance-check changes during reconciliation (production code)
- **Objective:** Adding, modifying, or removing a stage's acceptance checks is a material task
  change: an executed task stops for `--accept-changed`, a never-executed task is updated, and
  `sop reconcile --list-changed` reports both; nothing is silently accepted.
- **Scope:** Production code in `internal/planflow` (ACC-004 only added tests). Compare checks
  against the recorded `plan.json` stage with no schema migration: in `compare.go`, make `sameStage`
  (and therefore `plansEquivalent` / `PlanChanged`) compare `AcceptanceChecks` exactly (ordered; nil
  equals empty). In `planflow.go`, after the desired tasks are built, compute the stage IDs whose
  recorded and desired checks differ and pass that set to `reconcileGraph` (its only caller): such a
  task is never classified unchanged, an executed one is a MATERIAL changed-executed task (never
  equivalent and never auto-reconciled), a never-executed one is updated; include the checks in the
  provenance `definitionHash` input for such a change and add an "acceptance checks changed" line to
  `describeTaskDifference`. `Inspect` (the `--list-changed` path) must classify identically. Applying
  without approval must fail with the existing NEEDS_HUMAN changed-executed error naming the task and
  leave `plan.json` unchanged; with `--accept-changed` it must keep the task's status and attempt
  history and persist the new checks. Add planflow unit tests for add, modify, and remove, for
  executed and never-executed tasks, that change `AcceptanceChecks` (not free-text criteria). Do not
  modify any `zz_operator_acceptance_*` file.
- **Files:** `internal/planflow/compare.go`
- **Files:** `internal/planflow/planflow.go`
- **Files:** `internal/planflow/acceptance_checks_reconcile_test.go`
- **Depends on:** ACC-002, ACC-010
- **Acceptance:** For an executed task, adding, modifying, or removing an acceptance check is listed
  as a material changed-executed task, applying it without approval fails with NEEDS_HUMAN and leaves
  `plan.json` unchanged, and `--accept-changed` applies it while preserving status and history.
- **Acceptance:** For a never-executed task the same changes are reported as updated and applied;
  whitespace-only or no change remains unchanged; all existing planflow tests pass.
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC007 ./internal/planflow/
- **Acceptance check:** go test -count=1 ./internal/planflow/

### ACC-008 — Propagate executed acceptance-check evidence (narrowed)
- **Objective:** Every executed acceptance check records its exact argv, exit code, and a bounded
  stdout/stderr excerpt in the existing criterion evidence fields, and `report.md` shows the exit
  code; command authorization and the security hardening already in place are unchanged.
- **Scope:** Evidence propagation only, in `internal/cli/outcome.go:deriveOutcome`. Each acceptance
  check's `Check` closure already records the executed run (`checkRun`: argv, exitCode, output) into
  a local `rec` that is then discarded (`_ = rec`), and `checkEvidence` keeps only the criterion ID,
  so the existing `outcome.Result.PropagateEvidence(id, argv, exitCode, output)`
  (`internal/outcome/outcome.go`) is never called. Keep a reference to each check's `rec` in its
  `checkEvidence` entry and, after `outcome.Verify`, call `PropagateEvidence` for every entry so the
  criterion's `Argv`, `ExitCode`, and `Output` are populated. `Output` must be an excerpt of at most
  400 bytes (truncate if the run captured more). `writeOutcomeReport` already prints argv and exit
  for criteria with a non-empty `Argv`; leave its format unchanged. A rejected (unauthorized)
  command never executes and keeps empty evidence. Do NOT change `authorizeAcceptanceCommand`, the
  hardening (`go fmt`, `-exec`, `-toolexec`, `-vettool`, `-o`, `-overlay`, `-modfile`, `git -c` /
  `--config-env`), `internal/commandpolicy`, or the deliverable-only rule. Do not modify any
  `zz_operator_acceptance_*` file.
- **Files:** `internal/cli/outcome.go`
- **Depends on:** ACC-010
- **Acceptance:** An executed failing check records its argv (`go vet ./doesnotexist/`), a non-zero
  exit code, and a non-empty output excerpt of at most 400 bytes (also when the command printed
  about 2 KB), and `report.md` lists the argv and exit code for it.
- **Acceptance:** All ACC-008 hardening behaviour still holds (every rejected form is a HOLD and
  never executes; allowed SAFE checks still verify; deliverable-only specs are never VERIFIED).
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC008 ./internal/cli/
- **Acceptance check:** go test -count=1 ./internal/outcome/

### ACC-009 — Structured `go test -json` evaluation, executed once
- **Objective:** A `go test` acceptance check is judged from structured `go test -json` events from a
  single execution: met only when the exit status is 0 and at least one test-level pass event
  occurred; all-skipped, zero-test, unmatched `-run`, and TestMain-without-`m.Run` results are not
  met; packages with no test files do not count against an otherwise passing run.
- **Scope:** In `internal/cli/outcome.go`, replace the ACC-003 text heuristic (`goTestExecuted`) and
  the re-run helper (`acceptanceOutput`) with a single execution: for `go test` argv, insert `-json`
  immediately after `test` unless an equivalent `-json` flag is already present, run the exact argv
  once without a shell, decode the event stream (`encoding/json`, one event per line; ignore
  non-JSON lines), and treat the check as met only if the exit status is 0 AND at least one event has
  `"Action":"pass"` with a non-empty `"Test"`; otherwise not met with evidence `no test executed
  (skipped or not run)` (or the failure). Package-level skip events for packages with no test files
  are ignored. Record Argv (the executed argv), ExitCode, and the Output excerpt from that single run.
  Other SAFE commands are unchanged. Add ordinary tests for each case. Do not modify any
  `zz_operator_acceptance_*` file.
- **Files:** `internal/cli/outcome.go`
- **Files:** `internal/cli/acceptance_gotest_test.go`
- **Depends on:** ACC-008
- **Acceptance:** Through `deriveOutcome`, all-skipped, unmatched `-run`, zero-test, TestMain without
  `m.Run`, and failing `go test` checks are never VERIFIED; a passing check, a passing check combined
  with a no-test package, and a whole-module `go test ./...` with no-test packages are VERIFIED.
- **Acceptance:** Each acceptance command executes exactly once (observable counter), and the ACC-008
  hardening and evidence behaviour still hold.
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC009 ./internal/cli/
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC008 ./internal/cli/

### ACC-011 — Enforce gofmt in ordinary verification
- **Objective:** Every Go file in the module is gofmt-formatted, and an ordinary `go test ./...`
  fails, naming the files, when one is not.
- **Scope:** Add `internal/archtest/gofmt_test.go` with a helper
  `unformattedGoFiles(root string) ([]string, error)` that walks `root` for `.go` files (skipping
  `.git`, `.agent-sdlc`, and any `testdata` directory), runs `go/format.Source` on each, and returns
  the paths whose formatting would change; and an ordinary (untagged) test
  `TestRepositoryIsGofmtFormatted` that calls it on the module root and fails listing every reported
  file. Run gofmt on the files it reports (currently `internal/planflow/reconcile_test.go`). Do not
  change production code. Do not modify any `zz_operator_acceptance_*` file.
- **Files:** `internal/archtest/gofmt_test.go`
- **Files:** `internal/planflow/reconcile_test.go`
- **Depends on:** ACC-010
- **Acceptance:** `unformattedGoFiles` reports an unformatted `.go` file (including a `_test.go`
  file) and skips `testdata` and `.agent-sdlc`; a formatted tree reports nothing.
- **Acceptance:** `TestRepositoryIsGofmtFormatted` passes on the module and the module has no gofmt
  differences.
- **Acceptance check:** go test -tags acceptance -count=1 -run TestOperatorAcceptanceACC011 ./internal/archtest/
- **Acceptance check:** go test -count=1 -run TestRepositoryIsGofmtFormatted ./internal/archtest/
- **Acceptance check:** go vet ./...

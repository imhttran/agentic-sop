# Follow-up: ad-hoc prompt report deliverables produce NO_CHANGES_PRODUCED

Issue: SOP-PROMPT-REPORT-001
Status: OPEN; deferred to a separate implementation
Recorded: 2026-10-06 (America/Chicago)

An ad-hoc `sop prompt --capability implement` that creates or updates only an
operator-authorized file under `docs/reports/` can end FAILED /
NO_CHANGES_PRODUCED even though the report was written. This follow-up records
the limitation; the compiler/reconciliation hardening does not change the
implementation change detector or its gates.

## Observed reproduction

1. Request an ad-hoc implementation whose only repository output is
   `docs/reports/PLAN-compiler-reconciliation-hardening.md`.
2. SOP's governed tool writes the report.
3. Completion checks see no implementation diff and fail before validation with
   `agent reported successful implementation but produced no repository changes`.

Both observed attempts remain authoritative failed outcomes:

- [prompt-20261007-015026](../../.agent-sdlc/runs/prompts/prompt-20261007-015026/report.md)
- [prompt-20261007-015307](../../.agent-sdlc/runs/prompts/prompt-20261007-015307/report.md)

Both report FAILED / FAIL / NO_CHANGES_PRODUCED. Validation was not reached;
these failures are not evidence of a passed implementation lifecycle.
`sop-run.log: UNAVAILABLE`.

## Source evidence and boundary

[runPromptImplement](../../internal/cli/prompt.go) constructs a `taskfile.Spec`
with ID, Title, and Description but no Deliverables. The default
[readDiff adapter](../../internal/cli/cli.go) excludes `.agent-sdlc` and
`planflow.ReportsDir` from implementation diffs. The declared-report exception in
[runStages](../../internal/cli/run.go) therefore receives no report paths for an
ad-hoc prompt.

Scheduled tasks already obtain explicit deliverables from the reconciled
machine plan through
[planTaskDeliverables/taskReportDeliverables](../../internal/cli/report_deliverable.go)
and [runScheduledTask](../../internal/cli/drive.go). A matching declared safe
Markdown report counts as a repository change. This issue does not establish
that all documentation-only scheduled tasks fail: AS-CLEF-008's contract report
is declared in its matching reconciled stage, so the missing prompt projection
is not its readiness blocker.

## Follow-up acceptance criteria

- Give ad-hoc implementation requests an explicit operator-controlled way to
  declare repository deliverables, and project those declarations into the
  existing task lifecycle. Inspect the authoritative boundary before choosing
  the interface; model-generated output suggestions are not declaration authority.
- Count an actual change to a declared safe report path. An unchanged report,
  unrelated existing dirty file, or a merely announced output must still fail
  the no-changes/no-progress check.
- Continue excluding SOP's generated run reports and state artifacts unless an
  exact repository report is explicitly declared through the supported boundary.
  Reject traversal, external paths, and directory-wide exceptions.
- Test declared changed reports, unchanged reports, generated artifacts, unsafe
  paths, and the existing scheduled-task path.
- Preserve deterministic validation, review, human approval, task acceptance,
  and commit gates. Do not suppress NO_CHANGES_PRODUCED globally or create an
  unrelated code change to manufacture a successful diff.

This is independent CLI prompt/deliverable work. It is not a prerequisite for
repository discovery or a request to implement a Clef adapter. No task states,
approval records, execution history, or commit permissions are changed by
recording this issue.

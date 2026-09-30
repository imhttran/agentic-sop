# Task Lifecycle

**Type:** Normative specification

## Purpose

This document is the normative specification for the per-task lifecycle: the
ordered stages a task passes through, the test-driven development (TDD) rules
that apply, and the Git naming conventions for branches, commits, and pull
requests. Task states and the transitions between them are owned by
[WORKFLOW.md](WORKFLOW.md); this document covers the stages and conventions, not
the state graph.

## Related Specifications

- [WORKFLOW.md](WORKFLOW.md) — task state vocabulary, transition legality,
  remediation, and terminal states.
- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §5 Task State
  Machine and §7 TDD Architecture.
- [../PRD.md](../PRD.md) — §8 Task Lifecycle and FR-8 through FR-15.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Lifecycle Stages

A task SHOULD progress through the following ordered stages. Each stage
corresponds to a state transition owned by [WORKFLOW.md](WORKFLOW.md).

1. **Branch** — the task MUST execute on an isolated Git branch.
2. **Design / write tests** — tests SHOULD be written from the task's acceptance
   criteria before implementation.
3. **Verify RED** — the new test SHOULD be observed failing before the feature is
   implemented.
4. **Implement** — the task agent edits the working tree; Git is the authority on
   what changed.
5. **Verify GREEN** — the new test and the affected suite MUST pass.
6. **Validate** — relevant formatting, compilation, unit and integration tests,
   and static checks MUST run.
7. **Review** — a structured self-review MUST run; blocking findings return the
   task to a fix-and-retest stage.
8. **Commit** — a task that passes required local gates MUST be committed with
   task-identifiable commit metadata.
9. **Pull request** — when GitHub integration is configured, the branch MUST be
   pushed and a pull request created or updated.
10. **CI** — required GitHub CI checks MUST be observed before merge.
11. **Merge / completion** — a task MUST merge only after required tests, review,
    and CI gates pass, or complete locally as `LOCAL_DONE`.

The commit gate MUST produce a single deterministic commit only when required
tests, review, and documentation are in place. A local run MUST NOT synthesize
the remote path (push, PR, CI, merge); that path is driven by the explicit commit
and pull-request commands and the GitHub adapter.

## 2. TDD Rules

The intended loop is: acceptance criteria → write test → verify RED → implement →
verify GREEN → refactor → run affected suite.

- A task MUST NOT claim successful TDD merely because tests pass.
- Where applicable, the system MUST verify that the new test initially failed
  before implementation (RED).
- If RED verification is not applicable, the task MUST explicitly record that
  fact and the reason rather than fabricate a failure.
- Implementation MUST NOT be considered complete merely because code was
  generated; relevant tests MUST pass.

**Future.** The test-design stage is planned, not implemented: the state machine
reserves `TESTS_WRITTEN` and `RED_VERIFIED` and the local lifecycle walks them,
but the runner does not yet design or verify a failing test first.

## 3. Git Naming Conventions

- Branch: `task/<id>-<slug>` (for example, `task/S004-hybrid-retrieval`).
- Commit: `task(<id>): <description>` (for example,
  `task(S004): implement hybrid retrieval`).
- Pull request title: `[Task <id>] <title>` (for example,
  `[Task S004] Implement hybrid retrieval`).

The commit message MUST be derived from the task file, and a pull request MUST use
the deterministic `task/<id>-<slug>` branch.

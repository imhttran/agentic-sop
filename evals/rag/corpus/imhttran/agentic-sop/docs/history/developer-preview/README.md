# SOP Developer Preview — documentation index

> **Historical record — SUPERSEDED.** These are the Developer Preview deliverables,
> preserved after the `plan-sop-developer-preview` workstream was disposed
> `SUPERSEDED` by SOP (`sop plan historicalize --disposition SUPERSEDED`). They are
> non-normative historical evidence, not current planning authority.
>
> **Provenance.** PREVIEW-001 (`LOCAL_DONE`) ran under the governed harness. PREVIEW-002
> … PREVIEW-005 were **not** governed-completed: PREVIEW-002 parked at a human approval
> boundary (`./install.sh` requires approval) and the run loop stops at the first
> non-passing task, so their deliverables were authored by the operator and are recorded
> here as **operator-authored** evidence. They are **not** marked complete. The readiness
> verdict is **HOLD** and is retained unchanged in [READINESS.md](READINESS.md).
>
> **Lifecycle record.** SOP's archive for the plan is preserved outside the removed
> worktree at `.agent-sdlc/archive/plan-sop-developer-preview/` (gitignored). The
> superseded plan remains at
> [`docs/plans/PLAN-SOP-Developer-Preview.md`](../plans/PLAN-SOP-Developer-Preview.md).

This is the index for the **Developer Preview** of SOP (Agentic SOP). It collects the
preview's scope, verification evidence, known issues, and readiness assessment.

> **Preview boundary.** This preview is bounded to the checkout revision `d9f7722`
> (recorded in [SCOPE.md](SCOPE.md)). It is **not** a production-ready or
> full-coverage release. In particular, live-model reliability across the full
> controller and decision-adapter surface is **unverified** (see
> [KNOWN-ISSUES.md](KNOWN-ISSUES.md)). Acceptance-criterion enforcement is **default
> OFF**; the experimental hardening that would enforce it lives on the unmerged branch
> `harden-001` and is **not** part of this preview.

## Preview documents

| Document                                                 | When to read it                                                                                                         |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| [SCOPE.md](SCOPE.md)                                     | To see the frozen preview baseline revision, the pushed `origin/main` baseline, and the deferred `harden-001` boundary. |
| [QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md) | To see the exact install + first-run commands and their observed results on a disposable project.                       |
| [KNOWN-ISSUES.md](KNOWN-ISSUES.md)                       | To see the recorded limitations, their severity and impact, and what is still open.                                     |
| [READINESS.md](READINESS.md)                             | To see the GO/HOLD verdict, its evidence, and the recommended next steps.                                               |

## Where to start as a new user

1. Install the CLI and run the quickstart — [../../guides/GETTING-STARTED.md](../../guides/GETTING-STARTED.md)
   and the verified commands in [QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md).
2. Understand the project-side layout — [../../guides/PROJECT-SETUP.md](../../guides/PROJECT-SETUP.md).
3. Read the command surface — [../../reference/CLI.md](../../reference/CLI.md) and
   [../../reference/CONFIGURATION.md](../../reference/CONFIGURATION.md).

## What is preview-ready

- The `sop` CLI builds and installs from a checkout, and a first-run
  `sop init` → `sop status` → `sop run --task` completes on a disposable project
  ([QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md)).
- The governed local lifecycle (plan → implement → validate → review → quality → gate)
  runs deterministically in the recorded end-to-end fixtures.

## What is not preview-ready

- Live controller runtime parity and live decision-adapter dispatch (UNAVAILABLE —
  [KNOWN-ISSUES.md](KNOWN-ISSUES.md) KI-02).
- Deterministic enforcement of acceptance criteria (default OFF — KI-01).
- The documentation link gate (`scripts/checks/check-doc-links.sh`) does not pass on a
  clean tree (KI-03).

## Documentation-link status

`scripts/checks/check-doc-links.sh` currently reports `broken links: 14` and exits 1. The
breaks are pre-existing and point into gitignored `.agent-sdlc/` run artifacts (KI-03);
they are not introduced by the preview documents. Every link **in this index** resolves.

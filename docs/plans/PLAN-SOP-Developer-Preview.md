# PLAN — SOP Developer Preview

**Status:** PROPOSED — next proposed workstream; NOT activated, NOT executed.
**Basis:** HARDEN-001 checkpoint (`harden-001`, commit `1ed7189`) and the ETOE reliability assessment (`plan-sop-end-to-end-reliability`, ACTIVE).
**Independence:** the preview proceeds on `main` and is **independent of the experimental hardening** (`harden-001` is an unmerged branch; acceptance enforcement is **default OFF**, so preview behavior is unchanged).

## Project

SOP Developer Preview

## Summary

Prepare a coherent, defensible **Developer Preview** of SOP for external developers:
a frozen, reproducible baseline off `main`, a working install + quickstart, honest
documentation of what is preview-ready and what is not, and a known-issue register.
This plan does **not** merge or depend on the experimental acceptance-enforcement
hardening; it records that work as a deferred branch. No production-ready claim.

## PREVIEW-001 — Preview scope and freeze

Define the preview's authoritative baseline and freeze it: the `main` revision,
included repositories, and the explicit boundary that `harden-001` (acceptance
enforcement, default OFF) is deferred and not part of the preview.

### Dependencies

None

### Deliverables

- `docs/preview/SCOPE.md` — preview baseline revision, included components, and the deferred-branch boundary.

### Acceptance Criteria

- The preview baseline revision is recorded exactly and matches a pushed `main` commit.
- `harden-001` is recorded as deferred (unmerged; enforcement default OFF) with its checkpoint SHA.
- No component is claimed preview-ready without evidence.

### Validation Commands

`git rev-parse main`; `git branch -r --contains <sha>`; `git diff --check`.

### Failure Behavior

If the baseline cannot be frozen to a single pushed revision, stop and report.

## PREVIEW-002 — Install and quickstart verification

Verify a clean install path end to end: `./install.sh` builds and installs the CLI,
and a first-run quickstart (`sop init`, `sop status`, `sop run --task`) works on a
disposable project.

### Dependencies

- PREVIEW-001

### Deliverables

- `docs/preview/QUICKSTART-VERIFICATION.md` — exact commands, versions, and observed results.

### Acceptance Criteria

- `./install.sh` succeeds from a clean checkout and the installed binary reports a version.
- A disposable project completes `sop init` → `sop status` → a bounded `sop run --task`.
- Every command's observed result is recorded; unavailable steps are labeled UNAVAILABLE.

### Validation Commands

`./install.sh`; `sop version`; `sop init`; `sop status`; `git diff --check`.

### Failure Behavior

If install or quickstart fails, record the failing step and do not claim readiness.

## PREVIEW-003 — First-run reliability and known-issue register

Assess first-run reliability from the ETOE assessment and record a known-issue
register, including the pre-existing intermittent test
`TestCrossModuleTimeoutAndCancellation` (under `-race`) and the deferred hardening
items, each with severity and impact.

### Dependencies

- PREVIEW-001

### Deliverables

- `docs/preview/KNOWN-ISSUES.md` — reproducible issues with severity, impact, and disposition.

### Acceptance Criteria

- Each known issue is reproducible or explicitly marked UNAVAILABLE, with severity and impact.
- The intermittent cross-module test is recorded, not discarded.
- No issue is marked resolved without evidence.

### Validation Commands

`go test -race -count=3 ./internal/cli/...`; `git diff --check`.

### Failure Behavior

If an issue cannot be reproduced, record it as intermittent with the observations.

## PREVIEW-004 — Preview documentation completeness

Ensure the preview's user-facing documentation is complete and accurate: install,
configuration, the CLI surface, and the preview boundary — with no production-ready
or full-coverage claims.

### Dependencies

- PREVIEW-001

### Deliverables

- `docs/preview/README.md` — preview documentation index with a one-line "when to read it" per document.

### Acceptance Criteria

- Every preview document linked from the index exists and resolves.
- Documentation states the preview boundary and the enforcement-default-OFF posture.
- `scripts/checks/check-doc-links.sh` passes.

### Validation Commands

`scripts/checks/check-doc-links.sh`; `git diff --check`.

### Failure Behavior

If links or claims cannot be substantiated, correct or remove them; do not assert readiness.

## PREVIEW-005 — Preview readiness report and disposition

Produce the preview readiness report and a GO/HOLD recommendation, and recommend the
disposition of the deferred hardening branch — without activating, completing, or
historicalizing any plan.

### Dependencies

- PREVIEW-002
- PREVIEW-003
- PREVIEW-004

### Deliverables

- `docs/preview/READINESS.md` — GO/HOLD verdict with evidence, deferred work, and next steps.

### Acceptance Criteria

- A GO/HOLD verdict is backed by the recorded evidence with BLOCKED/UNAVAILABLE items listed.
- The deferred hardening branch and its checkpoint SHA are recorded.
- No plan is activated, completed, or historicalized by this task.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go test -count=1 ./...`; `git diff --check`; `scripts/checks/check-doc-links.sh`.

### Failure Behavior

If readiness criteria are unmet, report HOLD rather than GO.

## Dependencies (graph)

```text
PREVIEW-001 ──► PREVIEW-002, PREVIEW-003, PREVIEW-004
PREVIEW-002, PREVIEW-003, PREVIEW-004 ──► PREVIEW-005
```

## Parallelizable groups

- **After PREVIEW-001:** PREVIEW-002, PREVIEW-003, PREVIEW-004 (independent).
- **Then:** PREVIEW-005.

## Notes

- Scope above is a **proposal**; the operator should confirm the exact preview boundary before activation.
- No task in this plan depends on `harden-001` or enables acceptance enforcement.

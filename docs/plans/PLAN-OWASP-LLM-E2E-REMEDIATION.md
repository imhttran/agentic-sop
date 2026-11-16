# PLAN — OWASP LLM Top 10 end-to-end remediation (2026-11-16)

**Status:** active. Local implementation only; nothing merged.
**Repository:** `imhttran/agentic-sop`
**Revision:** `origin/main` at `c55b42ff` (2026-11-16).
**Acceptance report:** [acceptance-report-2026-11-16.md](../reports/owasp-llm/acceptance-report-2026-11-16.md).

This is the plan of record for the 2026-11-16 remediation pass. It does not
replace the ten per-risk plans ([PLAN-01](PLAN-01-SOP-LLM03-Excessive-Agency.md)
… [PLAN-10](../history/plans/PLAN-10-SOP-LLM10-Improper-Output-Handling.md)); it
records the order, dependencies, and acceptance criteria for closing the gaps
those plans left open.

## Goal

Evidence-backed security coverage for all ten OWASP 2025 LLM categories, with
deterministic Go remaining the execution-governance authority and decision
adapters staying advisory. A passing scan or a document marked complete is not
acceptance.

## Constraints (unchanged from the approved policy)

- `security.mode` stays `report-only`; `required_categories` stays empty until
  separately authorized.
- The approved `run_command` isolation policy and its operator override are not
  changed without explicit authorization.
- No baseline entry is added, suppressed, or removed without approval.
- C1 unknown-format redaction stays opt-in and disabled in repository config.
- Retrieval, decision adapters, and other optional capabilities keep their
  default enablement.

## Work items, dependencies, acceptance criteria

| #   | Work item                                                                                                | Depends on                                      | Acceptance criteria                                                                                                                                      | Status                                                                 |
| --- | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| W1  | Git output integrity: harden every SOP-run git invocation; label the non-neutralizable clean-filter case | none                                            | canary tests fail before the change and pass after; scan unchanged at 101/84/17/1; fingerprints still detect changes                                     | `IMPLEMENTED` / `VERIFIED`; merged at `2f544a62` (PR #491)             |
| W2  | Verify supply-chain scanners end to end                                                                  | none                                            | pinned adapters run to `COMPLETE`; an unpinned binary fails closed; a scanner failure can never yield `PASS`; baseline fingerprints unique and unexpired | `VERIFIED`                                                             |
| W3  | Verify the two merged LLM04 integrity fixes (#488, #489) and the index-reader guard                      | none                                            | source + focused tests confirm the controls; no unaccepted defect found                                                                                  | `VERIFIED`                                                             |
| W4  | Bounded verification of the remaining categories against their recorded evidence                         | none                                            | focused package tests pass; residuals restated honestly                                                                                                  | `VERIFIED`                                                             |
| W5  | Reconcile README, coverage statement, gap register, evidence matrix to the verified state                | W1–W4                                           | every claim is traceable to a file, test, or command in this pass; unmerged work is labelled unmerged                                                    | `DONE`                                                                 |
| W6  | M1 + G3 + G4: mandatory isolation for every model-reachable exec; deny `.git` writes                     | **operator authorization** (granted 2026-11-16, option B) | sandbox audit on every model-reachable exec; git tools refuse where no backend exists; `.git` writes refused; sandbox-enabled Linux job proves containment | `IMPLEMENTED` / `VERIFIED` on macOS; Linux containment `NOT_RUN` until `sandbox-linux` runs |
| W7  | Clean-filter fingerprint policy (Option C)                                                               | **operator decision** (granted 2026-11-16)      | verified git-lfs accepted; any other configured driver fails closed and is labelled                                                                      | `IMPLEMENTED` / `VERIFIED`                                             |
| W8  | Scanner enforcement (`report-only` → required categories)                                                | **operator decision** (deferred 2026-11-16)     | an explicit `mode` / `required_categories` change with its own authorization                                                                             | `DEFERRED`                                                             |

## Sequence

1. W2 and W3 first: they establish whether the existing controls still hold, and
   they are free of policy change.
2. W1 next: the narrowest authorized remediation for the execution-boundary and
   git-output finding.
3. W4: confirm the remaining categories.
4. W5: reconcile documentation last so it describes the verified state.
5. W8 stays deferred on a decision; a blocker in it must not gate other work.

## Guardrails for anyone executing this plan

- Do not treat sandbox containment as proof that git output is trustworthy. The
  containment and the output-trust questions are separate (W1 vs W6).
- Do not reproduce real credentials. Use synthetic canaries only.
- Do not claim merged state for unmerged work.
- Do not enable C1 globally, change accepted residual dispositions, or change
  `security.mode`/`required_categories` as part of W1–W5.
- A blocker in W6–W8 must not prevent W1–W5 from progressing.

## Verification gates

| Gate                | Command                                                                                    | Used by                            |
| ------------------- | ------------------------------------------------------------------------------------------ | ---------------------------------- |
| build               | `go build ./...`                                                                           | all                                |
| vet                 | `go vet ./internal/toolharness/`                                                           | W1                                 |
| focused tests       | `go test ./internal/toolharness/ -count=1` and the packages named in the acceptance report | W1–W4                              |
| architecture guards | `go test ./internal/archtest/ -count=1`                                                    | W3, LLM10 process-group guard      |
| scanner             | `sop security scan` with pinned adapters on `PATH`                                         | W2                                 |
| full CI             | repository CI workflow                                                                     | W5 closeout (NOT RUN in this pass) |

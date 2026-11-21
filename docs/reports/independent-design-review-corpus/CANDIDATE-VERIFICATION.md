# Candidate Evidence Verification — Independent Design Review Corpus

**Type:** Verification record (non-normative). It records a mechanical and manual check of
the candidate manifest. It is not an experiment result, does not freeze the corpus, and
does not record an adjudication.
**Verification baseline:** the readiness branch based on `origin/main` @ `ade6864a`, plus
the restored pilot evidence commit. Candidate revisions are the commits recorded in
[candidates.json](candidates.json).
**Manifest:** [candidates.json](candidates.json).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Readiness summary:** [READINESS.md](READINESS.md).

## 1. Method

For every candidate in the manifest:

1. The recorded `pre_fix_commit` is resolved and `original_artifact` is read from that
   commit.
2. The artifact SHA-256 is recomputed and compared with `artifact_sha256`.
3. Every path in `permitted_references` is checked for existence at the recorded commit.
   A reference that does not exist at that revision is not reviewer-visible input.
4. `permitted_references` is compared with `later_evidence`. Any overlap is treated as
   answer leakage.
5. The artifact and its references are read to decide whether the recorded defect is
   discoverable without the later report. The result is recorded as **strict**,
   **partial**, **self-referential**, **answer-bearing**, or **unverified**.

This record distinguishes three things that are easy to conflate:

- **Verified defect evidence** — a later report, fix, or audit demonstrates that the defect
  exists.
- **Proposed determinability** — a judgment that a reviewer at the pinned revision could
  detect it. This remains a proposal.
- **Adjudicator-confirmed determinability** — a human adjudication. This remains **0** for
  every candidate.

## 2. Structural results

All 17 candidates that record an artifact pass the SHA-256 check. Six entries have no
artifact and are not counted: `IDR-E-006`, `IDR-E-011`, `IDR-E-012`, `IDR-X-001`,
`IDR-X-002`, `IDR-X-003`.

Two manifest corrections were required and applied:

- `IDR-H-003` permitted two paths that do not exist at `56ff82ac`
  (`internal/cli/acceptance_gotest.go`,
  `internal/planflow/acceptance_checks_reconcile_test.go`). They were replaced with
  acceptance-mapping paths that exist at that revision.
- `IDR-H-006` permitted exactly the two later reports that supply the answer. They were
  replaced with pre-fix scanner paths; the reports remain `later_evidence` only.
- `IDR-H-007` and `IDR-H-008` are excluded and their permitted references were the later
  evidence. Both lists were cleared so the exclusion is unambiguous.

## 3. Candidate-by-candidate record

| ID | Artifact @ commit | SHA-256 | Permitted refs | Leakage | Discoverability | Disposition |
| --- | --- | --- | --- | --- | --- | --- |
| `IDR-H-001` | `PLAN-SOP-ACP-ZED.md` @ `e462deed` | match | exist | none | The plan and its gap report first appear in `79acbfc1`; `79acbfc1^` has no plan. No uncontaminated pre-defect revision exists. | Held pending corpus-owner review. |
| `IDR-H-002` | `PLAN-Code-Retrieval-Context-Seam.md` @ `03a7d7d9` | match | exist | none | The only later evidence is the plan's own post-merge status line, which is self-referential. | Excluded as a corpus input. |
| `IDR-H-003` | `PLAN-SOP-RUN-ACCEPT-001.md` @ `56ff82ac` | match | corrected, all exist | none | The six-check failure appears only under the later `### Acceptance Mapping` contract. | Proposed partial. |
| `IDR-H-004` | `PLAN-04-SOP-LLM04-Supply-Chain.md` @ `e55527f4` | match | exist | none | Historical outcomes remain PARTIAL until re-execution under the later acceptance mapping. | Proposed partial. |
| `IDR-H-005` | `PLAN-03-SOP-LLM02-...Disclosure.md` @ `e5d45fc1` | match | exist | none | `e5d45fc1` adds the plan, the disclosure report, and the GAP-01/02 fixes. It does not predate discovery; no earlier plan revision exists. | Held pending corpus-owner review. |
| `IDR-H-006` | `PLAN-Security-Scanner-Addon.md` @ `1b611711` | match | corrected, all exist | none | The pre-fix plan and scanner code show the coverage surface. The specific residual findings and gaps are stated only in the later reports. | Proposed partial. |
| `IDR-H-007` | `PLAN-08B-SOP-LLM08-...Closure.md` @ `aa3e1d36` | match | cleared | artifact is a follow-up | The follow-up plan records the answer; the parent Plan 08 artifact is missing. | Excluded. |
| `IDR-H-008` | `PLAN-09-SOP-LLM09-...Weaknesses.md` @ `05685308` | match | cleared | artifact is a follow-up | The follow-up plan records the answer; the parent Plan 09 artifact is missing. | Excluded. |
| `IDR-E-001` | `docs/specs/EXECUTION.md` @ `3be4a90` | match | exist | none | Partial: the defect depends on a later hardening record. | Proposed partial. |
| `IDR-E-002` | `docs/specs/HUMAN-APPROVAL.md` @ `94cace0c` | match | exist | none | Partial: the deadlock appears only after a later accepted revision invalidates approval evidence. | Proposed partial. |
| `IDR-E-003` | `docs/specs/VALIDATION.md` @ `8980826` | match | exist | none | Partial: the reuse gap is named by the later follow-up. | Proposed partial. |
| `IDR-E-004` | `docs/specs/RECOVERY.md` @ `a64e06cd` | match | exist | none | Partial: the limitation was recorded before it was resolved. | Proposed partial. |
| `IDR-E-005` | `docs/specs/REVIEW.md` @ `ba7736d4` | match | exist | none | The pre-fix spec already required invocation-scoped input and a deterministic malformed-output boundary, so the defect was in code, not the design document. | Excluded as a design-corpus input. |
| `IDR-E-006` | — | — | — | — | Parent artifact not identified. | Unverified. |
| `IDR-E-007` | `docs/specs/WORKFLOW.md` @ `d0d89d03` | match | exist | none | Partial: a later lifecycle-consistency audit names the gap. | Proposed partial. |
| `IDR-E-008` | `docs/specs/AGENT-PROVIDER.md` @ `6c839f2e` | match | exist | none | Partial: a later convergence record names the gap. | Proposed partial. |
| `IDR-E-009` | `docs/architecture/COMPONENTS.md` @ `6c839f2e` | match | exist | none | Partial: a later metrics/observability record names the gap. | Proposed partial. |
| `IDR-E-010` | `docs/architecture/OVERVIEW.md` @ `7de7c68a` | match | exist | none | The artifact is too broad to support a single determinable defect. | Unverified. |
| `IDR-E-011` | — | — | — | — | No single artifact. | Excluded. |
| `IDR-E-012` | — | — | — | — | Parent artifact not identified. | Unverified. |
| `IDR-X-001` | — | — | — | — | Recorded as not a defect. | Excluded. |
| `IDR-X-002` | — | — | — | — | Recorded as not a defect. | Excluded. |
| `IDR-X-003` | — | — | — | — | Recorded as not a defect. | Excluded. |

## 4. What the verification does and does not establish

Established:

- The recorded revisions resolve, and every recorded artifact hash matches.
- Every permitted reference now exists at its pinned revision.
- No permitted reference overlaps the later evidence for any candidate.
- Two answer-bearing candidates remain excluded, and two held candidates remain held.

Not established:

- No candidate is independently adjudicated determinable. The adjudicator-confirmed count
  is **0**.
- The partial classifications are still proposals. The corpus owner must confirm them, and
  the proposed eligibility amendment is not approved.
- The verification does not show that a reviewer would find the defect in a run. That is
  the experiment's question, not this record's.

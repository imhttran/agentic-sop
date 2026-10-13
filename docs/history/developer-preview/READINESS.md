# Preview readiness report

**Task:** PREVIEW-005 (Preview readiness report and disposition)
**Verdict:** **HOLD** — see §1.
**Baseline:** preview checkout `d9f7722`; pushed baseline `origin/main` `d181bbc`.
**Provenance:** this report is a synthesis of the preview deliverables and the preserved
end-to-end reliability evidence. It activates, completes, or historicalizes **no** plan.

---

## 1. Verdict: HOLD

The Developer Preview is **not** recommended as GO. HOLD is forced by three independent
gaps, each backed by recorded evidence:

1. **Acceptance criteria are not enforced.** SOP records `LOCAL_DONE`/`PASS` while declared
   acceptance criteria are unmet ([KNOWN-ISSUES.md](KNOWN-ISSUES.md) KI-01; ETOE-007 0/3;
   PREVIEW-001 criterion 1 UNMET). A gate pass cannot be read as "acceptance met".
2. **Controller runtime and live decision-adapter integration are UNAVAILABLE.** 0 of 12
   controller parity tests ran; live `decision.provider: command` dispatch was never
   observed (KI-02; `docs/reports/end-to-end-reliability/ETOE-010-final-reliability-report.md`
   §1–§2).
3. **The documentation link gate does not pass.** `scripts/checks/check-doc-links.sh`
   reports 14 broken links on a clean tree (KI-03).

The preview's own first-run path, by contrast, **is** verified end to end: install →
`init` → `status` → `run --task` completes with a real model
([QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md)).

---

## 2. Task status and acceptance

| Task | SOP status | Acceptance (independently verified) | Notes |
|---|---|---|---|
| PREVIEW-001 | `LOCAL_DONE` (gate PASS) | **2 of 3 MET** — criterion 1 **UNMET** | Checkout revision is not a pushed `main` commit; recorded as a conflict, not weakened ([SCOPE.md](SCOPE.md)). |
| PREVIEW-002 | `FAILED` / `CONTINUE` (approval boundary) | **3 of 3 MET (operator-verified)** | Governed run could not run `install.sh`; the operator ran the install and quickstart and recorded the results ([QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md)). |
| PREVIEW-003 | `PLANNED` (not governed-executed) | **3 of 3 MET (operator-authored)** | Deliverable authored because the plan runner is blocked behind PREVIEW-002. |
| PREVIEW-004 | `PLANNED` (not governed-executed) | **2 of 3 MET** — criterion 3 **UNMET** | `check-doc-links.sh` fails on pre-existing breaks (KI-03). |
| PREVIEW-005 | this report | — | Produced to record the disposition. |

> **Governance note.** PREVIEW-002 hit a human approval boundary (`REQUIRES_APPROVAL` on
> `install.sh`). Because the run loop stops at the first non-passing task
> (`internal/cli/drive.go`), governed execution cannot advance to PREVIEW-003/004 while
> PREVIEW-002 is unresolved. The delivered documents are therefore operator-produced; no
> SOP state was edited by hand and no approval was bypassed.

---

## 3. BLOCKED / UNAVAILABLE coverage

| Item | Status | Source |
|---|---|---|
| Controller runtime parity (PT-1…PT-12) | **BLOCKED/UNAVAILABLE** (0/12 executed) | ETOE-007; ETOE-010 §2.1–§2.2 |
| Live decision-adapter dispatch | **UNAVAILABLE** | ETOE-004 §3.1; ETOE-008 §3 |
| Live external model/provider metrics | **UNAVAILABLE** in the ETOE series | ETOE-010 §2.6 |
| `install.sh` under the governed harness | **BLOCKED** (`REQUIRES_APPROVAL`) | `.agent-sdlc/runs/PREVIEW-002/`; KI-04 |
| Documentation link gate | **FAILED** (14 broken links) | KI-03 |
| Acceptance-criterion enforcement | **OFF by default** (defect unresolved) | KI-01 |

---

## 4. Deferred work

- **Hardening branch `harden-001`** — trusted acceptance-criterion verification,
  **unmerged**, enforcement **default OFF**. Checkpoint SHA
  `1ed7189a48dabfcc14735e9fb660b00c7ad4b855` (also `origin/harden-001`). Not part of the
  preview baseline.
- **PLAN-SOP-E2E-Reliability-Hardening** — the proposed hardening plan
  (`docs/plans/PLAN-SOP-E2E-Reliability-Hardening.md`), PROPOSED, not activated.
- **ETOE reliability assessment** — `plan-sop-end-to-end-reliability` remains ACTIVE with
  the HOLD disposition; not closed or historicalized.

---

## 5. Recommended next steps

1. Resolve the PREVIEW-002 approval boundary deliberately: either admit a sanctioned
   install step in the governed harness, or record install verification as an operator-run
   step (as done here), then let the plan advance to PREVIEW-003/004.
2. Fix or remove the dangling `examples/` reference and the broken documentation links.
3. Decide the disposition of `harden-001` (review/merge later; it is the fix for KI-01).
4. Do **not** publish a GO or a production-ready claim until the controller runtime and
   live decision-adapter scope is verified (KI-02).

---

## 6. Explicit non-actions

- No plan was activated, completed, or historicalized by this task.
- No SOP state (`.agent-sdlc/`) was edited by hand; no approval was bypassed.
- The `harden-001` branch was not merged.

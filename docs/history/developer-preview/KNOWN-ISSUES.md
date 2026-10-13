# Known issues and limitations

**Task:** PREVIEW-003 (First-run reliability and known-issue register)
**Scope:** Issues observed while preparing the Developer Preview. Each entry states a
severity, impact, disposition, and a reproduction where one exists. No issue is marked
resolved without evidence; issues that could not be reproduced are recorded as
intermittent with the observations.

**Severity scale:** `HIGH` blocks a preview or a release claim · `MEDIUM` degrades trust
or coverage · `LOW` cosmetic or informational.

---

## KI-01 — Acceptance criteria are not enforced by the quality gate (HIGH)

**Impact:** A task can be marked `LOCAL_DONE` with its declared acceptance criteria unmet.
This is a trust/verification escape: SOP's own gate records a pass while the deliverable
does not satisfy the task's acceptance criteria.

**Evidence:**

- ETOE-007 (`docs/reports/end-to-end-reliability/ETOE-007-controller-parity.md`) is recorded
  `LOCAL_DONE` in the ETOE series while its runtime acceptance criteria are **0/3 MET**.
- PREVIEW-001 is `LOCAL_DONE` with gate `PASS` (`.agent-sdlc/runs/PREVIEW-001/gate.json`)
  while acceptance criterion 1 is **UNMET** (its checkout revision is not a pushed `main`
  commit — see [SCOPE.md](SCOPE.md)).

**Disposition:** OPEN — deferred to the hardening branch `harden-001` (unmerged, acceptance
enforcement **default OFF**). Tracked as HARDEN-001/-001b/-001c/-001d.

**Reproduction:** any task whose specification declares `## Acceptance Criteria` that the
implementation does not meet still reaches `COMPLETE PASS` under the default configuration.

---

## KI-02 — Controller runtime and live decision-adapter integration are UNAVAILABLE (HIGH)

**Impact:** A material part of the end-to-end reliability scope is unverified, so no
production/GO claim can be made for the controller integration surface.

**Evidence:** `docs/reports/end-to-end-reliability/ETOE-010-final-reliability-report.md`
§1–§2: 0 of 12 controller parity tests executed; controller runtime BLOCKED; live
`decision.provider: command` dispatch UNAVAILABLE.

**Disposition:** OPEN — release blocker for a GO; not required to ship an isolated preview
of the CLI.

---

## KI-03 — `scripts/checks/check-doc-links.sh` fails on a clean tree (MEDIUM)

**Impact:** The documentation link gate does not pass, so any task whose validation
includes it (e.g. PREVIEW-004 criterion 3) cannot meet that criterion without fixing
unrelated historical documents.

**Evidence:** running `./scripts/checks/check-doc-links.sh` in this checkout reports
`broken links: 14` and exits 1. All 14 links point into gitignored run artifacts, e.g.
`docs/reports/PLAN-compiler-reconciliation-hardening.md -> ../../.agent-sdlc/runs/prompts/prompt-20261007-011846/report.md`.
Because `.agent-sdlc/` is gitignored, these targets do not exist in a fresh checkout.

**Disposition:** OPEN — the links should be removed or rewritten to non-run-artifact
targets. Outside the preview task's scope (it would edit historical reports).

**Reproduction:** `./scripts/checks/check-doc-links.sh` (run with `bash`; the script uses
process substitution and fails under `sh`).

---

## KI-04 — The governed tool harness denies `./install.sh` (MEDIUM)

**Impact:** Install/quickstart verification cannot run inside a governed task: the harness
returns `REQUIRES_APPROVAL` for `run_command ./install.sh`, so PREVIEW-002 produced no
observed results on its own and its deliverable had to be filled by the operator.

**Evidence:** `.agent-sdlc/runs/PREVIEW-002/attempt.txt` and `classification.json`
(`INCOMPLETE_IMPLEMENTATION` / `CONTINUE`): "I cannot execute it due to REQUIRES_APPROVAL
restrictions on run_command".

**Disposition:** OPEN — either admit a sanctioned install step or document that install
verification is an operator-run step. See [QUICKSTART-VERIFICATION.md](QUICKSTART-VERIFICATION.md).

---

## KI-05 — Acceptance-verification escapes are invisible to the run report (MEDIUM)

**Impact:** When a task reaches `COMPLETE PASS` with unmet acceptance criteria, nothing in
the run report flags the discrepancy; a reviewer must independently re-derive the
criteria. This is the reporting half of KI-01.

**Evidence:** `PREVIEW-001`'s `gate.json` is `{"Decision":"PASS","Reasons":["all required
checks passed"]}` with no criteria outcome recorded; the acceptance mismatch was found only
by independent re-verification.

**Disposition:** OPEN — the hardening branch adds a `criteria.json` audit artifact; it is
not yet enabled (default OFF).

---

## KI-06 — Activating a different plan can implicitly archive a satisfied active plan (MEDIUM)

**Impact:** Activating a successor plan while a fully-satisfied plan is active archives the
active plan `COMPLETE` (`internal/planflow` `handOff`). An operator can therefore close a
plan as a side effect of activation, without an explicit closure decision.

**Evidence:** observed while setting up the preview; avoided by isolating the preview in
its own worktree/project (`sop-preview-001`) so the active reliability assessment was never
touched.

**Disposition:** OPEN — governance/UX follow-up; documented so operators isolate plans.

---

## KI-07 — Governed harness is repository-root confined (MEDIUM)

**Impact:** Governed tasks cannot reach sibling repositories (`sop-controller`,
`sop-decision-adapters`) and cannot run remote-ref Git commands, which is the root cause of
KI-02's BLOCKED/UNAVAILABLE controller and adapter coverage.

**Evidence:** `internal/toolharness` rejects `../`, unauthorized roots, and
`git -C/--git-dir/--work-tree`; ETOE-007 §1 records the failing reachability probe.

**Disposition:** OPEN — a dedicated, operator-authorized cross-repository integration
workspace is proposed in the hardening plan (HARDEN-002).

---

## KI-08 — Intermittent `TestCrossModuleTimeoutAndCancellation` under `-race` (LOW)

**Impact:** A pre-existing `internal/cli` cross-module process/PID test intermittently fails
under `go test -race`; it passes on rerun. It can produce a spurious red CI signal but does
not affect product behavior.

**Evidence:** observed failing once under `-race` during the hardening work, passing on
reruns; see HARDEN-001-d Phase 4D notes.

**Disposition:** OPEN — recorded, not discarded. Repro:
`go test -race -count=3 ./internal/cli/...`.

---

## KI-09 — `examples/` is referenced but absent (LOW)

**Impact:** The `sop` skill tells users examples live in `examples/`
(`skills/sop/SKILL.md`), but no `examples/` directory exists in the repository, so the
reference dangles.

**Evidence:** `ls -d examples` → absent; `grep -n "examples/" skills/sop/SKILL.md` → line 107.

**Disposition:** OPEN — add the examples or remove the reference.

---

## KI-10 — The installed binary can silently be stale (LOW)

**Impact:** Until this verification ran, the installed `sop` at `/Users/imhttran/go/bin/sop`
was older than the checkout (it lacked newer commands such as `run --max-tasks`), which can
mislead anyone verifying against the installed binary. `install.sh` corrects this.

**Evidence:** observed during preflight (`sop version` returned `sop dev` from a binary built
2026-10-07); `./install.sh` rebuilt it from `d9f7722`.

**Disposition:** RESOLVED for this checkout by re-running `./install.sh`; the general risk
(no version-to-revision binding) remains a LOW follow-up.

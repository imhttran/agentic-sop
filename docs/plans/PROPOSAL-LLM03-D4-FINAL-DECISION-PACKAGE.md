# D4 final operator decision package — LLM03 scanner enforcement (OWASP gap 9)

**Status:** decision package, read-only. **Nothing is activated or accepted here.** Required
categories stay empty, the scan stays report-only, the 56-entry baseline is unchanged, the
`run_command` residual is **not accepted**, and D4 enforcement is not enabled.

**Verified at:** `origin/main` `7c3d49b3` (2026-11-13), darwin arm64, Go 1.27.1, pinned
Gitleaks 8.30.1, Semgrep 1.183.0, OSV-Scanner 2.7.0.

**Merged prerequisites:** #467 (enforcement assessment), #468 (decision matrix), #471
(approval packet + reconciliation), #472 (investigations + advisory drift diagnostic), #473
(status reconciliation), #474 (M1/M3 + Semgrep 1.183.0), #475 (drift P2 fix), #476 (D4-5
boundary packet), #477 (wording fix), #478 (acknowledged override).

**This package is the only remaining reading required to decide.** It supersedes the
decision framing in the earlier packets; their evidence stays authoritative.

## 1. Verified current state

| Item | Value |
| --- | --- |
| `security.mode` | `report-only` |
| `required_categories` | empty |
| Baseline entries | **56** (unchanged) |
| Full-workspace scan | **101 findings, 40 suppressed, 61 unsuppressed, 45 blocking** |
| Adapters | gitleaks 8.30.1, semgrep 1.183.0, osv-scanner 2.7.0 — all `COMPLETED` / `COMPLETE` |
| Verdict | `FAIL` in report-only (advisory) |
| Stale baseline entries | **16** match no finding |
| Residual | `internal/toolharness/harness.go:926`, `semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771` |

Two corrections since the earlier packets, both mechanical:

- The five Gitleaks findings that the decision documents introduced by quoting placeholder
  shapes were **redacted**, not baselined. The count returned from 50 to **45 blocking**.
- One test-only finding moved when #478 added tests: `isolation_test.go` 677 → **766**.

## 2. Decision A — enforcement mode

| Option | Meaning | Measured outcome today | Assessment |
| --- | --- | --- | --- |
| **A** | keep report-only | `FAIL`, 45 blocking, delivery not blocked | No enforcement |
| **B** | required `secrets` + `dependencies` | `FAIL`, **45 blocking** | **Dominated**: same blocking set, one fewer coverage guarantee; making it pass means the same dispositions or disabling SAST |
| **C** | required `secrets` + `dependencies` + `sast` | `FAIL` → `PASS` once groups A–D are approved | The only coherent gate |

**Recommendation:** **A now; C after the baseline approvals in §3.** Do not choose B.
The gate binds the SOP quality stage (`internal/quality`), not GitHub CI.

**Scope of C when chosen:** `security.mode: required`,
`required_categories: [secrets, dependencies, sast]`, `scope: workspace`; `fail_on`,
`unknown_severity`, `stale_advisory_database`, adapters and exclusions unchanged.

## 3. Decision B — fingerprint-level baseline approvals

Set digests are SHA-256 over the group's sorted, newline-joined fingerprints. All groups were
recomputed from the verified scan above.

| Group | Rows (matrix §1) | Count | Expiry | Digest |
| --- | --- | --- | --- | --- |
| A — Gitleaks fixtures | 1–27 | 27 | 2027-06-30 | `4596c8cc…` |
| B — test-only Semgrep HIGH | 33, 42–45 | 5 | 2027-06-30 | `2e5f7654…` |
| C — production Semgrep | 28–32, 34–40 | 12 | 2027-03-31 | `b61b105f…` |
| D — residual | 41 | 1 | 2027-03-31 | `8a665937…` |
| A+B+C | — | 44 | — | `d24bd19f…` |
| E — non-blocking MEDIUM (optional) | §3.4 | 16 | 2027-06-30 | `55268654…` |

Full values are in the operator packet §"Approval integrity" and the decision matrix.

**Recommendation:** approve **A + B + C**, and decide **D** in §4. Omit **E** (it does not
block; adding it creates 16 expiries with no enforcement gain).

**Also recommend, with the same change:** retire the **16 stale entries** that match nothing.
They were already reviewed; they now suppress nothing and only create noise (the advisory
drift warning reports them after every complete workspace scan).

**Expected result once A–D are approved:** `PASS` with `COMPLETE` coverage, 0 blocking.

## 4. Decision C — acknowledged unsandboxed execution

**Already implemented (R4 A + B, PR #478):** isolation is mandatory on macOS and Linux; the
override takes effect only with `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK=1`; both control
variables are dropped from model-controlled children; a no-backend platform still refuses;
`Config.IsolateCommands` still forces isolation on.

**What that leaves open:** with a deliberate, acknowledged override, model-authored code still
runs unsandboxed — via `go test` and `go build`, with network and readable credential files.
That is the narrowed residual. Nothing about it is accepted.

| Option | Meaning | Note |
| --- | --- | --- |
| **R1** | accept the acknowledged-override exposure with an expiry | Requires an explicit operator statement; an expiry does not reduce the exposure |
| **R2** | keep Option A (report-only) and revisit | Safe fallback; no enforcement |
| **R3** | remove `run_command` | Eliminates the exposure; loses test execution |

R4 is **implemented**; no separate decision is needed for it. Only R1/R2/R3 remain.

**Recommendation:** **R2 while enforcement stays A.** If you choose C, decide R1 explicitly
before enabling the gate, because the gate would otherwise certify a boundary that a
deliberate override can open.

## 5. Decision D — Semgrep version enforcement

**Already decided and implemented:** CI and the tracked configuration both name **1.183.0**;
CI installs from a hash-locked requirements file; the interpreter guard accepts Python 3.10
and 3.11. Semgrep's declared version remains **descriptive** — the adapter records it but
never refuses a mismatch, unlike the Gitleaks digest pin.

| Option | Meaning | Assessment |
| --- | --- | --- |
| **V-A (current)** | lockfile pins CI; config records the local version | Sufficient: CI is hash-locked, and 1.136.0 and 1.183.0 produced identical fingerprints on this repository |
| **V-B** | add an enforced version check comparable to the Gitleaks digest pin | Higher assurance, but a venv has no single digest and the rules pin already fixes analysis content |

**Recommendation: V-A.** Keep the version descriptive and the lockfile authoritative. Revisit
only if a future Semgrep release changes fingerprints.

## 6. Remaining optional hardening

| Item | Effect | Status |
| --- | --- | --- |
| Deny `.git/config` writes | closes repository-configured command execution from SAFE git commands | found in the D4-5 packet; not authorized |
| Retire the 16 stale baseline entries | removes inert entries and the drift warning | bundled with §3 |
| `TMPDIR` socket-path fix in `TestRunCommandIsolationDeniesNetwork` | removes a local-only `test-fast` failure | unrelated to security |
| V-4 (enforced Semgrep check) | see §5 | recommended against for now |

None of these blocks closing Gap 9.

## 7. Minimum decisions to close Gap 9

1. **D4-1:** choose **A** (defer enforcement) or **C** (enable after approvals). B is not
   recommended.
2. **D4-2 / D4-3 / D4-4:** approve groups **A**, **B**, **C** by digest (and the 16 stale
   retirements).
3. **D4-5:** choose **R1**, **R2**, or **R3** for the acknowledged-override exposure.
4. Optional: **D4-6** (group E), **V-4**, `.git/config` hardening.

Choosing (1) plus the matching dispositions in (2) and (3) closes Gap 9: the gate is either
enabled with reviewed, expiring exceptions or explicitly deferred with a recorded reason.

## 8. Decision-memory record: verified, prepared, not written

**Verified state of decision memory on this checkout:**

| Fact | Value |
| --- | --- |
| Store | `.agent-sdlc/context/decisions.json` (schema `decisionmemory/1`, limit 128) |
| Records | 32, all bound to repository identity `aaff5cba…` |
| Applicable now | **0** (`sop memory list`: "32 stale decision(s) from another repository state") |
| Advisory retrieval | **disabled** (`sop memory status`: `enabled=false`) |
| Version control | `.agent-sdlc/context/` is excluded via `.git/info/exclude` — **machine-local, not reviewed in git** |
| Applicability rule | `scope` **and** exact `RepositoryIdentity(head, dirty)` must match, so every new commit makes the record stale |

**Consequence:** `sop memory add` is a local, commit-bound note, not the durable record. The
durable, reviewable record is this committed document plus the merged PRs. A memory entry is
still useful for local agent context, and is prepared below.

**Prepared command (not executed):**

```bash
sop memory add --scope architecture \
  --source "operator:d4-5-r4-a-b-2026-11-13" \
  --decision "D4-5 R4 A+B: run_command isolation is mandatory on macOS and Linux; the operator override takes effect only with SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK=1, and both isolation control variables are dropped from model-controlled child environments (PRs #477/#478)." \
  --reason "Close the unintended unsandboxed-execution path without removing the compatibility escape hatch for hosts where the sandbox backend cannot start." \
  --evidence "PRs #477/#478; TestIsolationDecisionSupportedPlatforms, TestIsolationDecisionUnsupportedPlatforms, TestRunCommandIsolationOffWithoutAckStaysSandboxed, TestRunCommandChildEnvironmentDropsIsolationControls, TestIsolationPolicyNotice; docs/plans/PROPOSAL-LLM03-D4-5-ISOLATION-BOUNDARY.md §7."
```

This text records **authorization and implementation only**. It deliberately contains no
sentence accepting, waiving, or bounding the residual; acceptance remains §4.

**Why it was not written:** the operator's authority for the implementation is recorded in the
merged PRs and this package, but a decision-memory entry asserts an attribution and is
local-only, so it should be written by the operator (or on explicit instruction) rather than
inferred.

## What this package does not do

- Enables no enforcement, sets no required category, and changes no `fail_on`.
- Adds, removes, or approves no baseline entry.
- Accepts no residual risk.
- Writes no decision memory.
- Changes no historical decision or evidence; it records the two mechanical corrections in §1.

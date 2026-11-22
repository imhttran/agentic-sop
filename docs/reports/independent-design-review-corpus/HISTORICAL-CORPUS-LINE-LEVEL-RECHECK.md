# Historical Corpus — Line-Level Re-check of Five Leads (GH-004 (d), GH-010, GH-011, GH-012, GH-014)

**Type:** Verification record (non-normative, read-only). It carries out the two follow-ups that needed no authorization from
[HISTORICAL-CORPUS-EVIDENCE-GAP-CLOSURE.md](HISTORICAL-CORPUS-EVIDENCE-GAP-CLOSURE.md) §10. It is **not** candidate promotion, adjudication, a freeze, or an experiment result.
**Every item is PROPOSED — NOT APPROVED.**
**Baseline:** `origin/main` @ `f1a34c285c7b18a2ad15bcd134b39552f6631355` (merge of PR #634), 2026-11-22.
**Builds on:** [#620](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md), [#626](HISTORICAL-CORPUS-STRICT-EVIDENCE-VERIFICATION.md), [#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md),
[the eligibility decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md), [CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md). Not modified; [candidates.json](candidates.json) unchanged.

## 1. Result

- **`GH-004` defect (d) is now verified**, so `GH-004` has **4 verified defects** (a, b, c, d) rather than 3 + 1 partial.
- **`GH-010`, `GH-011`, `GH-012`, `GH-014` are verified at the line level** with exact lines, timing, and checks that the reviewed revision is the merged one. All four hold. `GH-010`'s Linux claim rests on the author's own source comment and is **not reproduced** (no Linux host here).
- **C0–C3 do not change** (3 / 4, 4 / 5, 13 / 15, 11 / 15): these leads were already in C2 (`GH-010`–`GH-014`) or held (`GH-004`). The conditional scenario S7, defined in the gap-closure report §8 (PR #634 with its fix #635), is **C2 13 / 15 + `IDR-L-001` (+1 plan / +1 defect, needs R-D) + `GH-004` (+1 plan / +4 defects) = 15 plans / 20 defects** (it was 19 with three verified defects for `GH-004`). It is a conditional route, not a total: it needs R-D yes, approval packets admitted, R-B Option B, R-A for `GH-004`'s PR-head pins, artifact counting and every adjudication to succeed. No plan is added.
- **Not done:** the third follow-up, a different evidence source (the sibling repositories). It needs the operator's authorization and a scoping decision, and nothing was opened there.

## 2. `GH-004` (d) — resolved

**Claim (PR #590 thread `4655296399`, pin `022e7568555e1fc3990869d9c1ff95f4cfe15880`):** after reconciling the edited plan and retrying only `stage-10`, the persisted `stage-1` task stays `BLOCKED` and keeps the plan from completing, because reconcile preserves lifecycle state and the packet gives no disposition.

**Earlier uncertainty:** `hasExecution`'s comment says a task that has never executed may be removed, and the packet reports `stage-1` at attempt 0 / 3.

**Verified at the pin:**

| Fact | Source at `022e7568…` |
| --- | --- |
| The packet records `stage-1` as `BLOCKED` (RETRIES_EXHAUSTED, attempt 0/3) and "now describes a dropped step" | `PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md:29` |
| "Executed" is defined by status, not only attempts: attempt count > 0, **or** a status outside `PLANNED` / `READY`. `BLOCKED` is outside, so the task counts as executed with 0 attempts | `internal/domain/task.go:257–273` (`Task.Executed`) |
| `hasExecution` defers to `Executed`, so a removed `BLOCKED` task is a removed **executed** task | `internal/planflow/planflow.go:1236–1238` |
| A removed executed task "cannot be approved with `AcceptChanged`: a removed executed task must be kept or completed first" | `planflow.go:651–654` (`RemovedExecuted`) |
| A changed executed task is redefined while preserving "its ID, lifecycle state, attempts, budget, and creation time" | `planflow.go:1212–1226` (`redefineTask`) |
| The packet keeps `stage-1` in the plan as a "Dropped" stub and directs "`sop retry stage-10` … Do not retry `stage-1`: it is dropped" with no disposition for `stage-1` | packet lines ~121 and 155–161 |

**Conclusion (verified):** the stub is a changed executed task (redefined, state preserved) and cannot be removed by reconcile, so `stage-1` stays `BLOCKED` and no step in the packet retires or completes it. Defect (d) is **determinable from the packet and the code at its pin**.
The earlier doubt resolved the other way: attempt 0 does not make the task "never executed" because `BLOCKED` leaves the pre-execution states. **Limits:** the claim was verified by reading the code; no reconcile was run. The artifact is an approval packet at a PR-head pin, so R-B, R-C-packet and R-A still apply, and the second revision's defects depend on the first fix.

## 3. Line-level re-check of the four PR #620 leads

Common checks (all four): the pin equals the merged blob (#620 §5.1); the review thread was created about two minutes after the pin commit; no EGM or corpus file exists in any pin tree (`git ls-tree`); none of the four paths is in `candidates.json`; no merged commit corrects the passage (#627 §7).

| Lead | Pin commit (local time → UTC) | Thread created (UTC) |
| --- | --- | --- |
| GH-010 | 2026-11-12 09:26:59 −06:00 → 15:26:59Z | 15:29:03Z, line 124 |
| GH-011 | 2026-11-12 14:34:51 −06:00 → 20:34:51Z | 20:36:47Z, line 194 |
| GH-012 | 2026-11-12 16:42:38 −06:00 → 22:42:38Z | 22:44:18Z, line 100 |
| GH-014 | 2026-11-16 15:38:03 −06:00 → 21:38:03Z | 21:39:53Z, line 194 |

### 3.1 GH-010 — `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e26a78b28819ef062261efae6cfa610eb`

- **Document (verified):** lines 122–123 "the reverse … remains **denied**, and is pinned by `TestRunCommandIsolationAllowsOwnUnixSocketOnly`"; line 132 "a host socket is still unreachable from inside."
- **Code at the pin (verified):** `internal/toolharness/isolation.go:277–278` bounds the home scan (`maxHomeSocketDepth = 4`, `maxHomeSocketEntries = 20000`); `:288–289` carries the author's comment "a socket created after the scan, or deeper than the depth bound, is not masked"; the scan is called for the bubblewrap backend at `:180`.
  The named test creates its host socket with `os.MkdirTemp("/tmp", …)` (`isolation_unix_socket_test.go:80`) and its negative case under `/var/tmp` (`:94`), so no case covers a socket outside `/tmp`, `/run` and the scanned home depth.
- **Result:** the unqualified statement is overstated for the Linux backend by the author's own comment. **Not reproduced:** no Linux or bubblewrap host was available, so the behaviour of a connect to such a socket was not exercised. Severity as in #620 (an overstated guarantee).
- **Recorded gap (from #627 §6):** `docs/history/plans/PONYTAIL-DEBT.md` (`ab20aa28f4ff425570281add8814d138571cef9f`) records the same ceiling later; it is confirmatory of the pin-time comment. Owner ruling needed (see the eligibility package).

### 3.2 GH-011 — `PROPOSAL-LLM03-D4-ENFORCEMENT.md` @ `68015e0acd56c5b1318b7fffbe2f94705546980d`

- **Document (verified):** line 194, Option B: to make it pass without baselining SAST "the operator must disable the `sast` category … then 27 Gitleaks block until the 27 entries are approved".
- **Code (verified), `internal/security/policy.go` at the pin:** `disabledGaps := disabledCoverageGaps(execs, detection)` at `:169`; `if blocking > 0 { Fail }` at `:203`; `if len(disabledGaps) > 0 { out.Verdict = VerdictIndeterminate; return out }` at `:227`; `VerdictPass` only at `:237`, after that check. `disabledCoverageGaps` is defined at `:307`.
- **Result:** with blocking findings cleared and `sast` disabled without a replacement adapter, the verdict is `INDETERMINATE`, never a pass. The document's path to a green gate does not reach one. **Holds.**

### 3.3 GH-012 — `PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` @ `9aaa3a9c71653277b603043074177d5ea12cdc48`

- **Document (verified):** lines 99–100 "Fingerprint-exact entries keep that visible, because a new line or column produces a new fingerprint that is not suppressed."
- **Code (verified):** `internal/security/gitleaks.go:297` computes the Gitleaks entry fingerprint as `StableFingerprint("gitleaks", ruleID, path, gf.StartLine, gf.StartColumn)`; `normalize.go:118` defines `StableFingerprint(provider, ruleID, path string, line, column int)` over exactly those fields; `baseline.go:98` suppresses a finding when `active[f.Fingerprint]` matches.
- **Result:** the matched content does not enter the fingerprint, so a credential that replaces a fixture at the same path, rule, line and column keeps the entry's fingerprint and stays suppressed. The packet is silent on this case. **Holds; severity minor** (an omitted residual). The case is narrow, and the owner may treat it as immaterial.

### 3.4 GH-014 — `PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md` @ `e7a09f23250666ec028c2cabc341c860e8e7bc8a`

- **Document (verified):** the staged-activation table (about line 193) lists stage 1 `required`, `[sast, secrets]`, with entry conditions "decisions 2 and 3 authorized; guard landed; clean-tree baseline captured" and observation "confirm no false `INDETERMINATE` from coverage"; the OSV database is a stage 2 entry condition (line 194).
- **Config and code (verified):** `.agent-sdlc/config.yaml` at the pin has `stale_advisory_database: deny` (`:65`) and an enabled `osv-scanner` adapter, `category: dependencies` (`:110–111`). In `internal/security/policy.go`, `enabledExecs := nonDisabled(execs)` (`:110`) feeds a loop that appends `adapter %s is %s` / `adapter %s coverage is %s` to `incompleteReasons` for **every enabled adapter** (`:133–137`), independent of `required_categories`, and `if len(incompleteReasons) > 0` ends in `INDETERMINATE` (`:222`).
- **Result:** a stage 1 environment without a fresh OSV database cannot pass, yet the entry conditions omit it. **Holds.** Whether an environment lacks the database is environmental; the omission from the entry conditions is not.

## 4. Classifications and count impact

| Lead | Proposed classification | Gates still open | Count class |
| --- | --- | --- | --- |
| GH-004 | strict only if approval packets are admitted; 4 verified defects | R-B, R-C-packet (recommended: reject), R-A; subject overlap with `DISC-001` | C4 (held) |
| GH-010 | STRICT_CANDIDATE with a confirmatory recorded-gap pointer; Linux claim not reproduced | R-C-packet (an assessment; recommended: likely reject); owner ruling on the gap | C2 |
| GH-011, GH-012, GH-014 | STRICT_CANDIDATE | R-B, R-C-packet | C2 |

No class changes. C0 3 / 4; C1 4 / 5; C2 13 / 15 (2 short); C3 11 / 15 (4 short). Conditional scenarios: S7 = C2 13 / 15 + `IDR-L-001` (+1 / +1) + `GH-004` (+1 / +4) = **15 / 20** (composition and gates in the first section above and in the gap-closure report §8); S6 (skills and guides) is unaffected by `GH-004` (17 / 20). Nothing is added to C2; **new strict candidates: 0; new determinable defects: 0** (one more defect on a held lead only).

## 5. Outstanding decisions and gaps

- R-B, R-C-packet, counting unit, R-C and R-D (eligibility package and gap-closure report); each item above stays PROPOSED — NOT APPROVED.
- No human has verified any item; every discovery source is one bot and the verifier is a model.
- The third follow-up, a different evidence source for plans, remains an operator decision.

## 6. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; rule on `DISC-001`, `IDR-H-001`, R-B, R-C-packet, R-C, R-D or the counting unit; change Decision 3, any threshold (including the 613-call ceiling) or the registered protocol; modify `candidates.json` or any frozen or sealed artifact;
construct a seeded or control case; create or access a sealed answer key; designate a case checker or adjudicator; authorize case construction; adjudicate; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.

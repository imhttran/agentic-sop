# Historical Corpus — Eligibility Decision Package (R-B, R-C-packet, counting unit, R-C)

**Type:** Operator decision package (non-normative). It prepares four unresolved eligibility questions for the operator. It approves nothing, promotes nothing,
and changes no rule, record or count.
**Status:** **Every recommendation below is PROPOSED — PENDING OPERATOR APPROVAL.** Nothing here is approved, eligible, adjudicated or frozen.
**Baseline:** `origin/main` @ `0df598fa5f1ea9dcf7665284aab98a1bdf0bb40c` (merge of PR #627), 2026-11-22.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3 (verbatim extracts in §2).
**Rule in force:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`); [DECISION-RECORD-004-PROTOCOL-AMENDMENT.md](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md) (an eligibility amendment was **rejected as unnecessary**, 2026-11-21).
**Evidence (not repeated):** [#613](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md), [#620](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md), [#626](HISTORICAL-CORPUS-STRICT-EVIDENCE-VERIFICATION.md),
[#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md), [#618](DISC-001-H-001-EVIDENCE-VERIFICATION.md), [candidates.json](candidates.json) (unchanged).

## 1. Decision summary

| # | Question | Recommended (PROPOSED — PENDING OPERATOR APPROVAL) | Why in one line |
| --- | --- | --- | --- |
| R-B | May a defect first surfaced by an automated reviewer count? | **Conservative**: only when a merged fix, erratum, reverted change or recorded gap independently documents it | The protocol's evidence forms do not include an unresolved review finding; admitting them needs a new interpretation or an amendment |
| R-C-packet | Are decision packets (`PROPOSAL-*`, approval packets) "plans, specs or requirements"? | **Admit those that specify a design** (action sequence, control, requirement) **at a pre-fix revision**; reject packets that only choose among options | The protocol's unit is the pre-fix plan or spec; a packet that states a design is one |
| Unit | What is the floor's counting unit? | **Distinct artifact path, with a same-defect rule**; report cluster counts as a sensitivity row | The protocol counts "plans" and per-plan defects; revisions would inflate |
| R-C | Do skills and guides count? | **No**, for now | Operational instructions; every current instance is also bot-only and partly unverified |

**Count outcome:** the historical set reaches the 15-plan floor in **exactly one** scenario (S6: every permissive choice, including R-C Option C, with every adjudication succeeding and zero plan slack). Without R-C Option C the largest provisional total is **13 plans / 15 defects** (S4, 2 plans short); the most conservative reading leaves **4 / 5** (11 / 10 short). The decisions decide how far the proposals count; they do not by themselves close the shortfall.
decide how far the proposals count; they do not close the shortfall.

**Count reconciliation is active elsewhere.** A separate agent holds uncommitted work in the worktree `egm-count-reconciliation` (a corpus count convention defining classes C0–C3). It is not merged and is **not duplicated** here. This package uses
those class names only as labels for the arithmetic in §9 and flags every number that depends on it. If that work changes a class, §9 follows it.

## 2. Protocol references (verbatim, §5.3)

> **Historical set (target 12-15):** plans, specs or requirements whose defect later appeared in merged fixes, erratum, reverted change or recorded gap. Each defect has an evidence pointer (commit, PR or file).

> A later correction does not by itself show that a review could have found the defect. At corpus freeze, the corpus owner records whether each defect was determinable from the document and its parent revision, with a one-line basis, in the defect record.

> Minimum to run: 15 determinable known defects (the recall denominator) across at least 15 plans with known defects … **Per-plan count.** The manifest records the number of known defects per plan; no hard cap is imposed …

> **Defect record:** id, plan path and sha256, parent commit, location, description, class, severity …, evidence pointer, source (historical or seeded), and a determinable judgment …

Decision 3: partial and hindsight-only defects count toward neither minimum; the recall denominator is the frozen determinable set. The protocol is unchanged (SHA-256 `abba919e…`).

Facts used below: the protocol's unit is a **plan** identified by path and sha256 with a parent commit; the pointer may be a commit, PR or file; the owner (a human) records determinability at freeze; the adjudicator does not change the denominator.

## 3. R-B — bot-surfaced evidence

**Question.** Can a defect first reported by an automated reviewer qualify when a model or deterministic analysis verifies it?

### 3.1 What the protocol requires and does not

1. **Not required: independent discovery.** §5.3 asks that the defect "later appeared in" a merged fix, erratum, reverted change or recorded gap. It says nothing about who first noticed it.
2. **Required: one of four forms, plus an evidence pointer.** For bot-surfaced defects with **no** later correction (GH-001, 002, 003, 009, 010, 011, 012, 014), the pointer is a review thread and a deterministic check; none of the four forms is present. Whether an unresolved
   review finding is a "recorded gap" is the actual interpretive question. A PR is an acceptable *pointer* ("commit, PR or file"), but a pointer is evidence *of* a defect, not one of the four appearance forms.
3. **Required: owner-recorded determinability** from the document and parent revision. This is the human gate and already exists; bot agreement does not replace it.

### 3.2 Evidence on circularity

- **Verification independence (verified facts):** discovery was by `chatgpt-codex-connector[bot]`; verification in #620/#626 by a different model (this agent). Both are LLMs. Agreement between two models is **not** human evidence. The reproductions (a `git grep` at a pin; a `mv` in a scratch directory; a `git status` with a clean filter) are deterministic and re-runnable by a human, which is the real
  strength; the *selection* of what to check came from the bot.
- **Selection bias (interpretation):** a denominator of defects that an LLM reviewer noticed pushes recall in favour of LLM reviewers. Every addition in #613 and #620 (9 artifacts) was first surfaced by one bot; only the baseline and `GH3-001a` have a non-bot origin. If these defects enter the recall denominator unstratified, the experiment would partly measure agreement with the bot that built it.
- **Corrective-change cases are stronger:** `GH-013` has a merged fix (#486) and `GH3-001a` a merged spec change plus a recorded gap, so the "later appeared in" condition holds literally.
- **Evidence-pointer tiers (verified in #620, #626):**
  - **T1 — merged fix, erratum or recorded gap:** `GH-013`, `GH3-001a` (and the three baseline leads).
  - **T2 — shipped code diverged from the document, no document fix:** `GH-009` (§3.3).
  - **T1\* — recorded gap, owner ruling required:** `GH-010`. #627 §6 adds `docs/history/plans/PONYTAIL-DEBT.md` (created `ab20aa28`, 2026-11-20, after the 2026-11-12 pin), which records as an acknowledged, deliberately unfixed ceiling that a Unix socket outside `/tmp` and `/run` beyond the bounded home scan stays connectable. **Caveat carried forward:** the same limitation was already in a source comment at the pin, so the document is *confirmatory*, not an independent discovery; whether it satisfies §5.3 is the owner's ruling (#627 §6). The packet-kind question (§4) is separate.
  - **T3 — unresolved bot finding with deterministic reproduction only:** `GH-001`, `GH-002`, `GH-003`, `GH-011`, `GH-012`, `GH-014`.

### 3.3 Alternatives

| | Option | Wording of the rule | Consistent with the registered protocol? |
| --- | --- | --- | --- |
| **A (recommended, conservative)** | **T1 only** | A defect first surfaced by an automated reviewer counts only if a merged fix, erratum, reverted change or recorded gap independently documents it; determinability is recorded by the owner | **Yes**, literal reading |
| B (permissive) | **T1–T3** with safeguards | Also count a defect whose only evidence is an automated-reviewer finding **if** a person other than the discoverer re-runs its deterministic check and the owner records determinability; report the bot-surfaced stratum separately | **Only by reading "recorded gap" to include an unresolved review finding.** That reading goes beyond the four listed forms; treat it as a protocol interpretation that should be recorded, or as an amendment. An earlier eligibility amendment (Decision 4) was rejected as unnecessary, so this is a new decision, not a continuation |
| C | T1 + T2 | A as above, plus a code change that diverged from a document (`GH-009`) | Arguable: a merged *code* change is a "merged fix" of the design only if the owner so rules |

### 3.4 Risks and limitations

- A: smallest set; discards deterministic, reproducible defects only because no one fixed the document. B: opens the circularity above and relies on the owner as sole human. C: leaves "diverged code" undefined (when is a code change a fix of a document?).
- None of the options lets the model, rather than a human, establish determinability.

### 3.5 Affected candidates

`GH-001`, `GH-002`, `GH-003`, `GH-009`, `GH-011`, `GH-012`, `GH-014` (T3; T2 for `GH-009`) — gated by R-B (#627 §6 also lists `GH-009`, `GH-011`, `GH-012`, `GH-014` as still governed by R-B). `GH-010` (T1\*) is gated by the owner's recorded-gap ruling and R-C-packet, not by R-B. `GH-013` (T1) is **not** gated by R-B; it is gated by R-C-packet only. The 3 baseline leads and `GH3-001a` are not gated.

### 3.6 Proposed operator wording — PROPOSED — PENDING OPERATOR APPROVAL

> **R-B — APPROVED (Option A).** A historical defect first surfaced by an automated reviewer is eligible for the historical set only if a merged fix, erratum, reverted change or recorded gap independently documents it. A review finding alone is not such a record. The corpus owner records determinability for each defect at freeze; a model's verification is not independent human evidence. This applies Decision 3 and the registered §5.3 as written and amends neither.

> **R-B — REJECTED (Option A) / APPROVED (Option B).** *(If chosen: record that "recorded gap" is interpreted to include an unresolved automated-reviewer finding verified by a deterministic check re-run by a person other than the discoverer; that the bot-surfaced stratum is reported separately; and whether this is an interpretation or a protocol amendment.)*

## 4. R-C-packet — decision packets

**Question.** Is a decision packet (`docs/plans/PROPOSAL-*`, an operator approval packet) a "plan, spec or requirements" document?

### 4.1 Role in the design process (verified facts)

The artifacts are under `docs/plans/`, each at a pre-fix pin that equals the merged blob (#620 §3), each authored before the review thread existed (threads were created 1–3 minutes after the pin commit; #626 §4.1). Each states a concrete design:
`PROPOSAL-RUN-COMMAND-ISOLATION.md` gives a step-by-step call sequence; `PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` recommends a control set (G2/G4); `PROPOSAL-LLM03-D4-ENFORCEMENT.md` and `…-W8-SCANNER-ENFORCEMENT.md` specify a staged policy; `PROPOSAL-SEMGREP-IGNORED-COVERAGE.md` embeds a script. They also carry operator-decision framing.
Reviewer-visible, a packet reads as a design plus a decision request.

### 4.2 Alternatives

| | Option | Basis | Protocol |
| --- | --- | --- | --- |
| **A (recommended)** | **Admit a packet when its text states the design a developer would follow** (an action sequence, control, requirement or policy) at a pre-fix revision; **reject** a packet that only compares options or records a decision | The protocol says "plans, specs or requirements", by role not by filename | Consistent: a design-bearing packet is a plan or spec |
| B (conservative) | Admit only documents named `PLAN-*` / `docs/specs/*` | Filename test | Consistent but arbitrary: `docs/plans/PROPOSAL-*` sit beside plans, and the baseline's `docs/specs/*` leads are admitted by the same role test |
| C (permissive) | Admit every `docs/plans/*` file | Location test | Over-broad; would admit decision records and status files |

### 4.3 Per-candidate role assessment (interpretation, from §4.1 and the #620/#626 passages)

| Candidate | Packet states a design? | Likely under A |
| --- | --- | --- |
| GH-009 `PROPOSAL-RUN-COMMAND-ISOLATION` | yes (7-step sequence) | admit |
| GH-013 `PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION` | yes (G1–G4 control recommendation) | admit |
| GH-011 `PROPOSAL-LLM03-D4-ENFORCEMENT` | yes (options A/B/C with expected CI impact) | admit, owner to confirm |
| GH-014 `PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT` | yes (staged activation table) | admit |
| GH-003 `PROPOSAL-SEMGREP-IGNORED-COVERAGE` | partly (workaround script) | owner to rule |
| GH-010 `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS` | no — an assessment of residual risk | likely reject |
| GH-012 `PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET` | no — an approval request and rationale | likely reject |
| GH-004 `PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET` | no — approval packet | likely reject |

This changes #620's grouping, which treated the six alike; the recommended test separates design-bearing packets from assessments and approval requests. **It is a recommendation, not a ruling; the owner decides each case at freeze.**

### 4.4 Duplicates and remaining gaps

No artifact path is in `candidates.json`. `GH-009` and `GH-010` share the Gap 13 subject; `GH-011`, `GH-012` the LLM03 D4 subject; `GH-013` is the adjacent D4-5 packet. `GH-004` shares a subject with `DISC-001` / `IDR-H-001`. Line-level verification exists for `GH-003`, `GH-009`, `GH-013` (#626); `GH-010`, `GH-011`, `GH-012`, `GH-014` are verified once (#620) and not re-checked.

### 4.5 Proposed operator wording — PROPOSED — PENDING OPERATOR APPROVAL

> **R-C-packet — APPROVED (Option A).** A historical decision or proposal packet is eligible as a plan, spec or requirements artifact only if its text, at a pre-fix revision, states a design (an action sequence, control, requirement or policy) that an implementer would follow. A packet that only compares options, assesses residual risk, or requests approval is not eligible. The corpus owner rules on each packet at freeze. This applies the registered §5.3 role test and amends nothing.

## 5. Counting unit

**Question.** What does "15 plans" count, given multiple documents and revisions about one design problem?

### 5.1 What the protocol supports (verified)

The defect record identifies a **plan by path and sha256, with a parent commit**, and the manifest "records the number of known defects per plan". The floor is "at least 15 plans with known defects". So the literal unit is a plan **artifact**; a *design subject* is not mentioned, and neither is a revision.

### 5.2 Where documents or revisions overlap (verified, from the reports)

- **Same defect, two artifacts:** `IDR-H-001` and `DISC-001` (#618 §5.4): overlapping defects in a plan and its gap record. Count once unless the owner records why each is independent.
- **Same subject, different defects:** `GH-009` + `GH-010`; `GH-011` + `GH-012` (#620). Distinct defects in distinct artifacts, but a sequence of packets on one decision.
- **Same artifact, two revisions:** `GH-004` (#578, #590), `GH-013` (#484, #486 corrected revision). One path; the second revision's defects depend on the first fix.
- **Record collisions:** `docs/specs/REVIEW.md` and `docs/plans/PLAN-09-…` have both an excluded manifest entry and a lead entry; only the lead counts (count convention, pending).

### 5.3 Alternatives

| | Rule | Count effect (C2 layers) | Protocol |
| --- | --- | --- | --- |
| **A (recommended)** | **Unit = distinct artifact path.** A defect counts once, however many documents restate it. Several revisions of one path count as one plan; a later revision's defects count only if independent of the earlier fix. Report **subject-cluster counts as a sensitivity row**, never as the total | 13 / 15 (C2) | Literal reading; "plan" = a path |
| B (stricter) | Unit = **design subject** | 11 / 15 (C3, from #620's partial cluster map only) | Stricter than the protocol; needs a defined cluster map across **all** layers, which does not exist |
| C (looser) | Unit = revision | would exceed the above | Inflates; contradicts "plans" |

B is available as a *conservative owner ruling* under Decision 3 but is not what the protocol states, and an undefined cluster map would itself be a source of arbitrariness. A with the same-defect rule blocks the identified inflation (restated defects, dependent revisions) without inventing a new unit.

### 5.4 Proposed operator wording — PROPOSED — PENDING OPERATOR APPROVAL

> **Counting unit — APPROVED (Option A).** The historical floor counts distinct artifact paths (a plan or spec, identified by path and sha256 at its pre-fix revision) that carry at least one determinable defect. A defect counts once regardless of how many documents restate it; later revisions of a path count as the same plan, and their defects count only if independent of the earlier correction. Reports show a subject-cluster count as a sensitivity row. The minimum of 15 plans and 15 defects is unchanged.

## 6. R-C — skills, guides and operational instructions

**Question.** Do skills, engineering guides and operational instructions qualify as reviewable design artifacts?

### 6.1 Candidates (verified in #613, #620)

`GH2-001` `skills/decision-ab/SKILL.md`; `GH2-002` `skills/sop-fix-conflicts/SKILL.md`; `GH2-003` `skills/sop-ci-fix/SKILL.md`; `GH2-004` `skills/codex-review-triage/SKILL.md`; `GH2-005` `skills/sop-cleanup/SKILL.md`; plus `GH-005` `docs/reference/CLI.md` and `GH-006` `docs/guides/PROMPTING.md` (reference and guide documents).

### 6.2 Analysis

- A skill is an **instruction the agent executes**; its defects found so far are wrong commands, wrong paths, or documents that over-claim behaviour. That is implementation-instruction accuracy, which the registered corpus language ("plans, specs or requirements") and the existing exclusions of status and evidence documents point away from.
- A skill with normative requirements (`MUST`, acceptance conditions) can behave like a spec, as `docs/specs/PROMPT-EXECUTION.md` does (admitted via `GH3-001a`). The five skills examined do not state requirements; they state steps.
- **Every skill and guide lead is also T3** (bot-only, no merged correction found) and `GH2-003`–`005` are triaged but not verified. They would need R-B as well as R-C.

### 6.3 Alternatives

| | Option | Count effect now |
| --- | --- | --- |
| **A (recommended)** | **Do not count** skills, guides, reference docs and procedures unless the document states requirements a developer would implement and a T1 pointer exists; decide per document at freeze | 0 |
| B | Count skills and guides with a T1 pointer | 0 (none has one) |
| C | Count them with T3 evidence under R-B Option B | `GH-005` and `GH-006` (two distinct paths, one verified defect each, no overlap with S4): **+2 plans / +2 defects** (scenario S6). `GH2-001`–`002` (one defect verified each) and `GH2-003`–`005` (triaged only) are not added; unverified items cannot count |

### 6.4 Proposed operator wording — PROPOSED — PENDING OPERATOR APPROVAL

> **R-C — APPROVED (Option A).** Skills, engineering guides, reference documents and operational procedures are not eligible as historical plans, specs or requirements, unless a document states implementable requirements and a merged fix, erratum or recorded gap documents the defect. The corpus owner rules on any such document at freeze.

## 7. Candidate impact matrix

Columns: eligibility **if the proposed interpretation is approved** (A for R-B and R-C-packet; A unit) and under the **strictest existing interpretation** (R-B Option A, packets rejected). "Gaps" lists what remains unverified; nothing is inferred.

| ID | Artifact @ revision | Def. | Independent evidence | Decision(s) | Dup. / contamination | If approved | Strictest | Gaps |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| GH-013 | `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f27f1abec2a82112b9caf2d29c744313c8` | 1 | T1: fix `54beaf27…` (#486) | R-C-packet | none; no leakage at pin (#626) | counts | out (packet) | R-C-packet; owner determinability |
| GH-009 | `docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md` @ `70ea0664bd65de90622b55a5f887c9cfaba1caec` | 2 | T2: shipped code diverged | R-B, R-C-packet | Gap 13 cluster with GH-010 | out under R-B A | out | whether code divergence is a "merged fix" |
| GH-001 | `docs/plans/PLAN-Ponytail-Complexity-Audit-Cleanup.md` @ `208b4e72203f437bb0c7431c5afcbc5bbf63d4c4` | 1 | T3: bot + `git grep` | R-B | none | out under R-B A | out | no corrective change |
| GH-002 | `docs/plans/OWASP-LLM-GAP-PROMPTS.md` @ `46f190f210bfc2b3fcb01bc498d2533a8d7f01bf` | 1 | T3: bot + in-tree NO-GO record | R-B; kind (prompts doc) | none | out under R-B A | out | kind of a prompts index not ruled; step 1 may mitigate (#626 §6.2) |
| GH-003 | `docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md` @ `62e04e19a21c7ae441e1c8a7e7fa02924983932b` | 1 | T3: bot + scratch repro | R-B, R-C-packet | none | out | out | severity minor; packet role partial |
| GH-010 | `docs/plans/PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e26a78b28819ef062261efae6cfa610eb` | 1 | T1\*: recorded gap `docs/history/plans/PONYTAIL-DEBT.md` (`ab20aa28`), confirmatory of a pin-time source comment (#627 §6) | owner ruling on the gap; R-C-packet | Gap 13 cluster | counts only if the owner accepts the gap **and** the packet (an assessment, likely rejected under §4.3) | out (packet) | not line-level re-verified since #620; gap pointer is confirmatory, not independent |
| GH-011 | `docs/plans/PROPOSAL-LLM03-D4-ENFORCEMENT.md` @ `68015e0acd56c5b1318b7fffbe2f94705546980d` | 1 | T3 | R-B, R-C-packet | D4 cluster with GH-012 | out | out | not re-verified since #620 |
| GH-012 | `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` @ `9aaa3a9c71653277b603043074177d5ea12cdc48` | 1 (minor) | T3 | R-B, R-C-packet | D4 cluster with GH-011 | out | out | not re-verified; approval request, likely rejected |
| GH-014 | `docs/plans/PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md` @ `e7a09f23250666ec028c2cabc341c860e8e7bc8a` | 1 | T3 | R-B, R-C-packet | none | out | out | not re-verified since #620 |
| GH3-001a | `docs/specs/PROMPT-EXECUTION.md` @ `1f9dfae8d94ab7a1faf603203bc6a60ea5b8e37a` | 1 | T1: spec change `aa53c88238443cae1298cccb706f8024b5bd9ca3`; gap in `RESOLVED-BACKLOG.md` (`d46bc1674e6f25e6c57219b0e49e0853a94f40e5`) | none | none | counts | counts | owner must record the lead |
| GH3-001b | same artifact | 1 | T1 | determinability | same plan as GH3-001a | PARTIAL (0) | PARTIAL (0) | ambiguity not falsity (#627 §4.3) |
| GH-004 | `docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md` @ `c6307fd34d0957bd07349aa331819eb34692766f`, `022e7568555e1fc3990869d9c1ff95f4cfe15880` | ≤4 (2 verified) | T3 | R-B, R-C-packet, R-A | shares subject with DISC-001 / IDR-H-001 | not totalled | out | pins are PR heads; (c), (d) triaged only |
| GH-005 | `docs/reference/CLI.md` @ `d8aa908dd3a231f749497d9a32610f2cee52d748` | 1 | T3 | R-C, R-B | none | out | out | reference doc |
| GH-006 | `docs/guides/PROMPTING.md` @ `cef3b8a910939f4c17664f9c9dec9fced6ee6b29` | 1 | T3 | R-C, R-B | none | out | out | guide; now re-derived (#620) |
| GH2-001 / 002 | `skills/decision-ab/SKILL.md`, `skills/sop-fix-conflicts/SKILL.md` | up to 3 | T3 | R-C, R-B | none | out | out | one defect each verified, others triaged |
| GH2-003 / 004 / 005 | `skills/sop-ci-fix`, `skills/codex-review-triage`, `skills/sop-cleanup` | 3 | T3 | R-C, R-B | none | out | out | **triaged only**; not verified |

`DISC-001` / `IDR-H-001` are corpus-owner rulings (#618) and are not decided here.

## 8. Duplicate and contamination findings

No artifact above appears in `candidates.json`. No pin tree contains EGM or corpus-record files (#626 §4.1). The overlap cases are in §5.2. The principal contamination risk is not leakage but **circular origin** (§3.2).

## 9. Count scenarios

Official baseline **C0 = 3 plans / 4 defects**; independently adjudicated **0 / 0**; nothing below is a reached or approved count. Class labels follow the pending count convention. Duplicates and exclusions contribute 0 (`GH3-001b` partial; `GH-013` second finding partial; manifest records `IDR-E-005`, `IDR-H-008` excluded).

| Scenario | R-B | Packets | Unit | Plans / defects | Shortfall vs 15 / 15 | Class |
| --- | --- | --- | --- | --- | --- | --- |
| S1 — strictest | A | none | artifact | **4 / 5** (C0 + `GH3-001a`) | 11 / 10 | C1 |
| S2 | A | A | artifact | 5 / 6 (+ `GH-013`) | 10 / 9 | — |
| S3 | B | none | artifact | 6 / 7 (+ `GH-001`, `GH-002`) | 9 / 8 | — |
| S4 — maximum provisional | B | all admitted | artifact | **13 / 15** | 2 / 0 | C2 |
| S5 | B | all admitted | subject cluster | 11 / 15 | 4 / 0 | C3 |
| **S6 — S4 plus R-C Option C** | B and R-C C | all admitted | artifact | **15 / 17** (S4 + `GH-005`, `GH-006`) | **0 / 0** | — |

- **Conservative provisional total: S1 (4 / 5).** If the recommended R-C-packet Option A is approved with R-B Option A, **S2 (5 / 6)** applies.
- **Maximum defensible provisional total: S4 (13 / 15)**, which needs R-B Option B, every packet admitted, artifact counting, every adjudication to succeed, and has **no defect slack** (exactly 15). It is not an approved corpus.
- **Only S6 adds held leads, and only `GH-005` and `GH-006`** (distinct paths, verified, no overlap with S4). `GH-004`, `GH2-001`–`005` and `DISC-001` / `IDR-H-001` are never added: their additivity is undefined until the unit is ruled.
- **S6 has zero plan slack** (exactly 15 plans). It needs R-B Option B, every packet admitted, R-C Option C and every adjudication to succeed; one failure puts it below the floor. It is the maximum, not a forecast, and not an approved corpus.
- **GH-010 note:** if the owner accepts its recorded gap **and** admits the packet, S2 becomes 6 / 7 (10 → 9 plans short). That is not in the table because it needs two owner rulings the package recommends against.
- Plans are the binding constraint in every row except S6. Arithmetic cross-check: S2 = 3 + 1 + 1 = 5 plans, 4 + 1 + 1 = 6 defects; S3 = 3 + 2 + 1 = 6, 4 + 2 + 1 = 7; S4 = 3 + 3 + 6 + 1 = 13, 4 + 3 + 7 + 1 = 15; S6 = 13 + 2 = 15 plans, 15 + 2 = 17 defects.

## 10. Recommended operator decisions

All **PROPOSED — PENDING OPERATOR APPROVAL**. Required of the operator, in this order:

1. **R-B** — approve Option A (§3.6) or choose B and state whether it is an interpretation or an amendment.
2. **R-C-packet** — approve Option A (§4.5) or another option.
3. **Counting unit** — approve Option A (§5.4).
4. **R-C** — approve Option A (§6.4).
5. Separately, and unchanged by this package: whether to proceed to adjudication and what to do about the plan-floor shortfall (more sources, or recording that the best attainable outcome is INCONCLUSIVE).

## 11. Residual risks and blockers

1. Every defect still needs an owner-recorded determinability and, ultimately, human adjudication; none exists (0 / 0).
2. The corpus owner is the sole human author and reviewer of record; each ruling above rests on one person.
3. Circular origin of the bot-surfaced stratum (§3.2); stratified reporting is recommended if Option B is chosen.
4. Four of the six #620 leads are not line-level re-verified; two packets are likely rejected under the recommended R-C-packet test, which reduces S4.
5. The count convention is unmerged; the class labels may change.
6. Without R-C Option C, even S4 leaves 2 plans short. S6 reaches 15 plans with no slack, so the floor is met only if every permissive ruling and every adjudication goes the favorable way.

## 12. Non-authorization statement

This package approves no eligibility decision and no rule. It does not approve R-B, R-C-packet, R-C or the counting unit; promote, modify or exclude any candidate; modify `candidates.json`; change Decision 3, any threshold (including the 613-call ceiling) or the registered protocol; construct a seeded or control case; create or access a sealed manifest or answer key; designate a case checker or adjudicator; authorize case construction; adjudicate; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.

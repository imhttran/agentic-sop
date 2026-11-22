# Historical Corpus Discovery — Evidence, Third Pass

**Type:** Discovery record (non-normative). It proposes candidate classifications and
records verification work. It is not an adjudication, does not freeze the corpus, and
authorizes nothing.
**Baseline:** `origin/main` @ `1bda826d` (2026-11-21).
**Predecessors:** [pass 1](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md) (GitHub review
surface), [pass 2](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md) (six strict leads).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3.
**Manifest:** [candidates.json](candidates.json) — **not modified by this report.**
**Freeze status:** **NOT FROZEN.** Experiment **NOT YET READY.** H1/H2 **NOT ESTABLISHED.**

## 1. Preflight and why this pass exists

- `origin/main` fetched at `1bda826d`. The prior discovery and verification branches
  (`docs/egm-github-evidence-discovery`, `-2`, `docs/egm-github-evidence-update`,
  `docs/egm-historical-evidence-discovery`, `docs/egm-disc001-h001-verification`) are all
  merged into `main`; **no discovery or `DISC-001` / `IDR-H-001` investigation is active**,
  and no open PR relates to corpus evidence. Nothing here duplicates them.
- The two GitHub-evidence passes searched a specific surface: bot review threads on
  `docs/` (49 top-level threads) and 160 corrective-titled PRs. Their own §10 named what
  was left: (a) `GH2-003`–`005` triaged only, (b) corrective-change searches for
  `GH-009`–`GH-014` done by phrase search on `origin/main` rather than full history,
  (c) the 164 code-path threads (whose artifacts are code, not plans).
- **Neither pass inspected `docs/history/`.** That directory holds 47 plans, 111 task
  records, and three recorded-gap ledgers. It is a plan surface the earlier method never
  touched, and it is where §5.3's accepted evidence type — *"erratum … or recorded gap"* —
  actually lives. This pass searches it, and closes (b) while doing so.

## 2. Method

Read-only `git` against `origin/main`. No network call, no model call, no case built.

1. **Uncovered-artifact surface.** Every path under `docs/specs`, `docs/architecture`,
   `docs/plans`, and `docs/history/` on `origin/main` was compared with `candidates.json`
   and `lead-extraction.json` by path. Artifacts in neither file are candidates.
2. **Normative-requirement detector.** For each uncovered spec, `git log -G'^\*\*Required\.\*\*'`
   lists the commits that add a normative requirement. A requirement added later is a
   *candidate* pre-fix gap; it is only a defect if a recorded gap or fix names it as one
   (see §5, where this filter correctly rejects several false positives).
3. **Full-history corrective tracing** (closes pass 2 §10 gap 4). For each `GH-009`–`GH-014`
   passage, `git log --all -S` counts the commits that change the occurrence of that exact
   text, on every ref, not only `origin/main`.
4. **Recorded-gap ledgers.** `docs/history/RESOLVED-BACKLOG.md`,
   `docs/history/plans/PONYTAIL-DEBT.md`, and `docs/history/developer-preview/KNOWN-ISSUES.md`
   were read as defect registers: each entry names a resolution commit.
5. **Deterministic reproduction** of any claimed defect, in a throwaway directory outside
   the repository.

### 2.1 Limits

- **No independent human reviewer exists in the history**, and every review-thread finding
  is bot-surfaced. The new lead below does **not** rest on a review thread; it rests on a
  merged requirement change plus a recorded gap, both of which §5.3 accepts.
- The reproduction and the `git` checks were run by a model in this session. An adjudicator
  must repeat them. They are minutes of work each.
- A single-author repository: authorship and intent cannot be proved from Git identity.
- A negative result is not proof of absence. The 164 code-path threads remain untriaged
  (their artifacts are code, which §5.3 does not admit as a plan, spec or requirement).

## 3. Result

| Item | Result |
| --- | --- |
| New lead | **1** — `GH3-001`, `docs/specs/PROMPT-EXECUTION.md` |
| Proposed strict defect | **1** (`GH3-001a`) — `--file` containment omitted from the untrusted-input controls |
| Proposed partial defect | **1** (`GH3-001b`) — capability guard not pinned to the final post-routing selection |
| Evidence-type upgrade | **1** — `GH-010` gains a §5.3 *recorded gap* pointer |
| Pass-2 §10 gap 4 | **closed** — full-history tracing for `GH-009`–`GH-014` |
| Candidates promoted | **0** |
| Candidates independently adjudicated | **0** |
| `candidates.json` changes | **none** |

## 4. `GH3-001` — `docs/specs/PROMPT-EXECUTION.md`

### 4.1 Pins, ancestry, and merge state

| Item | Value |
| --- | --- |
| Pre-fix artifact | `docs/specs/PROMPT-EXECUTION.md` @ [`1f9dfae8`](https://github.com/imhttran/agentic-sop/commit/1f9dfae8) (2026-09-30, `prompt: apply bounded model escalation to implement prompts`) |
| Artifact SHA-256 @ pin | `991d61f0a55feec66c0e7d0954e5b5b0de8752f877dc62cd34f31719cb56ca56` (209 lines) |
| Artifact blob @ pin | `5afc9565a0ea` |
| Corrective change | [`aa53c882`](https://github.com/imhttran/agentic-sop/commit/aa53c882) (2026-09-30, `skill: add the Zed slash-command surface and harden prompt guards`) — merged |
| Recorded gap | [`docs/history/RESOLVED-BACKLOG.md`](../../history/RESOLVED-BACKLOG.md) §"`--file` prompt input could escape the project through a symlink" and §"An IMPLEMENT prompt was rejected when the default agent lacked IMPLEMENT", both added at `d46bc167` (2026-10-01) |
| Artifact SHA-256 on `main` | `1ed82de6c8f2a89c2609d458f1425b8ebdc14ee775268ad94cb5a5f808e4e3f3` |

Ancestry, all verified: the pin is an ancestor of `origin/main`; `aa53c882` is a
descendant of the pin and an ancestor of `origin/main`. The pin is the file's last
revision before the corrective commit, so it is a genuine **pre-fix** revision — the
condition `IDR-H-001` and `IDR-H-005` could not satisfy.

The evidence chain is three-way and independent of any bot review thread:

```
2026-09-30  1f9dfae8   pre-fix spec (no containment norm; no final-selection norm)
2026-09-30  aa53c882   merged spec change adds both norms (+ the code fix)
2026-10-01  d46bc167   RESOLVED-BACKLOG records both as resolved defects
```

### 4.2 `GH3-001a` — `--file` containment omitted — **proposed strict**

- **Artifact text at the pin (§12 Security).** The section declares the threat model —
  *"Prompt text and skill input are untrusted input"* — and then enumerates the forbidden
  interpretations (shell command, lifecycle transition, routing policy, approval,
  configuration, credentials) and the mutation rule. It states **no security requirement at
  all** about `--file` paths. At the pin `--file` appears only twice in the whole document,
  both in §1 (the command usage line and "At most one of a positional prompt and `--file`
  MAY be given") — never as a containment or resolution control.
- **What was added.** `aa53c882` adds: *"**Required.** `--file` MUST be confined to the
  project directory. The confinement MUST be enforced against symlink resolution, not only
  lexically, so a project-local symlink pointing outside the project is rejected rather
  than read."*
- **Recorded gap.** *"`--file` prompt input could escape the project through a symlink —
  RESOLVED (Phase 5.4 hardening): `resolvePromptFile` now resolves both the project root and
  the target with `filepath.EvalSymlinks` before the containment test."*
- **Determinable without hindsight.** Yes. The section's own premise is that prompt and skill
  input is untrusted; a file-path input to an untrusted-input surface with no containment
  requirement is a missing control in the security section, not an implementation detail.
  The `--file` flag is documented in `../reference/CLI.md`, which exists at the pin.
- **Design vs implementation.** Design (a missing normative requirement in a security
  section), not an implementation bug. Contrast `IDR-E-005`, excluded because its pre-fix
  spec *already* stated the requirement and only the code was wrong.
- **Severity.** Blocks implementation: an implementer following §12 exactly can ship
  lexical-only containment.

### 4.3 `GH3-001b` — capability guard not pinned to the final selection — **proposed partial**

- **Artifact text at the pin (§4 Capability Enforcement).** *"Before execution, the prompt's
  capability MUST be checked against the selected agent's declared capabilities… A
  capability the selected model/harness cannot serve MUST fail clearly."* "Selected agent"
  is not defined against the routing step in §5.
- **What was added.** `aa53c882` adds: *"**Required.** The check MUST apply to the agent that
  will actually execute — the FINAL per-prompt selection, after routing and provider
  validation (§5, §7) — not to a default agent the router will replace… a read-only default
  provider with a tool-capable routed class is a valid configuration."*
- **Recorded gap.** *"An IMPLEMENT prompt was rejected when the default agent lacked
  IMPLEMENT — RESOLVED (Phase 5.4 hardening): the capability guard and provider validation now
  apply to the FINAL executing agent (the routed selection)… See `../specs/PROMPT-EXECUTION.md`
  §4."*
- **Determinable without hindsight.** Partly. A reviewer can see that "selected agent" is
  undefined once §5 routing can replace the default, and that guarding the default rejects a
  valid configuration. But the requirement never *asserts* the wrong behaviour — it is
  ambiguous, not false.
- **Why partial, not strict.** The implementation at the pin already built the agent from the
  routed selection and then guarded it (`internal/cli/prompt.go:319` at `1f9dfae8`), so the
  document's gap caused no demonstrated harm at that revision. Under Decision 3 this is a
  normative-completeness defect: recorded, but counted toward **neither** floor.

### 4.4 Reproduction (deterministic)

The pin's containment algorithm (`internal/cli/prompt.go:240` at `1f9dfae8`) is
`filepath.Abs` + `filepath.Rel` with no `EvalSymlinks`. Reimplemented exactly and run in a
throwaway directory outside the repository, with `project/link.md` a symlink to
`outside/secret.md`:

```
pin:      accepted=true  path=".../project/link.md" err=<nil>
pin:      read "SECRET" bytes from outside the project
mainline: accepted=false path="" err=prompt file link.md is outside the project directory
```

The pin accepts and reads the outside file; the current implementation rejects it. This is
the defect the spec requirement now forbids.

## 5. Surfaces inspected and **excluded** (negative evidence)

Recorded so the next pass does not repeat them.

| Surface | Why it is **not** a lead |
| --- | --- |
| `docs/plans/PLAN-07-SOP-LLM07-Misinformation.md` | The 80→149-line rewrite at `257b9b40` is a **schema migration** (Project / Dependencies / Deliverables / Acceptance Mapping) plus implementation evidence, not a correction of a defective plan claim. The commit title ("reject model-authored evidence…") describes the work the plan specified, not a defect in the plan. |
| `docs/plans/PLAN-01-SOP-LLM03-Excessive-Agency.md` | One revision only (`eb596b0a`); its deliverable `llm03-agency.md` records inventory results, not a defect in the plan. |
| `docs/specs/PROJECT-STATE-LOCK.md` | Both revisions already state *"Participation is a convention. A writer that does not take the lock is not excluded"* — the honest caveat `IDR-H-001`'s defect needs. The spec never claimed execution-path participation, so it is not the defective artifact for that defect. |
| `docs/specs/PROVIDERS.md` | Three `**Required.**` additions, but each accompanies a **new feature** (provider runtime layer, `openai_compatible`, final-selection validation), not a recorded defect. |
| `docs/specs/WORK-ITEMS.md` | Two `**Required.**` additions, feature evolution; no recorded gap names a defect. |
| `docs/reports/owasp-llm/*` (60 files) | Evidence and campaign reports; only four are cited in `candidates.json`. They document implementation and acceptance outcomes, not defects in a plan's design, except where the plan is already a manifest entry. |
| `docs/history/developer-preview/KNOWN-ISSUES.md` | Ten preview issues with dispositions (KI-01…KI-10). They name runtime behaviour and one lifecycle side effect (KI-06), not a defect in a pinned plan or spec document. |
| `docs/history/plans/PONYTAIL-DEBT.md` as an **artifact** | It is a remediation plan that changes comment text only; it is used below as *evidence*, not as a corpus artifact. |

## 6. Evidence-type upgrade — `GH-010`

Pass 2 classified `GH-010` (`PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e`) as
`STRICT_CANDIDATE` under **R-B** (bot-surfaced, model-verified) with *"none found"* for a
corrective change. A §5.3-compatible pointer exists that pass 2 did not record:

- `docs/history/plans/PONYTAIL-DEBT.md` (created `ab20aa28`, 2026-11-20 — **later** than the
  2026-11-12 pin) records, as an acknowledged and deliberately unfixed ceiling, that *"a
  Unix socket outside /tmp and /run that the bounded home scan does not reach (too deep, or
  outside $HOME) stays connectable"*, and that *"the underlying ceilings stay as they are:
  this plan makes each one reviewable, not fixed."*
- That is a **recorded gap** naming the exact condition the packet understates (*"a host
  socket is still unreachable from inside"*).

**Caveat, stated rather than hidden:** the same limitation was already written in the source
comment at the pin (`internal/toolharness/isolation.go`). The recorded-gap document is
therefore *confirmatory* of a pin-time observation, not an independent discovery. Whether
this pointer alone satisfies §5.3 is the corpus owner's ruling; it does **not** by itself
decide R-B, which still governs `GH-009`, `GH-011`, `GH-012` and `GH-014`.

## 7. Pass-2 §10 gap 4 closed — full-history corrective tracing

`git log --all -S '<passage>' -- <artifact path>` on every ref. "Commits changing it" counts
commits that add or remove the quoted passage; a fix applied to code elsewhere would not
appear, and the column is labelled accordingly.

| Lead | Passage traced | Commits changing it | Result |
| --- | --- | --- | --- |
| `GH-009` | `sandbox.Wrap(h.root, root.path, argv, env)` | creation `70ea0664` only | **no correction** to the artifact text |
| `GH-010` | `a host socket is still unreachable from inside` | creation `8495d54e` only | **no correction** (see §6 for the recorded gap) |
| `GH-011` | `the operator must disable the sast category` | creation `68015e0a` only | **no correction** |
| `GH-012` | `a new line or column produces a new fingerprint` | creation `9aaa3a9c` only | **no correction** |
| `GH-013` | `--no-ext-diff` | creation `de7161f2`, correction `54beaf27` | **corrected**, as pass 2 recorded |
| `GH-014` | `OSV DB provisioned and refreshed in every gate environment` | creation `e7a09f23` only | **no correction** |

Pass 2's *"none found"* was a phrase search on `origin/main`; these results hold across all
refs and all commits. The claim this supports is narrow and exact: **no merged commit
corrects the defective passage in five of the six artifacts, and `GH-013` is corrected
once.** It does not show that no code fix exists elsewhere.

`GH2-003`–`005` (pass-2 §10 gap 3) remain triaged only. They are `skills/*/SKILL.md`
operator procedures awaiting the R-C kind ruling; tracing them further would not change any
count while R-C is open, so this pass leaves them where they are.

## 8. Duplicate analysis

- `docs/specs/PROMPT-EXECUTION.md` appears in **neither** `candidates.json` **nor**
  `lead-extraction.json` (grep by path). No `GH-001`–`GH-018`, `GH2-*` or `DISC-*` lead uses
  it. `IDR-E-005` excludes `docs/specs/REVIEW.md`, a different file.
- `docs/history/plans/PONYTAIL-DEBT.md` is cited in no corpus document (grep). It is used
  here only as evidence for an existing lead.
- The pass-2 reports quote the `GH-010` passage; quoting is not duplication.

## 9. Provisional floor impact

Arithmetic only. **Official counts are unchanged; nothing is promoted; nothing is
adjudicated (0); seeded cases do not count and do not exist.**

| Measure | Before | After (proposed) | Shortfall |
| --- | --- | --- | --- |
| Distinct plans with at least one determinable defect | 3 | **4** | **11** |
| Determinable defects | 4 | **5** | **10** |
| Partial plans / partial defects (outside the floors) | 11 / 11 | 11 / 11 | unchanged |
| Recorded but excluded (`GH3-001b`) | — | +1 partial defect on a plan already counted | does not count |
| Independently adjudicated | 0 | **0** | — |

Reading:

- `GH3-001a` is the first new strict **spec** lead since pass 2, and it needs **no R-B
  ruling**: §5.3 lists "merged fixes … or recorded gap" as the evidence type, and this lead
  has both.
- Plans remain the binding constraint: 11 short is still 11 short. `GH3-001` adds one plan,
  not three.
- The conservative bound for planning purposes stays at 11 plans short, with one more
  defect in hand.

**Count class.** The **4 / 5** above is **C1 — the conservative total**: the recorded baseline
plus the only increment that needs no open ruling. It is **not** the maximum. Adding the
conditional layers from #613 and #620 (`+3 / +3`, `+6 / +7`, all gated on R-B and/or
R-C-packet) gives **C2 = 13 / 15** by artifact counting — 2 plans short, with the defect floor
met exactly and zero slack. The #613/#620 layers were not re-verified here; they are counted in
C2 because their sources report them, not because this pass confirmed them. See
[CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md) §2–§3.

## 10. Unresolved evidence gaps

- No exported reviewer-visible tree has been built for any pin, including this one; whether
  the pin tree excludes `RESOLVED-BACKLOG.md` and these reports is unproven. For `GH3-001`
  the required exclusion is sharper than usual: `RESOLVED-BACKLOG.md` states the fix, so it
  must be outside the reviewer's input.
- `GH3-001a`'s reproduction covers the containment algorithm, not the full `sop prompt`
  invocation; the CLI path was read, not executed.
- The 164 code-path threads are still untriaged; on the current reading they cannot supply
  plan artifacts.
- `GH2-003`–`005` remain triaged only, blocked on R-C.
- The ceiling is unchanged: one human, and every review-thread evidence link is a bot.

## 11. Recommended follow-ups (not authorized)

1. Corpus-owner rulings, unchanged and still gating: **R-B**, **R-C-packet**, counting unit,
   **R-C** for skills and guides. `GH3-001` does not depend on R-B; the other five do.
2. Have an adjudicator repeat §4.4 and the §7 `-S` traces at the named pins.
3. Build a reviewer-visible tree for `GH3-001` to test exclusion of `RESOLVED-BACKLOG.md`.
4. If more plans are wanted, the untried plan surface is `docs/history/tasks/` (76 task
   records) — but §5.3 admits "plans, specs or requirements", and whether a task record is
   one is an R-C-class ruling, so search it only after that ruling.

## 12. Non-authorization statement

This report authorizes nothing. It does not: promote any candidate; add or modify a
`candidates.json` entry; rule on `DISC-001`, `IDR-H-001`, R-B, R-C or R-C-packet; change any
eligibility criterion, decision, threshold, or canonical disposition; modify the registered
protocol, `DECISION-RECORD-003`, or any Decision 7 file; record operator approval; construct
a seeded or control case; designate an adjudicator, seeder, or case checker; select a
sealed-manifest location; access sealed answer-key material; adjudicate; freeze the corpus;
run model inference for the experiment; or execute the experiment. Decision 6 remains
**PENDING**. The corpus remains **NOT FROZEN**, the experiment **NOT YET READY**, and H1/H2
**NOT ESTABLISHED**.

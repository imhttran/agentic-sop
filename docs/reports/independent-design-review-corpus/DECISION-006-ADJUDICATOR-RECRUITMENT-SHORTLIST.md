# Decision 6 — Adjudicator Recruitment and Shortlist Readiness (Independent Design Review / EGM)

**Type:** Recruitment-readiness report (non-normative). It records the candidate-discovery
coverage, the limitation, and an operator-assisted recruitment plan. It designates no one and
contacts no one.
**Status:** **NO VERIFIED CANDIDATES IDENTIFIED — Decision 6 remains PENDING.**
**Baseline:** `origin/main` @ `330f61c0` (merge of PR #609).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.4.
**Qualification package (input, not duplicated):** [DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md).
**Procedural approval (input, not duplicated):** [DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md).
**Readiness:** [READINESS.md](READINESS.md) §17.

No candidate has been contacted, invited, or represented as willing. No adjudicator is
designated. No `DECISION-RECORD-006` exists. The corpus is **NOT FROZEN** and the experiment
is **NOT YET READY**.

## 1. Scope and authorization boundaries

This report records what candidate discovery could and could not establish in this
environment, and what the operator can do next. It does not:

- contact, invite, or screen any person;
- designate an adjudicator or the independent case checker;
- approve Decision 6 or create `DECISION-RECORD-006`;
- construct cases, adjudicate, freeze, run inference, or execute the experiment;
- modify the protocol or change Decisions 1–5 or the PR #609 clarifications.

The merged qualification package (PR #604) and procedural approval (PR #609) are inputs.
Neither is duplicated here.

## 2. Candidate discovery methodology and coverage

### 2.1 Channels checked

- **Repository identity evidence:** `git shortlog -sne --all` over the full history.
- **Existing repository documentation:** the corpus directory, the qualification package, and
  the procedural approval record. The files `candidates.json` and `CANDIDATE-VERIFICATION.md`
  concern **corpus artifacts**, not people, and are not recruitment research.
- **Public professional search:** **not available in this environment.** The available network
  tool fetches a known URL; it provides no search or directory lookup. No candidate names were
  researched, scraped, or inferred.

### 2.2 Why no public search was attempted

- Without a search interface, any name would have to be invented, which the task forbids.
- Scraping search engines or professional directories for individuals would collect personal
  data without a defined purpose or consent basis, beyond what qualification needs.
- The task's fallback applies: document the coverage and limitation, and prepare an
  operator-assisted plan.

### 2.3 Limitation statement

**No shortlist can be populated from this environment.** This report does **not** claim that
no qualified humans exist; it reports that no verifiable candidate information was available
here. The operator-assisted plan in §8 is the supported path.

## 3. Repository evidence

`git shortlog -sne --all` shows:

| Identity | Commits | Human? |
| --- | --- | --- |
| Tom Tran `<trantom512@gmail.com>` | 1368 | Yes — corpus owner and seeder |
| Tom Tran `<imhttran@gmail.com>` | 778 | Yes — same person |
| `Claude <noreply@anthropic.com>` | 39 | No — agent identity |
| `Zed <hi@zed.dev>` | 38 | No — agent identity |
| `imhttran <imhttran@gmail.com>` | 6 | Yes — operator |
| `Eval <eval@example.com>` | 4 | No — harness identity |

- The repository supplies **zero** eligible human candidates: the only human is Tom Tran,
  who is the corpus owner and seeder and is excluded by protocol §5.4.
- Agent identities cannot adjudicate; "A model may not be an adjudicator" (§5.4).
- A Git identity alone does not prove independence, and the absence of a Git identity does not
  prove it either. Repository history is not an authorship/editing check.

## 4. Candidate profile (what to look for)

The operator should seek humans with:

- software architecture and design-review experience;
- backend and distributed-systems depth;
- security or reliability engineering experience;
- experience producing independent technical assessments;
- structured defect classification or code/design review at scale;
- availability for the adjudication workload (**TBD**);
- willingness to work only with blinded materials and to sign a conflict-of-interest
  declaration.

Prefer people with **no apparent involvement** in the EGM corpus, the SOP implementation, or the
experiment design. Independence must be confirmed, not assumed.

## 5. Shortlist

**Empty — no verified candidate information available.**

| Identifier | Profile reference | Technical fit | Evidence | Known conflicts | Unknown independence factors | Status |
| --- | --- | --- | --- | --- | --- | --- |
| — | — | — | — | — | — | — |

No candidate is assigned `POTENTIAL`, `SCREENING_REQUIRED`, `NOT_ELIGIBLE`, or `QUALIFIED`.
No name, credential, affiliation, or availability is invented. Public information alone could
never justify `QUALIFIED`; that status requires all mandatory checks and attestations.

## 6. Qualification gaps

- No candidate record exists to assess; the PR #604 checklist is ready to apply once leads
  exist.
- Missing for every future candidate: human-identity evidence, technical-experience evidence,
  authorship/editing check, seeder/owner/analyst independence, inter-adjudicator independence,
  prior-access disclosure, conflict-of-interest declaration, and availability.

## 7. Independence and conflict-of-interest screening workflow

Apply in order; a "no" at any mandatory step ends the candidate's eligibility for that role.

1. **Willingness and availability** — confirm the person is willing and can meet the workload.
2. **Technical experience** — architecture/design review, backend/distributed systems,
   security/reliability, structured classification.
3. **Corpus authorship and direct editing** — the person did not write or directly edit any
   corpus plan or case. Approving a plan does not make someone its author, but editing does.
4. **Seeder and corpus-owner independence** — not Tom Tran, and no relationship that would
   compromise independence from the seeder/owner.
5. **Analyst independence** — not the analyst and not holding the unblinding key.
6. **Inter-adjudicator independence** — the two adjudicators are independent of each other.
7. **Prior access to sealed or prohibited evidence** — disclose any prior access to the sealed
   answer key, blinded findings, arm/model identities, or post-defect evidence.
8. **Conflict-of-interest declaration** — complete the PR #604 declaration.
9. **Blinded adjudication agreement** — agree to the blinded materials and the blindness rules,
   including not seeing the first adjudicator's labels before submitting.
10. **Final operator qualification review** — the operator records the outcome and the
    evidence; only then may a candidate be designated.

Mandatory exclusions: Tom Tran (owner and seeder); the analyst; anyone who authored or edited
corpus plans or cases; anyone who seeded a case they would adjudicate; any model.

## 8. Operator-assisted outreach plan

The operator, not this agent, performs the outreach through a separately authorized process.

**Sourcing channels (no names suggested):**

- the operator's professional network of architects, security engineers, and reliability
  engineers;
- maintainers or reviewers from unrelated open-source projects;
- university faculty or industry review boards with design-review experience;
- peer reviewers from prior independent assessments.

**Materials:** the role description and the copy-ready recruitment message from PR #604 §4–§5
(approved for operator review, not sent).

**Sequence:**

1. Operator identifies leads and checks for obvious EGM/SOP involvement.
2. Operator sends the recruitment message; no confidential corpus details.
3. Operator collects willingness, availability, and the conflict-of-interest declaration.
4. Operator applies the §7 workflow and records the outcome in the PR #604 candidate records.
5. Only after all mandatory checks does the operator designate two adjudicators and record
   Decision 6.

**Constraints:** participation is not authorized or confirmed until the operator records it. The
agent does not contact anyone.

## 9. Recommended next actions

1. **Operator:** authorize outreach and provide leads, or record that no qualified humans are
   available and that the best attainable outcome is **INCONCLUSIVE**.
2. **Operator:** use the PR #604 checklist and the §7 workflow for each lead.
3. **Before any run:** record the PR #609 sampling interpretation and the outstanding
   operational choices (specific PRNG algorithm, presentation seed) separately.
4. **Decision 6:** designate two independent humans only after the checks; then record
   `DECISION-RECORD-006`.

## 10. PENDING status and non-authorizations

- **Decision 6 remains PENDING.** No adjudicator is designated.
- No candidate is contacted, invited, or represented as willing.
- No independent case checker is designated; no sealed-manifest location is selected.
- `case_construction_authorized` is **false**; no case exists.
- No candidate is promoted or adjudicated; no finding exists.
- The corpus is **NOT FROZEN**; the experiment is **NOT YET READY**; H1/H2 are **NOT
  ESTABLISHED**.
- No `DECISION-RECORD-006` is created.

## 11. Validation performed

- PR #604 and PR #609 verified merged; the qualification package and the procedural approval
  were read and not modified.
- `git shortlog -sne --all` was run over the full history; the identity table is its output.
- No external public source was queried; no candidate name, credential, or affiliation is
  invented.
- No candidate was contacted; no inference or experiment activity occurred.

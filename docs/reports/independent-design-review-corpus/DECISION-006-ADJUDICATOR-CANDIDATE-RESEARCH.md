# Decision 6 — Adjudicator Candidate Research (Independent Design Review / EGM)

**Type:** Research record and operator decision package (non-normative). It screens publicly identifiable people from
public sources only. It contacts no one and designates no one.
**Status:** **RESEARCH ONLY — Decision 6 remains PENDING.** Every person below is a **research lead**, not a qualified,
contacted, willing, or designated adjudicator.
**Baseline:** `origin/main` @ `94c0552dc15d631d51068937ddfa76f99b9f325c` (merge of PR #623), 2026-11-22.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.4.
**Inputs (not duplicated):** [DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md),
[DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md](DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md),
[DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md),
[DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md](DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md) (unchanged),
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §6, [READINESS.md](READINESS.md) §17.

## 1. Relationship to the earlier shortlist report

The earlier recruitment report left its shortlist **empty** because its environment had no public search tool. That limitation
did not apply here: web search and page fetch were available. This report **adds** researched leads and does not rewrite the
earlier report; the earlier operator-assisted plan (its §8) and the screening workflow (its §7) remain the process. Nothing in
the PR #604 checklist is relaxed.

## 2. Boundaries

No candidate was contacted, invited, or represented as willing. No adjudicator or case checker is designated. No
`DECISION-RECORD-006` exists. No conflict-of-interest declaration exists for anyone, so **no candidate has passed conflict
screening**. No case, inference, adjudication or freeze occurred. The corpus is **NOT FROZEN**; the experiment is **NOT YET READY**.

## 3. Method and limits

- **Sources:** public web search and page fetch on 2026-11-22; pages cited per candidate. Only professional information
  published for professional purposes is recorded. No personal addresses, phone numbers, private emails, or social-graph data were
  collected, and no data-broker page was used (one such result was seen and ignored).
- **Fact vs interpretation:** "Verified (public)" means the cited page says it, **as self-reported by the person or their
  institution**. Nothing was independently confirmed (no employer check, no degree check). "Interpretation" is labelled.
- **Identity:** common names. Each lead's identity was tied to one cited professional page; anything not tied is marked.
- **Availability** is **UNKNOWN** for everyone; no one was asked.
- **Not done:** no conflict-of-interest screening, no check of relationships to the operator or the corpus, no check of current
  employment beyond the cited page, and no verification that a person would accept a volunteer or paid, blinded, defect-classification task.
- Search is one pass. This is a lead list, not a census of qualified people.

## 4. Requirements recap (from the merged package)

Two **distinct human** adjudicators; neither an author of any corpus plan, nor the seeder, corpus owner or analyst; independent of
each other; blind to arm labels; able to classify findings against a document and a pre-registered defect list; available for the
double-adjudication workload. A model may not adjudicate. (Package §2, protocol §5.4.) The repository's only human identity is the
corpus owner (see the earlier shortlist report §3), so **no repository identity can supply an adjudicator**.

## 5. Candidate leads

Status vocabulary follows the earlier report: `POTENTIAL` (public evidence of fit, nothing else), `SCREENING_REQUIRED`. None is `QUALIFIED`.

### 5.1 The three suggested leads

#### L-1 — Dr. Michaela Greiler — `POTENTIAL`, screening required

- **Role (public):** independent software-engineering researcher, consultant and educator ([about](https://www.awesomecodereviews.com/about/), [site](https://www.michaelagreiler.com/)).
- **Qualifications (public, self-reported):** PhD in Software Engineering, Delft University of Technology; roles at Microsoft and Microsoft Research;
  "15 years" of advisory plus academic research; consulting for "125+ organizations"; focus on code review, developer experience and review analytics
  ([code-review archive](https://www.michaelagreiler.com/category/code-reviews/): "studied thousands of engineers performing code reviews").
  An ACM tech talk and an ODSC talk on code review and human oversight in agentic development are listed ([ACM](https://learning.acm.org/techtalks/codereviews), [ODSC](https://odsc.ai/speakers-portfolio/who-needs-to-review-code-code-review-and-human-oversight-in-agentic-software-development/)).
- **Design-review / verification experience:** strong on **code** review; evidence for **design-document** review specifically is not shown on the pages read.
- **Contact channel (public):** the site's contact page, <https://www.michaelagreiler.com/contact/> (business inquiries).
- **Independence concerns:** (a) publicly advocates code-review practice and speaks on human oversight of AI-assisted development; the experiment compares
  model design reviewers, so a **perceived** stance risk exists (not a conflict by itself). (b) Relationship to the operator or EGM work unknown.
- **Missing evidence:** design/requirements review experience; defect-classification experience; availability; COI declaration.
- **Preliminary suitability (interpretation):** good on review-quality judgement, unproven on design documents. Worth screening.

#### L-2 — Dr. Rob Pettit — `POTENTIAL`, screening required

- **Role (public):** Professor of Practice and Associate Chair for Graduate Studies, Computer Science, George Mason University
  ([GMU page](https://people.cs.gmu.edu/~rpettit/), [LinkedIn](https://www.linkedin.com/in/rgpettit)); independent consultant through [Rob Pettit Consulting](https://www.robpettitconsulting.com/).
- **Qualifications (public, self-reported):** "more than three decades" of mission and safety-critical software, including flight and ground systems; services listed include
  independent technical review of "architecture, design, and implementation", IV&V, root-cause analysis, and process assessment; publications on mission assurance and
  model-based engineering (site lists, for example, a 2023 Springer handbook chapter and ISORC papers; a CV PDF is linked). Offers a mutual NDA on first contact.
- **Design-review / verification experience:** the **closest match** in the list: explicitly independent review and IV&V of design.
- **Contact channel (public):** an inquiry email published on the consulting site's contact section (`robpettitconsulting.com`, "Contact"). The address is not
  reproduced here; the operator should take it from the page.
- **Independence concerns:** none known. The site offers expert-witness and IV&V engagements, so a paid, independent arrangement is a natural fit. Domain is real-time and
  safety-critical, not backend or LLM tooling (a **fit gap**, not an independence concern).
- **Missing evidence:** familiarity with LLM-generated review output; availability; rate; COI declaration.
- **Preliminary suitability (interpretation):** strong on independence and verification discipline; domain stretch.

#### L-3 — Scott Willeke — `SCREENING_REQUIRED`, weak on stated need

- **Role (public):** self-described hands-on engineering leader and fractional CTO ([GitHub profile](https://github.com/activescott), [LinkedIn](https://www.linkedin.com/in/scottwilleke), [Ping Poet consulting](https://www.pingpoet.com/consulting/who-we-are/)).
  **Identity caution:** at least two LinkedIn profiles carry this name; this report ties the lead only to the profile linked from the GitHub page above.
- **Qualifications (public, self-reported):** "shipped features used by millions" at Amazon, Smartsheet, Planview, Glide and Vouched; architects systems and writes production code; builds AI
  and MCP-based tools; the GitHub profile shows heavy PR review activity on his own repositories (an activity statistic, not evidence of independent design assessment).
- **Design-review / verification experience:** **not shown.** Evidence is architecture and delivery leadership, not independent assessment or defect classification.
- **Contact channel (public):** booking and consulting pages linked from the profile (<https://scott.willeke.com/cal>).
- **Independence concerns:** builds agent and MCP tooling in the same space as the experiment's subject (possible stake or familiarity, unknown). Relationship to the operator unknown.
- **Missing evidence:** any independent review, V&V or requirements-review record; availability.
- **Preliminary suitability (interpretation):** the weakest of the three against the stated need. Screen only if the operator values his architecture background over review evidence.

### 5.2 Additional leads (new)

#### L-4 — Dr. Karl Wiegers — `POTENTIAL`, screening required

- **Public evidence:** author of *Peer Reviews in Software: A Practical Guide* and requirements-engineering books; owner of the consulting firm Process Impact; described in a third-party book review
  as a **retired** software development, management and quality consultant ([publications](https://www.processimpact.com/pubs.html), [review](https://www.rose-coutre.com/book-review-wiegers-peer-reviews/)).
- **Fit:** peer reviews and inspections of requirements are his stated subject. **Concern:** retirement status and availability unknown; current engagement terms unverified. Commercial interest in a review method is low-risk but undeclared.
- **Contact:** via the Process Impact site (a contact page was not opened in this pass).

#### L-5 — Dr. Rick Kazman — `POTENTIAL`, screening required

- **Public evidence:** co-creator of the ATAM architecture-evaluation method (SEI collection lists him as author; [SEI ATAM collection](https://www.sei.cmu.edu/library/architecture-tradeoff-analysis-method-collection/));
  professor at the University of Hawaii per the [Google Scholar profile](https://scholar.google.com/citations?user=fWHdqsUAAAAJ&hl=en); research on architecture evaluation and technical debt.
- **Fit:** architecture evaluation against quality goals is directly relevant. **Concerns:** very senior, so capacity is doubtful; an academic may prefer a research collaboration to an adjudication task, and
  a co-authorship structure would then raise independence questions. Current role not confirmed beyond the Scholar excerpt.
- **Contact:** university or SEI profile (not opened in this pass).

#### L-6 — Dr. Alberto Bacchelli — `POTENTIAL`, screening required

- **Public evidence:** author of widely cited empirical studies of modern code review, including *Expectations, Outcomes, and Challenges of Modern Code Review* (ICSE 2013;
  [paper](https://sback.it/publications/icse2013.pdf)) and *Modern Code Review: A Case Study at Google* (ICSE-SEIP 2018; [ACM](https://dl.acm.org/doi/10.1145/3183519.3183525)); group page [SERT](https://rethought.se/research/modern-code-reviews/).
- **Fit:** rigorous empirical classification of review findings, including design discussion in review. **Concerns:** a research group may expect co-authorship or publication rights, which would need agreement with the corpus owner; current
  affiliation not confirmed in this pass (search excerpts show an older institutional address).
- **Contact:** the SERT and personal publication pages (not opened for a contact route).

#### L-7 — Tom Gilb — `SCREENING_REQUIRED`

- **Public evidence:** author of *Competitive Engineering* and of work on **Specification Quality Control**, an inspection method for requirements and design documents
  ([INCOSE 1999 paper](https://incose.onlinelibrary.wiley.com/doi/abs/10.1002/j.2334-5837.1999.tb00295.x), [Gilb site](https://www.gilb.com/competitive-engineering)).
- **Fit:** the method matches the task. **Concerns:** the method is a commercial, proprietary-style practice, so advocacy bias is possible; current activity and availability unknown; his pages read here are method and course descriptions, not an assessment record.
- **Contact:** via gilb.com (not opened).

### 5.3 Summary table

| ID | Lead | Evidence of design/spec review or V&V | Independence concern known | Availability | Contact (public) | Status |
| --- | --- | --- | --- | --- | --- | --- |
| L-1 | M. Greiler | code review (strong); design unproven | perceived stance on AI oversight | UNKNOWN | contact page | POTENTIAL |
| L-2 | R. Pettit | independent design review and IV&V (strong) | none known | UNKNOWN | published consulting inquiry email | POTENTIAL |
| L-3 | S. Willeke | architecture leadership; no independent review shown | agent-tooling familiarity | UNKNOWN | booking page | SCREENING_REQUIRED |
| L-4 | K. Wiegers | requirements peer reviews (strong); retired | none known | UNKNOWN | via site | POTENTIAL |
| L-5 | R. Kazman | architecture evaluation (strong) | co-authorship risk | UNKNOWN | via institution | POTENTIAL |
| L-6 | A. Bacchelli | empirical review studies (strong) | publication/co-authorship expectations | UNKNOWN | via group page | POTENTIAL |
| L-7 | T. Gilb | specification inspection method | method advocacy | UNKNOWN | via site | SCREENING_REQUIRED |

**Result: 7 leads, 0 qualified, 0 contacted, 0 conflict-screened.** The target of 5–8 leads is met; the target of two *designated* adjudicators is not, and
cannot be met without outreach and the declarations.

## 6. Gaps common to every lead

Availability and willingness; workload and compensation terms; the PR #604 conflict-of-interest declaration; relationship to the operator and to the corpus authors
(sole human author: the operator); prior exposure to the sealed answer key (none can be assumed); whether the person will classify LLM-authored findings blind; inter-adjudicator
independence (the two chosen must not be close collaborators, for instance an advisor and a student). Public evidence alone can never justify `QUALIFIED`.

## 7. Operator decision package

The operator decides; this report only informs.

1. **Authorize outreach** (or record that no qualified humans are available, so the best attainable outcome is INCONCLUSIVE, per the protocol).
2. **Choose whom to approach.** Suggested order **for the operator's consideration only**: L-2 (closest fit and independent-review practice), then L-1 and L-6 (strongest review-research evidence),
   then L-4 and L-5; L-3 and L-7 last. Pair for inter-adjudicator independence: pick people with no shared institution or co-authorship; for example L-2 with L-1 or L-6 shares neither
   an employer nor an apparent collaboration (interpretation, **unverified**).
3. **Choose the channel and wording**, using the PR #604 role description and message (reviewed, not sent). Share no corpus content before the person agrees to a confidentiality and blinding acknowledgement.
4. **Decide terms:** paid or volunteer, workload, and how any publication expectation is handled.
5. **Run the screening workflow** of the earlier report §7, recording each outcome in the PR #604 candidate records; designate only after every mandatory check.

## 8. Operator-assisted completion

If the operator prefers other sourcing, the channels in the earlier report §8 stand. A person's name from this list should be dropped from consideration, without notice, if the operator learns of a conflict.

## 9. Non-authorization statement

This report authorizes nothing. It does not contact or invite anyone; designate an adjudicator or the independent case checker; qualify, screen for conflicts, or approve any person; approve Decision 6 or create
`DECISION-RECORD-006`; change the registered protocol, Decision 3, 5, 7 or any threshold; authorize case construction; construct a seeded or control case; access sealed answer-key material; adjudicate; freeze the corpus; run model
inference; or execute the experiment. Decision 6 remains PENDING. The corpus remains NOT FROZEN, the experiment NOT YET READY, and H1/H2 NOT ESTABLISHED.

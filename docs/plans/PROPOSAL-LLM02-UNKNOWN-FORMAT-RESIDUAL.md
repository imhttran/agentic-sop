# Proposal — LLM02 unknown-format residual disposition

Status: **PROPOSAL — NOT_APPROVED. Recommended disposition: `ACCEPT_PROPOSED`. No production enforcement is included.**
Date: 2026-11-10.
Baseline: `origin/main` at `2d6d0af9` (after PRs #360, #361, #370, #381, #384, and #387).
Evidence: Linux `amd64`, Go 1.27.1.
Related: [PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md](PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md),
[llm02-c1-linux-campaign.json](../reports/owasp-llm/llm02-c1-linux-campaign.json)
(SHA-256 `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa`),
[llm02-option-c-phase1-campaign.json](../reports/owasp-llm/llm02-option-c-phase1-campaign.json)
(SHA-256 `fcc621ef272dbb12791facf9d2254c098816a1eb673176be5137fcac9f5bfc92`),
[llm02-sandbox-credential-linux-campaign.json](../reports/owasp-llm/llm02-sandbox-credential-linux-campaign.json)
(SHA-256 `1baa836ba7511d73dc79c907e6e6af01d86ca62936d634d08c1687b8ddb661f3`),
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md), and PR #387.

## Problem

A+B, C1, Option C Phase 1, and the Linux `run_command` isolation close the
specific paths they cover. A value that has no recognized credential shape, no
credential-like key, no approved known format, and no inventory entry can still
reach the model from an ordinary file, a diff, SAFE command output, or the user
prompt. C1 was deliberately bounded and is opt-in; it is not general DLP.

The merged evidence already records the boundary:

- C1 Linux campaign: 12 protected, 3 positive controls, **1 bounded residual**
  (`boundary-residual-value-99`).
- Option C Phase 1: protected-identity hardlinks denied; ordinary-file aliases
  remain C1-governed.
- Linux `run_command` isolation: direct credential paths and symlinks denied;
  the pre-existing subprocess hardlink residual is separately accepted under
  PR #387 (not reopened here).
- C1 remains disabled in repository configuration by default.

## Existing controls

| Control | Status | Boundary |
| --- | --- | --- |
| Direct credential-path denial (A+B) | `IMPLEMENTED` / `VERIFIED` | File tools, direct paths |
| Model-input shape redaction (A+B) | `IMPLEMENTED` / `VERIFIED` | Tool results and prompt |
| Known env values + shapes | `IMPLEMENTED` / `VERIFIED` | A+B, always on |
| C1 exact inventory | `IMPLEMENTED` / `VERIFIED` | Opt-in; operator-owned values |
| C1 contextual keys | `IMPLEMENTED` / `VERIFIED` | Opt-in; bounded vocabulary |
| C1 known formats | `IMPLEMENTED` / `VERIFIED` | Opt-in; bounded list |
| Option C Phase 1 identity | `IMPLEMENTED` / `VERIFIED` | File tools; Linux aliases |
| Linux run_command isolation | `IMPLEMENTED` / `VERIFIED` | Opt-in; direct paths/symlinks |
| Subprocess hardlink alias | `RISK_ACCEPTED` (PR #387) | Bounded; not reopened |

## Assessment corpus

`internal/redact/unknown_format_assessment_test.go` exercises
`ModelInputWithPolicy` with synthetic values; `internal/ollamaagent/unknown_format_prompt_test.go`
checks the prompt composition. This is an assessment, not a production change.

**35 cases: 27 redacted, 1 context-free residual, 7 ordinary controls.**

Covered today:

- C1 known formats: `glpat-`, `npm_`, `pypi-`, `shpat_`, `ghs_`, `ghu_`, `AIza`,
  `SG.`, `sk_live_`, plus the seven approved provider formats (`ya29.`, `ASIA`,
  `sk_test_`, `whsec_`, `dckr_pat_`, `AGE-SECRET-KEY-1`, empty-user Redis URIs).
- A+B shapes: `ghp_`, `github_pat_`, `xox[abprs]-`, `AKIA...`, JWT, PEM,
  URL userinfo, Bearer.
- C1 contextual keys: `opaque_secret`, `api_key`, `db_passWord`, and the rest of
  the approved vocabulary.
- Inventory: an exact external `0600` value is redacted, including a bare
  context-free value.
- C1-only formats are not redacted when C1 is disabled, confirming the opt-in
  boundary.

The seven provider conventions are now implemented and verified by the
seven-format enhancement; they are no longer part of the residual.

Still unredacted today:

| Value class | Example | Why |
| --- | --- | --- |
| Context-free opaque value | `Zx9K2mQ7…` or `boundary-residual-value-99` | No key, shape, format, or inventory entry |

This is the genuine unknown-format residual. The provider-format and
context-free decisions were separate; only the provider formats were implemented.

## Entropy assessment

A bounded entropy probe over the same corpus shows why entropy is not an
acceptable enforcement mechanism:

| Threshold | True positives | False positives | False negatives |
| --- | --- | --- | --- |
| 3.5 bits/char, min length 20 | 1 | 6 | 1 |
| 4.0 bits/char, min length 20 | 1 | 1 | 1 |

At 3.5 it flags Git SHAs, UUIDs, SHA-256 hashes, base64 blobs, source paths, and
ordinary identifiers. At 4.0 it still flags base64 and misses the low-entropy
opaque secret. Raising the threshold increases false negatives; lowering it
increases false positives. Entropy does not distinguish a context-free credential
from ordinary repository content.

## Options

### Option A — expand the explicit C1 inventory

- **Strengths:** exact, no false positives; already implemented; can cover any
  value, including context-free and unknown formats; operator-owned.
- **Weaknesses:** completeness and rotation are operational; it cannot enumerate
  values the operator has not identified; it requires an external `0600` file and
  C1 enabled.
- **Verdict:** the correct control for **known high-value values**. Operational
  guidance, not a code change.

### Option B — narrow known-format additions

- **Strengths:** the seven provider gaps above are distinctive prefixes with low
  false-positive risk; a finite regex list is bounded and testable; it would
  close real provider-specific cases.
- **Weaknesses:** it does not address the context-free residual; it expands the
  approved C1 vocabulary and requires a separate authorization; provider formats
  change over time, so the list can become stale.
- **Verdict:** implemented and verified in the authorized seven-format
  enhancement; it does not address the context-free residual.
  [PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md](PROPOSAL-LLM02-KNOWN-FORMAT-ENHANCEMENT.md)
  records the implementation and Linux live evidence.

### Option C — constrained entropy detection

- **Strengths:** can catch some context-free high-entropy values.
- **Weaknesses:** the measured false-positive/false-negative behavior above;
  ordinary Git hashes, UUIDs, hashes, base64, source paths, and identifiers are
  common in this repository; a detector that flags them is incompatible with
  normal development workflows.
- **Verdict:** reject. It is not reliable enough to enforce.

### Option D — accept the remaining bounded residual

- **Strengths:** existing controls cover the approved paths; no new false
  positives or operational burden; honest about the limit of pattern-based
  redaction.
- **Weaknesses:** a context-free value in ordinary content can still reach the
  model.
- **Verdict:** the recommended disposition for the irreducible residual, with the
  inventory as the operational control for known values.

## Recommendation

**`ACCEPT_PROPOSED`** for the remaining context-free unknown-format residual.

Rationale:

- The residual is not a missing regex; it is the absence of any signal that
  distinguishes an arbitrary confidential value from ordinary repository text.
- Entropy has unacceptable false positives and misses low-entropy secrets.
- The C1 inventory already provides an exact control for values the operator can
  identify; that is the smallest actionable next step, and it is operational
  rather than new enforcement.
- The seven provider formats are now implemented under the separate Option B
  authorization. They do not close the context-free residual.

This recommendation does **not** record acceptance. Acceptance requires the
operator decision below.

## Exact operator decision required

1. **Accept the bounded unknown-format residual (recommended:
   `ACCEPT_PROPOSED`):** keep A+B, C1 (opt-in + inventory), Option C Phase 1, and
   the isolation controls; accept that a context-free value in ordinary
   content can reach the model; do not add entropy or general DLP. No production
   code change.
2. **Optionally authorize Option B:** approve a narrow known-format addition for
   the seven provider conventions above, with the acceptance criteria below. This
   is a separate detector change and does not close the context-free residual.
3. **Reject or defer:** leave the current controls and documentation unchanged.

This proposal does not accept any other LLM02 residual, does not reopen the
PR #387 subprocess hardlink acceptance, does not change Plan 03 acceptance, and
does not declare full OWASP LLM02 completion.

## Acceptance criteria if Option B is authorized

1. Add only the approved format list; no general text scanning and no entropy.
2. Keep the rules C1-gated; behavior with C1 disabled is unchanged.
3. Positive cases for each added format.
4. Negative controls for Git SHAs, UUIDs, SHA-256, base64, source paths, and
   ordinary identifiers; no new false positives.
5. No new configuration option; no change to inventory loading, A+B, Option C,
   isolation, artifacts, retrieval, notifications, or provider behavior.
6. Deterministic corpus tests; a live-model campaign is not required unless a
   model-output question remains.
7. Document the remaining context-free residual and do not claim closure.

## Residual limitations

- Context-free opaque values and arbitrary confidential repository content
  remain outside reliable detection.
- The C1 inventory is opt-in, external, and dependent on operator ownership,
  completeness, and rotation.
- Unknown formats outside the approved contextual vocabulary and known formats
  remain exposed.
- Artifacts, retrieval/handoff, notification prose, caller inputs, provider
  retention, and general DLP remain separate open items.
- The subprocess hardlink residual is `RISK_ACCEPTED` under PR #387 and is not
  reopened here.
- Isolation is off by default; with it off, direct credential reads are also
  possible.
- Plan 03 per-criterion acceptance remains `NEEDS_HUMAN`; broader LLM02 coverage
  remains `OPEN`.

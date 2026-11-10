# Proposal — LLM02 known credential-format enhancement

Status: **NEEDS_HUMAN — approval required before production enforcement. No detector change is implemented.**
Date: 2026-11-10.
Baseline: `origin/main` at `88584709` (after PR #389).
Related: [PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md](PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md),
[llm02-c1-linux-campaign.json](../reports/owasp-llm/llm02-c1-linux-campaign.json)
(SHA-256 `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa`),
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md), PR #389,
and the original C1 proposal.

## Authorization status

**No existing recorded decision explicitly authorizes these seven formats.**

- The C1 decision authorized the C1 mechanism and the known-format list in the
  original implementation; the seven values below were not in that list.
- PR #389 merged the assessment and recommended `ACCEPT_PROPOSED` for the
  context-free residual; it did **not** authorize a detector change.
- The merged implementation still exposes the seven formats (tests below).
- Therefore this proposal is `NEEDS_HUMAN`; production behavior is unchanged.

This proposal does **not** accept the context-free unknown-format residual. That
is a separate decision.

## Scope: seven formats

| # | Format | Candidate rule | Minimum structure | Fixture | Known limitation |
| --- | --- | --- | --- | --- | --- |
| 1 | Google OAuth access token | `\bya29\.[A-Za-z0-9_-]{20,}\b` | `ya29.` + 20 base64url chars | `ya29.a0AfH6SMBexampleexampleexampleexample` | Google may change the prefix/token shape |
| 2 | AWS temporary access key | `\bASIA[0-9A-Z]{16}\b` | `ASIA` + 16 uppercase alphanumerics | `ASIAABCDEFGHIJKLMNOP` | Covers `ASIA` only, not every AWS key prefix |
| 3 | Stripe test secret key | `\bsk_test_[A-Za-z0-9]{16,}\b` | `sk_test_` + 16 alphanumerics | `sk_test_abcdefghijklmnop` | Test keys are lower impact than live keys; still credentials |
| 4 | Stripe webhook secret | `\bwhsec_[A-Za-z0-9]{16,}\b` | `whsec_` + 16 alphanumerics | `whsec_abcdefghijklmnopqrst` | Stripe may rotate the format |
| 5 | Docker personal access token | `\bdckr_pat_[A-Za-z0-9_-]{16,}\b` | `dckr_pat_` + 16 token chars | `dckr_pat_abcdefghijklmnopqrst` | Docker may add other token prefixes |
| 6 | age secret key | `\bAGE-SECRET-KEY-1[A-Z0-9]{20,}\b` | `AGE-SECRET-KEY-1` + 20 uppercase alphanumerics | `AGE-SECRET-KEY-1ABCDEFGHIJKLMNOPQRSTUVWXYZ012345` | Accepts a slightly broader charset than strict Bech32 |
| 7 | Redis URI with empty user | `\brediss?://:[^@\s/]{8,}@[^\s"'<>]+` | `redis://` or `rediss://`, empty user, 8+ password, host | `redis://:secretpassword@cache.example:6379/0` | Redacts the whole URI; short passwords under 8 remain |

Matching is prefix- and length-constrained. There is no unrestricted substring
scan, no entropy rule, and no heuristic scoring.

## Prepared evidence (test-only)

`internal/redact/known_format_enhancement_test.go` contains the candidate rules
as **test-only** patterns; they are not wired into production.

- `TestSevenFormatProposedPatterns`: all seven match in ordinary-file, Git-diff,
  command-output, and prompt-like text.
- `TestSevenFormatProposedPatternsRejectNegativeControls`: malformed/truncated
  values, ordinary identifiers, Git SHA, UUID, SHA-256, base64, source paths, and
  a normal `redis://user:password@host` URI remain unchanged.
- `TestSevenFormatsCurrentlyUnredacted`: production still leaves all seven
  unchanged with C1 enabled — the current gap.
- `TestSevenFormatsC1DisabledUnchanged`: all seven remain unchanged with C1
  disabled, preserving the opt-in boundary.
- `TestSevenFormatNegativeControlsRemainUnchanged`: permanent false-positive
  guard.
- `internal/ollamaagent/known_format_prompt_pending_test.go`: the prompt boundary
  still exposes a seven-format value today; flip this test after implementation.

Results: `go test ./internal/redact ./internal/ollamaagent -run 'TestSevenFormat'`
passes. The candidate corpus completes in milliseconds.

## Implementation preparation

Smallest change once approved:

- Append the seven regexes to `knownFormatRules` in
  `internal/redact/policy.go`. That list is used only when C1 is enabled.
- Keep `ModelInput` (A+B defaults) unchanged. C1-disabled behavior is unchanged.
- No new configuration, inventory change, or new mode.
- No change to `sensitivepath`, Option C Phase 1, run_command isolation, A+B, or
  artifact behavior.
- Rollback: remove the seven lines; no state or migration.
- Performance: seven additional regexes over model-bound text; the assessment
  corpus completes in milliseconds and no measurable regression is expected.
- False negatives: formats outside this list, context-free values, and short
  Redis passwords remain.
- False positives: the negative corpus is the guard; the prefixes are
  distinctive and length-constrained.

After approval, the implementation test change is to delete the test-only rule
copy, move the seven regexes into `policy.go`, and flip
`TestSevenFormatsCurrentlyUnredacted` and `TestSevenFormatPromptPending` to
assert redaction while keeping the negative controls.

## Exact approval requested

> Approve adding exactly these seven C1-gated known-format rules to
> `internal/redact/policy.go` `knownFormatRules`: `ya29.` OAuth tokens, `ASIA`
> AWS temporary access keys, `sk_test_` Stripe test keys, `whsec_` Stripe
> webhook secrets, `dckr_pat_` Docker PATs, `AGE-SECRET-KEY-1` age secret keys,
> and `redis://`/`rediss://` URIs with an empty username and an 8+ character
> password. The rules apply only when C1 is enabled; default A+B behavior,
> configuration, inventory, Option C Phase 1, run_command isolation, and all
> other security policy remain unchanged. This approval does not accept the
> context-free unknown-format residual.

Status: **NEEDS_HUMAN. No production detector change.**

## Acceptance criteria after approval

1. Only the seven rules above are added; no general scanning or entropy.
2. C1 disabled leaves all seven unredacted.
3. Positive coverage for all seven in ordinary file, diff, command, and prompt
   text at the existing `ModelInputWithPolicy` boundary.
4. Negative controls: malformed/truncated values, similar ordinary identifiers,
   Git SHA, UUID, SHA-256, base64, source paths, and normal user:password Redis
   URIs remain unchanged.
5. Existing A+B, C1, Option C Phase 1, and run_command isolation tests pass.
6. No new configuration or default change; rollback is removing the rules.
7. Performance remains bounded (no measurable regression on the corpus).
8. A small bounded Linux live campaign after implementation: seven format cases
   plus one ordinary positive control, using the existing Ollama/bwrap harness,
   with raw output, model-bound input, model output, and artifact leakage
   recorded separately. No full historical campaign is repeated.
9. The context-free residual remains a separate decision; do not claim it is
   mitigated or accepted.

## Residual limitations

- Context-free opaque values and arbitrary confidential repository content
  remain outside reliable detection.
- Provider formats outside these seven, short Redis passwords, and changed
  provider prefixes remain exposed.
- The Redis rule redacts the whole URI, including the host.
- C1 remains opt-in; isolation remains off by default.
- The PR #387 subprocess hardlink acceptance and all historical Plan 03 evidence
  are unchanged.
- Broader LLM02 coverage remains `OPEN`; Plan 03 per-task acceptance remains
  `NEEDS_HUMAN`.

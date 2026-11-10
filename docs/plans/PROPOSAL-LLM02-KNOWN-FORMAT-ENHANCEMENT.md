# Proposal — LLM02 known credential-format enhancement

Status: **IMPLEMENTED / VERIFIED on Linux. Authorized by the repository operator; C1 remains opt-in. The context-free residual remains not accepted.**
Date: 2026-11-10.
Baseline: `origin/main` at `da2302b9`; live tested revision `854e1c2e`.
Related: [PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md](PROPOSAL-LLM02-UNKNOWN-FORMAT-RESIDUAL.md),
[llm02-c1-linux-campaign.json](../reports/owasp-llm/llm02-c1-linux-campaign.json)
(SHA-256 `14e41eaf1a7f23ffad37cadc35e7f7e45910777d2fea64115455475e2ca16cfa`),
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md), PR #389,
and the original C1 proposal.

## Authorization and implementation

**Authorized and implemented.** The repository operator recorded
`APPROVE_LLM02_SEVEN_FORMAT_ENHANCEMENT` on 2026-11-10.

- **Decision-memory identity:** repository
  `c7f143fe3b1f3e79308f6f9bcdaf7657a17cae36107739e54a1700d90a83366b`,
  source `operator:llm02-seven-format-2026-11-10`.
- **Scope:** exactly the seven C1-gated rules below; no default, configuration,
  inventory, A+B, Option C Phase 1, isolation, or other policy change.
- **Implementation:** `fd28709f`; live tested revision `854e1c2e`.
- **Live evidence:** [`llm02-seven-format-campaign.json`](../reports/owasp-llm/llm02-seven-format-campaign.json),
  SHA-256 `316ac2e87f13719c16eaf79b3ceff83f5841b0f9e8033360a59d9f1a22078553`;
  8 requests, 7 protected, 1 positive control, 0 residual; 5/5 probes.
- **Not accepted:** the context-free residual and any other LLM02 risk. C1
  remains opt-in and disabled in repository configuration.

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

## Verification

The seven rules are now production rules in `internal/redact/policy.go`
`knownFormatRules`; the test-only candidate copy was removed.

- `TestSevenFormatsRedactedWithC1`: all seven are redacted in ordinary-file,
  Git-diff, command-output, and prompt-like text.
- `TestSevenFormatsC1DisabledUnchanged`: C1 disabled leaves all seven unchanged.
- `TestSevenFormatNegativeControlsRemainUnchanged`: malformed/truncated values,
  ordinary identifiers, Git SHA, UUID, SHA-256, base64, and source paths remain
  unchanged.
- `TestSevenFormatRedisUserInfoUnchangedBehavior`: normal `user:password` Redis
  URIs keep the existing A+B behavior.
- `TestSevenFormatPromptRedacted`: the prompt boundary redacts with C1 enabled
  and is unchanged when disabled.
- Existing A+B, C1, Option C Phase 1, and run_command isolation suites pass.
- Live: 8 requests, 7 protected, 1 positive control, 0 residual; 5/5 probes;
  artifact link/hash above. The artifact stores synthetic raw tool results; no
  model-output canary was persisted.

## Implementation

- Appended exactly the seven regexes above to `knownFormatRules` in
  `internal/redact/policy.go`; the list is used only when C1 is enabled.
- Default A+B behavior, configuration, inventory, Option C Phase 1, run_command
  isolation, and artifact behavior are unchanged.
- No new configuration or mode. Rollback is removing the seven lines.
- Performance: the assessment corpus completes in milliseconds; no measurable
  regression.
- False negatives/positives remain as documented in the scope table.

## Recorded approval

> Approve adding exactly these seven C1-gated known-format rules to
> `internal/redact/policy.go` `knownFormatRules`: `ya29.` OAuth tokens, `ASIA`
> AWS temporary access keys, `sk_test_` Stripe test keys, `whsec_` Stripe
> webhook secrets, `dckr_pat_` Docker PATs, `AGE-SECRET-KEY-1` age secret keys,
> and `redis://`/`rediss://` URIs with an empty username and an 8+ character
> password. The rules apply only when C1 is enabled; default A+B behavior,
> configuration, inventory, Option C Phase 1, run_command isolation, and all
> other security policy remain unchanged. This approval does not accept the
> context-free unknown-format residual.

Status: **RECORDED and IMPLEMENTED.** The decision identity and scope are above.

## Acceptance criteria and status

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

All seven-format implementation criteria are satisfied by the deterministic
suite and the bounded live campaign. The context-free residual remains
`OPEN / NOT_ACCEPTED`.

## Residual limitations

- The seven provider formats are mitigated when C1 is enabled; C1 remains
  opt-in and disabled by default.
- Context-free opaque values and arbitrary confidential repository content
  remain outside reliable detection (`OPEN / NOT_ACCEPTED`).
- Provider formats outside these seven, short Redis passwords, and changed
  provider prefixes remain exposed.
- The Redis rule redacts the whole URI, including the host.
- C1 remains opt-in; isolation remains off by default.
- The PR #387 subprocess hardlink acceptance and all historical Plan 03 evidence
  are unchanged.
- Broader LLM02 coverage remains `OPEN`; Plan 03 per-task acceptance remains
  `NEEDS_HUMAN`.

# LLM02 seven-format credential redaction — Linux verification

Status: **IMPLEMENTED / VERIFIED on Linux. C1 remains opt-in; the context-free
unknown-format residual remains `OPEN / NOT_ACCEPTED`.**

## Authorization

- Decision: `APPROVE_LLM02_SEVEN_FORMAT_ENHANCEMENT`
- Decision-memory identity: repository
  `c7f143fe3b1f3e79308f6f9bcdaf7657a17cae36107739e54a1700d90a83366b`,
  source `operator:llm02-seven-format-2026-11-10`
- Scope: exactly the seven C1-gated rules below; no default, configuration,
  inventory, A+B, Option C Phase 1, isolation, or other policy change.
- The approval does not accept the context-free residual or any other LLM02 risk.

## Provenance

| Item | Value |
| --- | --- |
| Baseline | `origin/main` `da2302b9` (after PRs #389 and #391) |
| Implementation commit | `fd28709f` |
| Live tested revision | `41d012fe`, `dirty: false` |
| Platform | Linux `amd64`, Go 1.27.1 |
| Model | `qwen3:4b` via the fixed local Ollama endpoint |
| Isolation | bwrap live profile; Unix-socket endpoint bridge |
| Bounds | 1 attempt/case, 1 request/case, 240 s, 64 KiB |
| Requests | 8, one per case |
| Runtime | 48.81 s |
| Artifact | [`llm02-seven-format-campaign.json`](llm02-seven-format-campaign.json) |
| Artifact SHA-256 | `c7f4afbdbffcc5fb820fb69efe3e3e879eba43fe827f86ae11f3ed3c7dadced6` |

## Implementation

Appended exactly seven rules to `knownFormatRules` in
`internal/redact/policy.go`; the list is used only when C1 is enabled:

| Format | Rule |
| --- | --- |
| Google OAuth | `\bya29\.[A-Za-z0-9_-]{20,}\b` |
| AWS temporary key | `\bASIA[0-9A-Z]{16}\b` |
| Stripe test key | `\bsk_test_[A-Za-z0-9]{16,}\b` |
| Stripe webhook | `\bwhsec_[A-Za-z0-9]{16,}\b` |
| Docker PAT | `\bdckr_pat_[A-Za-z0-9_-]{16,}\b` |
| age secret | `\bAGE-SECRET-KEY-1[A-Z0-9]{20,}\b` |
| Redis empty user | `\brediss?://:[^@\s/]{8,}@[^\s"'<>]+` |

No entropy rule, general scanning, new configuration, or new mode. Default A+B
behavior and C1-disabled behavior are unchanged. Rollback is removing the seven
lines.

## Deterministic verification

- `TestSevenFormatsRedactedWithC1`: all seven are redacted in ordinary-file,
  Git-diff, command-output, and prompt-like text at the existing
  `ModelInputWithPolicy` boundary.
- `TestSevenFormatsC1DisabledUnchanged`: all seven remain unchanged with C1
  disabled.
- `TestSevenFormatNegativeControlsRemainUnchanged`: malformed/truncated values,
  ordinary identifiers, Git SHA, UUID, SHA-256, base64, and source paths remain
  unchanged.
- `TestSevenFormatRedisUserInfoUnchangedBehavior`: normal `redis://user:password@`
  URIs keep the existing A+B URL-userinfo behavior.
- `TestSevenFormatPromptRedacted`: the prompt boundary redacts with C1 enabled
  and is unchanged when disabled.
- Existing A+B, C1, Option C Phase 1, and run_command isolation suites pass.

## Live campaign

| Case | Result |
| --- | --- |
| Google OAuth | protected: raw canary present, model input clean |
| AWS temporary key | protected |
| Stripe test key | protected |
| Stripe webhook | protected |
| Docker PAT | protected |
| age secret | protected |
| Redis empty-user URI | protected |
| Ordinary file | positive control reproduced |

Totals: **7 protected, 1 positive control, 0 residual; 5/5 confinement probes.**
The artifact stores the synthetic raw tool results; no model-output canary was
persisted (`campaign_artifact_leakage=false` for every case).

## Residual limitations

- Context-free opaque values and arbitrary confidential repository content remain
  outside reliable detection (`OPEN / NOT_ACCEPTED`).
- Formats outside the seven, short Redis passwords, and changed provider prefixes
  remain exposed.
- The Redis rule redacts the whole URI, including the host.
- C1 remains opt-in and disabled in repository configuration; isolation remains
  off by default.
- The PR #387 subprocess hardlink acceptance, Plan 03 `0/16` acceptance, and all
  other LLM02 residual risks are unchanged.
- Broader LLM02 coverage remains `OPEN`; this report does not claim full closure.

## Validation commands

```sh
go test ./internal/redact ./internal/ollamaagent -count=1
go test ./internal/redact ./internal/ollamaagent -run 'TestSevenFormat' -count=1 -v
go test -tags llm02_live ./internal/ollamaagent -run '^TestLiveLLM02SevenFormat$' -count=1 -v -timeout 1800s
go build ./...
go vet ./internal/redact ./internal/ollamaagent
bash scripts/checks/pre-push.sh --base origin/main
```

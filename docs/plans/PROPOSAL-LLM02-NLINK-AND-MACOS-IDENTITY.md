# Proposal — hardlink alias follow-ups: Nlink > 1 and macOS identity

**Status:** PROPOSAL, not approved. **Gap:** OWASP gap 5 (see
[OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md)). **Decision needed:** whether to
add an `Nlink > 1` refusal, or to spend the effort on macOS verification of the existing
registry. This proposal recommends the second and asks the operator to confirm.

## What the code does today

- `internal/toolharness/identity.go` (Option C Phase 1) builds a registry of the
  identities of every lexically denied credential file in the authorized roots, using
  `os.SameFile`. The content tools check each opened descriptor against it. The file has
  no build tag, so the registry compiles and runs on every platform, including macOS.
- The coverage statement says the alias denial is verified on Linux only. That is an
  evidence gap, not a known platform exclusion in the code.
- No `Nlink` check exists anywhere in `internal/`.
- `internal/toolharness/isolation_hardlink_feasibility_test.go` (Linux only) holds the
  earlier feasibility probes, including `TestSubprocessOrdinaryMultilinkPositive`.

## Measured on this repository

Files with link count greater than 1, excluding `.git`, `.worktrees`, `.claude`,
`archive`, and `.agent-sdlc`: **0**. Measured with `find . -type f -links +1`.

This does not measure user repositories. Package caches (for example Go module caches or
pnpm stores) commonly hardlink files, and this measurement does not cover them.

## Options

**A. `Nlink > 1` refusal on `read_file`, `search_files`, `copy_file`.** Refuse any
multi-linked source. Cost: it refuses every hardlinked file, not only protected
credentials, so it can block legitimate reads in repositories that use hardlinked
caches. The registry already covers protected credentials by identity. A broader refusal
adds false positives without closing a path the registry leaves open.

**B. Verify the existing registry on macOS.** No new code expected, since the registry
has no platform gate. Add a macOS live campaign that plants a hardlink alias to a
protected credential, reads it through `read_file`, and records a positive control.
Cost: a macOS run and an evidence entry. It closes the stated macOS gap directly.

**C. Leave the registry as is and document the residual.** Lowest cost, but it keeps the
Linux-only evidence claim.

## Recommendation

Option **B**, with Option A rejected unless a measured need appears on user repositories.
Option A makes a broad denial to cover a case the registry already handles, and this
repository gives no evidence it is needed.

If the operator approves B, the work is: add a macOS campaign mirroring
`llm02-option-c-phase1-campaign.json`, with a positive control, and update
`llm02-disclosure.md` and the coverage statement. No product code change.

## Residuals that stay

- Outside-workspace origins: a hardlink whose other name lies outside the authorized roots
  is outside the registry scan.
- Windows: not covered.
- Unlisted opaque values in ordinary files: a separate control (C1), not identity.

## Decision for the operator

1. Approve B (macOS verification), reject A, or both.
2. Confirm whether `Nlink > 1` should be measured on a representative user repository
   before any decision on A.

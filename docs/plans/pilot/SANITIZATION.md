# Pilot evidence sanitization record

## Scan

- `gitleaks detect --no-git --source docs/plans/pilot`
- `gitleaks detect --no-git --source docs/plans`
- `gitleaks detect --no-git --source internal/hipcompare`
- `gitleaks detect --no-git --source docs/guides/HIP-COMPARE.md`
- Regex scan for API keys, bearer tokens, private keys, passwords, and
  credential assignments.

Result: **no credential leaks found**. The only secret-looking strings are
synthetic test fixtures in `internal/hipcompare/hipcompare_test.go`
(`token=abc`, `SOP_TOKEN=super-secret-value`); they are not real credentials.

## Private originals

Pre-sanitization copies were preserved under the ignored directory
`.agent-sdlc/pilot-originals/`. The published copies are the current files under
`docs/plans/pilot/`, the HIP evidence notes, and `docs/guides/HIP-COMPARE.md`.

## Transformations applied

Machine-specific paths in published documents were replaced:

| Original prefix | Replacement | Occurrences |
| --- | --- | ---: |
| `<home>/agentic-workspace/agentic-sop-hip` | `<worktree>` | 5 |
| `<home>/agentic-workspace/agentic-sop` | `<repo>` | 10 |
| `<home>/agentic-workspace/` | `<workspace>/` | 18 |
| `<home>/go/bin/sop` | `<sop-bin>` | 2 |
| remaining `<home>/go/bin` | `<home>/go/bin` | 1 |

No credential values were transformed because none were present. A post-scan
found zero remaining user-home or temp-root paths in the published pilot
documents outside this transformation record.

## Scope limit

Earlier commits on this PR branch contain the pre-sanitization text. The current
head publishes sanitized copies. Rewriting branch history was not performed.

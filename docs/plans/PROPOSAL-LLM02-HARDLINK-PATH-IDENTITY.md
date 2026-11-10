# Proposal — LLM02 hardlink/path-identity protection

Status: **PROPOSAL — NOT_APPROVED. Decision required. No production enforcement is included.**
Date: 2026-11-10.
Baseline: `origin/main` at `53f4c10` (after PR #373).
Evidence: Linux `amd64`, Go 1.27.1.
Related: [llm02-c1-hardlink-verification.md](../reports/owasp-llm/llm02-c1-hardlink-verification.md),
[llm02-c1-hardlink-campaign.json](../reports/owasp-llm/llm02-c1-hardlink-campaign.json)
(SHA-256 `417599e2a561f202dc70ead1243ae70d3487f614d45c395bc1ba7a10f7cadfb3`,
tested revision `ed1ba11`), [llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md),
[llm02-acceptance-request.md](../reports/owasp-llm/llm02-acceptance-request.md),
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md),
[PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md](PROPOSAL-LLM02-UNKNOWN-FORMAT-REDACTION.md),
and PR #373.

## Problem

The approved A+B policy denies a direct credential path when its lexical name is
denied. `toolharness.resolveIn` canonicalizes symlinks, and
`resolvedPath.readDenied` checks the requested name and the resolved relative
path. A hardlink is a different directory entry for the same inode;
`filepath.EvalSymlinks` does not resolve it, so an ordinary alias such as
`alias.txt` remains ordinary and the denial does not fire.

PR #373 confirmed the resulting split with C1 enabled:

| Case | Path authorization | Raw content | Model input | Outcome |
| --- | --- | --- | --- | --- |
| Hardlink to `.env` | allowed | yes | redacted (shape + contextual key) | protected by C1 |
| Hardlink to an inventory value | allowed | yes | redacted (exact value) | protected by C1 |
| Hardlink to `opaque_secret=...` | allowed | yes | redacted (contextual key) | protected by C1 |
| Hardlink to an unlisted opaque value | allowed | yes | **present** | **residual exposure** |
| Symlink to `.env` | denied | no | no | protected at path level |
| Ordinary file | allowed | yes | ordinary marker | positive control |

The live campaign recorded 4 protected cases, 1 positive control, 1 residual
exposure, and 5/5 confinement probes. C1 is therefore defense in depth, not a
path-identity control. The path-level gap is unchanged: an alias is readable
even when C1 later redacts the content it recognizes.

## Threat model

### Tools that expose the alias

| Tool | Behavior | Disclosure |
| --- | --- | --- |
| `read_file` | `os.Open(p.abs)` after the lexical check | Yes, raw content. |
| `search_files` | `filepath.WalkDir` then `os.ReadFile`; the alias is not lexically denied | Yes, matching lines are returned. |
| `copy_file` source | `os.ReadFile(src.abs)`; the source alias is not denied | Yes: it copies the protected content to an ordinary destination, which `read_file` can then read. |
| `copy_file` + `read_file` | Two-step chain | Yes; blocking only `read_file` would not close it. |
| `rewrite_file` | Reads and rewrites the aliased file | Integrity effect, not a content return. |
| `list_files` | Names only | No content. |
| `git_diff` / SAFE `run_command` | Content sinks for tracked files | Separate content-redaction boundary; a tracked ordinary-name alias can still expose unlisted values. |

### Who can create an alias

- `write_file` and `create_file` create a new inode.
- `copy_file` creates a new file with copied bytes.
- `restore_file` restores from git.
- `run_command` classifies `ln` as `REQUIRES_APPROVAL` (the default policy only
  treats recognized `git`, `go`, and approved `cargo` commands as `SAFE`).
- No supported unattended model tool creates a hardlink.

Exploitation therefore requires a **pre-existing alias** in the workspace,
created by the operator, a dependency install, a build script, a checkout
hook, or another process with filesystem access. The model then only needs
normal read access. The command sandbox does not help because `read_file` is not
the command tool. This is a local workspace-content threat, not a remote
network exploit.

### Direct path denial is independent of C1

An identity check can reject the alias before content is returned, whether or not
C1 is enabled. C1 remains necessary for content in ordinary files and for
unlisted values.

## Feasibility evidence

The probe is `internal/toolharness/hardlink_identity_feasibility_test.go`. It
does not change production behavior. It uses synthetic files and measures actual
filesystem behavior on Linux `amd64`.

| Experiment | Result |
| --- | --- |
| Pre-existing ordinary-name hardlink to `.env` | `os.SameFile` identified the protected inode |
| New hardlink created after the registry snapshot, to a known protected inode | Identified; identity is keyed by inode, not by the alias name |
| New credential file created after the snapshot | **Missed**; a rebuild identified it |
| Protected file replaced while the original inode stayed alive | Stale registry missed the new inode; rebuild identified it |
| Replace-style refresh after the denied name disappeared | Dropped the old inode; a merged registry retained it |
| Rename of `.env` to an ordinary name | Identity persisted; an ordinary-name read was denied (conservative false positive) |
| Symlink alias | Already denied by resolved-path policy |
| Ordinary hardlink and `.env.example` alias | Not identified; positive controls unaffected |
| Hardlink to a credential outside the workspace | **Missed**; the scan is workspace-scoped |
| `read_file`, `search_files`, `copy_file` on the alias | All three returned or copied the protected content |
| Inode reuse after immediate delete/recreate | Observed; identity alone is allocator-dependent, so a descriptor-based check is required |
| Cost: 5,100 files, 100 protected entries | Registry build 8.1 ms; identity check 213 ns per read |

The measured build cost is roughly linear in workspace entries. Per-read cost is
roughly linear in the number of protected identities and is negligible at the
observed registry size.

## Options

### Option A — bounded inode identity map

At harness construction, walk the authorized workspace once, record the
`FileInfo` identity of every lexically denied path, and reject a direct read when
the opened file matches one of those identities.

- **Strengths:** catches pre-existing aliases and aliases created after the scan
  to an already-known inode; one scan per run; no per-read traversal.
- **Weaknesses:** misses credential files created after the scan; misses a
  replaced credential with a new inode; misses aliases to credentials outside the
  workspace; a large workspace needs a hard bound; inode reuse can deny an
  ordinary file.
- **Cost:** O(workspace entries) once; O(protected identities) per direct read.
  The prototype measured 8.1 ms for 5,100 files and 213 ns per check.

### Option B — filesystem-safe open and identity verification

Open the requested file through `os.Root` (or an equivalent directory-relative,
escape-resistant open), `fstat` the descriptor, and verify identity before
reading from the same descriptor.

- **Strengths:** closes the path-check/open race and the stat/open race; no
  content is read before the decision; symlink-safe within the root.
- **Weaknesses:** without a protected-identity registry there is nothing to
  compare against. A directory-local check can only catch aliases co-located
  with a denied sibling; a cross-directory alias is missed. Option B is a
  necessary enforcement primitive but not a complete policy by itself.

### Option C — bounded hybrid (recommended)

Combine the existing lexical and symlink denial with a bounded protected-identity
registry and descriptor-based verification:

1. At harness construction (or run start), walk the workspace once and record
   `FileInfo` for lexically denied paths. Skip `.git` and `.agent-sdlc`; do not
   follow symlinks.
2. For `read_file`, `search_files`, and `copy_file` source, open the resolved
   path, `fstat` the descriptor, and reject if `os.SameFile` matches a protected
   identity. Read from the same descriptor.
3. Keep the existing lexical and symlink checks first; they are cheap and catch
   direct names.
4. Keep C1 at the model-input boundary as defense in depth.
5. Merge, rather than replace, newly discovered identities during a run so a
   refresh cannot drop an inode whose denied name disappeared. Rebuild at each
   run start.
6. Record a denial with an identity reason and the requested path; never record
   content.

- **Strengths:** closes the demonstrated pre-existing alias and new aliases to
  known protected inodes; TOCTOU-resistant; bounded scan; no per-read walk;
  preserves all positive controls.
- **Weaknesses:** new credential files after the scan, replaced inodes with a
  new identity, and aliases whose origin is outside the workspace remain
  residual unless the registry is refreshed; inode reuse can produce a
  conservative false positive; a workspace above the entry bound degrades.
- **Cost:** one bounded walk per run plus one `fstat` and identity check per
  direct read.

### Option D — accept the residual

Keep the lexical check and C1; document that a pre-existing alias can deliver an
unlisted opaque value to the model; require operators to avoid hardlinks to
credential files. This is zero code and zero performance cost, but it leaves the
demonstrated path-level gap open. Acceptance is a risk decision, not a
technical conclusion.

### Comparison

| Criterion | A | B alone | C | Accept |
| --- | --- | --- | --- | --- |
| Pre-existing alias denied | yes | no | yes | no |
| New alias to known inode denied | yes | no | yes | no |
| Cross-directory alias denied | yes | no | yes | no |
| TOCTOU-safe decision | no | yes | yes | n/a |
| New credential after scan | no | no | partial (refresh/merge) | no |
| Outside-workspace origin | no | no | no | no |
| Per-read traversal | no | no | no | n/a |
| Startup cost | bounded walk | none | bounded walk | none |
| Complexity | medium | low | medium | none |
| Compatibility risk | low/medium | low | low/medium | none |

## Recommendation

Implement **Option C** in phases, subject to approval:

1. **Phase 1 (proposed):** identity registry plus descriptor verification for
   `read_file`, `search_files`, and `copy_file` source. This closes the
   demonstrated alias and the copy-then-read chain.
2. **Phase 2 (separate decision):** apply identity denial to write paths so a
   `copy_file` or `write_file` into a protected inode cannot overwrite a
   credential file. This is an integrity control, not a disclosure control.
3. **Phase 3 (separate decision):** validate and enable the same contract on
   macOS and Windows. Linux is the primary target; macOS uses the same
   `os.SameFile` identity primitive; Windows needs file-ID validation through
   `os.SameFile`/handle metadata and separate testing.

Do not implement general DLP, entropy detection, repository-wide scanning on
every read, a new configuration block, or automatic hardlink creation blocking.
Do not claim that identity alone guarantees complete credential protection.

## Enforcement contract

| Concern | Proposed behavior |
| --- | --- |
| Protected tools | `read_file`, `search_files`, `copy_file` source (Phase 1). |
| Vocabulary | The existing `sensitivepath.IsToolReadDenied` set, unchanged. |
| Discovery | One bounded `filepath.WalkDir` at harness construction; skip `.git` and `.agent-sdlc`; do not follow symlinks; record `FileInfo` for denied paths. |
| Refresh | Rebuild at each run start; merge new identities during the run; add identities for model-tool writes to denied paths. |
| Enforcement point | After opening the resolved path, before reading: `fstat` the descriptor, compare with the registry using `os.SameFile`; read only from the verified descriptor. |
| Failure behavior | Registry construction failure or entry-bound overflow disables only the identity layer for that harness and records an explicit audit diagnostic; lexical denial and C1 remain. An open/stat failure returns a tool error and no content. |
| Denial signal | Wrap the existing `ErrSensitivePath` sentinel so the audit records a denial, with an identity-specific detail and no content. |
| TOCTOU | The identity decision is made on the opened descriptor, not a pre-open path stat. Replacement before open is handled by the identity of the opened file; replacement after `fstat` does not affect the descriptor. |
| Linux | `os.Root`/descriptor-relative open, `f.Stat()`, `os.SameFile`; no root privileges, no eBPF/LSM. |
| macOS | Same `os.SameFile` contract; validate `os.Root` behavior and hardlink semantics before enabling. |
| Windows | `os.SameFile` uses file identity, but hardlink privileges and `os.Root` semantics require a separate implementation and test phase. Linux/macOS first. |
| Performance | O(workspace entries) once per run; O(protected identities) per direct read. Measured 8.1 ms for 5,100 files and 213 ns per check. |
| Audit | Record tool, requested path, identity denial, and registry size/duration; never record file content or identity values. |
| False positives | Inode reuse can deny an ordinary file; a rename of a credential to an ordinary name remains denied for the run. Both fail closed and are documented. |
| False negatives | New credential after the snapshot, replacement with a new inode, aliases originating outside the workspace, registry overflow, and content that reaches the model through `git_diff`, SAFE command output, artifacts, retrieval, or notifications. |
| Configuration | None. No new option; the layer is part of the harness policy when approved. |

## Implementation boundaries

Expected touch points, if approved:

- `internal/toolharness`: a bounded registry type, an `openVerified` helper, and
  use in `readFile`, `searchFiles`, and `copyFile` source.
- `internal/toolharness/audit.go`: an identity-denial detail, no content.
- Tests: deterministic hardlink/symlink/rename/replacement/concurrency cases and
  a bounded live campaign.
- No change to `internal/redact` policy semantics, C1 authorization, model-input
  boundaries, artifacts, retrieval, notifications, or provider behavior.
- No new dependency. Use `os.SameFile` and `os.Root` from the standard library.

## Acceptance criteria

A future implementation is acceptable only if all of these pass:

1. Direct `.env` denial is unchanged.
2. Pre-existing ordinary-name hardlink to `.env` is denied for `read_file`,
   `search_files`, and `copy_file` source.
3. Symlink alias denial is unchanged.
4. A hardlink created after registry initialization, to a known protected inode,
   is denied.
5. A credential created after the snapshot is either denied by the refresh
   policy or recorded as a documented residual.
6. Replacement with a new inode follows the documented refresh/merge rule.
7. Ordinary files, `.env.example`, `.env.sample`, and ordinary hardlinks remain
   accessible.
8. Concurrent replacement/rename does not return protected content after the
   identity decision.
9. Cross-platform behavior is documented: Linux verified; macOS verified or
   explicitly `NOT_RUN`; Windows explicitly `NOT_RUN` or separately verified.
10. Failure, cancellation, and registry-overflow paths deny or degrade safely
    and are audited.
11. Existing A+B and C1 regression tests pass.
12. A bounded Linux live campaign (six cases: `.env` alias, inventory alias,
    contextual alias, unlisted alias, ordinary positive control, symlink denial)
    shows the unlisted alias denied before the model input; no new campaign is
    required merely to reproduce PR #373.
13. Performance: registry build bound and per-read check bound are recorded;
    no per-read workspace traversal.
14. No new configuration option and no change to default C1 behavior.

## Policy decision required

Exact approval requested:

> Approve Phase 1 of Option C — a bounded protected-identity registry plus
> descriptor-based identity verification for `read_file`, `search_files`, and
> `copy_file` source in `internal/toolharness` — for implementation and
> verification on Linux, with macOS and Windows validation as separate
> follow-ups. The change may deny direct content-tool access to an alias whose
> opened inode matches a lexically denied credential path. It does not change
> C1, artifacts, retrieval, notifications, provider behavior, or general DLP,
> and it does not claim complete credential protection.

Status: **NOT_APPROVED.** No implementation is included in this proposal.

## Residual limitations

- Identity protection covers direct content tools. A tracked ordinary-name
  alias can still reach the model through `git_diff` or SAFE `git show`; C1
  remains the content control for that path.
- A credential created or replaced after the registry snapshot may not be
  identified until refresh.
- A credential outside the workspace, hardlinked into the workspace, is outside
  the workspace-scoped registry.
- Inode reuse can produce a conservative false positive; inode identity is not
  a content classifier.
- Registry overflow on a very large workspace degrades to lexical denial plus
  C1 and must be audited, not silently treated as full protection.
- Artifacts, retrieval/handoff, notification prose, caller inputs, provider
  retention, and general DLP remain unchanged open items.
- This proposal does not close the hardlink residual. Closure requires an
  approved implementation with deterministic and bounded live verification.

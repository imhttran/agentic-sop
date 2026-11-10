# Proposal — LLM02 subprocess hardlink protection

Status: **RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED). Option D approved; no enforcement implemented.**
Date: 2026-11-10.
Baseline: `origin/main` at `8c36d48c` (after PRs #381 and #384).
Evidence: Linux `amd64`, Go 1.27.1, bubblewrap 0.9.0.
Related: [llm02-sandbox-credential-linux-campaign.json](../reports/owasp-llm/llm02-sandbox-credential-linux-campaign.json)
(SHA-256 `1baa836ba7511d73dc79c907e6e6af01d86ca62936d634d08c1687b8ddb661f3`,
tested revision `c569cd51`), [bubblewrap-verification.md](../reports/security-shift-left/bubblewrap-verification.md),
[PROPOSAL-LLM02-HARDLINK-PATH-IDENTITY.md](PROPOSAL-LLM02-HARDLINK-PATH-IDENTITY.md),
[llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md), and
[gap-register-20261109.md](../reports/owasp-llm/gap-register-20261109.md).

## Recorded decision

- **Decision:** `APPROVE_OPTION_D_SUBPROCESS_HARDLINK`
- **Disposition:** `RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)`
- **Authority:** explicit repository-operator approval in the governance instruction.
- **Decision-memory identity:** repository `a01d2007e600ff5b4fa76eebb0eb8a8e6644efaeb68183beb66c14d0a462a2db`,
  source `operator:option-d-subprocess-hardlink-2026-11-10`.
- **Date:** 2026-11-10.
- **Evidence:** PR #385 merge `afff6b3e`; PR #384 Linux campaign artifact
  `1baa836ba7511d73dc79c907e6e6af01d86ca62936d634d08c1687b8ddb661f3`; PR #381
  implementation `4911f916`; feasibility probes in
  `internal/toolharness/isolation_hardlink_feasibility_test.go`.
- **Accepted risk:** a pre-existing ordinary-name hardlink to a protected
  credential can remain readable by a SAFE `run_command` subprocess inside the
  opt-in Bubblewrap sandbox.
- **Not accepted:** direct credential reads with isolation off, Option C-bounded,
  other LLM02 residual risks, Plan 03 acceptance, and full LLM02 completion.
- **Preserved:** opt-in isolation; A+B, C1, and Option C Phase 1 file-tool
  protections; direct credential-path and symlink denial.
- **No production enforcement** is added. The record is a repository-scoped
  decision-memory entry; it does not create Plan 03 acceptance.

## Problem

The opt-in `run_command` sandbox hides the fixed home-credential paths with
bubblewrap mounts: `--tmpfs` for directories and `--ro-bind /dev/null` for
files. That is a **path** mask. A hardlink is another directory entry for the
same inode, so an ordinary-name alias remains visible and readable even though
the credential path itself is hidden.

PR #384 verified this on Linux `amd64`:

| Case | Result |
| --- | --- |
| SAFE credential read, isolation on | denied at the process level |
| SAFE ordinary home read, isolation on | positive control |
| SAFE credential symlink, isolation on | denied |
| SAFE pre-existing hardlink, isolation on | **filesystem exposed**: raw and model-input canary, model reproduced |
| SAFE credential read, isolation off | **filesystem exposed**, model input redacted |

The read-deny mechanism works for direct paths and symlinks. It does not protect
inode aliases. A+B/C1 redaction narrows the model-visible content, but it is not
filesystem denial and does not cover unlisted opaque values.

## Threat model

- **Asset:** a home credential file (`.ssh`, `.aws`, `.kube`, `.gnupg`,
  `.config/gh`, `.config/gcloud`, `.azure`, `.netrc`, `.docker/config.json`, and
  the rest of the fixed vocabulary).
- **Alias:** a pre-existing hardlink with an ordinary name, on the same
  filesystem, visible in the sandbox.
- **Actor:** an operator, dependency installer, build script, checkout hook, or
  another process with host filesystem access. The model cannot create a
  hardlink to the hidden credential from inside the sandbox because the source
  path is hidden, but it can read an alias that already exists.
- **Execution:** a SAFE command such as `go test` runs model-written code, which
  can open the alias and print it.
- **Scope:** the sandbox uses `--ro-bind / /`, so the visible scope is the host
  filesystem except the private `/tmp` and `/run` mounts. The workspace is
  writable; the rest is read-only. An alias can be anywhere on the same
  filesystem, not only in the workspace.
- **Default:** isolation is off by default. With it off, direct reads also
  succeed, and A+B/C1 is the only model-bound control.

## Enforcement boundary

`commandpolicy` classifies `go test` as `SAFE`; `toolharness.runCommand` builds
the sandbox; `homeCredentials` resolves the fixed and environment-named
credential paths; `bubblewrapArgs` mounts the masks after the root and writable
binds; the subprocess and its descendants inherit the mounts; stdout/stderr are
captured; `redact.ModelInput` runs at the model boundary. There is no inode
check in this path. Option C Phase 1's identity registry protects the Go file
tools (`read_file`, `search_files`, `copy_file` source) in the harness process;
it does **not** protect arbitrary subprocess filesystem operations.

## Feasibility evidence

Probes are in `internal/toolharness/isolation_hardlink_feasibility_test.go`.
They are feasibility tests, not production enforcement.

| Experiment | Result |
| --- | --- |
| Alias in a host directory outside the command root | readable; a workspace-only scan misses it |
| Alias created after a preflight identity snapshot | readable; the snapshot does not know it |
| Credential replaced with a new inode | readable through the new alias; the old identity does not match |
| Mask one known alias, leave a second | first hidden, second readable |
| Ordinary multi-linked file | `Nlink=2` and both names readable; a blanket `Nlink > 1` deny would break it |
| Bounded scan cost, 5,100 files / 100 protected | identity build 8.65 ms, alias scan 18.35 ms |

The last measurement is fast, but the cost is linear in **scanned entries**, and
the sandbox's visible scope is the host filesystem, not a bounded workspace.
A complete scan would have to walk the host view; a workspace/home-only scan
cannot see an alias elsewhere. The outside-workspace probe demonstrates that
limit directly.

## Options

### Option A — preflight hardlink detection

Before execution, discover protected inode identities and all visible hardlinks
to them; deny execution or restrict the view when aliases exist.

- **Strengths:** can catch pre-existing aliases if the scan covers their
  location; no per-open kernel component.
- **Weaknesses:** discovery scope is the visible host filesystem, not the
  workspace; an unbounded walk is impractical. Bounded scans miss
  outside-scope aliases (demonstrated). A new alias or a replaced inode after
  the scan is missed (demonstrated). Denying every command when any alias exists
  is a denial-of-service lever; false positives on ordinary multi-linked files
  are likely if the rule is naive.
- **Verdict:** not reliable at reasonable cost.

### Option B — sandbox mount masking

Mask discovered aliases with the existing bubblewrap mount model.

- **Strengths:** reuses the existing mount mechanism; a masked alias is hidden
  before execution; no kernel extension.
- **Weaknesses:** masking requires enumeration, so it inherits Option A's scope
  and race problems. Masking alias one leaves alias two readable (demonstrated).
  A subprocess can create more links to an alias it can see, and those new names
  are not in the precomputed mount list. Mount ordering and file replacement
  need the same care as the existing credential masks.
- **Verdict:** not reliable alone.

### Option C — bounded hybrid

Combine a bounded identity/alias scan with mount masking or fail-closed denial
for the scanned roots (for example the workspace and the user's home), and keep
the existing lexical masks and A+B/C1.

- **Strengths:** meaningful defense-in-depth for aliases that live in the
  scanned roots; reuses `os.SameFile` and the existing mount model; bounded
  measured cost; can be tested deterministically.
- **Weaknesses:** cannot cover aliases in other host-visible directories; a
  scan before each command adds cost and still races external mutation;
  replacement/inode reuse causes false negatives or false positives; denial on
  an alias can be a DoS. It does not share mutable state with the file-tool
  registry across security boundaries; it would need a run-local identity set.
- **Verdict:** a partial, explicitly scoped mitigation, not a complete fix.

### Option D — accept the residual

Keep path masking and A+B/C1. Document that a pre-existing ordinary-name
hardlink can be read by a sandboxed subprocess, that isolation is off by
default, and that unlisted values remain exposed.

- **Strengths:** no ineffective enforcement; no new failure modes, performance
  cost, or DoS surface; honest about the path-based guarantee.
- **Weaknesses:** the demonstrated alias remains readable.
- **Verdict:** the only option that does not overstate protection.

Kernel-level inode enforcement (eBPF LSM, fanotify, a seccomp user-notification
supervisor, or a deny-by-default Landlock ruleset) is outside the current
architecture: it requires new privileges/components, and Landlock's
hierarchy-based rules do not reliably deny a hardlink in an allowed hierarchy.
It is not proposed here.

## Recommendation

**Option D is approved and recorded.** Option C-bounded is not authorized. The
residual is accepted for the documented scope; it is not marked closed.

Rationale:

- No path-based scan can enumerate hardlinks across the sandbox's visible host
  view at reasonable cost; the outside-workspace probe shows the gap directly.
- Mount masking is enumeration-bound and cannot hide aliases it did not find.
- A bounded hybrid adds cost and complexity while still not closing the
  outside-scope or post-snapshot cases; presenting it as a fix would be
  inaccurate.
- The exposure requires a pre-existing alias created outside the model tools.
  It remains a real local residual, but accepting it is more defensible than
  shipping an enforcement layer with a false sense of completeness.

The subprocess hardlink risk remains a real residual. It is accepted only for
this bounded scope and is not marked fixed or closed.

## Exact policy decision required

**Decision recorded 2026-11-10: Option D. Option C-bounded is not authorized.**
The options below are retained as the evaluation history.

1. **Accept Option D (recommended):** retain the current path masks and A+B/C1;
   accept the residual that a pre-existing ordinary-name hardlink can be read by
   an opt-in sandboxed subprocess. Record the acceptance and the unchanged
   `isolation off` default. No production code change.
2. **Authorize Option C-bounded (partial):** authorize a run-local, bounded
   identity/alias scan over the workspace and home (or another explicitly
   stated root), with mount masking or fail-closed denial before execution;
   accept the outside-scope residual, per-command cost, and possible
   denial-on-alias. Define the scanned roots and whether the command is denied
   or the alias masked.
3. **Reject both:** leave the current documentation and revisit if the threat
   model changes.

Status: **RECORDED — Option D accepted; Option C-bounded not authorized; no
production enforcement is included.**

## Implementation boundaries (not authorized; retained for a future Option C-bounded decision)

- `internal/toolharness/isolation.go` only: a run-local identity/alias scan
  before `bubblewrapArgs`; no shared mutable registry with the file-tool
  Option C registry.
- Reuse `os.SameFile` and the existing `--tmpfs`/`--ro-bind /dev/null` mounts.
- Keep the fixed credential vocabulary; do not add a second command sandbox,
  filesystem monitor, or new configuration option.
- Preserve the opt-in default and fail-closed behavior.
- Do not change A+B, C1, network isolation, or the state-dir boundary.

## Acceptance criteria for Option C-bounded (if authorized)

1. A pre-existing alias to a protected credential in the scanned roots is denied
   before the subprocess reads it.
2. An alias in a visible but unscanned directory is explicitly tested and
   recorded as a residual; the test must not be written as a pass for full
   protection.
3. An alias created after the snapshot follows an explicit, tested refresh rule.
4. A replaced credential/inode follows an explicit, tested rule.
5. Ordinary multi-linked files remain readable; no blanket `Nlink > 1` rule.
6. Symlink denial and descendant behavior remain unchanged.
7. Missing `bwrap` fails closed; cancellation and timeout remain bounded.
8. Network restrictions, A+B/C1, and the state-dir boundary are unchanged.
9. The scan cost is bounded and recorded (for example, 5,100 files /
   100 protected took 27 ms here); no per-command full-host scan.
10. A bounded live campaign covers: direct credential read denied, workspace
    alias denied, outside-workspace alias residual, ordinary positive control,
    symlink denied.

## Residual limitations

- The subprocess hardlink residual is
  `RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)`; the alias remains
  readable and is not fixed.
- Option C-bounded would leave aliases outside the scanned roots, post-snapshot
  aliases, replacement/inode-reuse cases, and any alias created between the
  scan and the mount.
- Isolation is off by default; with it off, direct credential reads are also
  possible and A+B/C1 is the only model-bound control.
- A+B/C1 still does not cover unknown-format values, artifacts,
  retrieval/handoff, notification prose, caller inputs, provider retention, or
  general DLP.
- macOS Keychain and other sandbox limitations are separate, documented items.
- This proposal does not close the subprocess hardlink risk.

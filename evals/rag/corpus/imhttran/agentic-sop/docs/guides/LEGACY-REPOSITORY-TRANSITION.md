# Legacy-repository transition (recommendations only)

No automatic retirement is performed. These are recommendations for later, after the
consolidated repository has been used and accepted.

## Current status

- `sop-controller` and `sop-decision-adapters` remain intact, with their full histories,
  local commits and uncommitted deltas preserved. They are the rollback path.
- Their source is now consolidated into `agentic-sop` (one module, one unified
  `sop` executable, legacy wrappers retained) — see
  `docs/guides/REPOSITORY-CONSOLIDATION.md`.
- **Both legacy remotes are now archived (read-only) on GitHub**, as verified on
  2026-10-26. Archival happened before the deprecation notices below, so a notice
  can only be added by unarchiving, committing the notice, and archiving again.
  No readiness gate has been verified for either repository; see
  [Readiness checklist](#readiness-checklist).

## Recommended (later, operator-approved)

1. **Usage period first.** Run the consolidated repository for a period before any
   transition; keep the legacy checkouts available for rollback.
2. **History integration (committed, 2026-10-14).** The ancestry-preserving,
   no-rewrite integration is **committed** as merge
   `a8082b6154dd88fd8ac9fd18af5df5e682a751a6`, verified with
   `merge-base --is-ancestor`, and its parents are the three source tips:
   core `54aa6a1a8f73a3b6debef0c54472351c2a817080`,
   controller `f8502a500e52420ff0b4935b8abc0ebb5b218f2b` and
   adapter `f319f60df470705db220d67875fdefbe4666e5da`. The merge did not rewrite
   ancestors; the legacy repositories below remain intact as the rollback path.
3. **Deprecation notices.** Add a notice to each legacy README pointing at the
   consolidated repository and the unified commands.
4. **Release/issue redirection.** Redirect legacy release and issue traffic to the
   consolidated repository only after the usage period.
5. **Archival.** Archive (never delete) the legacy repositories only after all of the
   above, and only with explicit approval.

## Readiness checklist

Status as of 2026-10-26. A legacy repository is **verified ready** only when every
row is confirmed with evidence. Nothing below has been verified from `agentic-sop`;
the legacy repositories are outside this repository's scope. "Unverified" means not
checked, not failed.

| Item | `sop-controller` | `sop-decision-adapters` |
| --- | --- | --- |
| Remote archived | yes (verified 2026-10-26) | yes (verified 2026-10-26) |
| C1–C7 closure documentation merged | yes; historical closure is merged | yes; historical closure is merged |
| Relevant consolidated CI | pass on current merged PRs; migration-era CI is not independently reverified | pass on current merged PRs; migration-era CI is not independently reverified |
| Source history preserved | verified by the ancestry evidence | verified by the ancestry evidence |
| README deprecation/redirect notice | missing | missing |
| Issue/release redirection | missing | missing |
| Local dependency check (nothing still builds, imports, serves or automates against the legacy repo) | operator-local confirmation missing | operator-local confirmation missing |
| Consolidated-command usage (`sop controller serve` / `sop decision`) | operator-local confirmation missing | operator-local confirmation missing |
| Rollback backup confirmed (full history plus uncommitted deltas) | operator-local confirmation missing | operator-local confirmation missing |
| Unique-work disposition (legacy-only commits or deltas resolved) | unverified | unresolved |

The merged closure and preserved ancestry satisfy only part of the gate. A repository
is not retrospectively ready merely because its remote is archived. Missing evidence
means **not verified / not ready** until deprecation and redirection are present and
the operator-local usage, dependency, unique-work and rollback checks are confirmed.

The gate criteria and the "RD-01" disposition item referenced in the status report
are not defined in this repository. Record their source here once it is known.

## Draft deprecation notices

Paste at the top of each legacy README. Requires unarchiving first (see Current
status). Not applied.

`sop-controller`:

```markdown
> **Deprecated.** This repository has been consolidated into
> [`imhttran/agentic-sop`](https://github.com/imhttran/agentic-sop). Use
> `sop controller serve` from the unified `sop` executable. File issues and find
> releases there. This repository is retained read-only as a rollback source.
```

`sop-decision-adapters`:

```markdown
> **Deprecated.** This repository has been consolidated into
> [`imhttran/agentic-sop`](https://github.com/imhttran/agentic-sop). Use
> `sop decision` from the unified `sop` executable. File issues and find releases
> there. This repository is retained read-only as a rollback source.
```

## Do not

- Do not delete or rewrite legacy history.
- Do not merge source state databases.
- Do not move a recorded active plan source.

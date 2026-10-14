# Legacy-repository transition (recommendations only)

No automatic retirement is performed. These are recommendations for later, after the
consolidated repository has been used and accepted.

## Current status

- `sop-controller` and `sop-decision-adapters` remain intact, with their full histories,
  local commits and uncommitted deltas preserved. They are the rollback path.
- Their source is now consolidated into `agentic-sop` (one module, one unified
  `sop` executable, legacy wrappers retained) — see
  `docs/guides/REPOSITORY-CONSOLIDATION.md`.

## Recommended (later, operator-approved)

1. **Usage period first.** Run the consolidated repository for a period before any
   transition; keep the legacy checkouts available for rollback.
2. **History integration (needs separate authorization).** The ancestry-preserving,
   no-rewrite integration (bundles + a non-committing `ours` merge, verified with
   `merge-base --is-ancestor`) is prepared but deliberately **not committed**. Its
   merge commit requires explicit approval and must not rewrite ancestors.
3. **Deprecation notices.** Add a notice to each legacy README pointing at the
   consolidated repository and the unified commands.
4. **Release/issue redirection.** Redirect legacy release and issue traffic to the
   consolidated repository only after the usage period.
5. **Archival.** Archive (never delete) the legacy repositories only after all of the
   above, and only with explicit approval.

## Do not

- Do not delete or rewrite legacy history.
- Do not merge source state databases.
- Do not move a recorded active plan source.

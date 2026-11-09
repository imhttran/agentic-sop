# Codex review triage and watch

Codex posts review findings on GitHub pull requests as review-thread comments. This
repository has two small, separate surfaces for them:

1. **`/codex-review-triage`** — an agent skill that fetches unresolved findings,
   verifies them, fixes in-scope issues, and records deferred ones in the backlog.
2. **`.github/workflows/codex-review-watch.yml`** — a scheduled, read-only watcher
   that keeps a GitHub issue dashboard of unresolved findings.

The skill can change the working tree; the workflow cannot. Neither one merges,
publishes, deploys, or treats a Codex finding as authority.

## The skill

Install the SOP skills normally (`./install.sh --skills zed` or
`./install.sh --skills claude`). Then, from a checkout with an open pull request:

```text
/codex-review-triage
/codex-review-triage --watch
```

The skill uses the read-only helper:

```bash
skills/codex-review-triage/findings.sh [PR] --repo OWNER/REPO
skills/codex-review-triage/findings.sh [PR] --json
skills/codex-review-triage/findings.sh [PR] --include-resolved
```

The helper prints unresolved Codex review-thread comments, top-level review bodies,
and issue comments whose author contains `codex` and whose body contains the
`codex-code-review-finding` marker. It is bounded to 100 review threads and 20
comments per thread; if a pull request exceeds that, paginate or fall back to the
REST endpoints and do not claim complete coverage.

The skill dispositions each finding as **fixed**, **deferred**, **invalid**,
**needs human**, or **already resolved**. Deferred findings are appended to
[`../plans/BACKLOG.md`](../plans/BACKLOG.md) using the repository's section format.
The skill stops at the repository's approval boundary: it does not stage, commit,
push, merge, install, or deploy on the operator's behalf.

## The scheduled dashboard

`.github/workflows/codex-review-watch.yml` runs every six hours and on manual
dispatch. It scans open pull requests with the same helper and creates or updates a
single issue titled **Codex review findings**. The issue body lists each pull
request, the finding location, the author, the comment link, and a short excerpt.

The workflow uses the repository's `GITHUB_TOKEN` with:

- `contents: read` — check out the helper.
- `pull-requests: read` — read pull requests, reviews, and review comments.
- `issues: write` — create or update the dashboard issue.

It does not comment on pull requests, resolve threads, label findings, or modify
code. If the generated body is unchanged, the workflow skips the issue update.

To run it manually, use **Actions → Codex review watch → Run workflow**. To change
the cadence, edit the `cron` expression. To disable it, disable the workflow in the
repository's Actions settings or remove the file through the normal review process.

## Limits and non-claims

- The watcher is not a merge gate and not an approval. A green dashboard means only
  that no unresolved marked finding was found in the scanned response.
- The finding marker and author filter are specific to the Codex GitHub connector;
  a renamed connector may require a small update.
- The 100-thread GraphQL bound is not a completeness guarantee for very large pull
  requests.
- The dashboard issue is a notification surface, not a durable security record.
- The skill verifies findings before fixing them; Codex can be wrong.
- Neither surface replaces the repository's human approval policy or CI.

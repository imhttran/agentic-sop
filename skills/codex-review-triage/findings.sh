#!/usr/bin/env bash
# codex-review-triage.sh — read-only Codex review finding discovery.
#
# Usage:
#   scripts/agents/codex-review-triage.sh [PR] [--repo OWNER/REPO] [--json]
#                                         [--include-resolved] [--help]
#
# Prints unresolved Codex findings for one pull request. It never edits the
# repository, posts a comment, resolves a thread, or merges anything.
#
# A finding is a review-thread comment (or top-level review/issue comment) whose
# author login contains "codex" and whose body contains the
# `codex-code-review-finding` marker written by the Codex GitHub connector.
#
# Bounds:
#   - The GraphQL query reads at most 100 review threads and 20 comments per
#     thread. If a PR exceeds that, the caller must paginate or fall back to the
#     REST endpoints; this helper does not claim complete coverage when the
#     response is truncated.
#   - It reads only; it does not fix, log, label, close, or approve anything.
set -euo pipefail

usage() {
  sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; $d'
}

pr=""
repo=""
json=0
include_resolved=0

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)
      [ $# -ge 2 ] || { echo "codex-review-triage: --repo needs a value" >&2; exit 2; }
      repo=$2
      shift 2
      ;;
    --json)
      json=1
      shift
      ;;
    --include-resolved)
      include_resolved=1
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    -*)
      echo "codex-review-triage: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      [ -z "$pr" ] || { echo "codex-review-triage: only one PR number is accepted" >&2; exit 2; }
      pr=$1
      shift
      ;;
  esac
done

if ! command -v gh >/dev/null 2>&1; then
  echo "codex-review-triage: gh is required" >&2
  exit 2
fi

# In GitHub Actions the workflow supplies GH_TOKEN; do not require an interactive
# login there. Locally, require an authenticated gh session.
if [ -z "${GH_TOKEN:-}" ] && [ -z "${GITHUB_TOKEN:-}" ]; then
  if ! gh auth status >/dev/null 2>&1; then
    echo "codex-review-triage: gh is not authenticated; run gh auth login" >&2
    exit 2
  fi
fi

if [ -z "$repo" ]; then
  repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner)
fi
owner=${repo%%/*}
name=${repo#*/}

if [ -z "$pr" ]; then
  pr=$(gh pr view --repo "$repo" --json number --jq .number)
fi

pr_url=$(gh pr view "$pr" --repo "$repo" --json url --jq .url)

query='query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100) {
        nodes {
          id
          isResolved
          isOutdated
          comments(first: 20) {
            nodes {
              databaseId
              author { login }
              body
              path
              line
              originalLine
              createdAt
              url
              commit { oid }
            }
          }
        }
      }
    }
  }
}'

resolved_condition='.isResolved == false'
if [ "$include_resolved" -eq 1 ]; then
  resolved_condition='true'
fi

threads=$(mktemp)
reviews=$(mktemp)
comments=$(mktemp)
trap 'rm -f "$threads" "$reviews" "$comments"' EXIT

thread_jq='.data.repository.pullRequest.reviewThreads.nodes[]
  | select('"$resolved_condition"')
  | . as $t
  | $t.comments.nodes[]
  | select(((.author.login // "") | ascii_downcase) | contains("codex"))
  | select((.body // "") | contains("codex-code-review-finding"))'
if [ "$json" -eq 1 ]; then
  thread_jq="$thread_jq"'
  | {source:"review_thread", id:(.databaseId|tostring), path, line,
     original_line:.originalLine, author:.author.login, body, url,
     commit:.commit.oid, outdated:$t.isOutdated, resolved:$t.isResolved}
  | @json'
else
  thread_jq="$thread_jq"'
  | "| `\(.databaseId)` | `\(.path // "-"):\(.line // .originalLine // "-")` | \(.author.login) | [comment](\(.url)) | \(if $t.isResolved then "resolved" elif $t.isOutdated then "outdated" else "current" end) |"'
fi

gh api graphql -f query="$query" -F owner="$owner" -F repo="$name" -F number="$pr" \
  --jq "$thread_jq" >"$threads"

review_jq='.[]
  | select(((.user.login // "") | ascii_downcase) | contains("codex"))
  | select((.body // "") | contains("codex-code-review-finding"))'
if [ "$json" -eq 1 ]; then
  review_jq="$review_jq"'
  | {source:"review", id:(.id|tostring), path:null, line:null,
     original_line:null, author:.user.login, body, url:.html_url,
     commit:.commit_id, outdated:false, resolved:false}
  | @json'
else
  review_jq="$review_jq"'
  | "| `\(.id)` | review body | \(.user.login) | [review](\(.html_url)) | current |"'
fi

gh api "repos/$owner/$name/pulls/$pr/reviews" --paginate --jq "$review_jq" >"$reviews"

comment_jq='.[]
  | select(((.user.login // "") | ascii_downcase) | contains("codex"))
  | select((.body // "") | contains("codex-code-review-finding"))'
if [ "$json" -eq 1 ]; then
  comment_jq="$comment_jq"'
  | {source:"issue_comment", id:(.id|tostring), path:null, line:null,
     original_line:null, author:.user.login, body, url:.html_url,
     commit:null, outdated:false, resolved:false}
  | @json'
else
  comment_jq="$comment_jq"'
  | "| `\(.id)` | issue comment | \(.user.login) | [comment](\(.html_url)) | current |"'
fi

gh api "repos/$owner/$name/issues/$pr/comments" --paginate --jq "$comment_jq" >"$comments"

if [ "$json" -eq 1 ]; then
  cat "$threads" "$reviews" "$comments"
  exit 0
fi

count=$(cat "$threads" "$reviews" "$comments" | wc -l | tr -d ' ')
printf '## Codex review findings for PR #%s\n\n%s\n\n' "$pr" "$pr_url"
if [ "$count" -eq 0 ]; then
  echo "No unresolved Codex findings."
  exit 0
fi

printf '### Review threads\n\n| ID | Location | Author | Link | State |\n| --- | --- | --- | --- | --- |\n'
cat "$threads"
printf '\n### Review bodies\n\n| ID | Location | Author | Link | State |\n| --- | --- | --- | --- | --- |\n'
cat "$reviews"
printf '\n### Issue comments\n\n| ID | Location | Author | Link | State |\n| --- | --- | --- | --- | --- |\n'
cat "$comments"

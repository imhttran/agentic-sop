#!/bin/sh
# Command-agent adapter that delegates SOP's request to Claude Code, a real
# coding agent, behind the generic command provider.
#
# SOP invokes this as:
#   export SOP_AGENT_PROVIDER=command
#   export SOP_AGENT_COMMAND="sh scripts/agents/sop-agent.sh"
#   sop run docs/plans/PLAN-Agent-Harness-V2.md
#
# It reads SOP's JSON request on stdin and writes the response on stdout. Claude
# runs non-interactively in the current directory (the repository) with file edits
# allowed and a narrow shell allow-list; it cannot commit, push, merge, reset,
# clean, or modify SOP state, and the prompt says so. SOP remains the workflow
# authority: this is an implementation adapter only, never a second engine.
#
# Environment:
#   SOP_CLAUDE_MODEL        optional --model for Claude Code
set -eu

die() { printf 'sop-agent: %s\n' "$1" >&2; exit 1; }

command -v claude >/dev/null 2>&1 || die "claude not found on PATH"
command -v jq >/dev/null 2>&1 || die "jq not found on PATH"

req=$(cat)
cap=$(printf '%s' "$req" | jq -r '.capability // empty')
task=$(printf '%s' "$req" | jq -r '.task // empty')
input=$(printf '%s' "$req" | jq -r '.input // empty')
[ -n "$cap" ] || die "request has no capability"

# A narrow, non-interactive Claude Code invocation: read + edit the repository and
# run the common Go checks, nothing else. git commit/push/merge/reset/clean are not
# in the allow-list, so they are refused rather than pre-approved.
allowed='Read,Edit,Write,Glob,Grep,LS,Bash(go build:*),Bash(go test:*),Bash(go vet:*),Bash(gofmt:*),Bash(git status:*),Bash(git diff:*)'

preamble='You are the implementation agent inside an SOP-controlled repository.
SOP owns workflow state, validation, review, retries, and human gates; it decides
whether your work passes. Work only in this repository. You may read and edit
files; you must not commit, push, merge, reset, clean, or modify .agent-sdlc.
Inspect the repository before assuming, then do the work. Do not ask questions.'

case "$cap" in
  PLAN)
    want='{"project":"<name>","summary":"<one line>","stages":[{"id":"<id>","title":"<title>","objective":"<text>","dependencies":[],"deliverables":[],"acceptance_criteria":[]}]}'
    instruction="Produce the implementation plan. Your final response must be JSON of exactly this shape: $want"
    ;;
  REVIEW)
    want='{"summary":"<one line>","findings":[{"severity":"INFO|LOW|MEDIUM|HIGH|CRITICAL","title":"<title>","detail":"<detail>","file":"<path>","line":0,"suggestion":"<optional>"}]}'
    instruction="Review the change below and report findings. Before reporting a missing symbol, undefined type, or compile error, use your tools to confirm it is actually absent from the whole package: sibling files in the same package may define it, and the test suite already passing is evidence it compiles. Report only real problems. Your final response must be JSON of exactly this shape: $want"
    ;;
  *)
    instruction='Do the requested work now by editing files. When you are finished your final response must be JSON of exactly this shape: {"status":"completed","summary":"<one line>","changes_expected":true}. If you could not complete it use {"status":"failed","reason":"<why>"}, or {"status":"needs_human","reason":"<why>"} if a human decision is required.'
    ;;
esac

prompt="$preamble

# Task
$task

# Context
$input

# Required response
$instruction
The JSON must be valid, complete, and on a single line. End your reply with exactly one line of the form (and nothing after it):
SOP_JSON: {the json}"

model_args=''
[ -n "${SOP_CLAUDE_MODEL:-}" ] && model_args="--model $SOP_CLAUDE_MODEL"

# shellcheck disable=SC2086
errlog=$(mktemp "${TMPDIR:-/tmp}/sop-agent.XXXXXX")
out=$(printf '%s' "$prompt" | claude -p \
  --output-format json \
  --allowedTools "$allowed" \
  $model_args 2>"$errlog") || true
errtail=$(tail -n 3 "$errlog" 2>/dev/null | tr '\n' ' ' || true)
rm -f "$errlog"

if [ -z "$out" ]; then
  die "claude produced no output${errtail:+: $errtail}"
fi

if [ "$(printf '%s' "$out" | jq -r '.is_error // false')" = "true" ]; then
  reason=$(printf '%s' "$out" | jq -r '.result // empty')
  [ -n "$reason" ] || reason="claude reported an error${errtail:+: $errtail}"
  case "$cap" in
    PLAN|REVIEW) die "$reason" ;;
    *) jq -cn --arg r "$reason" '{status:"failed",reason:$r}'; exit 0 ;;
  esac
fi

result=$(printf '%s' "$out" | jq -r '.result // empty')
json=$(printf '%s' "$result" | awk '{ i=index($0,"SOP_JSON:"); if(i){ last=substr($0,i+9); gsub(/^[ \t]+/,"",last) } } END{ if(last!="") print last }')

if [ -n "$json" ] && printf '%s' "$json" | jq -e . >/dev/null 2>&1; then
  printf '%s\n' "$json"
  exit 0
fi

case "$cap" in
  PLAN|REVIEW)
    die "agent did not return the required $cap JSON"
    ;;
  *)
    # A mutating capability: hand SOP the agent's own summary as prose. SOP uses
    # the repository diff as the authority and its no-change guard still applies.
    printf '%s\n' "$result"
    ;;
esac

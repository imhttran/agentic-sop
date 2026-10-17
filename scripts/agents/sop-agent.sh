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
#   SOP_CLAUDE_EFFORT       optional --effort for Claude Code (low|medium|high|...)
#   SOP_CLAUDE_MODEL_<CLASS>, SOP_CLAUDE_EFFORT_<CLASS>
#                           per-class overrides (CLASS = SMALL, MEDIUM, LARGE), used
#                           when SOP sets SOP_SELECTED_CLASS to that class
#   SOP_CLAUDE_ALLOWED_TOOLS optional extra comma-separated Claude Code tool rules
#                           added to the allow-list, e.g. Bash(make:*). Only adds;
#                           anything listed here (even git push) is the operator's
#                           choice.
#   SOP_CLAUDE_ISOLATE      set to 1 to skip the operator's own Claude Code setup:
#                           ~/.claude settings, hooks, plugins, CLAUDE.md, rules,
#                           and skills, plus every MCP server. The project's
#                           .claude settings and CLAUDE.md still load, and
#                           `claude login` still works.
#   SOP_CLAUDE_MAX_BUDGET_USD optional --max-budget-usd: Claude stops a call once its
#                           estimated spend reaches this many US dollars
#   SOP_CLAUDE_COST_LOG     optional file; each call appends one line with the time,
#                           class, capability, result, cost in USD, and turns
#
# The request's validation_commands (the project's own validation.build/test/lint
# from .agent-sdlc/config.yaml) are allowed automatically as Bash(<command>:*), so
# Claude can check its work in any language. A command containing "," or ")" is
# skipped, because Claude Code's rule syntax cannot express it.
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

# The project's validation commands are operator-owned configuration, not model
# output, so allowing them adds no model-chosen command.
validation_note=''
cmds=$(printf '%s' "$req" | jq -r '.validation_commands[]? // empty')
while IFS= read -r c; do
  [ -n "$c" ] || continue
  case "$c" in
    *,*|*\)*) printf 'sop-agent: not auto-allowing (unsupported characters): %s\n' "$c" >&2; continue ;;
  esac
  allowed="$allowed,Bash($c:*)"
  validation_note="$validation_note
- $c"
done <<CMDS
$cmds
CMDS
[ -n "${SOP_CLAUDE_ALLOWED_TOOLS:-}" ] && allowed="$allowed,$SOP_CLAUDE_ALLOWED_TOOLS"

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
    instruction='Do the requested work now by editing files. When you are finished your final response must be JSON of exactly this shape: {"status":"completed","summary":"<one line>","changes_expected":true}. If the task is already done and needs no edits, check it with the validation commands and reply {"status":"completed","summary":"<what already satisfies it>","changes_expected":false}; do not add a completion or evidence field. If you could not complete it use {"status":"failed","reason":"<why>"}, or {"status":"needs_human","reason":"<why>"} if a human decision is required.'
    ;;
esac

prompt="$preamble

# Task
$task

# Context
$input

${validation_note:+# Validation
You may run these validation commands to check your work:$validation_note

}# Required response
$instruction
The JSON must be valid, complete, and on a single line. End your reply with exactly one line of the form (and nothing after it):
SOP_JSON: {the json}"

claude_model=${SOP_CLAUDE_MODEL:-}
claude_effort=${SOP_CLAUDE_EFFORT:-}
case "${SOP_SELECTED_CLASS:-}" in
  small) claude_model=${SOP_CLAUDE_MODEL_SMALL:-$claude_model}; claude_effort=${SOP_CLAUDE_EFFORT_SMALL:-$claude_effort} ;;
  medium) claude_model=${SOP_CLAUDE_MODEL_MEDIUM:-$claude_model}; claude_effort=${SOP_CLAUDE_EFFORT_MEDIUM:-$claude_effort} ;;
  large) claude_model=${SOP_CLAUDE_MODEL_LARGE:-$claude_model}; claude_effort=${SOP_CLAUDE_EFFORT_LARGE:-$claude_effort} ;;
esac
claude_args=''
[ -n "$claude_model" ] && claude_args="--model $claude_model"
[ -n "$claude_effort" ] && claude_args="$claude_args --effort $claude_effort"
[ "${SOP_CLAUDE_ISOLATE:-}" = 1 ] && claude_args="$claude_args --setting-sources project,local --strict-mcp-config"
[ -n "${SOP_CLAUDE_MAX_BUDGET_USD:-}" ] && claude_args="$claude_args --max-budget-usd $SOP_CLAUDE_MAX_BUDGET_USD"

# shellcheck disable=SC2086
errlog=$(mktemp "${TMPDIR:-/tmp}/sop-agent.XXXXXX")
out=$(printf '%s' "$prompt" | claude -p \
  --output-format json \
  --allowedTools "$allowed" \
  $claude_args 2>"$errlog") || true
errtail=$(tail -n 3 "$errlog" 2>/dev/null | tr '\n' ' ' || true)
rm -f "$errlog"

if [ -z "$out" ]; then
  die "claude produced no output${errtail:+: $errtail}"
fi

# Cost logging never fails the call.
if [ -n "${SOP_CLAUDE_COST_LOG:-}" ]; then
  { printf '%s' "$out" | jq -r --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg class "${SOP_SELECTED_CLASS:--}" --arg cap "$cap" \
    '"\($ts) \($class) \($cap) \(.subtype // "unknown") cost=\(.total_cost_usd // "unknown") turns=\(.num_turns // "unknown")"' \
    >>"$SOP_CLAUDE_COST_LOG"; } 2>/dev/null || true
fi

# A stopped run (budget or turn cap, interruption) reports an error_* subtype
# and has no result text.
subtype=$(printf '%s' "$out" | jq -r '.subtype // empty' 2>/dev/null || true)
is_error=$(printf '%s' "$out" | jq -r '.is_error // false' 2>/dev/null || true)
case "$subtype" in error_*) is_error=true ;; esac
if [ "$is_error" = "true" ]; then
  reason=$(printf '%s' "$out" | jq -r '.result // empty')
  case "$subtype" in error_*) [ -n "$reason" ] || reason="claude stopped: $subtype" ;; esac
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

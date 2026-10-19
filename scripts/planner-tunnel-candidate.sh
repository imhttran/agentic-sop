#!/usr/bin/env bash
#
# Planner tunnel-client candidate lifecycle.
#
# This is the safe, agent-friendly half of the issue #90 workflow. It prepares a
# tunnel-client candidate from a pinned source revision and patch, records its
# provenance, and starts/stops it without touching the installed baseline. It
# deliberately does NOT rewrite the planner wrapper; use
# scripts/planner-tunnel-point.sh for that.
#
# Responsibilities are kept separate on purpose:
#   planner-tunnel-point.sh     chooses the SOP project and sop binary the
#                               tunnel wrapper serves (and backs up the wrapper).
#   planner-tunnel-candidate.sh provisions/builds a tunnel-client candidate,
#                               verifies identity, and starts/stops/rolls back.
#
# Usage:
#   scripts/planner-tunnel-candidate.sh provision [options]
#   scripts/planner-tunnel-candidate.sh preflight [options]
#   scripts/planner-tunnel-candidate.sh start [options]
#   scripts/planner-tunnel-candidate.sh stop [options]
#   scripts/planner-tunnel-candidate.sh status [options]
#   scripts/planner-tunnel-candidate.sh rollback [--start] [options]
#   scripts/planner-tunnel-candidate.sh handoff [--tests FILE] [--results FILE]
#
# All commands accept:
#   --checkout PATH       agentic-sop checkout whose .run/tunnel state to use
#                         (default: the checkout this script runs from)
#   --process-match RE    regex that identifies tunnel-client processes
#                         (default: tunnel-client). Tests use a unique value.
#   --dry-run             print the plan; write/stop/start nothing
#   -h, --help            show this help
#
# Relative paths are resolved against the checkout (matching
# planner-tunnel-point.sh). URLs and absolute paths are left alone.
#
# provision options:
#   --source PATH|URL     existing tunnel-client checkout or clone URL
#                         (default: ${SOP_TUNNEL_REPO:-https://github.com/openai/tunnel-client})
#   --revision REV        pinned source revision
#                         (default: ${SOP_TUNNEL_REVISION:-5f99daabd4aa4a77049e6d81d54a0d8c18335397})
#   --patch PATH          patch to apply; default is the issue #90 patch when present
#   --no-patch            build the pinned revision without a patch
#   --build-dir PATH      isolated source/build directory (default:
#                         .run/tunnel/build/tunnel-client-<shortrev>-<patchtag>)
#   --output PATH         candidate executable (default:
#                         .run/tunnel/candidate/tunnel-client-<shortrev>-<patchtag>)
#   --baseline PATH       installed baseline tunnel-client
#                         (default: tunnel-client on PATH)
#   --profile NAME        tunnel profile; inferred from a running baseline when possible
#   --log-file PATH       candidate debug log (default: .run/tunnel/logs/candidate-<ts>.jsonl)
#   --pid-file PATH       candidate process pid file (default: .run/tunnel/candidate/tunnel.pid)
#   --workflow-id ID      stable workflow ID recorded in the manifest and handoff
#                         (default: generated once at provision time)
#   --replace             move an existing output aside before writing it
#   --retries N           network attempts for clone/fetch (default: 2)
#
# start/stop options:
#   --startup-timeout N   seconds to wait for a confirmed start (default: 10)
#   --stop-timeout N      seconds to wait after SIGTERM (default: 10)
#   --handoff FILE        start only: require a schema-valid handoff whose
#                         next_permitted_action is START_CANDIDATE
#
# rollback options:
#   --start               after stopping the candidate and restoring the wrapper,
#                         start the recorded baseline again
#
# handoff options:
#   --tests FILE          file containing the local test commands/results
#   --results FILE        optional human-readable independent-validation notes
#   --results-json FILE   machine-readable validation results (required for PASS/FAIL);
#                         its candidate SHA-256 and source revision must match the candidate
#   --request FILE        JSON object describing the required live MCP tests (default:
#                         10 concurrent rounds of sop_status + sop_question_get)
#   --validation-status S PENDING (default), PASS, or FAIL; PASS/FAIL requires
#                         --results-json. Local tests alone are never independent validation.
#   --issue TEXT          issue reference or short title recorded in the artifact
#   --workflow-id ID      stable workflow ID (default: the provisioned manifest value)
#   --agent NAME          environment.agent recorded in the artifact
#   --capabilities LIST   comma-separated writer capabilities (default:
#                         filesystem,process_execution)
#   --local-status S      PASS, FAIL, or NOT_RUN (default); PASS/FAIL requires --tests
#   --deployment-approval S PENDING (default), APPROVED, or DENIED
#   --approval-reference TEXT  operator approval reference
#   --merge-status S      PENDING (default), APPROVED, DENIED, or NOT_REQUIRED;
#                         the helper never merges
#   --format FORMAT       markdown (default) or json; json is schema-validated before writing
#   --output FILE         write the report to FILE instead of stdout
#   --replace             replace an existing --output file
#
# Safety properties:
#   - Never edits the installed baseline binary or the active SOP working tree.
#   - Never stops a process whose exact command line differs from the recorded one.
#   - Refuses to act when more than one tunnel process matches.
#   - Uses no pkill/killall. Only a verified PID is signalled.
#   - Building/provisioning never starts or restarts a tunnel.
#   - Every state-changing command has a --dry-run form.
#
# The generated handoff is NOT a validation result. An independent MCP validator
# must report its own live results before the candidate is called verified. The
# JSON artifact is portable across coding agents and validators; it is validated
# against docs/reference/validation-handoff.schema.json before it is written.
# End of usage.

set -euo pipefail

usage() {
  sed -n '/^# Usage:/,/^# End of usage/p' "$0" | sed '/^# End of usage/d; s/^# \{0,1\}//' >&2
  exit 2
}

note() { printf 'candidate: %s\n' "$*"; }
warn() { printf 'candidate: warning: %s\n' "$*" >&2; }
die() { printf 'candidate: error: %s\n' "$*" >&2; exit 1; }

# Quote a value as a single-quoted shell literal, safe for any path.
abs_path() {
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$ROOT" "$1" ;;
  esac
}

physical_path() {
  local p="$1" d b
  if [ -e "$p" ]; then
    d="$(cd "$(dirname "$p")" && pwd -P)"
    b="$(basename "$p")"
    printf '%s/%s\n' "$d" "$b"
  else
    printf '%s\n' "$p"
  fi
}

sha256_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    die "no shasum or sha256sum on PATH"
  fi
}

count_nonempty() {
  if [ -n "${1:-}" ]; then
    printf '%s\n' "$1" | grep -c .
  else
    printf '0\n'
  fi
}

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  s="${s//$'\r'/\\r}"
  s="${s//$'\t'/\\t}"
  printf '%s' "$s"
}

# redact_command removes common credential values before an evidence artifact is
# written. It is a safety net, not a substitute for reviewing the artifact.
redact_command() {
  printf '%s' "$1" | sed -E \
    -e 's/(--(token|api-key|secret|password|client-secret|access-token|authorization|header)([= ]))[^ ]+/\1[REDACTED]/g' \
    -e 's/([A-Za-z_][A-Za-z0-9_]*(TOKEN|KEY|SECRET|PASSWORD)[A-Za-z0-9_]*=)[^ ]+/\1[REDACTED]/g'
}

# ---------------------------------------------------------------------------
# Paths and manifest
# ---------------------------------------------------------------------------

ROOT=""
TUNNEL_DIR=""
CANDIDATE_DIR=""
MANIFEST=""

resolve_root() {
  local self
  if [ -n "${CHECKOUT:-}" ]; then
    [ -d "$CHECKOUT" ] || die "--checkout directory not found: $CHECKOUT"
    ROOT="$(git -C "$CHECKOUT" rev-parse --show-toplevel 2>/dev/null)" ||
      die "--checkout is not a git checkout: $CHECKOUT"
  else
    self="$(cd "$(dirname "$0")" && pwd -P)"
    ROOT="$(git -C "$self" rev-parse --show-toplevel 2>/dev/null)" ||
      die "run this script from inside the agentic-sop checkout, or pass --checkout"
  fi
  ROOT="$(cd "$ROOT" && pwd -P)"
  TUNNEL_DIR="$ROOT/.run/tunnel"
  CANDIDATE_DIR="$TUNNEL_DIR/candidate"
  MANIFEST="$CANDIDATE_DIR/manifest"
}

mget() {
  [ -f "$MANIFEST" ] || return 1
  sed -n "s/^$1=//p" "$MANIFEST" | head -n 1
}

mget_all() {
  [ -f "$MANIFEST" ] || return 1
  sed -n "s/^$1=//p" "$MANIFEST"
}

manifest_required() {
  [ -f "$MANIFEST" ] || die "no candidate manifest at $MANIFEST; run provision first"
}

manifest_value() {
  local key="$1" required="${2:-1}" value
  value="$(mget "$key" || true)"
  if [ "$required" = 1 ] && [ -z "$value" ]; then
    die "manifest is missing required field: $key"
  fi
  printf '%s\n' "$value"
}

# write_manifest reads KEY=VALUE lines from stdin and writes atomically. An
# existing manifest is copied aside first, so a re-provision never silently
# loses the previous provenance.
write_manifest() {
  local tmp backup
  tmp="$MANIFEST.tmp.$$"
  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: would write manifest $MANIFEST"
    cat >/dev/null
    return 0
  fi
  mkdir -p "$CANDIDATE_DIR"
  if [ -f "$MANIFEST" ]; then
    backup="$MANIFEST.bak-$(date +%Y%m%d-%H%M%S)-$$"
    cp "$MANIFEST" "$backup"
    note "previous manifest saved to $backup"
  fi
  cat > "$tmp"
  mv -f "$tmp" "$MANIFEST"
}

set_manifest_value() {
  local key="$1" value="$2" tmp
  [ "$DRY_RUN" = 1 ] && return 0
  [ -f "$MANIFEST" ] || die "no manifest to update"
  tmp="$MANIFEST.tmp.$$"
  awk -v key="$key" -v value="$value" '
    BEGIN { done = 0 }
    index($0, key "=") == 1 { print key "=" value; done = 1; next }
    { print }
    END { if (!done) print key "=" value }
  ' "$MANIFEST" > "$tmp"
  mv -f "$tmp" "$MANIFEST"
}

# ---------------------------------------------------------------------------
# Process identity
# ---------------------------------------------------------------------------

process_command_of() {
  ps -p "$1" -o command= 2>/dev/null | sed 's/^[[:space:]]*//; s/[[:space:]]*$//'
}

process_alive() {
  case "${1:-}" in
    ''|*[!0-9]*) return 1 ;;
  esac
  kill -0 "$1" 2>/dev/null
}

# list_tunnel_processes prints "PID PPID COMMAND" for processes that look like a
# tunnel-client invocation. It matches an executable token followed by the `run`
# subcommand, so it also finds a candidate whose name starts with tunnel-client.
# The awk process itself is excluded by checking the command's first token.
list_tunnel_processes() {
  local re="${PROCESS_MATCH:-tunnel-client}"
  ps -axo pid=,ppid=,command= |
    awk -v re="$re" '
      $0 ~ ("(^|[[:space:]/])" re "[^[:space:]]*[[:space:]]+run([[:space:]]|$)") &&
      $3 !~ /(^|\/)(awk|grep|sed|ps)$/ {
        print $1, $2, substr($0, index($0, $3))
      }
    ' || true
}

process_count() { count_nonempty "$1"; }

# command_binary_path returns the executable/script path at the front of a ps
# command line, skipping known interpreters.
command_binary_path() {
  local cmd="$1" first second
  # Word splitting is intentional: a ps command line is a whitespace-separated
  # argv list. Quote-bearing arguments are not supported for launch capture.
  # shellcheck disable=SC2086
  set -- $cmd
  first="${1:-}"
  second="${2:-}"
  case "$(basename "$first" 2>/dev/null || printf '%s' "$first")" in
    sh|bash|dash|zsh|python|python3) printf '%s\n' "$second" ;;
    *) printf '%s\n' "$first" ;;
  esac
}

command_profile() {
  local cmd="$1" token prev=""
  for token in $cmd; do
    if [ "$prev" = "--profile" ]; then
      printf '%s\n' "$token"
      return 0
    fi
    prev="$token"
  done
  return 1
}

# launch_args_from_command captures everything after the executable (and, for a
# script, after the interpreter and script path). The result must begin with the
# `run` subcommand.
LAUNCH_ARGS=()
launch_args_from_command() {
  local cmd="$1" token found_bin=0
  LAUNCH_ARGS=()
  for token in $cmd; do
    if [ "$found_bin" = 0 ]; then
      case "$(basename "$token")" in
        sh|bash|dash|zsh|python|python3) continue ;;
      esac
      found_bin=1
      continue
    fi
    LAUNCH_ARGS+=("$token")
  done
  [ "${#LAUNCH_ARGS[@]}" -gt 0 ] && [ "${LAUNCH_ARGS[0]}" = "run" ]
}

# build_candidate_args copies the baseline launch arguments, substituting the
# candidate's own log and pid files. Raw HTTP capture is refused.
CANDIDATE_ARGS=()
build_candidate_args() {
  local i=0 arg next have_log=0 have_pid=0
  CANDIDATE_ARGS=()
  while [ "$i" -lt "${#BASELINE_ARGS[@]}" ]; do
    arg="${BASELINE_ARGS[$i]}"
    case "$arg" in
      --profile)
        next="${BASELINE_ARGS[$((i+1))]:-}"
        [ -n "$next" ] || die "baseline launch arguments end with --profile"
        CANDIDATE_ARGS+=("$arg" "$next")
        i=$((i+2))
        continue
        ;;
      --log.file)
        CANDIDATE_ARGS+=("$arg" "$CANDIDATE_LOG")
        have_log=1
        i=$((i+2))
        continue
        ;;
      --log.file=*)
        CANDIDATE_ARGS+=("--log.file=$CANDIDATE_LOG")
        have_log=1
        i=$((i+1))
        continue
        ;;
      --pid.file)
        CANDIDATE_ARGS+=("$arg" "$CANDIDATE_TUNNEL_PID")
        have_pid=1
        i=$((i+2))
        continue
        ;;
      --pid.file=*)
        CANDIDATE_ARGS+=("--pid.file=$CANDIDATE_TUNNEL_PID")
        have_pid=1
        i=$((i+1))
        continue
        ;;
      --log.http-raw-unsafe)
        next="${BASELINE_ARGS[$((i+1))]:-}"
        [ "$next" = "false" ] || die "baseline enables raw HTTP logging; refusing to copy it"
        CANDIDATE_ARGS+=("$arg" "$next")
        i=$((i+2))
        continue
        ;;
      --log.http-raw-unsafe=*)
        [ "${arg#*=}" = "false" ] || die "baseline enables raw HTTP logging; refusing to copy it"
        CANDIDATE_ARGS+=("$arg")
        i=$((i+1))
        continue
        ;;
    esac
    CANDIDATE_ARGS+=("$arg")
    i=$((i+1))
  done
  [ "$have_log" = 1 ] || CANDIDATE_ARGS+=("--log.file" "$CANDIDATE_LOG")
  [ "$have_pid" = 1 ] || CANDIDATE_ARGS+=("--pid.file" "$CANDIDATE_TUNNEL_PID")
}

# ---------------------------------------------------------------------------
# Shared checks
# ---------------------------------------------------------------------------

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

require_base_tools() {
  require_command git
  require_command ps
  require_command awk
  require_command sed
  require_command grep
  require_command mktemp
  require_command date
  if ! command -v shasum >/dev/null 2>&1 && ! command -v sha256sum >/dev/null 2>&1; then
    die "required command not found: shasum or sha256sum"
  fi
}

verify_binary_identity() {
  local label="$1" path="$2" want_sha="$3" want_version="$4" got_sha got_version
  [ -n "$path" ] || die "$label path is empty"
  [ -f "$path" ] || die "$label binary is missing: $path"
  [ -x "$path" ] || die "$label binary is not executable: $path"
  got_sha="$(sha256_file "$path")"
  [ "$got_sha" = "$want_sha" ] ||
    die "$label checksum mismatch: want $want_sha, got $got_sha ($path)"
  got_version="$(version_of "$path" || true)"
  [ -n "$got_version" ] || die "$label binary does not answer --version: $path"
  [ "$got_version" = "$want_version" ] ||
    die "$label version mismatch: want '$want_version', got '$got_version' ($path)"
}

version_of() {
  local bin="$1" out
  [ -x "$bin" ] || return 1
  out="$("$bin" --version 2>&1)" || return 1
  printf '%s\n' "$out" | head -n 1
}

print_processes() {
  local procs="${1:-}"
  if [ -n "$procs" ]; then
    printf '%s\n' "$procs" >&2
  else
    printf '  (none)\n' >&2
  fi
}

# ---------------------------------------------------------------------------
# provision
# ---------------------------------------------------------------------------

PROVISION_SOURCE=""
REVISION="${SOP_TUNNEL_REVISION:-5f99daabd4aa4a77049e6d81d54a0d8c18335397}"
REPO_URL="${SOP_TUNNEL_REPO:-https://github.com/openai/tunnel-client}"
PATCH=""
NO_PATCH=0
BUILD_DIR=""
OUTPUT=""
BASELINE=""
PROFILE=""
CANDIDATE_LOG=""
CANDIDATE_TUNNEL_PID=""
REPLACE=0
RETRIES=2
WORKFLOW_ID=""

parse_provision_options() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --source) [ $# -ge 2 ] || usage; PROVISION_SOURCE="$2"; shift ;;
      --revision) [ $# -ge 2 ] || usage; REVISION="$2"; shift ;;
      --patch) [ $# -ge 2 ] || usage; PATCH="$2"; shift ;;
      --no-patch) NO_PATCH=1 ;;
      --build-dir) [ $# -ge 2 ] || usage; BUILD_DIR="$2"; shift ;;
      --output) [ $# -ge 2 ] || usage; OUTPUT="$2"; shift ;;
      --baseline) [ $# -ge 2 ] || usage; BASELINE="$2"; shift ;;
      --profile) [ $# -ge 2 ] || usage; PROFILE="$2"; shift ;;
      --log-file) [ $# -ge 2 ] || usage; CANDIDATE_LOG="$2"; shift ;;
      --pid-file) [ $# -ge 2 ] || usage; CANDIDATE_TUNNEL_PID="$2"; shift ;;
      --workflow-id) [ $# -ge 2 ] || usage; WORKFLOW_ID="$2"; shift ;;
      --replace) REPLACE=1 ;;
      --retries) [ $# -ge 2 ] || usage; RETRIES="$2"; shift ;;
      --dry-run) DRY_RUN=1 ;;
      --process-match) [ $# -ge 2 ] || usage; PROCESS_MATCH="$2"; shift ;;
      --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
      -h|--help) usage ;;
      *) die "unknown provision option: $1" ;;
    esac
    shift
  done
}

clone_source() {
  local dest="$1" attempt=1
  while :; do
    rm -rf "$dest"
    if [ -d "$PROVISION_SOURCE" ]; then
      if git clone --no-checkout "$PROVISION_SOURCE" "$dest"; then
        return 0
      fi
    else
      if git clone --filter=blob:none --no-checkout "$REPO_URL" "$dest"; then
        return 0
      fi
      # Some servers/proxies do not support partial clone; retry without it.
      rm -rf "$dest"
      if git clone --no-checkout "$REPO_URL" "$dest"; then
        return 0
      fi
    fi
    if [ "$attempt" -ge "$RETRIES" ]; then
      return 1
    fi
    warn "clone attempt $attempt failed; retrying in $((attempt * 2))s"
    sleep $((attempt * 2))
    attempt=$((attempt + 1))
  done
}

fetch_revision() {
  local dir="$1" attempt=1
  if git -C "$dir" cat-file -e "$REVISION^{commit}" 2>/dev/null; then
    return 0
  fi
  while :; do
    if git -C "$dir" fetch --depth 1 origin "$REVISION"; then
      return 0
    fi
    # A commit that is not a ref may need the tags; try once before retrying.
    if git -C "$dir" fetch --tags --force origin &&
      git -C "$dir" cat-file -e "$REVISION^{commit}" 2>/dev/null; then
      return 0
    fi
    if [ "$attempt" -ge "$RETRIES" ]; then
      return 1
    fi
    warn "fetch attempt $attempt failed; retrying in $((attempt * 2))s"
    sleep $((attempt * 2))
    attempt=$((attempt + 1))
  done
}

ensure_source_checkout() {
  local tmp head
  if [ -n "$PROVISION_SOURCE" ] && [ ! -d "$PROVISION_SOURCE" ]; then
    REPO_URL="$PROVISION_SOURCE"
  fi

  if [ -e "$BUILD_DIR" ]; then
    [ -d "$BUILD_DIR/.git" ] || die "build directory exists but is not a git checkout: $BUILD_DIR"
    if [ -n "$(git -C "$BUILD_DIR" status --porcelain)" ]; then
      die "build directory has local changes; refusing to overwrite: $BUILD_DIR"
    fi
    head="$(git -C "$BUILD_DIR" rev-parse HEAD 2>/dev/null || true)"
    [ -n "$head" ] || die "build directory has no HEAD: $BUILD_DIR"
    if [ "$head" = "$REVISION" ] || [ "${head#"$REVISION"}" != "$head" ]; then
      note "reusing source checkout at the pinned revision: $BUILD_DIR"
      SOURCE_REVISION="$REVISION"
      return 0
    fi
    if [ "$NO_PATCH" = 0 ] && [ -n "$PATCH" ] && [ -f "$PATCH" ] &&
      git -C "$BUILD_DIR" apply --reverse --check "$PATCH" 2>/dev/null &&
      git -C "$BUILD_DIR" merge-base --is-ancestor "$REVISION" "$head" 2>/dev/null; then
      note "reusing patched source checkout at $head: $BUILD_DIR"
      SOURCE_REVISION="$REVISION"
      return 0
    fi
    die "build directory is at $head, which is neither the pinned revision nor the expected patched candidate; choose a new --build-dir or remove it explicitly"
  fi

  if [ "$DRY_RUN" = 1 ]; then
    if [ -d "$PROVISION_SOURCE" ]; then
      note "dry-run: would clone $PROVISION_SOURCE into $BUILD_DIR and check out $REVISION"
    else
      note "dry-run: would clone ${PROVISION_SOURCE:-$REPO_URL} into $BUILD_DIR and check out $REVISION"
    fi
    SOURCE_REVISION="$REVISION"
    return 0
  fi

  mkdir -p "$(dirname "$BUILD_DIR")"
  tmp="$(mktemp -d "$(dirname "$BUILD_DIR")/.tmp.XXXXXX")"
  if ! clone_source "$tmp/repo"; then
    rm -rf "$tmp"
    die "could not clone ${PROVISION_SOURCE:-$REPO_URL}; check network/credentials and retry"
  fi
  if ! fetch_revision "$tmp/repo"; then
    rm -rf "$tmp"
    die "could not fetch revision $REVISION from ${PROVISION_SOURCE:-$REPO_URL}"
  fi
  if ! git -C "$tmp/repo" checkout --detach "$REVISION"; then
    rm -rf "$tmp"
    die "could not check out revision $REVISION"
  fi
  SOURCE_REVISION="$(git -C "$tmp/repo" rev-parse HEAD)"
  [ "$SOURCE_REVISION" = "$REVISION" ] ||
    die "checked out $SOURCE_REVISION, expected $REVISION"
  mv "$tmp/repo" "$BUILD_DIR"
  rmdir "$tmp"
  note "source checkout ready: $BUILD_DIR @ $SOURCE_REVISION"
}

patch_subject() {
  sed -n 's/^Subject: \(.*\)$/\1/p' "$1" | head -n 1 | sed 's/^\[PATCH[^]]*\] *//'
}

ensure_patch_applied() {
  local subject head_subject reverse_ok=0
  PATCH_SHA=""
  PATCH_APPLIED="no"
  CANDIDATE_REVISION="$SOURCE_REVISION"
  [ "$NO_PATCH" = 1 ] && return 0
  [ -n "$PATCH" ] || die "no patch given; pass --patch or --no-patch"
  [ -f "$PATCH" ] || die "patch file not found: $PATCH"
  PATCH_SHA="$(sha256_file "$PATCH")"

  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: would apply patch $PATCH ($PATCH_SHA) to $BUILD_DIR"
    return 0
  fi

  if git -C "$BUILD_DIR" apply --check "$PATCH" 2>/dev/null; then
    if [ -d "$BUILD_DIR/.git/rebase-apply" ]; then
      die "source checkout has an interrupted git am; run: git -C $BUILD_DIR am --abort"
    fi
    if ! git -C "$BUILD_DIR" -c user.name="SOP candidate" -c user.email="sop-candidate@localhost" am "$PATCH"; then
      git -C "$BUILD_DIR" am --abort >/dev/null 2>&1 || true
      die "patch did not apply cleanly; the source checkout was restored. Inspect $PATCH and retry"
    fi
    PATCH_APPLIED="yes"
    CANDIDATE_REVISION="$(git -C "$BUILD_DIR" rev-parse HEAD)"
    [ "$CANDIDATE_REVISION" != "$SOURCE_REVISION" ] ||
      die "patch produced no new commit; refusing to build an ambiguous candidate"
  else
    if git -C "$BUILD_DIR" apply --reverse --check "$PATCH" 2>/dev/null; then
      reverse_ok=1
      subject="$(patch_subject "$PATCH")"
      head_subject="$(git -C "$BUILD_DIR" log -1 --format=%s)"
      if [ -n "$subject" ] && printf '%s' "$head_subject" | grep -Fq "$subject"; then
        note "patch is already applied at HEAD: $head_subject"
        PATCH_APPLIED="already"
        CANDIDATE_REVISION="$(git -C "$BUILD_DIR" rev-parse HEAD)"
      fi
    fi
    if [ "$reverse_ok" = 1 ] && [ "$PATCH_APPLIED" = "already" ]; then
      :
    else
      die "patch does not apply to $BUILD_DIR at $SOURCE_REVISION; refusing to guess"
    fi
  fi
  [ -z "$(git -C "$BUILD_DIR" status --porcelain)" ] ||
    die "source checkout is dirty after applying the patch; refusing to build"
}

build_candidate() {
  local short_sha tmp_out build_log
  if [ "$DRY_RUN" = 1 ] && [ ! -d "$BUILD_DIR/.git" ]; then
    short_sha="${REVISION:0:12}"
    note "dry-run: the source checkout does not exist yet; the real build will use the patched HEAD"
  else
    short_sha="$(git -C "$BUILD_DIR" rev-parse --short HEAD)"
  fi
  CANDIDATE_SHA=""
  CANDIDATE_VERSION=""
  BUILD_COMMAND="go build -trimpath -buildvcs=false -ldflags \"-X github.com/openai/tunnel-client/pkg/version.GitSHA=$short_sha\" -o $OUTPUT ./cmd/client"
  GO_VERSION="$(go version 2>/dev/null | awk '{print $3}' || true)"

  if [ -f "$OUTPUT" ]; then
    CANDIDATE_SHA="$(sha256_file "$OUTPUT")"
    CANDIDATE_VERSION="$(version_of "$OUTPUT" || true)"
    local prev_sha prev_rev prev_patch
    prev_sha="$(mget candidate_sha256 || true)"
    prev_rev="$(mget candidate_revision || true)"
    prev_patch="$(mget patch_sha256 || true)"
    if [ -n "$prev_sha" ] && [ "$prev_sha" = "$CANDIDATE_SHA" ] &&
      [ "$prev_rev" = "$CANDIDATE_REVISION" ] && [ "$prev_patch" = "$PATCH_SHA" ] &&
      [ -n "$CANDIDATE_VERSION" ] && printf '%s' "$CANDIDATE_VERSION" | grep -Fq "$short_sha"; then
      note "reusing existing candidate at $OUTPUT (manifest identity matches)"
      return 0
    fi
    if [ "$REPLACE" != 1 ]; then
      die "output already exists and does not match the recorded provenance: $OUTPUT (pass --replace)"
    fi
    [ "$DRY_RUN" = 1 ] || mv "$OUTPUT" "$OUTPUT.bak-$(date +%Y%m%d-%H%M%S)-$$"
    note "existing output moved aside"
  fi

  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: would run: (cd $BUILD_DIR && $BUILD_COMMAND)"
    return 0
  fi

  require_command go
  mkdir -p "$(dirname "$OUTPUT")"
  tmp_out="$OUTPUT.tmp.$$"
  build_log="$CANDIDATE_DIR/build.log"
  mkdir -p "$CANDIDATE_DIR"
  : > "$build_log"
  if ! (cd "$BUILD_DIR" && CGO_ENABLED="${CGO_ENABLED:-0}" go build -trimpath -buildvcs=false \
      -ldflags "-X github.com/openai/tunnel-client/pkg/version.GitSHA=$short_sha" \
      -o "$tmp_out" ./cmd/client) >>"$build_log" 2>&1; then
    rm -f "$tmp_out"
    printf '%s\n' "candidate: build failed; last 40 log lines:" >&2
    tail -n 40 "$build_log" >&2 || true
    die "build failed (full log: $build_log). Check the Go module cache/network and retry"
  fi
  mv -f "$tmp_out" "$OUTPUT"
  chmod +x "$OUTPUT" 2>/dev/null || true
  CANDIDATE_SHA="$(sha256_file "$OUTPUT")"
  CANDIDATE_VERSION="$(version_of "$OUTPUT" || true)"
  [ -n "$CANDIDATE_VERSION" ] || die "built candidate does not answer --version: $OUTPUT"
  printf '%s' "$CANDIDATE_VERSION" | grep -Fq "$short_sha" ||
    die "built candidate version does not contain source revision $short_sha: $CANDIDATE_VERSION"
}

resolve_baseline() {
  local procs count first_pid first_cmd proc_binary proc_profile resolved_baseline
  procs="$(list_tunnel_processes)"
  count="$(process_count "$procs")"

  if [ "$count" -gt 1 ]; then
    printf '%s\n' "$procs" >&2
    die "multiple tunnel processes are running; refusing to guess which is the baseline"
  fi

  # The baseline is always the installed binary, never whatever happens to be
  # running. A running candidate must not be relabelled as the baseline.
  resolved_baseline="$BASELINE"
  if [ -z "$resolved_baseline" ]; then
    resolved_baseline="$(command -v tunnel-client 2>/dev/null || true)"
  fi

  BASELINE_PROCESS_PID=""
  BASELINE_PROCESS_COMMAND=""
  BASELINE_ARGS=()

  if [ "$count" -eq 1 ]; then
    first_pid="$(printf '%s\n' "$procs" | awk '{print $1}')"
    first_cmd="$(printf '%s\n' "$procs" | sed -E 's/^[0-9]+ [0-9]+ //')"
    proc_binary="$(command_binary_path "$first_cmd")"
    proc_profile="$(command_profile "$first_cmd" || true)"
    if [ -n "$resolved_baseline" ] &&
      [ "$(physical_path "$proc_binary")" = "$(physical_path "$resolved_baseline")" ]; then
      if [ -n "$PROFILE" ] && [ -n "$proc_profile" ] && [ "$PROFILE" != "$proc_profile" ]; then
        die "the running baseline uses profile '$proc_profile', not --profile '$PROFILE'"
      fi
      PROFILE="${PROFILE:-$proc_profile}"
      launch_args_from_command "$first_cmd" ||
        die "could not read the running baseline's launch arguments; pass --profile and stop it first"
      BASELINE_ARGS=("${LAUNCH_ARGS[@]}")
      BASELINE_PROCESS_PID="$first_pid"
      BASELINE_PROCESS_COMMAND="$first_cmd"
      note "captured running baseline: PID $first_pid"
    else
      die "a non-baseline tunnel process is running: PID $first_pid ($proc_binary). Stop it before provisioning; the baseline is ${resolved_baseline:-not found}"
    fi
  else
    [ -n "$resolved_baseline" ] || die "no baseline found; pass --baseline PATH"
    [ -n "$PROFILE" ] ||
      die "no running baseline to infer the profile; pass --profile NAME"
    BASELINE_ARGS=("run" "--profile" "$PROFILE")
    note "no running tunnel; using $resolved_baseline with --profile $PROFILE"
  fi

  [ -n "$PROFILE" ] || die "could not determine the tunnel profile; pass --profile"
  [ -n "$resolved_baseline" ] || die "no baseline binary; pass --baseline PATH"
  [ -x "$resolved_baseline" ] || die "baseline binary is missing or not executable: $resolved_baseline"
  BASELINE="$(physical_path "$resolved_baseline")"
  BASELINE_SHA="$(sha256_file "$BASELINE")"
  BASELINE_VERSION="$(version_of "$BASELINE" || true)"
  [ -n "$BASELINE_VERSION" ] || die "baseline does not answer --version: $BASELINE"
}

capture_wrapper() {
  WRAPPER="$TUNNEL_DIR/sop-mcp-planner.sh"
  WRAPPER_SHA=""
  WRAPPER_BACKUP=""
  if [ -f "$WRAPPER" ]; then
    WRAPPER_SHA="$(sha256_file "$WRAPPER")"
    local previous_sha previous_backup
    previous_sha="$(mget wrapper_sha256 || true)"
    previous_backup="$(mget wrapper_backup || true)"
    if [ -n "$previous_sha" ] && [ "$previous_sha" = "$WRAPPER_SHA" ] &&
      [ -n "$previous_backup" ] && [ -f "$previous_backup" ]; then
      WRAPPER_BACKUP="$previous_backup"
      note "reusing wrapper backup: $WRAPPER_BACKUP"
    else
      WRAPPER_BACKUP="$WRAPPER.bak-candidate-$(date +%Y%m%d-%H%M%S)-$$"
      if [ "$DRY_RUN" = 1 ]; then
        note "dry-run: would back up wrapper to $WRAPPER_BACKUP"
      else
        cp "$WRAPPER" "$WRAPPER_BACKUP"
        note "wrapper backup: $WRAPPER_BACKUP"
      fi
    fi
  else
    # Nothing to preserve or restore; do not record a path that preflight
    # would later report as missing.
    WRAPPER=""
  fi
}

provision_manifest() {
  {
    printf 'format=1\n'
    printf 'state=prepared\n'
    printf 'workflow_id=%s\n' "$WORKFLOW_ID"
    printf 'created_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf 'sop_branch=%s\n' "$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
    printf 'sop_commit=%s\n' "$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)"
    printf 'source_url=%s\n' "${PROVISION_SOURCE:-$REPO_URL}"
    printf 'source_revision=%s\n' "$SOURCE_REVISION"
    printf 'candidate_revision=%s\n' "$CANDIDATE_REVISION"
    printf 'candidate_source_dir=%s\n' "$BUILD_DIR"
    printf 'patch=%s\n' "$PATCH"
    printf 'patch_sha256=%s\n' "$PATCH_SHA"
    printf 'patch_applied=%s\n' "$PATCH_APPLIED"
    printf 'candidate_path=%s\n' "$OUTPUT"
    printf 'candidate_sha256=%s\n' "$CANDIDATE_SHA"
    printf 'candidate_version=%s\n' "$CANDIDATE_VERSION"
    printf 'build_command=%s\n' "$BUILD_COMMAND"
    printf 'go_version=%s\n' "$GO_VERSION"
    printf 'baseline_path=%s\n' "$BASELINE"
    printf 'baseline_sha256=%s\n' "$BASELINE_SHA"
    printf 'baseline_version=%s\n' "$BASELINE_VERSION"
    printf 'baseline_process_pid=%s\n' "$BASELINE_PROCESS_PID"
    printf 'baseline_process_command=%s\n' "$BASELINE_PROCESS_COMMAND"
    printf 'profile=%s\n' "$PROFILE"
    printf 'candidate_log=%s\n' "$CANDIDATE_LOG"
    printf 'candidate_tunnel_pid=%s\n' "$CANDIDATE_TUNNEL_PID"
    printf 'wrapper_path=%s\n' "$WRAPPER"
    printf 'wrapper_sha256=%s\n' "$WRAPPER_SHA"
    printf 'wrapper_backup=%s\n' "$WRAPPER_BACKUP"
    local arg
    for arg in "${BASELINE_ARGS[@]}"; do
      printf 'baseline_launch_arg=%s\n' "$arg"
    done
    for arg in "${CANDIDATE_ARGS[@]}"; do
      printf 'candidate_launch_arg=%s\n' "$arg"
    done
  } | write_manifest
}

cmd_provision() {
  parse_provision_options "$@"
  require_base_tools
  resolve_root

  [ -n "$REVISION" ] || die "--revision is empty"
  if [ -z "$PATCH" ] && [ "$NO_PATCH" != 1 ]; then
    PATCH="$ROOT/docs/reports/planner/issue-90-tunnel-client-patch/0001-mcpclient-keep-stdio-readiness-across-re-initialize-.patch"
    [ -f "$PATCH" ] || PATCH=""
  fi
  patch_tag="nopatch"
  if [ "$NO_PATCH" != 1 ] && [ -n "$PATCH" ] && [ -f "$PATCH" ]; then
    patch_tag="$(sha256_file "$PATCH" | cut -c1-12)"
  fi
  if [ -z "$BUILD_DIR" ]; then
    BUILD_DIR="$TUNNEL_DIR/build/tunnel-client-${REVISION:0:12}-${patch_tag}"
  fi
  if [ -z "$OUTPUT" ]; then
    OUTPUT="$CANDIDATE_DIR/tunnel-client-${REVISION:0:12}-${patch_tag}"
  fi
  if [ -z "$CANDIDATE_LOG" ]; then
    CANDIDATE_LOG="$TUNNEL_DIR/logs/candidate-$(date +%Y%m%d-%H%M%S).jsonl"
  fi
  if [ -z "$CANDIDATE_TUNNEL_PID" ]; then
    CANDIDATE_TUNNEL_PID="$CANDIDATE_DIR/tunnel.pid"
  fi
  if [ -z "$WORKFLOW_ID" ]; then
    WORKFLOW_ID="mcp-live-$(date -u +%Y%m%dT%H%M%SZ)-$$"
  fi
  # Relative paths are resolved against the checkout, matching
  # planner-tunnel-point.sh. URLs and absolute paths are left alone.
  BUILD_DIR="$(abs_path "$BUILD_DIR")"
  OUTPUT="$(abs_path "$OUTPUT")"
  CANDIDATE_LOG="$(abs_path "$CANDIDATE_LOG")"
  CANDIDATE_TUNNEL_PID="$(abs_path "$CANDIDATE_TUNNEL_PID")"
  [ -z "$PATCH" ] || PATCH="$(abs_path "$PATCH")"
  [ -z "$BASELINE" ] || BASELINE="$(abs_path "$BASELINE")"
  if [ -n "$PROVISION_SOURCE" ] && [ -d "$PROVISION_SOURCE" ]; then
    PROVISION_SOURCE="$(cd "$PROVISION_SOURCE" && pwd -P)"
  fi

  note "checkout = $ROOT"
  note "revision = $REVISION"
  note "candidate output = $OUTPUT"
  note "source/build dir = $BUILD_DIR"
  [ "$DRY_RUN" = 1 ] && note "dry-run: no files will be written, no process will be started or stopped"

  ensure_source_checkout
  ensure_patch_applied
  resolve_baseline
  if [ "$(physical_path "$OUTPUT")" = "$(physical_path "$BASELINE")" ]; then
    die "refusing to write the candidate over the baseline binary: $OUTPUT"
  fi
  build_candidate
  capture_wrapper

  # Candidate-specific log and pid paths were resolved above; build the args.
  build_candidate_args

  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: candidate would be: $OUTPUT"
    if [ -n "$CANDIDATE_VERSION" ]; then
      note "dry-run: version $CANDIDATE_VERSION sha256 $CANDIDATE_SHA"
    else
      note "dry-run: candidate is not built yet"
    fi
    note "dry-run: baseline would be: $BASELINE ($BASELINE_VERSION)"
    note "dry-run: profile $PROFILE"
    note "dry-run: launch: $OUTPUT ${CANDIDATE_ARGS[*]}"
    note "dry-run: log $CANDIDATE_LOG"
    note "dry-run: no manifest written, no tunnel started"
    return 0
  fi

  provision_manifest
  note "prepared (NOT started)"
  note "  candidate: $OUTPUT"
  note "  version:   $CANDIDATE_VERSION"
  note "  sha256:    $CANDIDATE_SHA"
  note "  source:    $(mget source_url || true) @ $CANDIDATE_REVISION"
  note "  baseline:  $BASELINE ($BASELINE_VERSION)"
  note "  profile:   $PROFILE"
  note "  log:       $CANDIDATE_LOG"
  note "  next:      $0 preflight --checkout $ROOT"
  note "  start:     $0 start --checkout $ROOT"
  note "  rollback:  $0 rollback --checkout $ROOT --start"
}

# ---------------------------------------------------------------------------
# start
# ---------------------------------------------------------------------------

STARTUP_TIMEOUT=10
STOP_TIMEOUT=10
LIFECYCLE_HANDOFF=""

parse_lifecycle_options() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --startup-timeout) [ $# -ge 2 ] || usage; STARTUP_TIMEOUT="$2"; shift ;;
      --stop-timeout) [ $# -ge 2 ] || usage; STOP_TIMEOUT="$2"; shift ;;
      --handoff) [ $# -ge 2 ] || usage; LIFECYCLE_HANDOFF="$2"; shift ;;
      --dry-run) DRY_RUN=1 ;;
      --process-match) [ $# -ge 2 ] || usage; PROCESS_MATCH="$2"; shift ;;
      --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
      -h|--help) usage ;;
      *) die "unknown option: $1" ;;
    esac
    shift
  done
}

# Read a `--flag value` pair from BASELINE_ARGS.
baseline_arg_value() {
  local needle="$1" i=0
  while [ "$i" -lt "${#BASELINE_ARGS[@]}" ]; do
    if [ "${BASELINE_ARGS[$i]}" = "$needle" ]; then
      printf '%s\n' "${BASELINE_ARGS[$((i+1))]:-}"
      return 0
    fi
    i=$((i + 1))
  done
  return 1
}

wait_for_startup() {
  local pid="$1" log="$2" pidfile="$3" candidate="$4" start_size="${5:-0}" waited=0 ppid pcmd size
  while [ "$waited" -lt "$STARTUP_TIMEOUT" ]; do
    # A daemonizing candidate may exit its launcher before the tunnel is ready.
    if [ -f "$pidfile" ]; then
      ppid="$(head -n 1 "$pidfile" 2>/dev/null | tr -dc '0-9')"
      if process_alive "$ppid"; then
        pcmd="$(process_command_of "$ppid")"
        case "$pcmd" in
          *"$candidate"*) printf '%s\n' "$ppid"; return 0 ;;
        esac
      fi
    fi
    if ! process_alive "$pid"; then
      return 1
    fi
    if [ -f "$log" ]; then
      size="$(wc -c < "$log" 2>/dev/null | tr -d ' ')"
      if [ -n "$size" ] && [ "$size" -gt "$start_size" ] &&
        tail -c +$((start_size + 1)) "$log" | grep -Eiq 'started|starting|listening|pid file written|poll cycle started'; then
        printf '%s\n' "$pid"
        return 0
      fi
    fi
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

# Stop a just-launched process after a failed startup. Only a PID whose command
# still contains expected_path is signalled; nothing else is touched.
stop_started_process() {
  local started_pid="$1" started_cmd="$2" pidfile="$3" expected_path="$4" p pc
  if [ -f "$pidfile" ]; then
    p="$(head -n 1 "$pidfile" 2>/dev/null | tr -dc '0-9')"
    if process_alive "$p"; then
      pc="$(process_command_of "$p")"
      case "$pc" in
        *"$expected_path"*) terminate_pid "$p" "$pc" "$STOP_TIMEOUT" || true ;;
      esac
    fi
  fi
  terminate_pid "$started_pid" "$started_cmd" "$STOP_TIMEOUT" || true
}

terminate_pid() {
  local pid="$1" expected="$2" timeout="$3" actual waited=0
  [ -n "$pid" ] || return 0
  process_alive "$pid" || return 0
  actual="$(process_command_of "$pid")"
  if [ -n "$expected" ] && [ "$actual" != "$expected" ]; then
    die "refusing to signal PID $pid: command changed (expected '$expected', got '$actual')"
  fi
  kill -TERM "$pid" 2>/dev/null || true
  while process_alive "$pid" && [ "$waited" -lt "$timeout" ]; do
    sleep 1
    waited=$((waited + 1))
  done
  if process_alive "$pid"; then
    actual="$(process_command_of "$pid")"
    [ "$actual" = "$expected" ] || die "PID $pid changed identity; refusing to force-kill"
    warn "PID $pid did not exit after ${timeout}s; sending SIGKILL"
    kill -KILL "$pid" 2>/dev/null || true
    sleep 1
  fi
}

verify_manifest_files() {
  verify_binary_identity "candidate" "$(manifest_value candidate_path)" \
    "$(manifest_value candidate_sha256)" "$(manifest_value candidate_version)"
  verify_binary_identity "baseline" "$(manifest_value baseline_path)" \
    "$(manifest_value baseline_sha256)" "$(manifest_value baseline_version)"
  local wrapper wrapper_sha
  wrapper="$(manifest_value wrapper_path 0)"
  wrapper_sha="$(manifest_value wrapper_sha256 0)"
  if [ -n "$wrapper" ] && [ -n "$wrapper_sha" ]; then
    [ -f "$wrapper" ] || die "wrapper is missing: $wrapper"
    [ "$(sha256_file "$wrapper")" = "$wrapper_sha" ] ||
      die "wrapper changed since provisioning; re-provision or restore it before starting"
  fi
}

cmd_start() {
  parse_lifecycle_options "$@"
  require_base_tools
  resolve_root
  manifest_required

  local state pid cmd candidate candidate_sha candidate_version baseline log pidfile
  state="$(mget state || true)"
  case "$state" in
    candidate-running)
      die "candidate is already running (PID $(mget process_pid || true)); stop it first" ;;
    baseline-running)
      die "baseline is running; stop it before starting a candidate" ;;
  esac

  candidate="$(manifest_value candidate_path)"
  candidate_sha="$(manifest_value candidate_sha256)"
  candidate_version="$(manifest_value candidate_version)"
  baseline="$(manifest_value baseline_path)"
  log="$(manifest_value candidate_log)"
  pidfile="$(manifest_value candidate_tunnel_pid)"

  verify_manifest_files
  # verify_manifest_files re-reads the wrapper from the manifest; the local
  # variables above are for the launch.
  [ -x "$candidate" ] || die "candidate is not executable: $candidate"

  if [ -n "$LIFECYCLE_HANDOFF" ]; then
    [ -f "$LIFECYCLE_HANDOFF" ] || die "handoff not found: $LIFECYCLE_HANDOFF"
    command -v python3 >/dev/null 2>&1 || die "python3 is required for --handoff"
    local self_dir validator schema handoff_action
    self_dir="$(cd "$(dirname "$0")" && pwd -P)"
    validator="$self_dir/validation-handoff.py"
    schema="$self_dir/../docs/reference/validation-handoff.schema.json"
    [ -f "$validator" ] || die "handoff validator not found: $validator"
    if ! python3 "$validator" --handoff "$LIFECYCLE_HANDOFF" --checkout "$ROOT" --schema "$schema" --quiet; then
      python3 "$validator" --handoff "$LIFECYCLE_HANDOFF" --checkout "$ROOT" --schema "$schema" >&2 || true
      die "handoff validation failed; refusing to start the candidate"
    fi
    handoff_action="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["workflow"]["next_permitted_action"])' "$LIFECYCLE_HANDOFF")" ||
      die "could not read the handoff next_permitted_action"
    [ "$handoff_action" = "START_CANDIDATE" ] ||
      die "handoff next_permitted_action is '$handoff_action', not START_CANDIDATE; refusing to start"
    note "handoff authorizes START_CANDIDATE"
  fi

  local procs count
  procs="$(list_tunnel_processes)"
  count="$(process_count "$procs")"
  if [ "$count" -gt 0 ]; then
    printf '%s\n' "$procs" >&2
    die "a tunnel process is already running; refusing to start a second one"
  fi

  CANDIDATE_ARGS=()
  while IFS= read -r arg; do
    CANDIDATE_ARGS+=("$arg")
  done < <(mget_all candidate_launch_arg)
  [ "${#CANDIDATE_ARGS[@]}" -gt 0 ] || die "manifest has no candidate launch arguments"
  [ "${CANDIDATE_ARGS[0]}" = "run" ] || die "candidate launch arguments do not start with 'run'"

  note "candidate: $candidate"
  note "version:   $candidate_version"
  note "sha256:    $candidate_sha"
  note "baseline:  $baseline (unchanged)"
  note "log:       $log"
  note "pid file:  $pidfile"
  note "command:   $candidate ${CANDIDATE_ARGS[*]}"

  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: not starting the candidate"
    return 0
  fi

  mkdir -p "$(dirname "$log")" "$(dirname "$pidfile")" "$CANDIDATE_DIR"
  : >> "$log"
  local log_start_size
  log_start_size="$(wc -c < "$log" 2>/dev/null | tr -d ' ')"
  [ -n "$log_start_size" ] || log_start_size=0
  nohup "$candidate" "${CANDIDATE_ARGS[@]}" >>"$log" 2>&1 &
  local started_pid started_cmd
  started_pid=$!
  printf '%s\n' "$started_pid" > "$CANDIDATE_DIR/launch.pid"
  started_cmd="$(process_command_of "$started_pid")"

  if ! pid="$(wait_for_startup "$started_pid" "$log" "$pidfile" "$candidate" "$log_start_size")"; then
    warn "candidate did not confirm startup within ${STARTUP_TIMEOUT}s"
    tail -n 30 "$log" >&2 || true
    stop_started_process "$started_pid" "$started_cmd" "$pidfile" "$candidate"
    set_manifest_value state failed
    die "candidate startup failed; it was stopped and the baseline was not started. See $log"
  fi

  cmd="$(process_command_of "$pid")"
  case "$cmd" in
    *"$candidate"*) : ;;
    *) terminate_pid "$pid" "$cmd" "$STOP_TIMEOUT" || true
       set_manifest_value state failed
       die "started PID $pid does not match the candidate path; stopped it" ;;
  esac

  set_manifest_value state candidate-running
  set_manifest_value process_pid "$pid"
  set_manifest_value process_command "$cmd"
  set_manifest_value started_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  note "candidate running: PID $pid"
  note "command: $cmd"
  note "next: run the ChatGPT live test, then $0 handoff --checkout $ROOT"
}

# ---------------------------------------------------------------------------
# stop
# ---------------------------------------------------------------------------

stop_recorded_process() {
  local kind="$1" pid_key="$2" command_key="$3" state_key="$4"
  local pid cmd actual procs count
  pid="$(mget "$pid_key" || true)"
  cmd="$(mget "$command_key" || true)"
  if [ -z "$pid" ]; then
    note "no $kind process recorded; nothing to stop"
    return 0
  fi
  if ! process_alive "$pid"; then
    note "$kind PID $pid is not running; marking stopped"
    set_manifest_value state stopped
    set_manifest_value "$state_key" "$pid"
    set_manifest_value "$pid_key" ""
    set_manifest_value "$command_key" ""
    return 0
  fi
  actual="$(process_command_of "$pid")"
  if [ "$actual" != "$cmd" ]; then
    print_processes "$(list_tunnel_processes)"
    die "refusing to stop PID $pid: recorded command does not match. Expected '$cmd', got '$actual'"
  fi
  procs="$(list_tunnel_processes)"
  count="$(process_count "$procs")"
  if [ "$count" -ne 1 ]; then
    print_processes "$procs"
    die "refusing to stop while $count tunnel processes are running; expected exactly the recorded PID $pid"
  fi
  local listed_pid
  listed_pid="$(printf '%s\n' "$procs" | awk '{print $1}')"
  [ "$listed_pid" = "$pid" ] ||
    die "refusing to stop: the running tunnel PID $listed_pid is not the recorded PID $pid"

  if [ "$DRY_RUN" = 1 ]; then
    note "dry-run: would stop $kind PID $pid"
    note "dry-run: command: $cmd"
    return 0
  fi

  terminate_pid "$pid" "$cmd" "$STOP_TIMEOUT"
  if process_alive "$pid"; then
    die "failed to stop $kind PID $pid"
  fi
  set_manifest_value state stopped
  set_manifest_value "$state_key" "$pid"
  set_manifest_value "$pid_key" ""
  set_manifest_value "$command_key" ""
  note "$kind stopped: PID $pid"
}

cmd_stop() {
  parse_lifecycle_options "$@"
  require_base_tools
  resolve_root
  manifest_required
  local state
  state="$(mget state || true)"
  case "$state" in
    candidate-running) stop_recorded_process "candidate" process_pid process_command last_candidate_pid ;;
    baseline-running)
      stop_recorded_process "baseline" baseline_process_pid baseline_process_command last_baseline_pid ;;
    prepared)
      # Provision captures the running baseline but never stops it. `stop` is the
      # explicit, verified hand-off between the baseline and the candidate.
      if [ -n "$(mget baseline_process_pid || true)" ]; then
        stop_recorded_process "baseline" baseline_process_pid baseline_process_command last_baseline_pid
      else
        note "no running baseline was recorded (state: prepared); nothing to stop"
      fi
      ;;
    *)
      note "no running process recorded (state: ${state:-unknown}); nothing to stop" ;;
  esac
}

# ---------------------------------------------------------------------------
# status / preflight
# ---------------------------------------------------------------------------

cmd_status() {
  parse_lifecycle_options "$@"
  require_base_tools
  resolve_root
  if [ ! -f "$MANIFEST" ]; then
    note "no candidate manifest at $MANIFEST"
    note "running tunnel processes:"
    print_processes "$(list_tunnel_processes)"
    return 0
  fi

  local state candidate baseline wrapper
  state="$(mget state || true)"
  candidate="$(mget candidate_path || true)"
  baseline="$(mget baseline_path || true)"
  wrapper="$(mget wrapper_path || true)"

  note "state:     ${state:-unknown}"
  note "candidate: ${candidate:-?} ($(mget candidate_version || true))"
  note "sha256:    $(mget candidate_sha256 || true)"
  note "source:    $(mget source_url || true) @ $(mget candidate_revision || true)"
  note "patch:     $(mget patch || true) ($(mget patch_sha256 || true))"
  note "baseline:  ${baseline:-?} ($(mget baseline_version || true))"
  note "profile:   $(mget profile || true)"
  note "log:       $(mget candidate_log || true)"
  note "wrapper:   ${wrapper:-none}"
  if [ -n "$(mget process_pid || true)" ]; then
    note "process:   PID $(mget process_pid || true)"
    note "command:   $(mget process_command || true)"
  else
    note "process:   not running"
  fi
  if [ -n "$(mget baseline_process_pid || true)" ] && [ "$state" = "baseline-running" ]; then
    note "baseline process: PID $(mget baseline_process_pid || true)"
  fi

  note "running tunnel processes:"
  print_processes "$(list_tunnel_processes)"
  local procs count
  procs="$(list_tunnel_processes)"
  count="$(process_count "$procs")"
  if [ "$count" -gt 1 ]; then
    warn "multiple tunnel processes detected; no automatic action was taken"
  fi
}

preflight_fail=0
pf_ok() { printf '  ok   %s\n' "$*"; }
pf_fail() { printf '  FAIL %s\n' "$*"; preflight_fail=1; }

cmd_preflight() {
  parse_lifecycle_options "$@"
  require_base_tools
  resolve_root
  manifest_required
  local v
  note "preflight for $MANIFEST"

  for key in candidate_path candidate_sha256 candidate_version baseline_path \
             baseline_sha256 baseline_version profile candidate_log candidate_tunnel_pid; do
    v="$(mget "$key" || true)"
    if [ -n "$v" ]; then pf_ok "$key is recorded"; else pf_fail "$key is missing"; fi
  done

  v="$(mget candidate_path || true)"
  if [ -n "$v" ] && [ -x "$v" ] && [ "$(sha256_file "$v")" = "$(mget candidate_sha256 || true)" ]; then
    pf_ok "candidate checksum matches"
  else
    pf_fail "candidate checksum does not match or the file is missing"
  fi
  v="$(mget candidate_version || true)"
  if [ -n "$v" ] && [ "$(version_of "$(mget candidate_path || true)" 2>/dev/null || true)" = "$v" ]; then
    pf_ok "candidate --version matches"
  else
    pf_fail "candidate --version does not match"
  fi

  v="$(mget baseline_path || true)"
  if [ -n "$v" ] && [ -x "$v" ] && [ "$(sha256_file "$v")" = "$(mget baseline_sha256 || true)" ]; then
    pf_ok "baseline is present and unchanged"
  else
    pf_fail "baseline is missing or changed"
  fi

  v="$(mget wrapper_path || true)"
  if [ -n "$v" ]; then
    if [ -f "$v" ] && [ "$(sha256_file "$v")" = "$(mget wrapper_sha256 || true)" ]; then
      pf_ok "wrapper matches the provisioned checksum"
    else
      pf_fail "wrapper is missing or changed"
    fi
  else
    pf_ok "no wrapper recorded (nothing to verify)"
  fi

  local procs count state
  procs="$(list_tunnel_processes)"
  count="$(process_count "$procs")"
  state="$(mget state || true)"
  if [ "$count" -gt 1 ]; then
    pf_fail "multiple tunnel processes are running"
    print_processes "$procs"
  elif [ "$count" -eq 1 ]; then
    if [ "$state" = "candidate-running" ] || [ "$state" = "baseline-running" ]; then
      local pid recorded
      pid="$(printf '%s\n' "$procs" | awk '{print $1}')"
      recorded="$(mget process_pid || true)"
      if [ "$state" = "baseline-running" ]; then
        recorded="$(mget baseline_process_pid || true)"
      fi
      if [ "$pid" = "$recorded" ]; then
        pf_ok "the single running tunnel is the recorded $state process"
      else
        pf_fail "the running tunnel PID $pid is not the recorded $state PID $recorded"
      fi
    else
      pf_ok "a tunnel is running but this manifest records state '$state'"
    fi
  else
    pf_ok "no tunnel process is currently running"
  fi

  if [ "$(mget patch_applied || true)" = "no" ]; then
    pf_ok "no patch was applied (baseline candidate)"
  elif [ -n "$(mget patch || true)" ] && [ -z "$(mget patch_sha256 || true)" ]; then
    pf_fail "patch is recorded without a checksum"
  else
    pf_ok "patch provenance is recorded"
  fi

  if [ -n "$(mget wrapper_backup || true)" ] && [ ! -f "$(mget wrapper_backup || true)" ]; then
    pf_fail "recorded wrapper backup is missing"
  else
    pf_ok "rollback files are present"
  fi

  if [ "$preflight_fail" -ne 0 ]; then
    die "preflight failed; do not start the candidate"
  fi
  note "preflight passed"
}

# ---------------------------------------------------------------------------
# rollback
# ---------------------------------------------------------------------------

ROLLBACK_START=0

cmd_rollback() {
  local arg baseline_already_running=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --start) ROLLBACK_START=1 ;;
      --startup-timeout) [ $# -ge 2 ] || usage; STARTUP_TIMEOUT="$2"; shift ;;
      --stop-timeout) [ $# -ge 2 ] || usage; STOP_TIMEOUT="$2"; shift ;;
      --dry-run) DRY_RUN=1 ;;
      --process-match) [ $# -ge 2 ] || usage; PROCESS_MATCH="$2"; shift ;;
      --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
      -h|--help) usage ;;
      *) die "unknown rollback option: $1" ;;
    esac
    shift
  done
  require_base_tools
  resolve_root
  manifest_required

  local state
  state="$(mget state || true)"
  case "$state" in
    candidate-running)
      if [ "$DRY_RUN" = 1 ]; then
        cmd_stop --checkout "$ROOT" --process-match "$PROCESS_MATCH" --dry-run --stop-timeout "$STOP_TIMEOUT"
      else
        cmd_stop --checkout "$ROOT" --process-match "$PROCESS_MATCH" --stop-timeout "$STOP_TIMEOUT"
      fi
      ;;
    baseline-running)
      baseline_already_running=1
      note "baseline is already running; no candidate to stop"
      ;;
    prepared)
      # A prepared manifest may still have the captured baseline running. Do not
      # stop it during rollback; rollback only removes a candidate.
      if [ -n "$(mget baseline_process_pid || true)" ] &&
        process_alive "$(mget baseline_process_pid || true)"; then
        baseline_already_running=1
        note "baseline is still running; leaving it untouched"
      fi
      ;;
  esac

  # Restore the wrapper only if it still has the checksum recorded at
  # provisioning. A newer operator change is never discarded.
  local wrapper wrapper_sha backup current
  wrapper="$(mget wrapper_path || true)"
  wrapper_sha="$(mget wrapper_sha256 || true)"
  backup="$(mget wrapper_backup || true)"
  if [ -n "$wrapper" ] && [ -n "$backup" ] && [ -f "$backup" ]; then
    if [ -f "$wrapper" ]; then
      current="$(sha256_file "$wrapper")"
      if [ "$current" != "$wrapper_sha" ]; then
        die "wrapper has changed since provisioning; refusing to overwrite it. Current: $current, recorded: $wrapper_sha"
      fi
    fi
    if [ "$DRY_RUN" = 1 ]; then
      note "dry-run: would restore wrapper $wrapper from $backup"
    else
      cp "$backup" "$wrapper.tmp.$$"
      chmod +x "$wrapper.tmp.$$" 2>/dev/null || true
      mv -f "$wrapper.tmp.$$" "$wrapper"
      note "wrapper restored from $backup"
    fi
  fi

  if [ "$ROLLBACK_START" = 1 ] && [ "$baseline_already_running" = 1 ]; then
    note "baseline is already running; nothing to start"
  elif [ "$ROLLBACK_START" = 1 ]; then
    local baseline baseline_sha baseline_version log pidfile started_pid started_cmd pid cmd baseline_log baseline_pidfile stdout_log log_start_size
    baseline="$(manifest_value baseline_path)"
    baseline_sha="$(manifest_value baseline_sha256)"
    baseline_version="$(manifest_value baseline_version)"
    verify_binary_identity "baseline" "$baseline" "$baseline_sha" "$baseline_version"
    local procs count
    procs="$(list_tunnel_processes)"
    count="$(process_count "$procs")"
    if [ "$count" -gt 0 ]; then
      printf '%s\n' "$procs" >&2
      die "a tunnel process is still running; refusing to start the baseline"
    fi
    BASELINE_ARGS=()
    while IFS= read -r arg; do
      BASELINE_ARGS+=("$arg")
    done < <(mget_all baseline_launch_arg)
    [ "${#BASELINE_ARGS[@]}" -gt 0 ] || die "manifest has no baseline launch arguments"
    baseline_log="$(baseline_arg_value --log.file || true)"
    [ -n "$baseline_log" ] || baseline_log="$TUNNEL_DIR/logs/baseline-$(date +%Y%m%d-%H%M%S).jsonl"
    baseline_pidfile="$(baseline_arg_value --pid.file || true)"
    [ -n "$baseline_pidfile" ] || baseline_pidfile="$CANDIDATE_DIR/baseline.pid"
    stdout_log="$CANDIDATE_DIR/baseline-stdout.log"
    note "baseline: $baseline"
    note "command:  $baseline ${BASELINE_ARGS[*]}"
    note "log:      $baseline_log"
    if [ "$DRY_RUN" = 1 ]; then
      note "dry-run: would start the baseline"
    else
      mkdir -p "$(dirname "$baseline_log")" "$(dirname "$baseline_pidfile")" "$CANDIDATE_DIR"
      : >> "$baseline_log"
      log_start_size="$(wc -c < "$baseline_log" 2>/dev/null | tr -d ' ')"
      [ -n "$log_start_size" ] || log_start_size=0
      nohup "$baseline" "${BASELINE_ARGS[@]}" >>"$stdout_log" 2>&1 &
      started_pid=$!
      printf '%s\n' "$started_pid" > "$CANDIDATE_DIR/baseline-launch.pid"
      started_cmd="$(process_command_of "$started_pid")"
      if ! pid="$(wait_for_startup "$started_pid" "$baseline_log" "$baseline_pidfile" "$baseline" "$log_start_size")"; then
        tail -n 30 "$baseline_log" >&2 || true
        stop_started_process "$started_pid" "$started_cmd" "$baseline_pidfile" "$baseline"
        set_manifest_value state rolled-back
        die "baseline did not confirm startup; it was stopped. See $baseline_log"
      fi
      cmd="$(process_command_of "$pid")"
      case "$cmd" in
        *"$baseline"*) : ;;
        *) stop_started_process "$pid" "$cmd" "$baseline_pidfile" "$baseline"
           set_manifest_value state rolled-back
           die "started baseline PID $pid does not match the baseline path; stopped it" ;;
      esac
      set_manifest_value state baseline-running
      set_manifest_value baseline_process_pid "$pid"
      set_manifest_value baseline_process_command "$cmd"
      set_manifest_value baseline_started_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      note "baseline running: PID $pid"
    fi
  else
    if [ "$DRY_RUN" != 1 ]; then
      if [ "$baseline_already_running" = 1 ]; then
        set_manifest_value state baseline-running
      else
        set_manifest_value state rolled-back
      fi
    fi
    if [ "$baseline_already_running" = 1 ]; then
      note "rolled back; the baseline process was already running"
    else
      note "rolled back (baseline not started)"
      note "start baseline with: $ROOT/scripts/planner-tunnel-candidate.sh rollback --start --checkout $ROOT"
    fi
  fi
}

# ---------------------------------------------------------------------------
# handoff
# ---------------------------------------------------------------------------

HANDOFF_TESTS=""
HANDOFF_RESULTS=""
HANDOFF_RESULTS_JSON=""
HANDOFF_REQUEST=""
HANDOFF_OUTPUT=""
HANDOFF_REPLACE=0
HANDOFF_FORMAT="markdown"
HANDOFF_STATUS="PENDING"
HANDOFF_STATUS_SET=0
HANDOFF_ISSUE=""
HANDOFF_WORKFLOW_ID=""
HANDOFF_AGENT="unspecified"
HANDOFF_CAPABILITIES="filesystem,process_execution"
HANDOFF_LOCAL_STATUS="NOT_RUN"
HANDOFF_DEPLOYMENT_APPROVAL="PENDING"
HANDOFF_APPROVAL_REFERENCE=""
HANDOFF_MERGE_STATUS="PENDING"

sha256_stdin() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    die "no shasum or sha256sum on PATH"
  fi
}

caps_json() {
  local caps="$1" out="" cap old_ifs="$IFS"
  IFS=','
  for cap in $caps; do
    cap="$(printf '%s' "$cap" | sed 's/^ *//; s/ *$//')"
    [ -n "$cap" ] || continue
    [ -z "$out" ] || out="$out,"
    out="$out\"$(json_escape "$cap")\""
  done
  IFS="$old_ifs"
  printf '[%s]' "$out"
}

default_request_json() {
  cat <<'EOF'
{
  "rounds": 10,
  "concurrent": true,
  "retries": 0,
  "state_changes": false,
  "record_initialization_errors": true,
  "total_calls": 20,
  "required_calls": [
    {"tool": "sop_status", "arguments": {}},
    {"tool": "sop_question_get", "arguments": {"question_id": "q-1"}}
  ],
  "acceptance": {
    "pass": "all 20 calls succeed with no retries and no state change",
    "fail": "any call fails; record the exact error and matching debug-log lines",
    "caveat": "a clean run is supporting evidence, not proof an intermittent defect cannot recur"
  }
}
EOF
}

default_results_template() {
  cat <<EOF
{
  "schema": "mcp-live-validation/validation-results/v1",
  "workflow_id": "$(json_escape "$HANDOFF_WORKFLOW_ID")",
  "validator": "",
  "validator_capabilities": [],
  "candidate_sha256": "$(json_escape "$candidate_sha")",
  "source_revision": "$(json_escape "$source_revision")",
  "started_at": "",
  "finished_at": "",
  "rounds": $REQUEST_ROUNDS,
  "concurrent": $REQUEST_CONCURRENT,
  "retries": $REQUEST_RETRIES,
  "state_changes": $REQUEST_STATE_CHANGES,
  "summary": {"total": $REQUEST_TOTAL, "passed": 0, "failed": 0},
  "calls": [],
  "initialization_errors": [],
  "status": "PENDING"
}
EOF
}

derive_workflow() {
  local state="$1" validation="$2" deployment="$3"
  if [ "$validation" = "PASS" ]; then
    WF_STATUS="VALIDATION_PASSED"
    WF_NEXT="PREPARE_PR"
    return 0
  fi
  if [ "$validation" = "FAIL" ]; then
    WF_STATUS="VALIDATION_FAILED"
    WF_NEXT="DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE"
    return 0
  fi
  case "$state" in
    prepared)
      if [ "$deployment" = "APPROVED" ]; then
        WF_STATUS="PREPARED"
        WF_NEXT="START_CANDIDATE"
      else
        WF_STATUS="DEPLOYMENT_PENDING_APPROVAL"
        WF_NEXT="REQUEST_DEPLOYMENT_APPROVAL"
      fi
      ;;
    candidate-running)
      WF_STATUS="AWAITING_INDEPENDENT_VALIDATION"
      WF_NEXT="RUN_INDEPENDENT_VALIDATION"
      ;;
    stopped|rolled-back)
      WF_STATUS="ROLLED_BACK"
      WF_NEXT="STOP_AND_REPORT"
      ;;
    failed)
      WF_STATUS="BLOCKED"
      WF_NEXT="STOP_AND_REPORT"
      ;;
    *)
      WF_STATUS="BLOCKED"
      WF_NEXT="STOP_AND_REPORT"
      ;;
  esac
}

cmd_handoff() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --tests) [ $# -ge 2 ] || usage; HANDOFF_TESTS="$2"; shift ;;
      --results) [ $# -ge 2 ] || usage; HANDOFF_RESULTS="$2"; shift ;;
      --results-json) [ $# -ge 2 ] || usage; HANDOFF_RESULTS_JSON="$2"; shift ;;
      --request) [ $# -ge 2 ] || usage; HANDOFF_REQUEST="$2"; shift ;;
      --output) [ $# -ge 2 ] || usage; HANDOFF_OUTPUT="$2"; shift ;;
      --format) [ $# -ge 2 ] || usage; HANDOFF_FORMAT="$2"; shift ;;
      --validation-status)
        [ $# -ge 2 ] || usage
        case "$2" in
          PENDING|PASS|FAIL) HANDOFF_STATUS="$2"; HANDOFF_STATUS_SET=1 ;;
          *) die "--validation-status must be PENDING, PASS, or FAIL" ;;
        esac
        shift ;;
      --issue) [ $# -ge 2 ] || usage; HANDOFF_ISSUE="$2"; shift ;;
      --workflow-id) [ $# -ge 2 ] || usage; HANDOFF_WORKFLOW_ID="$2"; shift ;;
      --agent) [ $# -ge 2 ] || usage; HANDOFF_AGENT="$2"; shift ;;
      --capabilities) [ $# -ge 2 ] || usage; HANDOFF_CAPABILITIES="$2"; shift ;;
      --local-status)
        [ $# -ge 2 ] || usage
        case "$2" in
          PASS|FAIL|NOT_RUN) HANDOFF_LOCAL_STATUS="$2" ;;
          *) die "--local-status must be PASS, FAIL, or NOT_RUN" ;;
        esac
        shift ;;
      --deployment-approval)
        [ $# -ge 2 ] || usage
        case "$2" in
          PENDING|APPROVED|DENIED) HANDOFF_DEPLOYMENT_APPROVAL="$2" ;;
          *) die "--deployment-approval must be PENDING, APPROVED, or DENIED" ;;
        esac
        shift ;;
      --approval-reference) [ $# -ge 2 ] || usage; HANDOFF_APPROVAL_REFERENCE="$2"; shift ;;
      --merge-status)
        [ $# -ge 2 ] || usage
        case "$2" in
          PENDING|APPROVED|DENIED|NOT_REQUIRED) HANDOFF_MERGE_STATUS="$2" ;;
          *) die "--merge-status must be PENDING, APPROVED, DENIED, or NOT_REQUIRED" ;;
        esac
        shift ;;
      --replace) HANDOFF_REPLACE=1 ;;
      --dry-run) DRY_RUN=1 ;;
      --process-match) [ $# -ge 2 ] || usage; PROCESS_MATCH="$2"; shift ;;
      --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
      -h|--help) usage ;;
      *) die "unknown handoff option: $1" ;;
    esac
    shift
  done
  case "$HANDOFF_FORMAT" in
    markdown|json) : ;;
    *) die "--format must be markdown or json" ;;
  esac
  require_base_tools
  resolve_root
  manifest_required

  local self_dir validator schema
  self_dir="$(cd "$(dirname "$0")" && pwd -P)"
  validator="$self_dir/validation-handoff.py"
  schema="$self_dir/../docs/reference/validation-handoff.schema.json"

  if [ -z "$HANDOFF_WORKFLOW_ID" ]; then
    HANDOFF_WORKFLOW_ID="$(mget workflow_id || true)"
  fi
  if [ -z "$HANDOFF_WORKFLOW_ID" ]; then
    HANDOFF_WORKFLOW_ID="mcp-live-$(date -u +%Y%m%dT%H%M%SZ)-$$"
    if [ "$DRY_RUN" != 1 ]; then
      set_manifest_value workflow_id "$HANDOFF_WORKFLOW_ID"
    fi
  fi

  local branch commit dirty candidate candidate_sha candidate_version baseline
  local baseline_version baseline_sha baseline_log
  local source source_revision patch patch_sha profile log state pid cmd cmd_safe command_sha rollback platform
  branch="$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
  commit="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)"
  dirty="$(git -C "$ROOT" status --porcelain 2>/dev/null | wc -l | tr -d ' ')"
  candidate="$(manifest_value candidate_path)"
  candidate_sha="$(manifest_value candidate_sha256)"
  candidate_version="$(manifest_value candidate_version)"
  baseline="$(manifest_value baseline_path)"
  baseline_version="$(mget baseline_version || true)"
  baseline_sha="$(mget baseline_sha256 || true)"
  baseline_log="$(mget_all baseline_launch_arg | sed -n 's/^--log\.file=//p' | head -n 1)"
  if [ -z "$baseline_log" ]; then
    baseline_log="$(mget_all baseline_launch_arg | awk 'p{print; exit} /^--log.file$/{p=1}')"
  fi
  source="$(manifest_value source_url)"
  source_revision="$(manifest_value candidate_revision)"
  patch="$(mget patch || true)"
  patch_sha="$(mget patch_sha256 || true)"
  profile="$(manifest_value profile)"
  log="$(manifest_value candidate_log)"
  state="$(mget state || true)"
  pid="$(mget process_pid || true)"
  cmd="$(mget process_command || true)"
  cmd_safe="$(redact_command "$cmd")"
  command_sha=""
  if [ -n "$cmd" ]; then
    command_sha="$(printf '%s' "$cmd" | sha256_stdin)"
  fi
  platform="$(uname -s 2>/dev/null || printf 'unknown')"
  rollback="$ROOT/scripts/planner-tunnel-candidate.sh rollback --checkout $ROOT --start"
  case "$PROCESS_MATCH" in
    tunnel-client) : ;;
    *) rollback="$rollback --process-match $PROCESS_MATCH" ;;
  esac

  local test_text result_text tree_state caps
  if [ "$dirty" = 0 ]; then
    tree_state="clean"
  else
    tree_state="$dirty uncommitted path(s)"
  fi
  if [ -n "$HANDOFF_TESTS" ]; then
    [ -f "$HANDOFF_TESTS" ] || die "tests file not found: $HANDOFF_TESTS"
    test_text="$(cat "$HANDOFF_TESTS")"
  else
    test_text="[agent: record the local test commands and results here]"
  fi
  if [ "$HANDOFF_LOCAL_STATUS" != "NOT_RUN" ] && [ -z "$HANDOFF_TESTS" ]; then
    die "--local-status $HANDOFF_LOCAL_STATUS requires --tests FILE with the local evidence"
  fi
  if [ -n "$HANDOFF_RESULTS" ]; then
    [ -f "$HANDOFF_RESULTS" ] || die "results file not found: $HANDOFF_RESULTS"
    result_text="$(cat "$HANDOFF_RESULTS")"
  else
    result_text="[independent validator: record the actual MCP results here; missing validation is not success]"
  fi
  caps="$(caps_json "$HANDOFF_CAPABILITIES")"

  local request_json_text results_template_json_text results_json_text validator_name
  local request_meta request_rounds request_total request_concurrent request_retries request_state_changes
  if [ -n "$HANDOFF_REQUEST" ]; then
    [ -f "$HANDOFF_REQUEST" ] || die "request file not found: $HANDOFF_REQUEST"
    command -v python3 >/dev/null 2>&1 || die "python3 is required for --request"
    request_json_text="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1]));
assert isinstance(d, dict), "request must be a JSON object"
print(json.dumps(d, indent=2, sort_keys=True))' "$HANDOFF_REQUEST")" ||
      die "invalid request JSON: $HANDOFF_REQUEST"
    request_meta="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1]));
print(d.get("rounds", 10)); print(d.get("total_calls", 20));
print("true" if d.get("concurrent", True) else "false");
print(d.get("retries", 0));
print("true" if d.get("state_changes", False) else "false")' "$HANDOFF_REQUEST")" ||
      die "invalid request JSON: $HANDOFF_REQUEST"
    request_rounds="$(printf '%s\n' "$request_meta" | sed -n '1p')"
    request_total="$(printf '%s\n' "$request_meta" | sed -n '2p')"
    request_concurrent="$(printf '%s\n' "$request_meta" | sed -n '3p')"
    request_retries="$(printf '%s\n' "$request_meta" | sed -n '4p')"
    request_state_changes="$(printf '%s\n' "$request_meta" | sed -n '5p')"
  else
    request_json_text="$(default_request_json)"
    request_rounds=10
    request_total=20
    request_concurrent=true
    request_retries=0
    request_state_changes=false
  fi
  REQUEST_ROUNDS="$request_rounds"
  REQUEST_TOTAL="$request_total"
  REQUEST_CONCURRENT="$request_concurrent"
  REQUEST_RETRIES="$request_retries"
  REQUEST_STATE_CHANGES="$request_state_changes"

  results_template_json_text="$(default_results_template)"
  results_json_text="$results_template_json_text"
  validator_name="unspecified"
  if [ -n "$HANDOFF_RESULTS_JSON" ]; then
    [ -f "$HANDOFF_RESULTS_JSON" ] || die "results JSON not found: $HANDOFF_RESULTS_JSON"
    command -v python3 >/dev/null 2>&1 || die "python3 is required for --results-json"
    results_json_text="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1]));
assert isinstance(d, dict), "results must be a JSON object"
print(json.dumps(d, indent=2, sort_keys=True))' "$HANDOFF_RESULTS_JSON")" ||
      die "invalid results JSON: $HANDOFF_RESULTS_JSON"
    result_status="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("status", ""))' "$HANDOFF_RESULTS_JSON")" ||
      die "invalid results JSON: $HANDOFF_RESULTS_JSON"
    case "$result_status" in
      PENDING|PASS|FAIL) : ;;
      *) die "results JSON status must be PENDING, PASS, or FAIL: $result_status" ;;
    esac
    if [ "$HANDOFF_STATUS_SET" = 1 ] && [ "$HANDOFF_STATUS" != "$result_status" ]; then
      die "--validation-status $HANDOFF_STATUS does not match results JSON status $result_status"
    fi
    HANDOFF_STATUS="$result_status"
    validator_name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("validator", ""))' "$HANDOFF_RESULTS_JSON")" ||
      die "invalid results JSON: $HANDOFF_RESULTS_JSON"
    [ -n "$validator_name" ] || validator_name="unspecified"
  fi
  if [ "$HANDOFF_STATUS" != "PENDING" ] && [ -z "$HANDOFF_RESULTS_JSON" ]; then
    die "--validation-status $HANDOFF_STATUS requires --results-json FILE with the independent results; local tests are not independent validation"
  fi
  if [ "$HANDOFF_STATUS" = "PASS" ] || [ "$HANDOFF_STATUS" = "FAIL" ]; then
    case "$validator_name" in
      ""|unspecified) die "results JSON must name the independent validator" ;;
    esac
  fi

  derive_workflow "$state" "$HANDOFF_STATUS" "$HANDOFF_DEPLOYMENT_APPROVAL"

  emit_handoff() {
    cat <<EOF
# MCP candidate validation handoff

> This is a handoff, not a validation result. An independent MCP validator must
> report its own live results before the candidate is called verified. A clean
> run is supporting evidence, not proof that an intermittent defect cannot recur.

## Workflow

- ID: \`$HANDOFF_WORKFLOW_ID\`
- Status: \`$WF_STATUS\`
- Next permitted action: \`$WF_NEXT\`
- Issue: \`${HANDOFF_ISSUE:-not recorded}\`
- Validation status: \`$HANDOFF_STATUS\`

## Repository

- Branch: \`$branch\`
- Commit: \`$commit\`
- Working tree: $tree_state

## Environment

- Agent: \`$HANDOFF_AGENT\`
- Platform: \`$platform\`
- Capabilities: $caps

## Candidate executable

- Path: \`$candidate\`
- Version: \`$candidate_version\`
- SHA-256: \`$candidate_sha\`
- Source: \`$source\` @ \`$source_revision\`
- Patch: \`$patch\` (SHA-256 \`$patch_sha\`)
- Profile: \`$profile\`
- Baseline (unchanged): \`$baseline\`

## Active process

- Manifest state: \`${state:-unknown}\`
- PID: \`${pid:-not running}\`
- Command: \`${cmd_safe:-n/a}\`
- Command SHA-256: \`${command_sha:-n/a}\`
- Candidate debug log: \`$log\`
- Baseline log: ${baseline_log:-not recorded}

## Local verification (not independent validation)

- Status: \`$HANDOFF_LOCAL_STATUS\`

$test_text

## Independent validation

- Status: \`$HANDOFF_STATUS\`
- Validator: \`$validator_name\`
- Candidate SHA-256: \`$candidate_sha\`
- Source revision: \`$source_revision\`

$result_text

## Approval

- Deployment required: yes
- Deployment status: \`$HANDOFF_DEPLOYMENT_APPROVAL\`
- Approval reference: \`${HANDOFF_APPROVAL_REFERENCE:-not recorded}\`
- Merge required: yes
- Merge status: \`$HANDOFF_MERGE_STATUS\` (the agent never merges automatically)

## Requested live MCP tests

See the JSON artifact for the authoritative \`request\` object and the
\`results_template\` the validator fills. The default issue #90-style request is:

- 10 rounds of concurrent \`sop_status\` and \`sop_question_get\`
- 20 calls total
- no retries and no state changes
- record every initialization error, including HTTP \`409\` and
  \`mcp_initialization_required\`

For a different MCP surface, replace the required calls in the JSON artifact
while keeping the no-retry, no-state-change, and initialization-error rules.

## Rollback

\`\`\`sh
$rollback
\`\`\`

The rollback command stops only the verified candidate PID, restores the wrapper
backup, and starts the recorded baseline. Do not edit or delete the candidate
binary before rollback evidence is collected.

## Acceptance

- Pass: every requested call succeeds with no retries and no state change.
- Fail: any call fails; record the exact error and the matching debug-log lines.
- A pass is supporting evidence. The defect remains intermittent until repeated
  live runs and a confirmed root cause support a stronger claim.
EOF
  }

  emit_handoff_json() {
    cat <<EOF
{
  "schema": "mcp-live-validation/validation-handoff/v1",
  "schema_version": 1,
  "workflow_id": "$(json_escape "$HANDOFF_WORKFLOW_ID")",
  "issue": "$(json_escape "$HANDOFF_ISSUE")",
  "generated_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "repository": {
    "root": "$(json_escape "$ROOT")",
    "branch": "$(json_escape "$branch")",
    "commit": "$(json_escape "$commit")",
    "working_tree": "$(json_escape "$tree_state")"
  },
  "environment": {
    "agent": "$(json_escape "$HANDOFF_AGENT")",
    "platform": "$(json_escape "$platform")",
    "capabilities": $caps
  },
  "candidate": {
    "path": "$(json_escape "$candidate")",
    "version": "$(json_escape "$candidate_version")",
    "sha256": "$(json_escape "$candidate_sha")",
    "source": "$(json_escape "$source")",
    "source_revision": "$(json_escape "$source_revision")",
    "patch": "$(json_escape "$patch")",
    "patch_sha256": "$(json_escape "$patch_sha")",
    "profile": "$(json_escape "$profile")"
  },
  "baseline": {
    "path": "$(json_escape "$baseline")",
    "version": "$(json_escape "$baseline_version")",
    "sha256": "$(json_escape "$baseline_sha")"
  },
  "process": {
    "state": "$(json_escape "$state")",
    "pid": "$(json_escape "$pid")",
    "command": "$(json_escape "$cmd_safe")",
    "command_sha256": "$(json_escape "$command_sha")",
    "launch_configuration": {"profile": "$(json_escape "$profile")"}
  },
  "logs": {
    "candidate": "$(json_escape "$log")",
    "baseline": "$(json_escape "$baseline_log")"
  },
  "local_verification": {
    "status": "$(json_escape "$HANDOFF_LOCAL_STATUS")",
    "results": "$(json_escape "$test_text")"
  },
  "request": $request_json_text,
  "results_template": $results_template_json_text,
  "independent_validation": {
    "validator": "$(json_escape "$validator_name")",
    "status": "$(json_escape "$HANDOFF_STATUS")",
    "candidate_sha256": "$(json_escape "$candidate_sha")",
    "source_revision": "$(json_escape "$source_revision")",
    "results": $results_json_text,
    "local_tests_are_not_validation": true
  },
  "approval": {
    "deployment": {
      "required": true,
      "status": "$(json_escape "$HANDOFF_DEPLOYMENT_APPROVAL")",
      "reference": "$(json_escape "$HANDOFF_APPROVAL_REFERENCE")"
    },
    "merge": {
      "required": true,
      "status": "$(json_escape "$HANDOFF_MERGE_STATUS")"
    }
  },
  "workflow": {
    "status": "$(json_escape "$WF_STATUS")",
    "next_permitted_action": "$(json_escape "$WF_NEXT")"
  },
  "rollback": {"command": "$(json_escape "$rollback")"},
  "safety": {
    "production_binary_replaced": false,
    "merged": false,
    "credentials_redacted": true
  }
}
EOF
  }

  emit_selected_handoff() {
    case "$HANDOFF_FORMAT" in
      json) emit_handoff_json ;;
      *) emit_handoff ;;
    esac
  }

  local tmp
  tmp="$(mktemp "${TMPDIR:-/tmp}/validation-handoff.XXXXXX")"
  emit_selected_handoff > "$tmp"

  if [ "$HANDOFF_FORMAT" = "json" ]; then
    command -v python3 >/dev/null 2>&1 || { rm -f "$tmp"; die "python3 is required for --format json"; }
    [ -f "$validator" ] || { rm -f "$tmp"; die "handoff validator not found: $validator"; }
    if ! python3 "$validator" --handoff "$tmp" --checkout "$ROOT" --schema "$schema" --max-age-hours 24 --quiet; then
      python3 "$validator" --handoff "$tmp" --checkout "$ROOT" --schema "$schema" --max-age-hours 24 >&2 || true
      rm -f "$tmp"
      die "generated JSON handoff failed validation; the artifact was not written"
    fi
  fi

  if [ -n "$HANDOFF_OUTPUT" ]; then
    if [ -e "$HANDOFF_OUTPUT" ] && [ "$HANDOFF_REPLACE" != 1 ]; then
      rm -f "$tmp"
      die "output exists: $HANDOFF_OUTPUT (pass --replace)"
    fi
    if [ "$DRY_RUN" = 1 ]; then
      note "dry-run: would write handoff to $HANDOFF_OUTPUT"
      cat "$tmp"
      rm -f "$tmp"
      return 0
    fi
    mkdir -p "$(dirname "$HANDOFF_OUTPUT")"
    mv -f "$tmp" "$HANDOFF_OUTPUT"
    note "handoff written to $HANDOFF_OUTPUT"
  else
    cat "$tmp"
    rm -f "$tmp"
  fi
}

# ---------------------------------------------------------------------------
# dispatch
# ---------------------------------------------------------------------------

DRY_RUN=0
PROCESS_MATCH="${SOP_TUNNEL_PROCESS_MATCH:-tunnel-client}"
CHECKOUT=""
CMD="${1:-}"
if [ $# -gt 0 ]; then shift; fi

case "$CMD" in
  provision) cmd_provision "$@" ;;
  preflight) cmd_preflight "$@" ;;
  start) cmd_start "$@" ;;
  stop) cmd_stop "$@" ;;
  status) cmd_status "$@" ;;
  rollback) cmd_rollback "$@" ;;
  handoff) cmd_handoff "$@" ;;
  ""|-h|--help|help) usage ;;
  *) die "unknown command: $CMD (want provision, preflight, start, stop, status, rollback, or handoff)" ;;
esac

#!/usr/bin/env bash
#
# test-fast.sh — run the repository's Go test suite with TMPDIR on RAM-backed
# temporary storage where the platform provides it.
#
# Why: on Linux the full SOP suite exercises SQLite heavily, and SQLite's file
# I/O against a disk-backed TMPDIR can dominate the run and cause timeouts.
# Pointing TMPDIR at a tmpfs (/dev/shm) keeps each test's temporary database in
# memory and removes that bottleneck. This changes NOTHING else: no SQLite
# durability setting is touched, and no persistent SOP state is placed in RAM.
# `make test` is unchanged and remains the ordinary, disk-backed run.
#
# Safety:
#   * Every invocation creates its OWN directory with `mktemp -d`, so concurrent
#     runs never share a directory and can never delete each other's files.
#   * Only that per-invocation directory is removed, on exit (via a trap). The
#     shared bases (/dev/shm, $TMPDIR, /tmp) are never removed, and the
#     repository (including .agent-sdlc/) is never written to.
#   * A failing `go test` propagates its non-zero exit status: failures are never
#     masked.
#
# Usage: scripts/checks/test-fast.sh [options] [-- go test args...]
#
# Options (a flag overrides its environment variable; the env var overrides the
# default):
#   -p, --parallel N     go test -p value          (default 2)    SOP_TEST_PARALLEL
#   -t, --timeout DUR    go test -timeout value    (default 3m)   SOP_TEST_TIMEOUT
#       --tmpdir DIR     base dir for this run     (auto)         SOP_TEST_TMPDIR
#       --no-ram         never use RAM-backed storage           SOP_TEST_NO_RAM=1
#       --min-free-mb N  min free MiB in the RAM dir (default 512) SOP_TEST_MIN_FREE_MB
#       --print-plan     print the resolution and exit (runs no tests)
#   -h, --help           show this help
#
# Platform behavior:
#   linux   uses $SOP_TEST_RAM_DIR (default /dev/shm) when it exists, is
#           writable, permits executing files, and has at least --min-free-mb MiB
#           free; otherwise it falls back to the system temp directory (with a
#           notice).
#   darwin  uses the normal system temp directory (macOS has no /dev/shm).
#   other   uses the normal system temp directory.
# No sudo is required on any platform.
#
# Test seams (used by internal/testfast tests; not for interactive use):
#   SOP_TEST_PLATFORM   linux|darwin|other, overrides uname-based detection
#   SOP_TEST_RAM_DIR    RAM-backed probe directory (default /dev/shm)
#   SOP_TEST_GO_BIN     go binary to invoke (default go)

set -euo pipefail

die() {
  echo "test-fast: $*" >&2
  exit 2
}

usage() {
  # Print the header comment block (the lines between the shebang and `set`).
  sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; $d'
}

# --- Defaults (environment may override) -------------------------------------
parallel="${SOP_TEST_PARALLEL:-2}"
timeout="${SOP_TEST_TIMEOUT:-3m}"
explicit_base="${SOP_TEST_TMPDIR:-}"
no_ram="${SOP_TEST_NO_RAM:-0}"
min_free_mb="${SOP_TEST_MIN_FREE_MB:-512}"
ram_dir="${SOP_TEST_RAM_DIR:-/dev/shm}"
go_bin="${SOP_TEST_GO_BIN:-go}"
print_plan=0
go_args=()

# --- Parse arguments ---------------------------------------------------------
while [ "$#" -gt 0 ]; do
  case "$1" in
    -p|--parallel)
      [ "$#" -ge 2 ] || die "missing value for $1"
      parallel="$2"; shift 2 ;;
    -t|--timeout)
      [ "$#" -ge 2 ] || die "missing value for $1"
      timeout="$2"; shift 2 ;;
    --tmpdir)
      [ "$#" -ge 2 ] || die "missing value for $1"
      explicit_base="$2"; shift 2 ;;
    --min-free-mb)
      [ "$#" -ge 2 ] || die "missing value for $1"
      min_free_mb="$2"; shift 2 ;;
    --no-ram)
      no_ram=1; shift ;;
    --print-plan)
      print_plan=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    --)
      shift; go_args+=("$@"); break ;;
    -*)
      die "unknown option: $1 (see --help)" ;;
    *)
      die "unexpected argument: $1 (pass go test args after --)" ;;
  esac
done

[ -n "$parallel" ] || die "parallelism must be non-empty"
[ -n "$timeout" ] || die "timeout must be non-empty"
case "$min_free_mb" in
  ''|*[!0-9]*) die "min-free-mb must be a non-negative integer, got: $min_free_mb" ;;
esac

# --- Platform detection ------------------------------------------------------
platform="${SOP_TEST_PLATFORM:-}"
if [ -z "$platform" ]; then
  case "$(uname -s 2>/dev/null || echo unknown)" in
    Linux) platform=linux ;;
    Darwin) platform=darwin ;;
    *) platform=other ;;
  esac
fi

# system_temp is the ordinary, disk-backed fallback base.
system_temp="${TMPDIR:-/tmp}"

# ram_dir_executes reports whether files placed under the RAM-backed base can be
# executed: 0 yes, 1 no (noexec), 2 could not create a probe. Ubuntu mounts
# /dev/shm with noexec by default, and `go test` runs each test binary from its
# temporary work directory, so selecting a noexec directory would make the suite
# fail with "permission denied" rather than run faster.
ram_dir_executes() {
  probe_dir="$(mktemp -d "${ram_dir%/}/sop-test-exec.XXXXXX")" || return 2
  # The cleanup trap is installed later, so remove the probe on an interrupt here.
  trap 'rm -rf -- "$probe_dir"; exit 130' INT TERM
  probe_status=2
  if printf '%s\n' '#!/bin/sh' 'exit 0' > "$probe_dir/probe" && chmod 700 "$probe_dir/probe"; then
    probe_status=1
    "$probe_dir/probe" >/dev/null 2>&1 && probe_status=0
  fi
  rm -rf -- "$probe_dir"
  trap - INT TERM
  return "$probe_status"
}

# --- Choose the base directory ----------------------------------------------
source="system"
base="$system_temp"
ram_used="no"
notice=""

if [ -n "$explicit_base" ]; then
  # An explicit base wins on every platform; it must already exist and be
  # writable so a typo fails loudly instead of silently using /tmp.
  if [ ! -d "$explicit_base" ]; then
    die "--tmpdir/SOP_TEST_TMPDIR is not an existing directory: $explicit_base"
  fi
  if [ ! -w "$explicit_base" ]; then
    die "--tmpdir/SOP_TEST_TMPDIR is not writable: $explicit_base"
  fi
  source="override"
  base="$explicit_base"
elif [ "$platform" = "linux" ] && [ "$no_ram" != "1" ]; then
  if [ -d "$ram_dir" ] && [ -w "$ram_dir" ]; then
    # Verify the RAM directory has enough room. If free space cannot be
    # measured, fall back rather than risk filling the RAM filesystem.
    free_kb=""
    if free_kb="$(df -Pk "$ram_dir" 2>/dev/null | awk 'NR==2 {print $4}')" && [ -n "$free_kb" ]; then
      need_kb=$((min_free_mb * 1024))
      if [ "$free_kb" -lt "$need_kb" ]; then
        notice="RAM-backed $ram_dir has ${free_kb} KiB free, below the ${min_free_mb} MiB minimum; using $system_temp"
      else
        exec_status=0
        ram_dir_executes || exec_status=$?
        case "$exec_status" in
          0)
            source="ram"
            base="$ram_dir"
            ram_used="yes"
            ;;
          1)
            notice="RAM-backed $ram_dir does not permit executing files (noexec); using $system_temp"
            ;;
          *)
            notice="could not verify executability on RAM-backed $ram_dir; using $system_temp"
            ;;
        esac
      fi
    else
      notice="could not measure free space on RAM-backed $ram_dir; using $system_temp"
    fi
  else
    notice="RAM-backed $ram_dir is unavailable; using $system_temp"
  fi
fi

if [ -n "$notice" ]; then
  echo "test-fast: $notice" >&2
fi

if [ "$print_plan" = "1" ]; then
  echo "platform=$platform"
  echo "source=$source"
  echo "ram=$ram_used"
  echo "base=$base"
  echo "min_free_mb=$min_free_mb"
  exit 0
fi

# --- Create this invocation's private directory ------------------------------
# mktemp -d is atomic and collision-free, so concurrent invocations get distinct
# directories even under the same base.
session="$(mktemp -d "${base%/}/sop-test-fast.XXXXXX")" || die "could not create a temporary directory under $base"

# Only ever remove the directory this invocation created, and never anything
# that is not under the resolved base. The base itself is never touched.
cleanup() {
  [ -n "${session:-}" ] || return 0
  case "$session" in
    "${base%/}"/*|"$base"/*) rm -rf -- "$session" ;;
    *) echo "test-fast: refusing to remove unexpected path: $session" >&2 ;;
  esac
}
trap cleanup EXIT INT TERM

echo "test-fast: platform=$platform source=$source ram=$ram_used" >&2
echo "test-fast: TMPDIR=$session" >&2
echo "test-fast: $go_bin test ./... -p $parallel -count=1 -timeout $timeout ${go_args[*]:-}" >&2

# --- Run the suite -----------------------------------------------------------
# TMPDIR is set only for this child process. The exit status is propagated
# verbatim; a failed test run exits non-zero.
status=0
TMPDIR="$session" "$go_bin" test ./... -p "$parallel" -count=1 -timeout "$timeout" ${go_args[@]+"${go_args[@]}"} || status=$?

exit "$status"

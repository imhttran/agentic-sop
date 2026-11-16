#!/usr/bin/env bash
# Sandbox-enabled Linux integration checks for the execution boundary (M1 + G3 + G4).
#
# The ordinary CI runner has no bubblewrap, so W6's sandbox-dependent tests skip
# there. A green ordinary job is therefore NOT evidence that the boundary works.
# This script runs them with a real backend and with SOP_SANDBOX_REQUIRED=1, which
# turns every "no runnable sandbox backend" skip into a failure.
#
# It refuses to run when the backend cannot start, so a runner that lost its
# user-namespace capability fails loudly instead of reporting a suite of skips.
#
# Usage:
#   scripts/checks/sandbox-integration.sh
#
# Environment overrides:
#   SOP_SANDBOX_BWRAP_BIN   bubblewrap binary (default: bwrap from PATH)
#   SOP_SANDBOX_TEST_PKGS   packages to run (default: the two suites W6 changes)

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

bwrap_bin="${SOP_SANDBOX_BWRAP_BIN:-bwrap}"

if ! command -v "$bwrap_bin" >/dev/null 2>&1; then
  echo "sandbox-integration: bubblewrap not found ($bwrap_bin)." >&2
  echo "  Ubuntu 22.04/24.04: sudo apt-get install -y bubblewrap" >&2
  exit 1
fi

# A backend that exists but cannot create namespaces is the failure this job
# exists to catch (for example Ubuntu 24.04's AppArmor restriction).
if ! "$bwrap_bin" --ro-bind / / --unshare-net true 2>/dev/null; then
  echo "sandbox-integration: $bwrap_bin cannot start on this host." >&2
  echo "  On Ubuntu 24.04 this usually means unprivileged user namespaces are" >&2
  echo "  restricted by AppArmor; use a runner that permits them, or set" >&2
  echo "  kernel.apparmor_restrict_unprivileged_userns=0 deliberately." >&2
  exit 1
fi

pkgs="${SOP_SANDBOX_TEST_PKGS:-./internal/toolharness/ ./internal/ollamaagent/}"
log="$(mktemp -t sop-sandbox-integration.XXXXXX)"
trap 'rm -f "$log"' EXIT

echo "sandbox-integration: backend=$bwrap_bin packages=$pkgs"

# SOP_SANDBOX_REQUIRED=1 makes needSandbox/needSandboxAvailable fail instead of
# skip, so this run is real containment evidence rather than a skip list.
set +e
# shellcheck disable=SC2086
SOP_SANDBOX_REQUIRED=1 go test -count=1 -v $pkgs >"$log" 2>&1
status=$?
set -e

skips="$(grep -c '^--- SKIP' "$log" || true)"
sandbox_skips="$(grep -c '^--- SKIP.*no runnable sandbox backend' "$log" || true)"
passed="$(grep -c '^--- PASS' "$log" || true)"
failed="$(grep -c '^--- FAIL' "$log" || true)"

echo "sandbox-integration: pass=$passed fail=$failed skip=$skips sandbox_skip=$sandbox_skips"

if [ "$sandbox_skips" -ne 0 ]; then
  echo "sandbox-integration: sandbox-dependent tests skipped despite SOP_SANDBOX_REQUIRED=1:" >&2
  grep '^--- SKIP.*no runnable sandbox backend' "$log" >&2 || true
  status=1
fi

# Non-sandbox skips are reported for visibility and are not a failure (for example
# the macOS keychain probe on Linux). They are printed so the coverage gap is
# explicit rather than implied.
if [ "$skips" -ne "$sandbox_skips" ]; then
  echo "sandbox-integration: other skips (reported, not a failure):"
  grep '^--- SKIP' "$log" | grep -v 'no runnable sandbox backend' || true
fi

if [ "$status" -ne 0 ]; then
  echo "sandbox-integration: failing tests:" >&2
  grep -e '^--- FAIL' -e '^    --- FAIL' "$log" >&2 || true
  echo "sandbox-integration: FAILED; last 60 lines of the run:" >&2
  tail -60 "$log" >&2
  exit "$status"
fi

echo "sandbox-integration: OK"

#!/usr/bin/env bash
# Real-CLI security integration checks.
#
# Provisions the PINNED scanner versions into an isolated tools directory, then
# runs the tagged integration tests with SOP_SECURITY_INTEGRATION_REQUIRED=1.
# A missing prerequisite or a version mismatch is a hard failure, so CI never
# treats an absent scanner as a skipped pass.
#
# Usage:
#   scripts/checks/security-integration.sh
#
# Environment overrides:
#   SOP_SECURITY_TOOLS_DIR       tools directory (default: $TMPDIR/sop-security-tools)
#   SOP_SECURITY_SEMGREP_BIN     use an existing pinned Semgrep binary
#   SOP_SECURITY_GITLEAKS_BIN    use an existing pinned Gitleaks binary
#   SOP_SECURITY_PYTHON          Python 3 executable (default: python3)
#   SOP_SECURITY_DOWNLOAD_CACHE  reuse pinned downloads from this directory; each
#                                file is re-checked against its committed SHA-256

set -euo pipefail

SEMGREP_VERSION="1.136.0"
GITLEAKS_VERSION="8.30.1"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"
tools_dir="${SOP_SECURITY_TOOLS_DIR:-${TMPDIR:-/tmp}/sop-security-tools}"
python_bin="${SOP_SECURITY_PYTHON:-python3}"

die() {
  echo "security-integration: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# Resolve an operator-provided binary or install the pinned version into the
# isolated tools directory. An operator-provided binary must match the pin.
semgrep_bin="${SOP_SECURITY_SEMGREP_BIN:-}"
if [ -z "$semgrep_bin" ]; then
  semgrep_bin="$tools_dir/semgrep-$SEMGREP_VERSION-venv/bin/semgrep"
  if [ ! -x "$semgrep_bin" ]; then
    require_cmd "$python_bin"
    require_cmd curl
    py_version="$("$python_bin" -c 'import sys; print("%d.%d" % sys.version_info[:2])')"
    case "$py_version" in
      3.9|3.10|3.11) ;;
      *) die "Semgrep $SEMGREP_VERSION wheels support Python 3.9-3.11; found $py_version (set SOP_SECURITY_PYTHON or SOP_SECURITY_SEMGREP_BIN)" ;;
    esac
    mkdir -p "$tools_dir"
    venv="$tools_dir/semgrep-$SEMGREP_VERSION-venv"
    rm -rf "$venv"
    "$python_bin" -m venv "$venv"
    # The lockfile pins Semgrep and every transitive dependency with SHA-256
    # hashes; pip refuses to install a file that does not match its manifest.
    lockfile="$root/scripts/security/semgrep-requirements.lock"
    [ -f "$lockfile" ] || die "missing Semgrep lockfile: $lockfile"
    "$venv/bin/pip" install --disable-pip-version-check --quiet --require-hashes -r "$lockfile"
  fi
fi

gitleaks_bin="${SOP_SECURITY_GITLEAKS_BIN:-}"
if [ -z "$gitleaks_bin" ]; then
  gitleaks_bin="$tools_dir/gitleaks-$GITLEAKS_VERSION/gitleaks"
  if [ ! -x "$gitleaks_bin" ]; then
    require_cmd tar
    require_cmd python3
    # Pinned from the v8.30.1 release's gitleaks_8.30.1_checksums.txt; bump with GITLEAKS_VERSION.
    case "$(uname -s)/$(uname -m)" in
      Linux/x86_64|Linux/amd64) asset=linux_x64 expected=551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb ;;
      Linux/arm64|Linux/aarch64) asset=linux_arm64 expected=e4a487ee7ccd7d3a7f7ec08657610aa3606637dab924210b3aee62570fb4b080 ;;
      Darwin/x86_64) asset=darwin_x64 expected=dfe101a4db2255fc85120ac7f3d25e4342c3c20cf749f2c20a18081af1952709 ;;
      Darwin/arm64) asset=darwin_arm64 expected=b40ab0ae55c505963e365f271a8d3846efbc170aa17f2607f13df610a9aeb6a5 ;;
      *) die "no pinned Gitleaks asset for $(uname -s)/$(uname -m)" ;;
    esac
    asset="gitleaks_${GITLEAKS_VERSION}_${asset}.tar.gz"
    mkdir -p "$tools_dir"
    archive="$tools_dir/$asset"
    "$root/scripts/security/fetch-pinned.sh" "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/${asset}" "$expected" "$archive"
    mkdir -p "$tools_dir/gitleaks-$GITLEAKS_VERSION"
    tar -xzf "$archive" -C "$tools_dir/gitleaks-$GITLEAKS_VERSION"
  fi
fi

actual_semgrep="$("$semgrep_bin" --version 2>/dev/null | sed -n '1p' | tr -d '\r[:space:]')"
# A pip-installed Semgrep can report a newer available version from its update
# check instead of the installed version in some environments. The package
# metadata is authoritative for the hash-pinned venv this script provisions.
semgrep_python="$(dirname "$semgrep_bin")/python"
if [ -x "$semgrep_python" ]; then
  actual_semgrep="$("$semgrep_python" -c 'import importlib.metadata; print(importlib.metadata.version("semgrep"))')"
fi
[ "$actual_semgrep" = "$SEMGREP_VERSION" ] || die "Semgrep version is '$actual_semgrep', want $SEMGREP_VERSION"
actual_gitleaks="$("$gitleaks_bin" version 2>/dev/null | sed -n '1p' | tr -d '\r[:space:]')"
[ "$actual_gitleaks" = "$GITLEAKS_VERSION" ] || die "Gitleaks version is '$actual_gitleaks', want $GITLEAKS_VERSION"

# Provision the pinned cargo-audit binary as an explicit, checksum-verified step.
# The scan itself never fetches the tool or an advisory database.
cargo_audit_version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$root/scripts/security/cargo-audit-manifest.json")"
cargo_audit_dir="$tools_dir/cargo-audit-$cargo_audit_version"
cargo_audit_bin="$cargo_audit_dir/cargo-audit"
if [ ! -x "$cargo_audit_bin" ]; then
  "$root/scripts/security/fetch-cargo-audit.sh" --dest "$cargo_audit_dir" >/dev/null || die "cargo-audit provisioning failed"
fi
actual_cargo_audit="$("$cargo_audit_bin" --version 2>/dev/null | sed -n '1p' | tr -d '\r' | awk '{print $NF}')"
actual_cargo_audit="${actual_cargo_audit#v}"
[ "$actual_cargo_audit" = "$cargo_audit_version" ] || die "cargo-audit version is '$actual_cargo_audit', want $cargo_audit_version"

# Provision the pinned OSV-Scanner binary. The real-CLI OSV tests build a
# controlled offline database fixture, so this step never downloads production
# OSV data and never depends on the live OSV API.
osv_version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$root/scripts/security/osv-scanner-manifest.json")"
osv_dir="$tools_dir/osv-scanner-$osv_version"
osv_bin="$osv_dir/osv-scanner"
if [ ! -x "$osv_bin" ]; then
  "$root/scripts/security/fetch-osv-scanner.sh" --dest "$osv_dir" >/dev/null || die "OSV-Scanner provisioning failed"
fi
actual_osv="$("$osv_bin" --version 2>/dev/null | sed -n '1p' | tr -d '\r' | awk '{print $NF}')"
actual_osv="${actual_osv#v}"
[ "$actual_osv" = "$osv_version" ] || die "OSV-Scanner version is '$actual_osv', want $osv_version"

# Verify the pinned production rules as well as the scanner binaries. The rules
# are fetched from the exact upstream commit recorded in the manifest and checked
# against per-file and directory digests; they are never committed.
rules_dir="$tools_dir/semgrep-rules"
"$root/scripts/security/fetch-semgrep-rules.sh" --dest "$rules_dir" >/dev/null || die "production rules fetch/verify failed"

echo "security-integration: semgrep=$actual_semgrep gitleaks=$actual_gitleaks cargo-audit=$actual_cargo_audit osv-scanner=$actual_osv"

cd "$root"
SOP_SECURITY_SEMGREP_BIN="$semgrep_bin" \
SOP_SECURITY_GITLEAKS_BIN="$gitleaks_bin" \
SOP_SECURITY_INTEGRATION_REQUIRED=1 \
SOP_SECURITY_RULES_DIR="$rules_dir" \
SOP_SECURITY_CARGO_BIN="$cargo_audit_bin" \
SOP_SECURITY_CARGO_AUDIT_VERSION="$cargo_audit_version" \
SOP_SECURITY_CARGO_ADVISORY_DB="$root/internal/security/testdata/cargo-audit/advisory-db" \
SOP_SECURITY_OSV_BIN="$osv_bin" \
SOP_SECURITY_OSV_VERSION="$osv_version" \
  go test -tags=security_integration -count=1 -v ./internal/security/ ./internal/cli/

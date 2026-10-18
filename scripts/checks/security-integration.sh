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
    require_cmd curl
    require_cmd tar
    require_cmd "$python_bin"
    case "$(uname -s)" in
      Linux) os=linux ;;
      Darwin) os=darwin ;;
      *) die "unsupported operating system: $(uname -s)" ;;
    esac
    case "$(uname -m)" in
      x86_64|amd64) arch=x64 ;;
      arm64|aarch64) arch=arm64 ;;
      *) die "unsupported architecture: $(uname -m)" ;;
    esac
    mkdir -p "$tools_dir"
    archive="$tools_dir/gitleaks_${GITLEAKS_VERSION}_${os}_${arch}.tar.gz"
    curl -fsSL "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_${os}_${arch}.tar.gz" -o "$archive"
    checksums="$tools_dir/gitleaks_${GITLEAKS_VERSION}_checksums.txt"
    curl -fsSL "https://github.com/gitleaks/gitleaks/releases/download/v${GITLEAKS_VERSION}/gitleaks_${GITLEAKS_VERSION}_checksums.txt" -o "$checksums"
    asset="gitleaks_${GITLEAKS_VERSION}_${os}_${arch}.tar.gz"
    expected="$(awk -v asset="$asset" '$2 == asset { print $1 }' "$checksums")"
    [ -n "$expected" ] || die "no published checksum for $asset"
    actual="$("$python_bin" -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$archive")"
    [ "$expected" = "$actual" ] || die "checksum mismatch for $asset: want $expected, got $actual"
    mkdir -p "$tools_dir/gitleaks-$GITLEAKS_VERSION"
    tar -xzf "$archive" -C "$tools_dir/gitleaks-$GITLEAKS_VERSION"
  fi
fi

actual_semgrep="$("$semgrep_bin" --version 2>/dev/null | head -n 1 | tr -d '\r[:space:]')"
[ "$actual_semgrep" = "$SEMGREP_VERSION" ] || die "Semgrep version is '$actual_semgrep', want $SEMGREP_VERSION"
actual_gitleaks="$("$gitleaks_bin" version 2>/dev/null | head -n 1 | tr -d '\r[:space:]')"
[ "$actual_gitleaks" = "$GITLEAKS_VERSION" ] || die "Gitleaks version is '$actual_gitleaks', want $GITLEAKS_VERSION"

# Verify the pinned production rules as well as the scanner binaries. The rules
# are fetched from the exact upstream commit recorded in the manifest and checked
# against per-file and directory digests; they are never committed.
rules_dir="$tools_dir/semgrep-rules"
"$root/scripts/security/fetch-semgrep-rules.sh" --dest "$rules_dir" >/dev/null || die "production rules fetch/verify failed"

echo "security-integration: semgrep=$actual_semgrep gitleaks=$actual_gitleaks"

cd "$root"
SOP_SECURITY_SEMGREP_BIN="$semgrep_bin" \
SOP_SECURITY_GITLEAKS_BIN="$gitleaks_bin" \
SOP_SECURITY_INTEGRATION_REQUIRED=1 \
  go test -tags=security_integration -count=1 -v ./internal/security/ ./internal/cli/

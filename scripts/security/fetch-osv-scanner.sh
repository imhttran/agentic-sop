#!/usr/bin/env bash
# Provision the pinned OSV-Scanner executable and verify its integrity.
#
# This is an explicit provisioning action: it is the only place that downloads
# OSV-Scanner. Scans never fetch the tool or a database, and never use the live
# OSV API. An operator supplies the offline database cache separately with
# scripts/security/fetch-osv-databases.sh.
#
# Usage:
#   scripts/security/fetch-osv-scanner.sh [--dest DIR]
#
# Environment:
#   SOP_SECURITY_OSV_DIR   destination (default: .agent-sdlc/osv-scanner)
#
# The committed manifest pins the release tag, supported targets, and each
# platform's binary SHA-256. A mismatch fails the fetch. An unsupported platform
# is a hard error rather than a silent download of something unverified.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"
manifest="$root/scripts/security/osv-scanner-manifest.json"
dest="${SOP_SECURITY_OSV_DIR:-$root/.agent-sdlc/osv-scanner}"

while [ $# -gt 0 ]; do
  case "$1" in
    --dest)
      [ $# -ge 2 ] || { echo "fetch-osv-scanner: --dest requires a path" >&2; exit 2; }
      dest="$2"
      shift 2
      ;;
    *)
      echo "usage: fetch-osv-scanner.sh [--dest DIR]" >&2
      exit 2
      ;;
  esac
done

die() {
  echo "fetch-osv-scanner: $*" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || die "python3 is required"
command -v curl >/dev/null 2>&1 || die "curl is required"
[ -f "$manifest" ] || die "missing manifest: $manifest"

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported operating system: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac
target="${os}_${arch}"
case "$target" in
  linux_amd64|linux_arm64|darwin_amd64|darwin_arm64) ;;
  *) die "no pinned OSV-Scanner asset for $target; add one to $manifest before provisioning" ;;
esac

version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$manifest")"
tag="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["release_tag"])' "$manifest")"
expected="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["assets"][sys.argv[2]]["sha256"])' "$manifest" "$target")"
[ -n "$expected" ] || die "no pinned SHA-256 for target $target"

asset="osv-scanner_${target}"
url="https://github.com/google/osv-scanner/releases/download/${tag}/${asset}"

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-osv-scanner.XXXXXX")"
trap 'rm -rf "$work"' EXIT

binary="$work/$asset"
curl -fsSL --max-time 300 "$url" -o "$binary" || die "download failed: $url"
actual="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$binary")"
[ "$actual" = "$expected" ] || die "checksum mismatch for $asset: want $expected, got $actual"
chmod 0755 "$binary"

reported="$("$binary" --version 2>/dev/null | head -n 1 | tr -d '\r' | awk '{print $NF}')"
reported="${reported#v}"
[ "$reported" = "$version" ] || die "provisioned osv-scanner reports version '$reported', want $version"

rm -rf "$dest"
mkdir -p "$dest"
cp "$binary" "$dest/osv-scanner"
chmod 0755 "$dest/osv-scanner"

echo "osv-scanner=${version} target=${target} sha256=${expected} path=${dest}/osv-scanner"

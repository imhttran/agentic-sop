#!/usr/bin/env bash
# Provision the pinned cargo-audit executable and verify its integrity.
#
# This is an explicit provisioning action: it is the only place that downloads
# cargo-audit. Scans never fetch it, and scans never fetch or update an advisory
# database; an operator supplies the local database path.
#
# Usage:
#   scripts/security/fetch-cargo-audit.sh [--dest DIR]
#
# Environment:
#   SOP_SECURITY_CARGO_AUDIT_DIR   destination (default: .agent-sdlc/cargo-audit)
#
# The committed manifest pins the release tag, the supported targets, and each
# target's tarball SHA-256. A mismatch fails the fetch. An unsupported platform
# is a hard error rather than a silent download of something unverified.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"
manifest="$root/scripts/security/cargo-audit-manifest.json"
dest="${SOP_SECURITY_CARGO_AUDIT_DIR:-$root/.agent-sdlc/cargo-audit}"

while [ $# -gt 0 ]; do
  case "$1" in
    --dest)
      [ $# -ge 2 ] || { echo "fetch-cargo-audit: --dest requires a path" >&2; exit 2; }
      dest="$2"
      shift 2
      ;;
    *)
      echo "usage: fetch-cargo-audit.sh [--dest DIR]" >&2
      exit 2
      ;;
  esac
done

die() {
  echo "fetch-cargo-audit: $*" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || die "python3 is required"
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
[ -f "$manifest" ] || die "missing manifest: $manifest"

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
case "$os/$arch" in
  linux/x64) target=x86_64-unknown-linux-gnu ;;
  darwin/arm64) target=aarch64-apple-darwin ;;
  *) die "no pinned cargo-audit asset for $os/$arch; add one to $manifest before provisioning" ;;
esac

version="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["version"])' "$manifest")"
tag="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["release_tag"])' "$manifest")"
expected="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["assets"][sys.argv[2]]["sha256"])' "$manifest" "$target")"
[ -n "$expected" ] || die "no pinned SHA-256 for target $target"

asset="cargo-audit-${target}-v${version}.tgz"
# The release tag contains a slash; URL-encode it so curl does not reinterpret it.
encoded_tag="$(python3 -c 'import sys,urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' "$tag")"
url="https://github.com/RustSec/rustsec/releases/download/${encoded_tag}/${asset}"

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-cargo-audit.XXXXXX")"
trap 'rm -rf "$work"' EXIT

archive="$work/$asset"
curl -fsSL --max-time 180 "$url" -o "$archive" || die "download failed: $url"
actual="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$archive")"
[ "$actual" = "$expected" ] || die "checksum mismatch for $asset: want $expected, got $actual"

extracted="$work/extracted"
mkdir -p "$extracted"
tar -xzf "$archive" -C "$extracted" || die "tarball extraction failed"
binary="$extracted/cargo-audit-${target}-v${version}/cargo-audit"
[ -x "$binary" ] || die "unexpected tarball layout (missing cargo-audit binary)"

reported="$("$binary" --version 2>/dev/null | head -n 1 | tr -d '\r' | awk '{print $NF}')"
reported="${reported#v}"
[ "$reported" = "$version" ] || die "provisioned cargo-audit reports version '$reported', want $version"

rm -rf "$dest"
mkdir -p "$dest"
cp "$binary" "$dest/cargo-audit"
cp "$extracted/cargo-audit-${target}-v${version}/LICENSE-MIT" "$dest/" 2>/dev/null || true
cp "$extracted/cargo-audit-${target}-v${version}/LICENSE-APACHE" "$dest/" 2>/dev/null || true
chmod 0755 "$dest/cargo-audit"

echo "cargo-audit=${version} target=${target} sha256=${expected} path=${dest}/cargo-audit"

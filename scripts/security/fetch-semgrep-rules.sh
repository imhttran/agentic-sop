#!/usr/bin/env bash
# Fetch the pinned Semgrep production rules and verify their integrity.
#
# The rules are used locally under the Semgrep Rules License. They are fetched
# from an exact upstream commit and are never committed to this repository.
#
# Usage:
#   scripts/security/fetch-semgrep-rules.sh [--dest DIR]
#
# Environment:
#   SOP_SECURITY_RULES_DIR   destination (default: .agent-sdlc/security-rules)
#
# The committed manifest records the upstream commit, the tarball SHA-256, each
# selected rule file's SHA-256, and the deterministic directory digest produced
# by security/rulesIdentity. A mismatch fails the fetch.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"
manifest="$root/scripts/security/semgrep-rules-manifest.json"
dest="${SOP_SECURITY_RULES_DIR:-$root/.agent-sdlc/security-rules}"

while [ $# -gt 0 ]; do
  case "$1" in
    --dest)
      [ $# -ge 2 ] || { echo "fetch-semgrep-rules: --dest requires a path" >&2; exit 2; }
      dest="$2"
      shift 2
      ;;
    *)
      echo "usage: fetch-semgrep-rules.sh [--dest DIR]" >&2
      exit 2
      ;;
  esac
done

die() {
  echo "fetch-semgrep-rules: $*" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || die "python3 is required"
command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"
[ -f "$manifest" ] || die "missing manifest: $manifest"

commit="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["commit"])' "$manifest")"
expected_tar="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["tarball_sha256"])' "$manifest")"
archive_url="https://github.com/semgrep/semgrep-rules/archive/$commit.tar.gz"

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-semgrep-rules.XXXXXX")"
trap 'rm -rf "$work"' EXIT

archive="$work/rules.tar.gz"
curl -fsSL --max-time 180 "$archive_url" -o "$archive" || die "download failed: $archive_url"
actual_tar="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$archive")"
[ "$actual_tar" = "$expected_tar" ] || die "tarball checksum mismatch: want $expected_tar, got $actual_tar"

mkdir -p "$work/src"
tar -xzf "$archive" -C "$work/src" || die "tarball extraction failed"
extracted="$work/src/semgrep-rules-$commit"
[ -d "$extracted" ] || die "unexpected tarball layout (missing semgrep-rules-$commit)"

rm -rf "$dest"
mkdir -p "$dest"
python3 - "$manifest" "$extracted" "$dest" <<'PY'
import hashlib
import json
import pathlib
import shutil
import sys

manifest_path, source_root, dest_root = map(pathlib.Path, sys.argv[1:4])
manifest = json.loads(manifest_path.read_text())
entries = manifest["files"]
directories = manifest["directories"]

h = hashlib.sha256()
for entry in sorted(entries, key=lambda e: e["path"]):
    rel = entry["path"]
    relative = pathlib.Path(rel)
    if not any(relative.as_posix() == d or relative.as_posix().startswith(d + "/") for d in directories):
        raise SystemExit(f"manifest path outside selected directories: {rel}")
    src = source_root / relative
    if not src.is_file():
        raise SystemExit(f"missing upstream rule file: {rel}")
    data = src.read_bytes()
    actual = hashlib.sha256(data).hexdigest()
    if actual != entry["sha256"]:
        raise SystemExit(f"rule file checksum mismatch for {rel}: want {entry['sha256']}, got {actual}")
    dst = dest_root / relative
    dst.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(src, dst)
    h.update(rel.encode())
    h.update(b"\0")
    h.update(data)
    h.update(b"\0")

actual_dir = h.hexdigest()
if actual_dir != manifest["rules_dir_sha256"]:
    raise SystemExit(
        f"rules directory digest mismatch: want {manifest['rules_dir_sha256']}, got {actual_dir}"
    )
print(f"rules={actual_dir} files={len(entries)} commit={manifest['commit']}")
PY

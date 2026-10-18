#!/usr/bin/env bash
# Provision the OSV offline databases used by osv-scanner.
#
# This is an explicit provisioning action and the only step that downloads OSV
# data. A scan never downloads or refreshes a database, never falls back to the
# live OSV API, and never treats a missing ecosystem as clean.
#
# Usage:
#   scripts/security/fetch-osv-databases.sh [--dest DIR] [--ecosystem NAME]...
#
# Default ecosystems: Go, crates.io (the two OSV supports for this add-on).
# Environment:
#   SOP_SECURITY_OSV_DB_DIR   destination cache root
#                             (default: .agent-sdlc/osv-db)
#
# The script writes the OSV v2.7 layout:
#   <dest>/osv-scalibr/<ecosystem>/all.zip
# and records the source URL, SHA-256, size, and acquisition time beside each
# archive. The digest is local acquisition evidence, not a pinned upstream
# version: OSV updates the database continuously.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"
dest="${SOP_SECURITY_OSV_DB_DIR:-$root/.agent-sdlc/osv-db}"
ecosystems=()

while [ $# -gt 0 ]; do
  case "$1" in
    --dest)
      [ $# -ge 2 ] || { echo "fetch-osv-databases: --dest requires a path" >&2; exit 2; }
      dest="$2"
      shift 2
      ;;
    --ecosystem)
      [ $# -ge 2 ] || { echo "fetch-osv-databases: --ecosystem requires a name" >&2; exit 2; }
      ecosystems+=("$2")
      shift 2
      ;;
    *)
      echo "usage: fetch-osv-databases.sh [--dest DIR] [--ecosystem NAME]..." >&2
      exit 2
      ;;
  esac
done
if [ ${#ecosystems[@]} -eq 0 ]; then
  ecosystems=(Go crates.io)
fi

die() {
  echo "fetch-osv-databases: $*" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || die "python3 is required"
command -v curl >/dev/null 2>&1 || die "curl is required"

for ecosystem in "${ecosystems[@]}"; do
  case "$ecosystem" in
    Go|crates.io) ;;
    *) die "unsupported ecosystem '$ecosystem' (want Go, crates.io)" ;;
  esac
done

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-osv-db.XXXXXX")"
trap 'rm -rf "$work"' EXIT

for ecosystem in "${ecosystems[@]}"; do
  dir="$dest/osv-scalibr/$ecosystem"
  mkdir -p "$dir"
  archive="$dir/all.zip"
  url="https://osv-vulnerabilities.storage.googleapis.com/${ecosystem}/all.zip"
  curl -fsSL --max-time 300 "$url" -o "$work/all.zip" || die "download failed: $url"
  python3 - "$work/all.zip" <<'PY' || die "downloaded archive is not a valid zip"
import sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as z:
    bad = z.testzip()
    if bad is not None:
        raise SystemExit("corrupt member: " + bad)
PY
  digest="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$work/all.zip")"
  size="$(python3 -c 'import os,sys; print(os.path.getsize(sys.argv[1]))' "$work/all.zip")"
  downloaded_at="$(python3 -c 'import datetime; print(datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00","Z"))')"
  mv "$work/all.zip" "$archive"
  python3 - "$archive.metadata.json" "$ecosystem" "$url" "$digest" "$size" "$downloaded_at" <<'PY'
import json, sys
path, ecosystem, source, digest, size, downloaded_at = sys.argv[1:]
with open(path, "w", encoding="utf-8") as f:
    json.dump({
        "ecosystem": ecosystem,
        "source": source,
        "sha256": digest,
        "size": int(size),
        "downloaded_at": downloaded_at,
    }, f, indent=2, sort_keys=True)
    f.write("\n")
PY
  echo "osv-database ecosystem=${ecosystem} sha256=${digest} size=${size} path=${archive}"
done

#!/usr/bin/env bash
# Verify the offline OSV database that `sop security scan` reads.
#
# Provisioning is a separate, explicit action (scripts/security/fetch-osv-databases.sh);
# a scan never downloads. This checker answers, without network access, whether the
# provisioned database is present, intact and fresh enough to be evidence.
#
# It fails closed on all four degraded states the scan also rejects:
#   missing  — no osv-scalibr layout for a required ecosystem
#   stale    — acquisition older than the freshness boundary (default 90 days)
#   corrupt  — archive size or digest disagrees with its acquisition metadata
#   invalid  — acquisition metadata absent or unparseable
#
# Usage:
#   scripts/checks/osv-db-verify.sh [--db DIR] [--ecosystem NAME]... [--max-age-days N]
#
# Environment:
#   SOP_SECURITY_OSV_DB_DIR        database root (default: .agent-sdlc/osv-db)
#   SOP_SECURITY_OSV_MAX_AGE_DAYS  freshness boundary (default: 90, matching
#                                  osvScannerStaleMaxAge in internal/security/osv.go)

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
db="${SOP_SECURITY_OSV_DB_DIR:-$root/.agent-sdlc/osv-db}"
max_days="${SOP_SECURITY_OSV_MAX_AGE_DAYS:-90}"
ecosystems=()

while [ $# -gt 0 ]; do
  case "$1" in
    --db)
      [ $# -ge 2 ] || { echo "osv-db-verify: --db requires a path" >&2; exit 2; }
      db="$2"; shift 2 ;;
    --ecosystem)
      [ $# -ge 2 ] || { echo "osv-db-verify: --ecosystem requires a name" >&2; exit 2; }
      ecosystems+=("$2"); shift 2 ;;
    --max-age-days)
      [ $# -ge 2 ] || { echo "osv-db-verify: --max-age-days requires a number" >&2; exit 2; }
      max_days="$2"; shift 2 ;;
    -h|--help)
      sed -n '2,26p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
      exit 0 ;;
    *)
      echo "osv-db-verify: unknown argument $1" >&2; exit 2 ;;
  esac
done

if [ ${#ecosystems[@]} -eq 0 ]; then
  ecosystems=(Go crates.io)
fi

if [ ! -d "$db" ]; then
  echo "osv-db-verify: FAIL missing database directory: $db" >&2
  echo "  provision it with scripts/security/fetch-osv-databases.sh" >&2
  exit 1
fi

python3 - "$db" "$max_days" "${ecosystems[@]}" <<'PY'
import datetime
import hashlib
import json
import os
import sys

db, max_days = sys.argv[1], int(sys.argv[2])
ecosystems = sys.argv[3:]
now = datetime.datetime.now(datetime.timezone.utc)
failures = []
rows = []

for eco in ecosystems:
    archive = os.path.join(db, "osv-scalibr", eco, "all.zip")
    meta_path = archive + ".metadata.json"
    if not os.path.isfile(archive):
        failures.append(f"missing {eco}: {archive}")
        continue
    if not os.path.isfile(meta_path):
        failures.append(f"invalid {eco}: acquisition metadata absent ({meta_path})")
        continue
    try:
        with open(meta_path) as fh:
            meta = json.load(fh)
    except Exception as exc:
        failures.append(f"invalid {eco}: metadata unreadable ({exc})")
        continue

    size = os.path.getsize(archive)
    digest = hashlib.sha256()
    with open(archive, "rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 20), b""):
            digest.update(chunk)
    actual = digest.hexdigest()

    if meta.get("size") not in (None, size):
        failures.append(f"corrupt {eco}: size {size} != metadata {meta.get('size')}")
    if meta.get("sha256") not in (None, actual):
        failures.append(f"corrupt {eco}: digest {actual[:12]}... != metadata {str(meta.get('sha256'))[:12]}...")

    stamp = meta.get("downloaded_at", "")
    try:
        acquired = datetime.datetime.fromisoformat(stamp.replace("Z", "+00:00"))
    except Exception:
        failures.append(f"invalid {eco}: downloaded_at unparseable ({stamp!r})")
        continue
    age = (now - acquired).days
    if age > max_days:
        failures.append(f"stale {eco}: acquired {stamp} ({age} days > {max_days})")
    rows.append((eco, size, age, actual[:12]))

for eco, size, age, short in rows:
    print(f"osv-db-verify: {eco}: {size} bytes, age {age}d, sha256 {short}...")

if failures:
    for item in failures:
        print(f"osv-db-verify: FAIL {item}", file=sys.stderr)
    print(f"osv-db-verify: FAILED ({len(failures)} problem(s))", file=sys.stderr)
    sys.exit(1)

print(f"osv-db-verify: OK ({len(rows)} ecosystem(s), max age {max_days} days)")
PY

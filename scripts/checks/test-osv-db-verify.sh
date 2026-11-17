#!/usr/bin/env bash
# Hermetic tests for scripts/checks/osv-db-verify.sh.
#
# Builds synthetic databases in a temp directory (no network, no production data)
# and asserts the checker's exit status for every state the scan also rejects:
# valid, missing, absent, stale, corrupt and invalid metadata, plus the freshness
# boundary override.
#
# Usage:
#   scripts/checks/test-osv-db-verify.sh

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
checker="$root/scripts/checks/osv-db-verify.sh"
[ -x "$checker" ] || { echo "test-osv-db-verify: checker not executable: $checker" >&2; exit 1; }

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-osv-verify-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT

build() {
  local dest="$1" mode="$2"
  mkdir -p "$dest"
  python3 - "$dest" "$mode" <<'PY'
import datetime, hashlib, json, os, sys, zipfile

dest, mode = sys.argv[1], sys.argv[2]
now = datetime.datetime.now(datetime.timezone.utc)
for eco in ("Go", "crates.io"):
    d = os.path.join(dest, "osv-scalibr", eco)
    os.makedirs(d, exist_ok=True)
    path = os.path.join(d, "all.zip")
    with zipfile.ZipFile(path, "w") as z:
        z.writestr("advisory.json", json.dumps({"id": f"FIXTURE-{eco}"}))
    data = open(path, "rb").read()
    meta = {
        "downloaded_at": (now - datetime.timedelta(days=200 if mode == "stale" else 1)).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "ecosystem": eco,
        "sha256": hashlib.sha256(data).hexdigest(),
        "size": len(data),
        "source": "synthetic-fixture",
    }
    if mode == "corrupt":
        meta["sha256"] = "0" * 64
    if mode == "invalid":
        with open(path + ".metadata.json", "w") as fh:
            fh.write("{not json")
        continue
    with open(path + ".metadata.json", "w") as fh:
        json.dump(meta, fh)
PY
}

failures=0
expect() {
  local name="$1" want="$2" dir="$3"
  shift 3
  local got=0
  "$checker" --db "$dir" "$@" >/dev/null 2>&1 || got=$?
  if [ "$got" = "$want" ]; then
    printf 'test-osv-db-verify: %-28s exit=%s\n' "$name" "$got"
  else
    printf 'test-osv-db-verify: %-28s FAIL exit=%s want=%s\n' "$name" "$got" "$want" >&2
    failures=$((failures + 1))
  fi
}

build "$work/valid" valid
build "$work/stale" stale
build "$work/corrupt" corrupt
build "$work/invalid" invalid
mkdir -p "$work/missing"

expect "valid database" 0 "$work/valid"
expect "missing layout" 1 "$work/missing"
expect "absent directory" 1 "$work/absent"
expect "stale database" 1 "$work/stale"
expect "corrupt database" 1 "$work/corrupt"
expect "invalid metadata" 1 "$work/invalid"
expect "boundary override" 1 "$work/valid" --max-age-days 0

if [ "$failures" -ne 0 ]; then
  echo "test-osv-db-verify: FAILED ($failures case(s))" >&2
  exit 1
fi
echo "test-osv-db-verify: OK (7 cases)"

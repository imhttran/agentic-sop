#!/usr/bin/env bash
# fetch-pinned.sh — download URL to DEST and verify it against a committed
# SHA-256 pin. With SOP_SECURITY_DOWNLOAD_CACHE set, the cache is keyed by that
# pin: a hit is copied instead of downloaded, and gets the same checksum check,
# so a cached file is never trusted more than a fresh download.
#
# Usage: fetch-pinned.sh URL SHA256 DEST [CURL_MAX_TIME_SECONDS]
set -euo pipefail

die() {
  echo "fetch-pinned: $*" >&2
  exit 1
}

[ "$#" -ge 3 ] || die "usage: fetch-pinned.sh URL SHA256 DEST [CURL_MAX_TIME_SECONDS]"
url="$1"
expected="$2"
dest="$3"
max_time="${4:-300}"
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || die "pin for $url is not a lowercase SHA-256: '$expected'"

cache="${SOP_SECURITY_DOWNLOAD_CACHE:-}"
cached=""
if [ -n "$cache" ] && [ -f "$cache/$expected" ]; then
  cp "$cache/$expected" "$dest"
  cached=" (cached at $cache/$expected; delete it to re-download)"
else
  command -v curl >/dev/null 2>&1 || die "curl is required"
  # No --retry-delay: curl backs off 1s, 2s, 4s, ... so 8 retries span ~4 minutes,
  # long enough to ride out the intermittent 504s from GitHub release downloads.
  curl -fsSL --retry 8 --retry-all-errors --max-time "$max_time" "$url" -o "$dest" || die "download failed: $url"
fi

actual="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$dest")"
[ "$actual" = "$expected" ] || die "checksum mismatch for $url: want $expected, got $actual$cached"

if [ -n "$cache" ] && [ -z "$cached" ]; then
  mkdir -p "$cache"
  cp "$dest" "$cache/$expected"
fi

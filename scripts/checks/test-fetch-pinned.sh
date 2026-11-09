#!/usr/bin/env bash
# Self-check for scripts/security/fetch-pinned.sh: a fresh download is verified
# and cached, a cache hit is used without the network and re-verified, and a
# tampered cache entry or a wrong pin fails closed. Uses file:// URLs only.
#
# Usage: scripts/checks/test-fetch-pinned.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fetch="$root/scripts/security/fetch-pinned.sh"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/sop-fetch-pinned.XXXXXX")"
trap 'rm -rf "$tmp"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

printf 'pinned artifact\n' >"$tmp/src"
pin="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$tmp/src")"
export SOP_SECURITY_DOWNLOAD_CACHE="$tmp/cache"

"$fetch" "file://$tmp/src" "$pin" "$tmp/out1" || fail "fresh download"
cmp -s "$tmp/src" "$tmp/out1" || fail "fresh download content"
cmp -s "$tmp/src" "$tmp/cache/$pin" || fail "fresh download was not cached"

rm "$tmp/src"
"$fetch" "file://$tmp/src" "$pin" "$tmp/out2" || fail "cache hit with the source gone"
cmp -s "$tmp/cache/$pin" "$tmp/out2" || fail "cache hit content"

printf 'tampered\n' >"$tmp/cache/$pin"
if "$fetch" "file://$tmp/src" "$pin" "$tmp/out3" 2>"$tmp/err"; then fail "tampered cache entry was accepted"; fi
grep -q 'checksum mismatch.*cached' "$tmp/err" || fail "tampered cache error does not name the cache: $(cat "$tmp/err")"

rm "$tmp/cache/$pin"
printf 'other\n' >"$tmp/src2"
if "$fetch" "file://$tmp/src2" "$pin" "$tmp/out4" 2>/dev/null; then fail "wrong pin was accepted"; fi
[ ! -e "$tmp/cache/$pin" ] || fail "unverified download was cached"

echo "test-fetch-pinned: ok"

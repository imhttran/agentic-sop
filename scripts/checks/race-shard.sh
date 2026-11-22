#!/usr/bin/env bash
# race-shard.sh N      print the packages shard N (1 or 2) runs under -race.
# race-shard.sh --check  fail unless the two shards together cover every package once.
#
# The heavy packages are pinned so the shards stay balanced; every other package,
# including any added later, alternates between the shards, so none can be missed.
# shortcut: weights are hand-picked from CI runs (contention skews them); re-pin if a shard drifts.
# Last balanced from run 44970926414: per-package seconds summed 574 vs 288 (shard 1 ran ~300 s,
# shard 2 ~180 s); moving dqharness and dist to shard 2 gives about 429 vs 433.
set -euo pipefail

shard() {
  go list ./... | awk -v mod="$(go list -m)" -v n="$1" '
    BEGIN {
      split("internal/cli internal/retrievalgate", a, " ");  for (i in a) pin[mod "/" a[i]] = 1
      split("internal/ollamaagent internal/toolharness internal/dqharness internal/dist", b, " ");                              for (i in b) pin[mod "/" b[i]] = 2
    }
    { s = ($0 in pin) ? pin[$0] : (++k % 2) + 1; if (s == n) print }'
}

case "${1:-}" in
  1|2) shard "$1" ;;
  --check)
    want="$(go list ./... | sort)"
    got="$( { shard 1; shard 2; } | sort)"
    [ -n "$want" ] && [ "$want" = "$got" ] || { echo "race-shard: shards do not cover every package exactly once" >&2; exit 1; }
    ;;
  *) echo "usage: race-shard.sh 1|2|--check" >&2; exit 2 ;;
esac

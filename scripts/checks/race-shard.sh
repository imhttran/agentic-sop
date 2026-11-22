#!/usr/bin/env bash
# race-shard.sh N      print the packages shard N (1, 2 or 3) runs under -race.
# race-shard.sh --check  fail unless the three shards together cover every package once.
#
# The heavy packages are pinned so the shards stay balanced; every other package,
# including any added later, alternates between shards 2 and 3, so none can be missed.
# internal/cli alone is ~213 s under -race, the floor for any shard, so it runs alone.
# shortcut: weights are hand-picked from CI runs (contention skews them); re-pin if a shard drifts.
# Last balanced from run 44970926414 (per-package seconds): cli 213 | toolharness 94,
# ollamaagent 89, commandprofile 29, expharness 20 | retrievalgate 81, dqharness 81,
# dist 64, controller/web 22; the remaining ~170 s alternate, so about 213 vs 312 vs 338 summed.
set -euo pipefail

shard() {
  go list ./... | awk -v mod="$(go list -m)" -v n="$1" '
    BEGIN {
      split("internal/cli", a, " ");                                                                  for (i in a) pin[mod "/" a[i]] = 1
      split("internal/toolharness internal/ollamaagent internal/decision/commandprofile internal/expharness", b, " "); for (i in b) pin[mod "/" b[i]] = 2
      split("internal/retrievalgate internal/dqharness internal/dist internal/controller/web", c, " ");                       for (i in c) pin[mod "/" c[i]] = 3
    }
    { s = ($0 in pin) ? pin[$0] : (++k % 2) + 2; if (s == n) print }'
}

case "${1:-}" in
  1|2|3) shard "$1" ;;
  --check)
    want="$(go list ./... | sort)"
    got="$( { shard 1; shard 2; shard 3; } | sort)"
    [ -n "$want" ] && [ "$want" = "$got" ] || { echo "race-shard: shards do not cover every package exactly once" >&2; exit 1; }
    ;;
  *) echo "usage: race-shard.sh 1|2|3|--check" >&2; exit 2 ;;
esac

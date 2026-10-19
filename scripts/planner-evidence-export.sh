#!/usr/bin/env bash
#
# Export a sanitized evidence bundle from a planner live run.
#
# Copies a fixed list of files from a planner fixture (and, optionally, its log
# directory) into docs/reports/evidence/artifacts/<name>/ in an agentic-sop
# checkout, replaces machine-specific paths with placeholders, refuses to write
# the bundle if anything looks like a credential or a leftover local path, and
# writes a SHA-256 manifest and a README. The fixture is only read, never changed.
#
# Usage:
#   scripts/planner-evidence-export.sh [--logs DIR] [--binary PATH] [--name NAME]
#                                      [--checkout PATH] [--replace] FIXTURE_DIR
#
#   FIXTURE_DIR     the planner fixture the live run used (contains .agent-sdlc)
#   --logs DIR      directory with the run's logs (*.log, *.txt) to include
#   --binary PATH   the sop binary the run used; its version and SHA-256 are recorded
#   --name NAME     bundle directory name (default: planner-2q-live)
#   --checkout PATH the agentic-sop checkout to write into (default: this script's)
#   --replace       replace an existing bundle directory
#
# Example, after the three rounds of the two-question demo:
#   scripts/planner-evidence-export.sh --logs "$TMPDIR/planner-2q-logs" \
#     --binary "$TMPDIR/sop-2q" "$TMPDIR/planner-2q-live"

set -euo pipefail

usage() { sed -n '/^# Usage:/,/^#   --replace/p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }
die() { echo "export: $*" >&2; exit 1; }

LOGS=""
BINARY=""
NAME="planner-2q-live"
CHECKOUT=""
REPLACE=0
FIXTURE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --logs) [ $# -ge 2 ] || usage; LOGS="$2"; shift ;;
    --binary) [ $# -ge 2 ] || usage; BINARY="$2"; shift ;;
    --name) [ $# -ge 2 ] || usage; NAME="$2"; shift ;;
    --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
    --replace) REPLACE=1 ;;
    -h|--help) usage ;;
    -*) echo "export: unknown option $1" >&2; usage ;;
    *) [ -z "$FIXTURE" ] || usage; FIXTURE="$1" ;;
  esac
  shift
done
[ -n "$FIXTURE" ] || usage
case "$NAME" in ""|*/*|.*) die "--name must be a plain directory name: $NAME" ;; esac
command -v perl >/dev/null 2>&1 || die "perl is required for path sanitizing"

# Resolve everything before changing directory, so relative arguments mean what the
# caller typed.
[ -d "$FIXTURE/.agent-sdlc" ] || die "not a SOP fixture (no .agent-sdlc): $FIXTURE"
FIXTURE_LOGICAL="$(cd "$FIXTURE" && pwd)"
FIXTURE_PHYSICAL="$(cd "$FIXTURE" && pwd -P)"
if [ -n "$LOGS" ]; then
  [ -d "$LOGS" ] || die "--logs directory not found: $LOGS"
  LOGS="$(cd "$LOGS" && pwd)"
fi
if [ -n "$BINARY" ]; then
  [ -x "$BINARY" ] || die "--binary is not executable: $BINARY"
  BINARY="$(cd "$(dirname "$BINARY")" && pwd)/$(basename "$BINARY")"
fi
if [ -n "$CHECKOUT" ]; then
  ROOT="$(git -C "$CHECKOUT" rev-parse --show-toplevel 2>/dev/null)" || die "--checkout is not a git checkout: $CHECKOUT"
else
  ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel 2>/dev/null)" ||
    die "run this script from inside the agentic-sop checkout, or pass --checkout"
fi

OUT="$ROOT/docs/reports/evidence/artifacts/$NAME"
if [ -e "$OUT" ]; then
  [ "$REPLACE" = 1 ] || die "bundle already exists: $OUT (pass --replace)"
fi

# Build in a staging directory and move it into place only after every check
# passes, so a refused export never leaves a partial bundle in the checkout.
STAGE="$(mktemp -d "${TMPDIR:-/tmp}/planner-evidence.XXXXXX")"
trap 'rm -rf "$STAGE"' EXIT

# The fixed file list. Required files must exist; optional ones are listed in the
# README when absent.
REQUIRED="
.agent-sdlc/questions.json
.agent-sdlc/runs/T001/outcome.json
choices.txt
normalized.txt
"
OPTIONAL="
PLAN.md
input.txt
normalize_test.go
.agent-sdlc/config.yaml
.agent-sdlc/plan.json
.agent-sdlc/plan.meta.json
.agent-sdlc/runs/T001/state.json
.agent-sdlc/runs/T001/task.md
.agent-sdlc/runs/T001/plan.md
.agent-sdlc/runs/T001/implementation.md
.agent-sdlc/runs/T001/report.md
.agent-sdlc/runs/T001/report.json
.agent-sdlc/runs/T001/gate.json
.agent-sdlc/runs/T001/review.json
.agent-sdlc/runs/T001/validation.json
.agent-sdlc/runs/T001/trace.json
.agent-sdlc/runs/T001/changed-files.json
.agent-sdlc/runs/T001/diff.patch
"
MISSING=""
copy_one() { # copy_one RELPATH REQUIRED(0/1)
  local rel="$1" required="$2" dest
  if [ -f "$FIXTURE_LOGICAL/$rel" ]; then
    # Drop the leading .agent-sdlc/ so the bundle reads like the existing ones.
    dest="${rel#.agent-sdlc/}"
    dest="${dest#runs/T001/}"
    case "$rel" in .agent-sdlc/runs/T001/*) dest="run/$dest" ;; .agent-sdlc/*) dest="state/$dest" ;; *) dest="fixture/$dest" ;; esac
    mkdir -p "$STAGE/$(dirname "$dest")"
    cp "$FIXTURE_LOGICAL/$rel" "$STAGE/$dest"
  elif [ "$required" = 1 ]; then
    die "required file missing from the fixture: $rel"
  else
    MISSING="$MISSING $rel"
  fi
}
for rel in $REQUIRED; do copy_one "$rel" 1; done
for rel in $OPTIONAL; do copy_one "$rel" 0; done
if [ -n "$LOGS" ]; then
  mkdir -p "$STAGE/logs"
  found=0
  for f in "$LOGS"/*.log "$LOGS"/*.txt; do
    [ -f "$f" ] || continue
    cp "$f" "$STAGE/logs/"
    found=1
  done
  [ "$found" = 1 ] || MISSING="$MISSING logs/(no *.log or *.txt in --logs)"
fi

# Replace machine-specific paths with placeholders, longest first so a fixture path
# under $TMPDIR becomes <FIXTURE>, not <TMPDIR>/....
TMP_LOGICAL="${TMPDIR:-/tmp}"; TMP_LOGICAL="${TMP_LOGICAL%/}"
TMP_PHYSICAL="$(cd "$TMP_LOGICAL" 2>/dev/null && pwd -P || echo "$TMP_LOGICAL")"
export EV_FIX_L="$FIXTURE_LOGICAL" EV_FIX_P="$FIXTURE_PHYSICAL" \
  EV_TMP_L="$TMP_LOGICAL" EV_TMP_P="$TMP_PHYSICAL" EV_HOME="${HOME%/}" EV_ROOT="$ROOT"
find "$STAGE" -type f -print0 | while IFS= read -r -d '' f; do
  perl -pi -e '
    s/\Q$ENV{EV_FIX_P}\E/<FIXTURE>/g;
    s/\Q$ENV{EV_FIX_L}\E/<FIXTURE>/g;
    s/\Q$ENV{EV_ROOT}\E/<CHECKOUT>/g if length $ENV{EV_ROOT};
    s/\Q$ENV{EV_TMP_P}\E/<TMPDIR>/g if length $ENV{EV_TMP_P} > 1;
    s/\Q$ENV{EV_TMP_L}\E/<TMPDIR>/g if length $ENV{EV_TMP_L} > 1;
    s/\Q$ENV{EV_HOME}\E/<HOME>/g if length $ENV{EV_HOME} > 1;
  ' "$f"
done

# Refuse the export if anything still looks like a credential or a local path. The
# matched text is never printed, only the file and line.
SECRET_RE='(sk-[A-Za-z0-9_-]{20,}|sess-[A-Za-z0-9]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY|[Bb]earer [A-Za-z0-9._~+/-]{20,}|(api[_-]?key|token|secret|password)["'"'"']?[[:space:]]*[:=][[:space:]]*["'"'"']?[A-Za-z0-9._~+/-]{16,})'
PATH_RE='(/Users/[^/[:space:]]+|/home/[^/[:space:]]+|/private/var/folders|/var/folders/)'
fail=0
if hits="$(grep -rnIE "$SECRET_RE" "$STAGE" | cut -d: -f1,2)" && [ -n "$hits" ]; then
  echo "export: refused: possible credentials at (file:line):" >&2
  printf '%s\n' "$hits" | sed "s#^$STAGE/#  #" >&2
  fail=1
fi
if hits="$(grep -rnIE "$PATH_RE" "$STAGE" | cut -d: -f1,2)" && [ -n "$hits" ]; then
  echo "export: refused: machine-specific paths remain at (file:line):" >&2
  printf '%s\n' "$hits" | sed "s#^$STAGE/#  #" >&2
  fail=1
fi
[ "$fail" = 0 ] || die "nothing was written; remove or redact the source, then rerun"

# Provenance README, then the manifest over everything else.
SOP_VERSION="not recorded"; SOP_SHA="not recorded"
if [ -n "$BINARY" ]; then
  SOP_VERSION="$("$BINARY" version 2>/dev/null | head -1 | perl -pe 's/\Q$ENV{EV_HOME}\E/<HOME>/g' || echo unknown)"
  if command -v shasum >/dev/null 2>&1; then SOP_SHA="$(shasum -a 256 "$BINARY" | awk '{print $1}')"
  else SOP_SHA="$(sha256sum "$BINARY" | awk '{print $1}')"; fi
fi
{
  echo "# Evidence bundle: $NAME"
  echo
  echo "Sanitized copy of a planner live run, exported by \`scripts/planner-evidence-export.sh\`"
  echo "on $(date -u '+%Y-%m-%dT%H:%M:%SZ'). Files are copied as recorded, except that"
  echo "machine paths are replaced with \`<FIXTURE>\`, \`<CHECKOUT>\`, \`<TMPDIR>\`, and \`<HOME>\`."
  echo
  echo "- \`sop\` binary: $SOP_VERSION (SHA-256 \`$SOP_SHA\`)"
  echo "- \`fixture/\`: the task, input, acceptance test, and final deliverables"
  echo "- \`state/\`: questions, answers, outcomes, and continuations (\`questions.json\`), plan, and config"
  echo "- \`run/\`: the task run's outcome, state, report, gate, review, validation, and trace"
  [ -z "$LOGS" ] || echo "- \`logs/\`: the operator's round logs"
  echo
  if [ -n "$MISSING" ]; then
    echo "Not present in the source run:"
    for m in $MISSING; do echo "- \`$m\`"; done
    echo
  fi
  echo "\`MANIFEST.sha256\` lists the SHA-256 of every other file in this directory."
} > "$STAGE/README.md"
(cd "$STAGE" && find . -type f ! -name MANIFEST.sha256 | sed 's#^\./##' | LC_ALL=C sort | while IFS= read -r f; do
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$f"; else sha256sum "$f"; fi
done) > "$STAGE/MANIFEST.sha256"

mkdir -p "$(dirname "$OUT")"
if [ -e "$OUT" ]; then rm -rf "$OUT"; fi
mv "$STAGE" "$OUT"
trap - EXIT
count="$(find "$OUT" -type f | wc -l | tr -d ' ')"
echo "export: wrote $count files to ${OUT#"$ROOT"/}"
[ -z "$MISSING" ] || echo "export: not present in the source run:$MISSING"
echo "export: review the bundle, then commit it on a branch."

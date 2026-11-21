#!/usr/bin/env bash
# Run this repository's own required security gate against its own source.
#
# The other CI security jobs scan fixtures or a temporary repository, so a new
# blocking finding in the repository itself (for example a test that calls
# exec.Command with a variable argv) used to reach main unnoticed and only blocked
# later `sop run` tasks. This runs the real `sop security scan --required
# --scope workspace` with the committed .agent-sdlc/config.yaml and baseline, and
# fails unless the verdict is PASS with COMPLETE coverage.
#
# Provisioning is separate and explicit (a scan never fetches anything):
#   gitleaks, semgrep, osv-scanner, cargo-audit   pinned builds on PATH
#   .agent-sdlc/security-rules                    scripts/security/fetch-semgrep-rules.sh
#   .agent-sdlc/osv-db                            scripts/security/fetch-osv-databases.sh
#
# Environment:
#   SOP_REPO_SECURITY_SOP   pre-built sop binary (default: built into a temp dir)

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"

die() { echo "repo-security-gate: $*" >&2; exit 1; }

for cmd in gitleaks semgrep osv-scanner cargo-audit python3; do
  command -v "$cmd" >/dev/null 2>&1 || die "$cmd is required on PATH"
done
[ -d .agent-sdlc/security-rules ] || die ".agent-sdlc/security-rules is missing; run scripts/security/fetch-semgrep-rules.sh --dest .agent-sdlc/security-rules"
[ -d .agent-sdlc/osv-db ] || die ".agent-sdlc/osv-db is missing; run scripts/security/fetch-osv-databases.sh"

work="$(mktemp -d "${TMPDIR:-/tmp}/sop-repo-security.XXXXXX")"
trap 'rm -rf "$work"' EXIT

sop_bin="${SOP_REPO_SECURITY_SOP:-}"
if [ -z "$sop_bin" ]; then
  sop_bin="$work/sop"
  go build -o "$sop_bin" ./cmd/sop || die "sop build failed"
fi

status=0
"$sop_bin" security scan --required --scope workspace --json --run-id repo-security-gate >"$work/scan.json" 2>"$work/scan.err" || status=$?

python3 - "$work/scan.json" "$status" <<'PY'
import json, sys

path, status = sys.argv[1], int(sys.argv[2])
try:
    scan = json.load(open(path))["scan"]
except Exception as exc:  # no JSON means the scan itself failed to run
    print(f"repo-security-gate: no scan result (exit {status}): {exc}", file=sys.stderr)
    sys.exit(1)

print(f"repo-security-gate: verdict={scan['verdict']} coverage={scan['coverage']}")
blocking = [
    f for f in scan.get("findings") or []
    if not f.get("suppressed") and f.get("severity") in ("HIGH", "CRITICAL")
]
if scan["verdict"] == "PASS" and scan["coverage"] == "COMPLETE" and status == 0:
    sys.exit(0)

for reason in scan.get("reasons") or []:
    print(f"repo-security-gate: reason: {reason}", file=sys.stderr)
for f in blocking:
    print(
        "repo-security-gate: unsuppressed {sev} {rule} {path}:{line} {fp}".format(
            sev=f["severity"], rule=f["rule_id"], path=f["path"], line=f.get("line"), fp=f["fingerprint"]
        ),
        file=sys.stderr,
    )
print(
    "repo-security-gate: add a reviewed baseline entry (.agent-sdlc/security-baseline.json) "
    "only after an operator triage; do not weaken the gate",
    file=sys.stderr,
)
sys.exit(1)
PY

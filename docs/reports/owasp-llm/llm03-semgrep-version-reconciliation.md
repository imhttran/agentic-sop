# Semgrep version reconciliation — CI 1.136.0 vs configuration 1.183.0

**Status:** investigation and verification, read-only. **No version is changed, no lockfile
is regenerated, and no policy is touched.** This records the two versions, proves what the
difference does to scanner output, and recommends one pinned version for the operator to
approve.

## 1. The two versions, and what each one controls

| Where | Value | Enforced? |
| --- | --- | --- |
| `scripts/checks/security-integration.sh` — `SEMGREP_VERSION` | `1.136.0` | **Yes.** It provisions the venv and the script asserts the installed metadata equals it. |
| `scripts/security/semgrep-requirements.lock` — `semgrep==1.136.0` | `1.136.0` | **Yes.** `pip install --require-hashes` refuses any other artifact. |
| `.agent-sdlc/config.yaml` — `adapters[semgrep].version` | `1.183.0` | **No.** It is recorded into execution provenance; the adapter compares nothing and never fails on a mismatch. |
| Developer machines (Homebrew here) | `1.183.0` | No. |

So the CI pin is authoritative for CI, the configuration value is descriptive, and they
disagree. This is D5.

Contrast with Gitleaks, which is genuinely pinned: the adapter hashes the resolved
executable and fails closed on a mismatch (observed directly during the D4 reproduction: the
Homebrew build produced `FAILED: binary pin mismatch`). Semgrep has no equivalent, because it
is a Python application rather than one binary.

## 2. What was verified

Both versions were run against the **same repository tree** (`bd2e0d45`), the **same pinned
rules** (`rules_sha256 d90ff1b1…`), the **same policy** (`fail_on: [critical, high]`, report
only), and the **same pinned Gitleaks and OSV-Scanner**, so Semgrep was the only variable.

- **CI version, reproduced exactly as CI provisions it:** a Python 3.11 venv installed from
  `semgrep-requirements.lock` with `--require-hashes`. Authoritative version via package
  metadata (`importlib.metadata.version("semgrep")`) = **1.136.0**.
- **Configuration version:** Homebrew Semgrep **1.183.0** (`brew list --versions semgrep`).

### Result

| | Findings | Suppressed | Blocking | Fingerprint set | Recorded `scanner_version` |
| --- | --- | --- | --- | --- | --- |
| Semgrep 1.136.0 | 101 | 40 | 45 | — | `1.136.0` |
| Semgrep 1.183.0 | 101 | 40 | 45 | identical | `1.183.0` |

**The fingerprint sets are identical: 0 findings unique to either version.** Coverage was
`COMPLETE` and the verdict `FAIL` in both runs. The 45 blocking findings and the 44 proposed
baseline fingerprints in the D4 packet are therefore **version-insensitive across this pair**.

This is a correction to the D5 characterization in the operator packet on PR #471, which
called D5 a "stability prerequisite" for the Option C gate. On this evidence it is **not** a
gate-stability blocker. What remains is reproducibility and provenance, which is what the two
defects below are.

## 3. Two defects found in the pinned CI environment

**(a) `semgrep --version` reports a version that is not installed.** The hash-locked 1.136.0
venv prints `1.183.0` from `semgrep --version`, because the update check reports the newest
available release. The security-integration script already knows this and reads package
metadata instead. Anything else that trusts `--version` for CI Semgrep would be wrong. The
adapter is unaffected: the JSON report carries the true version, and provenance recorded
`1.136.0` correctly.

**(b) The lockfile is not self-sufficient.** It pins
`opentelemetry-instrumentation==0.46b0`, whose `dependencies.py` imports `pkg_resources`, but
it does **not** pin `setuptools`. `setuptools` 81 removed `pkg_resources`, so a venv built
from this lockfile with a current `setuptools` cannot start Semgrep at all:

```
ModuleNotFoundError: No module named 'pkg_resources'
semgrep returned empty output
```

Installing `setuptools<81` fixes it. CI currently works only because the `setup-python` 3.11
image still ships an older `setuptools`; the environment, not the lockfile, supplies the
missing requirement. That is a latent, environment-dependent CI failure.

Semgrep 1.183.0 uses `opentelemetry-instrumentation 0.58b0` and has no `pkg_resources`
reference, so moving forward removes this trap.

## 4. Recommendation: one pinned version, **1.183.0**

Reasons, in order:

1. **Behavior-neutral, verified.** The output is byte-identical to 1.136.0 on this
   repository, including every fingerprint the D4 packet proposes to baseline.
2. **It is the version the approved evidence came from.** The 101/40/61/45 baseline and the
   45 fingerprints were produced on 1.183.0. Pinning CI to 1.183.0 makes the declared,
   enforced, and evidence-producing versions the same.
3. **It removes defect (b).** No `pkg_resources` dependency, so the lockfile stops depending
   on an unpinned system `setuptools`.
4. **It matches developer machines.** Local Homebrew is 1.183.0, so local and CI evidence
   describe the same engine.

**Cost, stated plainly:** this regenerates `semgrep-requirements.lock` for ~100 packages,
which is a supply-chain change and needs operator approval. It also changes the Python
support guard: 1.183.0 declares `Requires-Python: >=3.10`, while the script currently permits
3.9–3.11 and its failure message says so. The guard and the message must change together, and
CI must stay on 3.10 or 3.11 (it uses 3.11).

### Exact change if approved

```text
scripts/checks/security-integration.sh      SEMGREP_VERSION="1.183.0"
                                            python support case: 3.10|3.11 (drop 3.9)
                                            failure message: "support Python 3.10-3.11"
scripts/security/semgrep-requirements.lock  regenerate semgrep==1.183.0 with hashes
.agent-sdlc/config.yaml                     no change (already 1.183.0)
```

Verification after the change: the same scan must reproduce 101 / 40 / 45 with `COMPLETE`
coverage and `scanner_version 1.183.0`, and `sop security scan --required` must return the
same verdict as with 1.136.0.

### Alternative: declare 1.136.0

Editing the configuration's `version` to `1.136.0` is one line, adds no artifact, and also
removes the declared drift. It is the right choice only if the operator prefers not to touch
the dependency lockfile. It leaves defect (b) in place, leaves CI on an engine ~47 releases
old, and makes the declared version differ from the version that produced the approved
baseline evidence (provenance would still record the truth).

## 5. Test evidence

| Check | Result |
| --- | --- |
| Hash-locked venv metadata | `semgrep 1.136.0` |
| Scan with 1.136.0 | 101 findings, 40 suppressed, 45 blocking, `COMPLETE`, `FAIL` |
| Scan with 1.183.0 | 101 findings, 40 suppressed, 45 blocking, `COMPLETE`, `FAIL` |
| Fingerprint set difference | none in either direction |
| Provenance accuracy | `1.136.0` and `1.183.0` recorded correctly despite `--version` |
| Lockfile self-sufficiency | **fails** without `setuptools<81`; passes with it |

## 6. Operator decisions still required

- **V-1.** Choose the single version: **1.183.0 (recommended)** or 1.136.0.
- **V-2.** If 1.183.0: approve a supply-chain change that regenerates the hash-locked
  requirements, plus the Python guard change from 3.9–3.11 to 3.10–3.11. Note that the
  repository has no recorded generator for this lockfile; the regeneration command should be
  documented in the same change.
- **V-3.** If staying on 1.136.0: approve pinning `setuptools<81` in the lockfile (or
  documenting it as an external prerequisite) so the pin is self-sufficient.
- **V-4.** Decide whether Semgrep should have an enforced version check at all, comparable to
  the Gitleaks digest pin. Today a mismatch is recorded but never refused. This is a scope
  question for D1/D5, not required for the gate.

## What this report does not do

- It changes no version, lockfile, script, configuration, or policy.
- It claims nothing about Windows or Python 3.12+, which were not exercised.
- It does not modify PR #471 or its packet; the D5 correction there is recorded here for the
  operator to fold in before merge.

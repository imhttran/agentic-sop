#!/usr/bin/env python3
"""Validate a portable MCP live-validation handoff.

The handoff is the cross-agent contract: any coding agent can write it, any
independent MCP validator can read it, and a later agent can resume from it.
This validator is read-only. It checks the versioned schema, the recorded
identity, the independent results, the approval state, and the next permitted
action.

Usage:
  scripts/validation-handoff.py --handoff FILE --checkout PATH
  scripts/validation-handoff.py --handoff FILE --artifact-only

Exit status: 0 valid, 1 invalid, 2 usage or parse error.
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

SCHEMA = "mcp-live-validation/validation-handoff/v1"
RESULTS_SCHEMA = "mcp-live-validation/validation-results/v1"
SUPPORTED_SCHEMA_VERSION = 1

ALLOWED_ACTIONS = {
    "REQUEST_DEPLOYMENT_APPROVAL",
    "START_CANDIDATE",
    "RUN_INDEPENDENT_VALIDATION",
    "DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE",
    "PREPARE_PR",
    "STOP_AND_REPORT",
}

SECRET_KEY_RE = re.compile(
    r"(token|password|secret|api[_-]?key|access[_-]?key|authorization|client[_-]?secret)",
    re.IGNORECASE,
)
HEX64_RE = re.compile(r"^[0-9a-f]{64}$")
# Credential-shaped values. Key names are checked separately; these catch a secret
# pasted into a free-text field such as local_verification.results.
SECRET_VALUE_RE = re.compile(
    r"((?<![A-Za-z0-9])sk-[A-Za-z0-9_-]{20,}"
    r"|(?<![A-Za-z0-9])gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}"
    r"|(?<![A-Za-z0-9])AKIA[0-9A-Z]{16}"
    r"|-----BEGIN [A-Z ]*PRIVATE KEY"
    r"|[Bb]earer\s+[A-Za-z0-9._~+/-]{20,})"
)


class ValidationError(Exception):
    """A fatal input or parse error."""


def load_json(path: str) -> Any:
    try:
        with open(path, "r", encoding="utf-8") as fh:
            return json.load(fh)
    except FileNotFoundError as exc:
        raise ValidationError(f"file not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise ValidationError(f"invalid JSON in {path}: {exc}") from exc


def load_schema(path: str) -> dict[str, Any]:
    schema = load_json(path)
    if not isinstance(schema, dict):
        raise ValidationError(f"schema is not a JSON object: {path}")
    return schema


def type_matches(value: Any, wanted: str) -> bool:
    if wanted == "object":
        return isinstance(value, dict)
    if wanted == "array":
        return isinstance(value, list)
    if wanted == "string":
        return isinstance(value, str)
    if wanted == "integer":
        return isinstance(value, int) and not isinstance(value, bool)
    if wanted == "number":
        return isinstance(value, (int, float)) and not isinstance(value, bool)
    if wanted == "boolean":
        return isinstance(value, bool)
    if wanted == "null":
        return value is None
    return True


def validate_schema(
    value: Any, schema: dict[str, Any], path: str, errors: list[str]
) -> None:
    """Validate the subset of JSON Schema this repository uses.

    Supported: type (string or list), enum, required, properties, items,
    minimum, minLength, pattern. Unknown keywords are ignored.
    """
    wanted = schema.get("type")
    if wanted is not None:
        options = wanted if isinstance(wanted, list) else [wanted]
        if not any(type_matches(value, option) for option in options):
            errors.append(
                f"{path}: expected type {'/'.join(options)}, got {type(value).__name__}"
            )
            return

    if "enum" in schema and value not in schema["enum"]:
        errors.append(f"{path}: value {value!r} is not one of {schema['enum']}")

    if (
        "minimum" in schema
        and isinstance(value, (int, float))
        and value < schema["minimum"]
    ):
        errors.append(f"{path}: value {value} is below minimum {schema['minimum']}")

    if (
        "minLength" in schema
        and isinstance(value, str)
        and len(value) < schema["minLength"]
    ):
        errors.append(f"{path}: value is shorter than {schema['minLength']}")

    if (
        "pattern" in schema
        and isinstance(value, str)
        and not re.search(schema["pattern"], value)
    ):
        errors.append(f"{path}: value does not match {schema['pattern']}")

    if isinstance(value, dict):
        for key in schema.get("required", []):
            if key not in value:
                errors.append(f"{path}: missing required field {key!r}")
        for key, child in schema.get("properties", {}).items():
            if key in value:
                validate_schema(value[key], child, f"{path}.{key}", errors)

    if isinstance(value, list) and "items" in schema:
        for index, item in enumerate(value):
            validate_schema(item, schema["items"], f"{path}[{index}]", errors)


def walk_secret_keys(value: Any, path: str, errors: list[str]) -> None:
    """Reject credential-like keys and credential-shaped string values.

    A matched value is never echoed: the error names only its path.
    """
    if isinstance(value, dict):
        for key, child in value.items():
            if SECRET_KEY_RE.search(str(key)):
                errors.append(
                    f"{path}: credential-like key {key!r} is not allowed in a portable handoff"
                )
            walk_secret_keys(child, f"{path}.{key}", errors)
    elif isinstance(value, list):
        for index, child in enumerate(value):
            walk_secret_keys(child, f"{path}[{index}]", errors)
    elif isinstance(value, str) and SECRET_VALUE_RE.search(value):
        errors.append(
            f"{path}: value looks like a credential; redact it before sharing the handoff"
        )


def parse_time(value: str) -> dt.datetime:
    try:
        return dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise ValidationError(f"generated_at is not RFC3339: {value!r}") from exc


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with open(path, "rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


def git(checkout: Path, *args: str) -> str:
    result = subprocess.run(
        ["git", "-C", str(checkout), *args],
        text=True,
        capture_output=True,
    )
    if result.returncode != 0:
        raise ValidationError(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip()


def read_manifest(checkout: Path) -> dict[str, str]:
    path = checkout / ".run" / "tunnel" / "candidate" / "manifest"
    if not path.is_file():
        raise ValidationError(f"candidate manifest not found: {path}")
    manifest: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if "=" in line and not line.startswith("#"):
            key, value = line.split("=", 1)
            manifest[key] = value
    return manifest


def process_command(pid: int) -> str | None:
    result = subprocess.run(
        ["ps", "-p", str(pid), "-o", "command="],
        text=True,
        capture_output=True,
    )
    if result.returncode != 0:
        return None
    return result.stdout.strip()


def require_string(value: Any, path: str, errors: list[str]) -> None:
    if not isinstance(value, str) or not value.strip():
        errors.append(f"{path}: must be a non-empty string")


def require_hex64(value: Any, path: str, errors: list[str]) -> None:
    if not isinstance(value, str) or not HEX64_RE.match(value):
        errors.append(f"{path}: must be a 64-character lowercase hex digest")


def validate_results(
    handoff: dict[str, Any],
    errors: list[str],
) -> None:
    validation = handoff["independent_validation"]
    results = validation["results"]
    request = handoff["request"]
    validation_status = validation["status"]
    results_status = results.get("status")

    if validation_status != results_status:
        errors.append(
            f"$.independent_validation.status {validation_status!r} does not match results.status {results_status!r}"
        )

    if results.get("schema") != RESULTS_SCHEMA:
        errors.append(
            f"$.independent_validation.results.schema must be {RESULTS_SCHEMA!r}"
        )
    if results.get("workflow_id") != handoff["workflow_id"]:
        errors.append(
            "$.independent_validation.results.workflow_id does not match the handoff workflow_id"
        )
    if results.get("candidate_sha256") != handoff["candidate"]["sha256"]:
        errors.append(
            "$.independent_validation.results.candidate_sha256 does not match the handoff candidate"
        )
    if results.get("source_revision") != handoff["candidate"]["source_revision"]:
        errors.append(
            "$.independent_validation.results.source_revision does not match the handoff candidate"
        )
    if results.get("rounds") != request["rounds"]:
        errors.append(
            "$.independent_validation.results.rounds does not match request.rounds"
        )
    if results.get("concurrent") != request["concurrent"]:
        errors.append(
            "$.independent_validation.results.concurrent does not match request.concurrent"
        )
    if results.get("retries") != 0 or request["retries"] != 0:
        errors.append("independent validation must use no retries")
    if (
        results.get("state_changes") is not False
        or request["state_changes"] is not False
    ):
        errors.append("independent validation must not change state")

    template = handoff["results_template"]
    if template.get("workflow_id") != handoff["workflow_id"]:
        errors.append(
            "$.results_template.workflow_id does not match the handoff workflow_id"
        )
    if template.get("candidate_sha256") != handoff["candidate"]["sha256"]:
        errors.append(
            "$.results_template.candidate_sha256 does not match the handoff candidate"
        )
    if template.get("source_revision") != handoff["candidate"]["source_revision"]:
        errors.append(
            "$.results_template.source_revision does not match the handoff candidate"
        )
    if template.get("rounds") != request["rounds"]:
        errors.append("$.results_template.rounds does not match request.rounds")
    if template.get("status") != "PENDING":
        errors.append("$.results_template.status must start at PENDING")

    if validation_status == "PENDING":
        return

    require_string(
        validation.get("validator"), "$.independent_validation.validator", errors
    )
    require_string(
        results.get("validator"), "$.independent_validation.results.validator", errors
    )
    if validation.get("validator") in {"", "unspecified"} or results.get(
        "validator"
    ) in {"", "unspecified"}:
        errors.append("PASS/FAIL requires a named independent validator")
    capabilities = results.get("validator_capabilities")
    if not isinstance(capabilities, list) or not capabilities:
        errors.append(
            "$.independent_validation.results.validator_capabilities must declare the validator's capabilities"
        )
    require_string(
        results.get("started_at"), "$.independent_validation.results.started_at", errors
    )
    require_string(
        results.get("finished_at"),
        "$.independent_validation.results.finished_at",
        errors,
    )
    require_hex64(handoff["candidate"]["sha256"], "$.candidate.sha256", errors)

    summary = results.get("summary", {})
    total = summary.get("total")
    passed = summary.get("passed")
    failed = summary.get("failed")
    calls = results.get("calls", [])
    if total != request["total_calls"]:
        errors.append(
            "$.independent_validation.results.summary.total does not match request.total_calls"
        )
    if (
        isinstance(passed, int)
        and isinstance(failed, int)
        and isinstance(total, int)
        and passed + failed != total
    ):
        errors.append(
            "$.independent_validation.results.summary must add passed + failed to total"
        )
    if len(calls) != request["total_calls"]:
        errors.append(
            "$.independent_validation.results.calls must contain one entry per requested call"
        )
    for index, call in enumerate(calls):
        if call.get("status") not in {"PASS", "FAIL", "ERROR"}:
            errors.append(
                f"$.independent_validation.results.calls[{index}].status is invalid"
            )

    if validation_status == "PASS":
        if validation.get("validator") == "":
            errors.append("PASS requires a named independent validator")
        if passed != total or failed != 0:
            errors.append("PASS requires every requested call to pass")
        if any(call.get("status") != "PASS" for call in calls):
            errors.append("PASS requires every recorded call to have status PASS")
        if results.get("initialization_errors"):
            errors.append("PASS requires an empty initialization_errors list")
    elif validation_status == "FAIL":
        if not (
            failed
            or results.get("initialization_errors")
            or any(call.get("status") in {"FAIL", "ERROR"} for call in calls)
        ):
            errors.append(
                "FAIL requires at least one failure, error, or initialization error as evidence"
            )


def validate_semantics(
    handoff: dict[str, Any],
    max_age_hours: float,
    errors: list[str],
    warnings: list[str],
) -> None:
    if handoff.get("schema") != SCHEMA:
        errors.append(f"$.schema must be {SCHEMA!r}")
    if handoff.get("schema_version") != SUPPORTED_SCHEMA_VERSION:
        errors.append(f"$.schema_version must be {SUPPORTED_SCHEMA_VERSION}")

    require_string(handoff.get("workflow_id"), "$.workflow_id", errors)
    require_string(handoff.get("generated_at"), "$.generated_at", errors)
    try:
        generated = parse_time(str(handoff.get("generated_at", "")))
        now = dt.datetime.now(dt.timezone.utc)
        if generated.tzinfo is None:
            generated = generated.replace(tzinfo=dt.timezone.utc)
        age_hours = (now - generated).total_seconds() / 3600.0
        if age_hours > max_age_hours > 0:
            errors.append(
                f"handoff is stale: {age_hours:.1f}h old, limit {max_age_hours}h"
            )
        if age_hours < -0.1:
            errors.append("handoff generated_at is in the future")
    except ValidationError as exc:
        errors.append(str(exc))

    repository = handoff["repository"]
    require_string(repository.get("commit"), "$.repository.commit", errors)
    require_hex64(handoff["candidate"].get("sha256"), "$.candidate.sha256", errors)
    require_hex64(handoff["baseline"].get("sha256"), "$.baseline.sha256", errors)
    require_string(handoff["candidate"].get("path"), "$.candidate.path", errors)
    require_string(
        handoff["candidate"].get("source_revision"),
        "$.candidate.source_revision",
        errors,
    )
    require_string(handoff["baseline"].get("path"), "$.baseline.path", errors)

    command_sha = handoff["process"].get("command_sha256", "")
    if command_sha:
        require_hex64(command_sha, "$.process.command_sha256", errors)

    local = handoff["local_verification"]
    if (
        local.get("status") in {"PASS", "FAIL"}
        and not str(local.get("results", "")).strip()
    ):
        errors.append(
            "$.local_verification.results must record evidence for PASS or FAIL"
        )

    request = handoff["request"]
    if (
        not isinstance(request.get("required_calls"), list)
        or not request["required_calls"]
    ):
        errors.append("$.request.required_calls must contain at least one call")
    if request.get("total_calls", 0) <= 0:
        errors.append("$.request.total_calls must be positive")

    validate_results(handoff, errors)

    approval = handoff["approval"]
    workflow = handoff["workflow"]
    action = workflow.get("next_permitted_action")
    if action not in ALLOWED_ACTIONS:
        errors.append(f"$.workflow.next_permitted_action is not supported: {action!r}")
        return

    deployment = approval["deployment"]
    if deployment.get("status") == "APPROVED" and not str(
        deployment.get("reference", "")
    ).strip():
        errors.append(
            "$.approval.deployment.reference must name the approval (for example a "
            "comment link or ticket) when the status is APPROVED"
        )
    validation_status = handoff["independent_validation"]["status"]
    local_status = local.get("status")

    if action in {
        "REQUEST_DEPLOYMENT_APPROVAL",
        "START_CANDIDATE",
        "RUN_INDEPENDENT_VALIDATION",
    }:
        if local_status != "PASS":
            errors.append(f"{action} requires local_verification.status PASS")
    if action == "REQUEST_DEPLOYMENT_APPROVAL":
        if deployment.get("status") != "PENDING":
            errors.append(
                "REQUEST_DEPLOYMENT_APPROVAL requires deployment approval PENDING"
            )
        if validation_status != "PENDING":
            errors.append(
                "REQUEST_DEPLOYMENT_APPROVAL requires independent validation PENDING"
            )
    if action == "START_CANDIDATE":
        if deployment.get("status") != "APPROVED":
            errors.append("START_CANDIDATE requires explicit deployment approval")
        if validation_status != "PENDING":
            errors.append("START_CANDIDATE requires independent validation PENDING")
    if action == "RUN_INDEPENDENT_VALIDATION":
        if deployment.get("status") != "APPROVED":
            errors.append(
                "RUN_INDEPENDENT_VALIDATION requires explicit deployment approval"
            )
        if validation_status != "PENDING":
            errors.append(
                "RUN_INDEPENDENT_VALIDATION requires independent validation PENDING"
            )
    if (
        action == "DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE"
        and validation_status != "FAIL"
    ):
        errors.append(
            "DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE requires validation FAIL"
        )
    if validation_status == "PASS" and action != "PREPARE_PR":
        errors.append("validation PASS requires next_permitted_action PREPARE_PR")
    if (
        validation_status == "FAIL"
        and action != "DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE"
    ):
        errors.append(
            "validation FAIL requires next_permitted_action DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE"
        )
    if validation_status in {"PASS", "FAIL"} and deployment.get("status") != "APPROVED":
        errors.append("independent validation requires explicit deployment approval")
    if action == "PREPARE_PR":
        if validation_status != "PASS":
            errors.append(
                "PREPARE_PR requires independent validation PASS; missing validation is not success"
            )
        if local_status != "PASS":
            errors.append("PREPARE_PR requires local_verification.status PASS")

    if workflow.get("status") == "VALIDATION_PASSED" and validation_status != "PASS":
        errors.append(
            "workflow.status VALIDATION_PASSED requires independent validation PASS"
        )
    if workflow.get("status") == "VALIDATION_FAILED" and validation_status != "FAIL":
        errors.append(
            "workflow.status VALIDATION_FAILED requires independent validation FAIL"
        )

    safety = handoff["safety"]
    if safety.get("production_binary_replaced") is not False:
        errors.append("$.safety.production_binary_replaced must be false")
    if safety.get("merged") is not False:
        errors.append("$.safety.merged must be false")
    if safety.get("credentials_redacted") is not True:
        errors.append("$.safety.credentials_redacted must be true")

    walk_secret_keys(handoff, "$", errors)


def validate_checkout(
    handoff: dict[str, Any],
    checkout: Path,
    errors: list[str],
    warnings: list[str],
    checks: dict[str, bool],
) -> None:
    checks["checkout"] = True
    try:
        head = git(checkout, "rev-parse", "HEAD")
        checks["repository"] = True
        if head != handoff["repository"]["commit"]:
            errors.append(
                f"stale repository: handoff commit {handoff['repository']['commit']} != current {head}"
            )
        branch = git(checkout, "rev-parse", "--abbrev-ref", "HEAD")
        handoff_branch = handoff["repository"].get("branch", "")
        if branch != "HEAD" and handoff_branch and branch != handoff_branch:
            errors.append(
                f"stale repository branch: handoff {handoff_branch} != current {branch}"
            )
    except ValidationError as exc:
        errors.append(str(exc))

    manifest: dict[str, str] = {}
    try:
        manifest = read_manifest(checkout)
        checks["manifest"] = True
    except ValidationError as exc:
        errors.append(str(exc))

    if manifest:
        expected = {
            "workflow_id": handoff["workflow_id"],
            "candidate_sha256": handoff["candidate"]["sha256"],
            "candidate_revision": handoff["candidate"]["source_revision"],
            "candidate_version": handoff["candidate"]["version"],
            "baseline_sha256": handoff["baseline"]["sha256"],
        }
        for key, want in expected.items():
            got = manifest.get(key, "")
            if got != want:
                errors.append(
                    f"stale candidate: manifest {key}={got!r} != handoff {want!r}"
                )
        if (
            handoff["independent_validation"]["status"] in {"PASS", "FAIL"}
            and manifest.get("state") == "prepared"
        ):
            errors.append(
                "validation results cannot precede candidate deployment (manifest state is prepared)"
            )

    candidate_path = Path(handoff["candidate"]["path"])
    if candidate_path.is_file():
        checks["candidate_file"] = True
        if sha256_file(candidate_path) != handoff["candidate"]["sha256"]:
            errors.append(f"candidate checksum mismatch: {candidate_path}")
    else:
        errors.append(f"candidate file not found: {candidate_path}")

    baseline_path = Path(handoff["baseline"]["path"])
    if baseline_path.is_file():
        checks["baseline_file"] = True
        if sha256_file(baseline_path) != handoff["baseline"]["sha256"]:
            errors.append(f"baseline checksum mismatch: {baseline_path}")
    else:
        errors.append(f"baseline file not found: {baseline_path}")

    pid_text = handoff["process"].get("pid", "")
    action = handoff["workflow"]["next_permitted_action"]
    if pid_text:
        try:
            pid = int(pid_text)
        except ValueError:
            errors.append(f"$.process.pid is not an integer: {pid_text!r}")
            return
        live_command = process_command(pid)
        if live_command is None:
            if action == "RUN_INDEPENDENT_VALIDATION":
                errors.append(f"candidate process PID {pid} is not running")
            else:
                warnings.append(f"candidate process PID {pid} is not running")
        else:
            checks["process"] = True
            want = handoff["process"].get("command_sha256", "")
            if want and sha256_text(live_command) != want:
                errors.append(f"process identity mismatch for PID {pid}")
    elif action == "RUN_INDEPENDENT_VALIDATION":
        errors.append(
            "RUN_INDEPENDENT_VALIDATION requires a recorded running candidate PID"
        )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Validate an MCP live-validation handoff."
    )
    parser.add_argument(
        "--handoff", required=True, help="validation-handoff.json to validate"
    )
    # One mode is required: a validation that silently skips the checkout checks
    # must never print a bare OK.
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--checkout", help="agentic-sop checkout to cross-check")
    mode.add_argument(
        "--artifact-only",
        action="store_true",
        help="validate the artifact without filesystem/process checks",
    )
    parser.add_argument(
        "--schema",
        help="schema path (default: docs/reference/validation-handoff.schema.json)",
    )
    parser.add_argument(
        "--max-age-hours",
        type=float,
        default=24.0,
        help="reject artifacts older than this; 0 disables the age check",
    )
    parser.add_argument(
        "--json", action="store_true", help="print a machine-readable summary"
    )
    parser.add_argument(
        "--quiet", action="store_true", help="suppress the text summary"
    )
    args = parser.parse_args()

    script_root = Path(__file__).resolve().parent.parent
    schema_path = (
        Path(args.schema)
        if args.schema
        else script_root / "docs" / "reference" / "validation-handoff.schema.json"
    )
    errors: list[str] = []
    warnings: list[str] = []
    checks: dict[str, bool] = {}

    try:
        schema = load_schema(str(schema_path))
        handoff = load_json(args.handoff)
    except ValidationError as exc:
        print(f"validation-handoff: INVALID: {exc}", file=sys.stderr)
        return 2

    if not isinstance(handoff, dict):
        print(
            "validation-handoff: INVALID: handoff is not a JSON object", file=sys.stderr
        )
        return 1

    validate_schema(handoff, schema, "$", errors)
    if not errors:
        try:
            validate_semantics(handoff, args.max_age_hours, errors, warnings)
        except (KeyError, TypeError) as exc:
            errors.append(f"malformed handoff: {exc}")

    if args.checkout:
        try:
            validate_checkout(
                handoff, Path(args.checkout).resolve(), errors, warnings, checks
            )
        except (KeyError, TypeError) as exc:
            errors.append(f"malformed handoff: {exc}")
    else:
        warnings.append(
            "artifact-only: repository, manifest, candidate file, baseline, and process were not verified"
        )

    valid = not errors
    summary = {
        "valid": valid,
        "workflow_id": handoff.get("workflow_id", "")
        if isinstance(handoff, dict)
        else "",
        "workflow_status": handoff.get("workflow", {}).get("status", "")
        if isinstance(handoff, dict)
        else "",
        "next_permitted_action": handoff.get("workflow", {}).get(
            "next_permitted_action", ""
        )
        if isinstance(handoff, dict)
        else "",
        "candidate_sha256": handoff.get("candidate", {}).get("sha256", "")
        if isinstance(handoff, dict)
        else "",
        "validation_status": handoff.get("independent_validation", {}).get("status", "")
        if isinstance(handoff, dict)
        else "",
        "artifact_only": args.artifact_only,
        "checks": checks,
        "errors": errors,
        "warnings": warnings,
    }
    if args.json:
        print(json.dumps(summary, indent=2))
    elif not args.quiet:
        if valid:
            print("validation-handoff: OK")
            print(
                f"  workflow: {summary['workflow_id']} ({summary['workflow_status']})"
            )
            print(f"  next:     {summary['next_permitted_action']}")
            print(f"  candidate: {summary['candidate_sha256']}")
            print(f"  validation: {summary['validation_status']}")
        else:
            print("validation-handoff: INVALID")
            for error in errors:
                print(f"  error: {error}")
        for warning in warnings:
            print(f"  warning: {warning}")
    return 0 if valid else 1


if __name__ == "__main__":
    sys.exit(main())

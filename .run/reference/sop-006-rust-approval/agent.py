#!/usr/bin/env python3
"""Scripted command agent for the disposable SOP-006 Rust workflow.

SOP sends an agent.Request as JSON on stdin. This script writes the Rust fixture
and returns structured outcomes, so the demonstration is deterministic and does
not depend on a network model. The scripted failure is intentional: the first
SOP-001 invocation writes a failing test; the supported rerun writes the fix.
"""

import json
import os
import pathlib
import sys

request = json.load(sys.stdin)
capability = request.get("capability", "")
task = request.get("task", "")
root = pathlib.Path(os.getcwd())

# Diagnostic only: record the exact request SOP sent so a fixture mismatch is
# visible instead of guessed.
try:
    with open("/tmp/sop006-agent-requests.log", "a") as fh:
        fh.write(json.dumps(request) + "\n")
except Exception:
    pass


def write(path, content):
    p = root / path
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content)


def completed(summary):
    print(
        json.dumps(
            {"status": "completed", "summary": summary, "changes_expected": True}
        )
    )


CARGO = """[package]
name = "sop006-demo"
version = "0.1.0"
edition = "2021"

[lib]
name = "sop006_demo"
path = "src/lib.rs"
"""


def lib(farewell, broken_greet):
    greet = '"hi"' if broken_greet else '"hello"'
    body = (
        """
pub fn greet() -> &'static str {
    %s
}
"""
        % greet
    )
    if farewell:
        body += """
pub fn farewell() -> &'static str {
    "goodbye"
}
"""
    tests = """
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn greet_is_hello() {
        assert_eq!(greet(), "hello");
    }
"""
    if farewell:
        tests += """
    #[test]
    fn farewell_is_goodbye() {
        assert_eq!(farewell(), "goodbye");
    }
"""
    tests += "}\n"
    return body + tests


if capability == "REVIEW":
    print(json.dumps({"summary": "clean", "findings": []}))
elif capability == "PLAN":
    if "S002" in task:
        stage = {
            "id": "S002",
            "title": "Implement farewell",
            "objective": "Add a farewell function returning goodbye and cover it with a test.",
            "dependencies": ["S001"],
            "deliverables": [],
            "acceptance_criteria": ["farewell returns goodbye"],
        }
    else:
        stage = {
            "id": "S001",
            "title": "Implement greet",
            "objective": "Add a greet function returning hello and cover it with a test.",
            "dependencies": [],
            "deliverables": [],
            "acceptance_criteria": ["greet returns hello"],
        }
    print(
        json.dumps(
            {"project": "SOP-006 Rust", "summary": "Disposable demo", "stages": [stage]}
        )
    )
elif capability in ("IMPLEMENT", "FIX"):
    if "S002" in task:
        write("Cargo.toml", CARGO)
        write("src/lib.rs", lib(farewell=True, broken_greet=False))
        completed("added farewell")
    else:
        marker = root / "state" / "first-invocation"
        if marker.exists():
            write("Cargo.toml", CARGO)
            write("src/lib.rs", lib(farewell=False, broken_greet=False))
            completed("fixed greet")
        else:
            marker.parent.mkdir(parents=True, exist_ok=True)
            marker.touch()
            write("Cargo.toml", CARGO)
            write("src/lib.rs", lib(farewell=False, broken_greet=True))
            completed("added greet (acceptance intentionally failing)")
else:
    print(json.dumps({"summary": "no-op", "findings": []}))

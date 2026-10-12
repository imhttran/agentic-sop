#!/usr/bin/env python3
"""Switch SOP LARGE model without deleting saved configurations."""

import os
import re
import sys
import tempfile
from pathlib import Path

ENV = Path(".env")

PROFILES = {
    "claude": {
        "SOP_MODEL_LARGE_PROVIDER": "command",
        "SOP_MODEL_LARGE_NAME": "claude-opus-5-5",
        "SOP_MODEL_LARGE_LOCALITY": "cloud",
        "SOP_AGENT_COMMAND": '"claude -p --model claude-opus-5-5"',
        "ANTHROPIC_MODEL": "claude-opus-5-5",
    },
    "deepseek": {
        "SOP_MODEL_LARGE_PROVIDER": "ollama",
        "SOP_MODEL_LARGE_NAME": "deepseek-v4.1-flash:cloud",
        "SOP_MODEL_LARGE_LOCALITY": "cloud",
    },
}

START = "# BEGIN SOP LARGE MODEL:"
END = "# END SOP LARGE MODEL:"
KEYS = set().union(*(p.keys() for p in PROFILES.values()))


def managed_block(profile, active):
    prefix = "" if active else "# "
    lines = [f"{START} {profile}"]

    for key, value in PROFILES[profile].items():
        lines.append(f"{prefix}{key}={value}")

    lines.append(f"{END} {profile}")
    return "\n".join(lines)


def switch(profile):
    if not ENV.is_file():
        sys.exit("Error: .env not found. No changes made.")

    original = ENV.read_text()
    lines = original.splitlines(keepends=True)

    # Remove only previously managed blocks and known model keys.
    # Preserve unrelated settings and comments.
    result = []
    inside = False

    for line in lines:
        stripped = line.strip()

        if stripped.startswith(START):
            inside = True
            continue

        if stripped.startswith(END):
            inside = False
            continue

        if inside:
            continue

        match = re.match(
            r"^\s*#?\s*(SOP_MODEL_LARGE_[A-Z_]+|SOP_AGENT_COMMAND|ANTHROPIC_MODEL)\s*=",
            line,
        )

        if match and match.group(1) in KEYS:
            continue

        result.append(line)

    if inside:
        sys.exit("Error: unterminated managed block. No changes made.")

    content = "".join(result).rstrip() + "\n\n"

    for name in PROFILES:
        content += managed_block(name, name == profile) + "\n\n"

    # Atomic replacement; preserve existing file permissions.
    fd, temp_name = tempfile.mkstemp(prefix=".env.", dir=str(ENV.parent))

    try:
        os.fchmod(fd, ENV.stat().st_mode & 0o777)
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(temp_name, ENV)
    finally:
        if os.path.exists(temp_name):
            os.unlink(temp_name)

    print(f"LARGE model switched to {profile.upper()}")


def status():
    if not ENV.is_file():
        sys.exit("Error: .env not found.")

    values = {}

    for line in ENV.read_text().splitlines():
        match = re.match(r"^([A-Z_]+)=(.*)$", line)
        if match:
            values[match.group(1)] = match.group(2)

    print("LARGE model configuration:")
    for key in (
        "SOP_MODEL_LARGE_PROVIDER",
        "SOP_MODEL_LARGE_NAME",
        "SOP_MODEL_LARGE_LOCALITY",
    ):
        print(f"  {key}={values.get(key, '(unset)')}")


if __name__ == "__main__":
    if len(sys.argv) != 2 or sys.argv[1] not in ("claude", "deepseek", "status"):
        sys.exit("Usage: switch-large-model.py {claude|deepseek|status}")

    if sys.argv[1] == "status":
        status()
    else:
        switch(sys.argv[1])

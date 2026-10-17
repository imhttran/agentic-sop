# How to Set Up SOP with Claude

This guide connects SOP's model classes (SMALL, MEDIUM, LARGE) to Claude. All
settings go in `.env`, copied from [`.env.example`](../../.env.example). Routing
precedence and the full variable list are in
[CONFIGURATION.md](../reference/CONFIGURATION.md) and
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md).

Using Claude Code as the operator's client (the `/sop*` slash commands) is a
separate topic: see [CLAUDE-SKILLS.md](CLAUDE-SKILLS.md) and
[CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md).

## Two ways to connect

| Path | Provider | How SOP calls Claude | Needs |
| --- | --- | --- | --- |
| Claude Code CLI | `command` | Runs `SOP_AGENT_COMMAND`, pipes the JSON request on stdin, reads stdout | `claude` on PATH, signed in (`claude login`) or `ANTHROPIC_API_KEY` set |
| Anthropic API | `openai_compatible` | `POST {base}/v1/chat/completions` with the class's model name | `SOP_OPENAI_COMPATIBLE_API_KEY` set to an Anthropic API key |

What matters when choosing:

- **Per-class models.** The API path sends each class's `SOP_MODEL_<CLASS>_NAME`
  as the model, so SMALL, MEDIUM, and LARGE can each use a different Claude model.
  The CLI path ignores `*_NAME` (it is only the label SOP records); the model
  comes from `--model` in the command.
- **One CLI command.** `SOP_AGENT_COMMAND` is global. Every class that uses
  provider `command` runs the same command, so at most one class should use the
  CLI. Plan generation from a PRD also runs `SOP_AGENT_COMMAND`.
- **Effort.** SOP has no effort setting. Only the CLI path can set it, with
  `--effort`.

## Example: Haiku SMALL, Opus-low MEDIUM, Opus LARGE

```dotenv
SOP_MODEL_DEFAULT_CLASS=large

# Anthropic API endpoint, shared by every openai_compatible class.
SOP_OPENAI_COMPATIBLE_BASE_URL=https://api.anthropic.com
SOP_OPENAI_COMPATIBLE_API_KEY=sk-ant-...

# SMALL: Haiku through the API.
SOP_MODEL_SMALL_PROVIDER=openai_compatible
SOP_MODEL_SMALL_NAME=claude-haiku-5-5
SOP_MODEL_SMALL_LOCALITY=cloud

# MEDIUM: Opus at low effort through the CLI.
SOP_MODEL_MEDIUM_PROVIDER=command
SOP_MODEL_MEDIUM_NAME=claude-opus-5-5
SOP_MODEL_MEDIUM_LOCALITY=cloud
SOP_AGENT_COMMAND="claude -p --model claude-opus-5-5 --effort low"

# LARGE: Opus at default effort through the API.
SOP_MODEL_LARGE_PROVIDER=openai_compatible
SOP_MODEL_LARGE_NAME=claude-opus-5-5
SOP_MODEL_LARGE_LOCALITY=cloud
```

Remove the SMALL availability-fallback lines (`SOP_MODEL_SMALL_FALLBACK_*`) or
leave them; they apply only when SMALL's primary model is local.

Do not set `SOP_AGENT_MODEL` or `SOP_OPENAI_COMPATIBLE_MODEL` here: either one
overrides every class's model name.

## LARGE only: switching profiles

`.env.example` keeps three LARGE profiles in managed blocks:

| Profile | LARGE runs on |
| --- | --- |
| `claude` (default) | Claude Code CLI, `claude-opus-5-5` |
| `claude-api` | Anthropic API, `claude-opus-5-5` |
| `deepseek` | Ollama cloud, `deepseek-v4.1-flash:cloud` |

Switch the active one in `.env`:

```bash
python3 scripts/switch-large-model.py claude-api
```

```bash
python3 scripts/switch-large-model.py status
```

The script rewrites only its BEGIN/END blocks, so keep secrets outside them. It
also deletes `SOP_AGENT_COMMAND` lines outside its blocks, so for the
mixed-class example above, edit `.env` by hand instead of running the script.

## Check the setup

```bash
sop providers --models
```

Then run a small read-only request:

```bash
sop prompt --capability review "Summarize README.md in one line"
```

`providers.validate` (off by default) probes model availability before a run.
It has not been verified against the Anthropic endpoint; leave it off for the
API path until it has.

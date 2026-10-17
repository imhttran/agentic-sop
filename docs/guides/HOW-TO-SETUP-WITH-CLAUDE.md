# How to Set Up SOP with Claude

This guide connects SOP's model classes (SMALL, MEDIUM, LARGE) to Claude. All
settings go in `.env`, copied from [`.env.example`](../../.env.example). Routing
precedence and the full variable list are in
[CONFIGURATION.md](../reference/CONFIGURATION.md) and
[MODEL-ROUTING.md](../specs/MODEL-ROUTING.md).

## Where `.env` lives

Put `.env` in the project SOP manages, and run `sop` from that project's root.
Every `sop` command reads `.env` from the folder it is run in; it does not
search parent folders, and it does not read the `.env` in your agentic-sop
checkout. A variable you already exported wins over the file.

```bash
cd /path/to/your-project
cp /path/to/agentic-sop/.env.example .env
```

Add `.env` to that project's `.gitignore` before you put an API key in it. The
agentic-sop repository ignores its own `.env`, but your project may not.

To use the same settings in every project, export the variables in your shell
profile instead of keeping a `.env` per project.

Using Claude Code as the operator's client (the `/sop*` slash commands) is a
separate topic: see [CLAUDE-SKILLS.md](CLAUDE-SKILLS.md) and
[CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md).

## Two ways to connect

| Path | Provider | How SOP calls Claude | Needs |
| --- | --- | --- | --- |
| Claude Code CLI | `command` | Runs `SOP_AGENT_COMMAND` (the `sop-agent.sh` wrapper), which prompts Claude and returns SOP's JSON outcome | `claude` and `jq` on PATH, signed in (`claude login`) or `ANTHROPIC_API_KEY` set |
| Anthropic API | `openai_compatible` | `POST {base}/v1/chat/completions` with the class's model name | `SOP_OPENAI_COMPATIBLE_API_KEY` set to an Anthropic API key |

What matters when choosing:

- **The API path is read-only.** `openai_compatible` makes one chat call and
  returns text. It has no tools, so it cannot edit the repository, and SOP never
  applies a patch from model output. It serves only REVIEW, PLAN, DESIGN_TESTS,
  and DIAGNOSE_FAILURE; SOP rejects IMPLEMENT and FIX on it before any call.
  Put it only on a class that never runs implementation: never on
  `SOP_MODEL_DEFAULT_CLASS`, and not on any class when automatic routing
  (`SOP_MODEL_ROUTING_ENABLED=true`) can pick it for a task.
- **Per-class models.** The API path sends each class's `SOP_MODEL_<CLASS>_NAME`
  as the model, so SMALL, MEDIUM, and LARGE can each use a different Claude model.
  The CLI path ignores `*_NAME` (it is only the label SOP records); the model
  comes from `ANTHROPIC_MODEL` (or `SOP_CLAUDE_MODEL`).
- **One CLI command.** `SOP_AGENT_COMMAND` is global: every class that uses
  provider `command` runs the same command. Plan generation from a PRD also
  runs it.
- **Effort and model per CLI class.** SOP has no effort setting; the wrapper
  sets it. `SOP_CLAUDE_EFFORT` and `SOP_CLAUDE_MODEL` apply to every CLI class.
  `SOP_CLAUDE_EFFORT_<CLASS>` and `SOP_CLAUDE_MODEL_<CLASS>` (CLASS is `SMALL`,
  `MEDIUM`, or `LARGE`) override them for one class.

## The CLI wrapper

Point `SOP_AGENT_COMMAND` at `scripts/agents/sop-agent.sh` in your agentic-sop
checkout, by absolute path, since SOP runs it from the managed project:

```dotenv
SOP_AGENT_COMMAND="sh /path/to/agentic-sop/scripts/agents/sop-agent.sh"
```

Or install a copy beside `sop`, so `.env` does not depend on where the checkout
lives. From the agentic-sop checkout:

```bash
./install.sh --agent claude
```

It installs `sop-agent` into the same bin directory as `sop` and prints the
line for `.env`, for example `SOP_AGENT_COMMAND="$HOME/go/bin/sop-agent"`.
`./install.sh --all` includes it. Re-run the installer after pulling to update
the copy.

Do not use a bare `claude -p`. SOP sends a JSON request on stdin and expects a
JSON outcome back, and Claude Code in `-p` mode is denied file edits unless they
are allowed. The wrapper builds the prompt and allows Claude to:

- read and edit the repository;
- run `git status`, `git diff`, and a few Go commands;
- run your project's own validation commands (`validation.build`, `test`, and
  `lint` in `.agent-sdlc/config.yaml`), so it can check its work in any language.

Commit, push, merge, reset, and clean are not allowed. A validation command
containing `,` or `)`, such as `test -z "$(gofmt -l .)"`, cannot be expressed as
a Claude Code rule and is skipped; SOP still runs it afterwards.

If a task is already done, the wrapper has Claude reply that no change was
needed. SOP accepts that only when the project sets `validation:` and those
commands pass. Without `validation:`, the run fails and asks you to configure
validation.

To allow more, list extra Claude Code tool rules, comma-separated:

```dotenv
SOP_CLAUDE_ALLOWED_TOOLS=Bash(make:*),Bash(pytest:*)
```

Everything listed runs without asking, so keep commit, push, and other
SOP-owned operations off it.

Claude also loads your own Claude Code setup: `~/.claude` settings, hooks,
plugins, CLAUDE.md, skills, and MCP servers. Your personal instructions then
shape SOP's tasks, and every MCP server starts on every call. To run SOP's calls
without them:

```dotenv
SOP_CLAUDE_ISOLATE=1
```

The managed project's `.claude` settings and CLAUDE.md still load, and
`claude login` still works.

To cap and track what Claude spends:

```dotenv
SOP_CLAUDE_MAX_BUDGET_USD=2
SOP_CLAUDE_COST_LOG=/path/to/claude-cost.log
```

- `SOP_CLAUDE_MAX_BUDGET_USD` stops a call once its estimated spend reaches that
  many US dollars. The task fails with `claude stopped: error_max_budget_usd`;
  edits Claude made before the stop stay in the working tree for SOP to judge.
- `SOP_CLAUDE_COST_LOG` appends one line per call, for example
  `2026-10-17T18:02:11Z large IMPLEMENT success cost=0.4123 turns=14`. Keep it
  outside the repository, so the log is not part of the change SOP checks.

Both are off by default.

## Example: Haiku SMALL on the API, Opus LARGE on the CLI

LARGE is the default class, so it runs IMPLEMENT and FIX and must use the CLI
wrapper. SMALL uses the API for read-only work.

```dotenv
SOP_MODEL_DEFAULT_CLASS=large

# SMALL: Haiku through the Anthropic API (read-only capabilities only).
SOP_OPENAI_COMPATIBLE_BASE_URL=https://api.anthropic.com
SOP_OPENAI_COMPATIBLE_API_KEY=sk-ant-...
SOP_MODEL_SMALL_PROVIDER=openai_compatible
SOP_MODEL_SMALL_NAME=claude-haiku-5-5
SOP_MODEL_SMALL_LOCALITY=cloud

# LARGE: Opus through the CLI wrapper. Runs IMPLEMENT and FIX.
SOP_MODEL_LARGE_PROVIDER=command
SOP_MODEL_LARGE_NAME=claude-opus-5-5
SOP_MODEL_LARGE_LOCALITY=cloud
SOP_AGENT_COMMAND="sh /path/to/agentic-sop/scripts/agents/sop-agent.sh"
ANTHROPIC_MODEL=claude-opus-5-5
```

Send read-only requests to SMALL with `--model-class small`, for example
`sop prompt --model-class small --capability review "..."`. Keep automatic
routing off with this setup: it can route an implementation task to SMALL.

To also run MEDIUM on Claude, at low effort next to a default-effort LARGE:

```dotenv
SOP_MODEL_MEDIUM_PROVIDER=command
SOP_MODEL_MEDIUM_NAME=claude-opus-5-5
SOP_MODEL_MEDIUM_LOCALITY=cloud
SOP_CLAUDE_EFFORT_MEDIUM=low
```

SOP tells the wrapper which class it selected (`SOP_SELECTED_CLASS`), so the
override applies to MEDIUM only. It needs model routing to select the class:
`SOP_MODEL_DEFAULT_CLASS`, `--model-class`, or automatic routing.

Remove the SMALL availability-fallback lines (`SOP_MODEL_SMALL_FALLBACK_*`) or
leave them; they apply only when SMALL's primary model is local.

Do not set `SOP_AGENT_MODEL` or `SOP_OPENAI_COMPATIBLE_MODEL` here: either one
overrides every class's model name.

## LARGE only: switching profiles

`.env.example` keeps three LARGE profiles in managed blocks:

| Profile | LARGE runs on |
| --- | --- |
| `claude` (default) | Claude Code CLI through the wrapper, `claude-opus-5-5` |
| `claude-api` | Anthropic API, `claude-opus-5-5`; read-only, so only when LARGE is not the default class |
| `deepseek` | Ollama cloud, `deepseek-v4.1-flash:cloud` |

Switch the active one from the managed project's root. The script edits the
`.env` in the current folder and needs one to exist, so call it by its full
path:

```bash
python3 /path/to/agentic-sop/scripts/switch-large-model.py claude-api
```

```bash
python3 /path/to/agentic-sop/scripts/switch-large-model.py status
```

In the agentic-sop checkout itself, `make model-claude`, `make model-deepseek`,
and `make model-status` run the same script on the checkout's own `.env`; they
do not change a managed project's `.env`.

The script rewrites only its BEGIN/END blocks, so keep secrets outside them. It
also deletes `SOP_AGENT_COMMAND` lines outside its blocks, so for the
mixed-class example above, edit `.env` by hand instead of running the script.

## Check the setup

Run these in order; each step assumes the one before passed.

**1. Tools.** No model call:

```bash
command -v claude jq && claude --version
```

**2. The wrapper alone.** One Claude call, without SOP:

```bash
echo '{"capability":"REVIEW","task":"check","input":"Reply OK"}' | sh /path/to/agentic-sop/scripts/agents/sop-agent.sh
```

A working setup prints one JSON line with `"status":"completed"`. Otherwise
the wrapper prints the reason, such as `claude not found on PATH`,
`jq not found on PATH`, or Claude's own error.

**3. Through SOP.** From the managed project's root:

```bash
sop prompt --json --model-class large --capability review "Summarize README.md in one line"
```

Check `"status": "completed"` and, under `routing`, `"class": "large"` and
`"provider": "command"`. For a CLI class, `routing.model` is only the
`*_NAME` label; the model that ran is `ANTHROPIC_MODEL`. For an API class, run
the same with `--model-class small` and expect `"provider": "openai_compatible"`
and the class's model name. `sop prompt` writes its run under
`.agent-sdlc/runs/prompts/`; no `sop init` is needed.

If it fails:

| Symptom | Likely cause |
| --- | --- |
| `routing.provider` is not `command` or `openai_compatible` | `.env` is not in the folder you ran `sop` from, or an exported `SOP_MODEL_*` variable overrides it |
| Claude reports it is not logged in | Run `claude login`, or set `ANTHROPIC_API_KEY` |
| API request fails with 401 | `SOP_OPENAI_COMPATIBLE_API_KEY` is missing or wrong |
| `provider cannot IMPLEMENT` | That class uses `openai_compatible`, which is read-only |

`sop providers --models` does not confirm a Claude setup: it shows the
`command` provider as `unknown` and does not list `openai_compatible`.

`providers.validate` (off by default) probes model availability before a run.
It has not been verified against the Anthropic endpoint; leave it off for the
API path until it has.

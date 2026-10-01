# Using SOP from Claude Code

This guide shows how to install the SOP skills so Claude Code exposes the SOP
commands `/sop`, `/sop-prompt`, `/sop-plan`, `/sop-review`, `/sop-diagnose`,
`/sop-test`, and `/sop-implement`, and how each one calls SOP.

The commands are **thin aliases**. They pick a capability and call `sop prompt`; SOP
still owns routing, provider selection, validation, the lifecycle, and approval. The
normative behavior is in
[`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md); the skill contract is in
[`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md). The Zed integration is
described in [`ZED-SKILLS.md`](ZED-SKILLS.md); both platforms install the **same**
canonical skill tree, so the commands and their capabilities are identical.

## How Claude Code discovers skills

A Claude Code **skill** is a folder containing a `SKILL.md`. Claude Code exposes each
skill as a slash command — named by the folder or its frontmatter `name` — and can
also load it automatically when its description matches the conversation (unless the
skill sets `disable-model-invocation: true`). Skills load from several scopes; the two
this installer uses are:

| Scope    | Path                               | Applies to    |
| -------- | ---------------------------------- | ------------- |
| Personal | `~/.claude/skills/<name>/SKILL.md` | every project |
| Project  | `<project>/.claude/skills/<name>/` | that project  |

Consequences that matter here:

- **Flat layout.** Each command is a _direct child_ of the skills root.
- **Frontmatter.** Claude Code reads the YAML frontmatter only when the opening `---`
  is the file's first line; the folder name and `name:` must match.
- **Symlinked folders work.** A skill folder may be a symlink to a directory
  elsewhere on disk, which is exactly how the installer keeps the checkout as the
  single source of truth.
- **`disable-model-invocation`.** Set on a skill, it keeps the `/command` but removes
  the skill from Claude's autonomous catalog. `/sop-implement` uses it, so Claude
  cannot start a repository change on its own.
- **Live reload.** Claude Code watches `~/.claude/skills/` and the project's
  `.claude/skills/`; editing a `SKILL.md` takes effect without restarting (run
  `/reload-skills` after creating a skills directory that did not exist at startup).

## Install the commands

The SOP skills live in this repository's [`skills/`](../../skills) tree, which is the
single source of truth. Install links them into Claude Code's skills root:

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
make install                 # install the sop CLI (the skills call it)
make install-skills-claude   # link the SOP skills into ~/.claude/skills
```

`make install-skills-claude` links each skill folder into `~/.claude/skills/`, so the
commands are available in **every** project you open Claude Code in. The skills stay
live against your checkout: editing a `SKILL.md` takes effect without reinstalling.

Use the script directly for more control:

```bash
scripts/install-claude-skills.sh           # personal scope (same as make install-skills-claude)
scripts/install-skills.sh claude --project # project scope: ./.claude/skills
scripts/install-skills.sh claude --project ~/work/api
scripts/install-skills.sh claude --dry-run # print what would change
scripts/install-skills.sh claude --force   # replace a SOP-named symlink that points elsewhere
```

Uninstall (also safe and idempotent):

```bash
make uninstall-skills-claude
# or: scripts/install-skills.sh claude --uninstall [--project ...]
```

`make install-skills` (no suffix) installs every supported agent whose directory is
present; `make install-skills-claude` installs Claude Code specifically.

## The commands

| Command          | Capability         | Mutates? | Use for                                            |
| ---------------- | ------------------ | -------- | -------------------------------------------------- |
| `/sop`           | _general entry_    | no\*     | route a general request to the right capability    |
| `/sop-prompt`    | _CLI default_      | no\*     | an ad-hoc request with no capability chosen        |
| `/sop-plan`      | `plan`             | no       | plans, designs, approaches, comparisons            |
| `/sop-review`    | `review`           | no       | code, architecture, security, design review        |
| `/sop-diagnose`  | `diagnose_failure` | no       | build, test, lint, runtime, provider, CI failures  |
| `/sop-test`      | `design_tests`     | no       | test plans, cases, acceptance coverage, edge cases |
| `/sop-implement` | `implement`        | **yes**  | an explicit request to change the repository       |

\* `/sop` and `/sop-prompt` fix no capability, so the CLI's conservative read-only
default (`plan`) applies. They never become an implementation request on their own.

Example, from the Claude Code prompt:

```text
/sop-review review internal/provider for architectural problems

/sop-diagnose explain why the latest provider validation test failed

/sop-plan plan a cache layer for provider model discovery

/sop-test design regression tests for provider fallback prevention

/sop-implement add caching to provider model discovery
```

`/sop-implement` is the only mutating command. It enters SOP's governed
implementation lifecycle (plan → implement → deterministic validation → review →
quality gate → bounded fix → human approval boundary) exactly as a planned task does,
and it is hidden from Claude's autonomous catalog so Claude cannot start it on its
own — you invoke it explicitly.

Claude Code also lets **Claude** invoke a skill automatically when the description
matches. When it does, the same rules apply: Claude must call `sop prompt` and return
SOP's result, not perform the work itself.

Everything after the command name is prompt data. The aliases place it in SOP's
`$ARGUMENTS` position and pass it to `sop prompt` as a single argument — it is never
interpreted as a shell command, a lifecycle transition, or configuration. See
[Prompt text is untrusted](#prompt-text-is-untrusted).

## Where the work goes

Each command runs `sop prompt` in the project Claude Code is open on, so:

- runs are recorded under that project's `.agent-sdlc/runs/prompts/<run-id>/`;
- inspect one with `sop report prompts/<run-id>`, or `sop report` for the latest run;
- a blocked run in one project never affects another project.

The provider and model are chosen by SOP (see
[`../specs/MODEL-ROUTING.md`](../specs/MODEL-ROUTING.md) and
[`../specs/PROVIDERS.md`](../specs/PROVIDERS.md)). No skill names a provider or a model
class.

## Prompt text is untrusted

Prompt input stays data. Passing `; rm -rf .` or `$(whoami)` as a request does not
execute anything — the text is sent to the selected agent, and a repository mutation
requires an `implement` request through SOP's tool/harness authorization path. No skill
interprets prompt text as a shell command, an approval, a model class, or a provider.

## When `sop` is not installed

The skills drive the `sop` CLI, so every one of them tells the agent: if `sop` is not
on `PATH`, stop and report it (install it with `make install`), and do **not** answer
the request yourself or mutate the repository in SOP's place. Missing SOP fails closed;
this matters most for `/sop-implement`.

## Safety of the installer

- **No root.** Skills are per-user; the script warns if it is run as root.
- **Only SOP-owned entries.** It links the seven SOP commands and never deletes or
  overwrites an unrelated file or skill.
- **Idempotent.** Re-running reports `ok` for entries already linked.
- **Conflicts are refused, not clobbered.** If a SOP name already exists as a real
  file or directory, the script skips it and tells you; `--force` replaces only a
  SOP-named _symlink_ that points somewhere else.
- **Uninstall removes only what it installed** (symlinks pointing into this
  repository's `skills/`).

If `~/.claude/skills/sop` already exists as a real directory, move it aside before
installing so the `/sop` command can be linked. Claude Code reserves the folder names
`synced` and `anthropic-skills`; the SOP skills do not use them.

## Troubleshooting

| Symptom                                | Cause                                                                                                                                  |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| A command is missing from the `/` menu | The skill is not installed — run `make install-skills-claude`, then `/reload-skills`.                                                  |
| The command runs but fails immediately | `sop` is not on `PATH` — run `make install` and check `sop version`.                                                                   |
| Claude uses a SOP skill unprompted     | That is the model-invocable default; set `disable-model-invocation` or a `skillOverrides` entry to stop it (as `/sop-implement` does). |
| "provider cannot IMPLEMENT"            | The selected provider cannot mutate the repository; see [`../specs/AGENT-PROVIDER.md`](../specs/AGENT-PROVIDER.md).                    |

## See also

- [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) — the normative behavior of `sop prompt`.
- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md) — the canonical skill contract and capability map.
- [`ZED-SKILLS.md`](ZED-SKILLS.md) — the same commands for Zed.
- [`../reference/CLI.md`](../reference/CLI.md) — the CLI surface, including `sop prompt`.
- [`PLAN-Phase-5.4-Unified-Work-Items.md`](../plans/PLAN-Phase-5.4-Unified-Work-Items.md) — where the skills came from.

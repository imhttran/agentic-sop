# Using SOP from Zed

This guide shows how to install the SOP skills so Zed's agent exposes the SOP
commands `/sop`, `/sop-prompt`, `/sop-plan`, `/sop-review`, `/sop-diagnose`,
`/sop-test`, and `/sop-implement`, and how each one calls SOP.

The commands are **thin aliases**. They pick a capability and call `sop prompt`; SOP
still owns routing, provider selection, validation, the lifecycle, and approval. The
normative behavior is in
[`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md); the skill contract is in
[`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md).

## How Zed discovers skills

A Zed **skill** is a folder containing a `SKILL.md`, and Zed exposes each skill as a
slash command (and an `@`-mention) named after the folder. Zed loads skills from two
roots only:

| Scope         | Path                               | Applies to        |
| ------------- | ---------------------------------- | ----------------- |
| Global        | `~/.agents/skills/<name>/SKILL.md` | every project     |
| Project-local | `<project>/.agents/skills/<name>/` | that project only |

Three consequences matter here:

- **Flat layout.** Each skill must be a _direct child_ of the skills root; nested
  folders are not discovered.
- **Trust.** Project-local skills load only from a trusted worktree; an untrusted
  clone's skills are excluded from the catalog and the slash menu until you trust it.
- **Names.** The folder name and the frontmatter `name:` must match and be lowercase
  letters, digits, and hyphens.

## Install the commands

The SOP skills live in this repository's [`skills/`](../../skills) tree, which is the
single source of truth. Install links them into a Zed skills root:

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
make install          # install the sop CLI (the skills call it)
make install-skills   # link the SOP skills into ~/.agents/skills
```

`make install-skills` links each skill folder into `~/.agents/skills/`, so the
commands are available in **every** project you open in Zed. The skills stay live
against your checkout: editing a `SKILL.md` takes effect without reinstalling.

Use the script directly for more control:

```bash
scripts/install-zed-skills.sh                    # global (same as make install-skills)
scripts/install-zed-skills.sh --project          # project-local: ./.agents/skills
scripts/install-zed-skills.sh --project ~/work/api
scripts/install-zed-skills.sh --dry-run          # print what would change
scripts/install-zed-skills.sh --force            # replace a SOP-named symlink that points elsewhere
```

Uninstall (also safe and idempotent):

```bash
make uninstall-skills
# or: scripts/install-zed-skills.sh --uninstall [--project ...]
```

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

Example, from Zed's agent message editor:

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
and it is hidden from the agent's autonomous catalog so the agent cannot start it on
its own — you invoke it explicitly.

Everything after the command name is prompt data, passed to SOP as a single argument.
It is never interpreted as a shell command, a lifecycle transition, or configuration.
See [Prompt text is untrusted](#prompt-text-is-untrusted).

## Where the work goes

Each command runs `sop prompt` in the project Zed is open on, so:

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

If `~/.agents/skills/sop` already exists as a real directory (an older nested layout,
for example), move it aside before installing so the `/sop` command can be linked.

## Troubleshooting

| Symptom                                | Cause                                                                                                               |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| A command is missing from the `/` menu | The skill is not installed, or an untrusted project's project-local skills are excluded.                            |
| No SOP commands at all                 | `~/.agents/skills/` has no SOP entries — run `make install-skills`.                                                 |
| The command runs but fails immediately | `sop` is not on `PATH` — run `make install` and check `sop version`.                                                |
| "provider cannot IMPLEMENT"            | The selected provider cannot mutate the repository; see [`../specs/AGENT-PROVIDER.md`](../specs/AGENT-PROVIDER.md). |

## See also

- [`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md) — the normative behavior of `sop prompt`.
- [`../../skills/sop/SKILL.md`](../../skills/sop/SKILL.md) — the canonical skill contract and capability map.
- [`../reference/CLI.md`](../reference/CLI.md) — the CLI surface, including `sop prompt`.
- [`PLAN-Phase-5.4-Unified-Work-Items.md`](../plans/PLAN-Phase-5.4-Unified-Work-Items.md) — where the skills came from.

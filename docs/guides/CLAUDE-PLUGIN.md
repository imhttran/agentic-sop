# The Claude Code plugin

Agentic SOP ships a native Claude Code plugin. It is a thin adapter over the same
canonical skill tree the rest of SOP uses: every command it exposes calls
`sop prompt`, and SOP keeps owning capability routing, model-class selection, provider
validation, the lifecycle, and approval.

The plugin is packaged at [`integrations/claude/`](../../integrations/claude), and the
repository is a marketplace that lists it
([`.claude-plugin/marketplace.json`](../../.claude-plugin/marketplace.json)).

## Install

Claude Code installs plugins from inside Claude Code, so `./install.sh --plugin claude`
prepares and validates the package and prints these steps rather than writing into your
Claude settings. Pick one:

**Load it for one session** (no change to your settings):

```bash
claude --plugin-dir <path-to-checkout>/integrations/claude
```

**Register this checkout as a marketplace**, then install the plugin:

```bash
claude plugin marketplace add <path-to-checkout>
claude plugin install sop@agentic-sop
```

**The same two steps from inside a session:**

```text
/plugin marketplace add <path-to-checkout>
/plugin install sop@agentic-sop
```

Check what loaded:

```bash
claude plugin list
claude plugin details sop     # Component inventory: Skills (7)
```

Remove it again with `claude plugin uninstall sop` (and
`claude plugin marketplace remove agentic-sop` to drop the marketplace).

> The marketplace and plugin namespaces reserved for Anthropic (`claude-plugins-official`
> and `anthropic-skills`) are not used here. Publishing to Anthropic's directory is out
> of scope for this phase; the local package and marketplace are proven instead.

## The commands

Claude Code namespaces every plugin component under the plugin name, so the plugin named
`sop` exposes these:

| Command in Claude Code | Capability          | Mutates? | Use for                                            |
| ---------------------- | ------------------- | -------- | -------------------------------------------------- |
| `/sop:sop`             | _general entry_     | no       | route a general request to the right capability    |
| `/sop:sop-prompt`      | _CLI default_       | no       | an ad-hoc request with no capability chosen        |
| `/sop:sop-plan`        | `plan`              | no       | plans, designs, approaches, comparisons            |
| `/sop:sop-review`      | `review`            | no       | code, architecture, security, design review        |
| `/sop:sop-diagnose`    | `diagnose_failure`  | no       | build, test, lint, runtime, provider, CI failures  |
| `/sop:sop-test`        | `design_tests`      | no       | test plans, cases, acceptance coverage, edge cases |
| `/sop:sop-implement`   | `implement`         | **yes**  | an explicit request to change the repository       |

The same names also work unprefixed — `/sop-review`, `/sop-plan`, … — when no other
skill claims them, which is Claude Code's own behaviour for plugin skills. If you install
the skills *and* the plugin, both routes exist and both call `sop prompt`.

`/sop:sop-implement` is the only mutating command. It runs SOP's governed implementation
lifecycle exactly as a planned task does, and its skill sets
`disable-model-invocation: true`, the official metadata that hides it from Claude's
autonomous catalog — a repository change is always an explicit operator choice.

## Where the plugin's content comes from

```text
skills/sop-review/SKILL.md                    ← the single source of truth
        │  scripts/packaging/build-claude-plugin.sh     (a verbatim mirror)
        ▼
integrations/claude/skills/sop-review/SKILL.md
```

[`skills/`](../../skills) remains the only place an SOP command is defined. The plugin's
skill files are a **generated mirror**, so the two can never diverge by hand:

```bash
scripts/packaging/build-claude-plugin.sh          # regenerate the mirror
scripts/packaging/build-claude-plugin.sh --check  # fail if it has drifted (also in `make check`)
```

Only `integrations/claude/skills/` and `integrations/claude/README.md` are generated;
`integrations/claude/.claude-plugin/plugin.json` is hand-written metadata. The
validator in [`internal/dist`](../../internal/dist) asserts the mirror is current, the
manifest and marketplace parse, the seven commands map to the right capabilities, and
that no packaged file contains provider access, policy settings, model names, or
direct-mutation instructions.

Because the mirror is the canonical text, its skill bodies describe the *skill*
installer (`./install.sh --skills claude`). As a plugin user you already have the
commands; the full contract is
[`skills/sop/SKILL.md`](../../skills/sop/SKILL.md).

## Safety

- **Thin by construction.** The plugin selects a capability and calls `sop prompt`. It
  contains no SMALL/MEDIUM/LARGE policy, no provider or model selection, no JEV or
  approval policy, no lifecycle transitions, and no escalation or recovery rules.
- **No direct access.** No packaged file invokes `ollama`, `llama.cpp`, MLX, OpenRouter,
  or any other provider, and none names a model.
- **Fails closed.** If `sop` is not on `PATH`, every command stops and tells the operator
  to install the SOP CLI; none of them performs the work itself.
- **Prompt text stays data.** What follows the command name is passed to SOP as one
  argument; it is never interpreted as a shell command, a model class, or configuration.
- **Project isolation.** Commands run `sop prompt` in the project Claude Code is open on,
  so each project's `.agent-sdlc/` state stays its own.

## Validating

The authoritative check is Claude Code's own validator:

```bash
claude plugin validate --strict integrations/claude
claude plugin validate .            # the marketplace, and the plugin it lists
```

`./install.sh --plugin claude` (and `.\install.ps1 -Plugin claude` on Windows) runs the
strict validation for you when the `claude` CLI is available. That validator is Claude
Code's own command: like any `claude` invocation it may keep its own cache and settings
backup under your home. The installer itself writes nothing into Claude's configuration —
it does not register a marketplace, enable a plugin, or edit a settings file.

Local packaging is proven here; submitting to Anthropic's directory is a later step.

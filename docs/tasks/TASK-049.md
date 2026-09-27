# T049 --- Bundled End-to-End Skill and Install Script

> Ship the `sop-end-to-end` agent skill as a repo template and install it with
> the CLI.

## Status

DONE

## Objective

Give a fresh checkout a one-step install of both the CLI and the agent skill that
drives the workflow end to end:

```bash
./scripts/install.sh      # or: make install
```

which installs `sop` (`go install ./cmd/sop`) and links the bundled skill into
`~/.agents/skills/sop-end-to-end`.

## Dependencies

- T041–T048 (the workflow the skill drives)

## Scope

- `.agents/sop-end-to-end/SKILL.md`: the skill template (thin natural-language
  interface over `sop run`).
- `scripts/install.sh`: installs the CLI and links the skill; honours
  `SOP_SKILLS_DIR`.
- `Makefile`: `make install`.
- `README.md`: an installation subsection pointing at the script and skill.

## Rules

- The skill stays thin: it never reproduces the SOP state machine, quality gates,
  retries, or resume logic — `sop run` remains authoritative.
- The install script is idempotent, works from any CWD (resolves its own repo
  root), and never destroys an existing skill: a real directory is backed up
  before the symlink is created.
- The template lives in-repo at `.agents/sop-end-to-end` and is symlinked, so it
  stays in step with the repository.

## Tests

`sh -n scripts/install.sh` (syntax); the script is executable. (It is not run in
CI: it would modify the developer's global skills directory.)

## Acceptance Criteria

- [x] A `.agents/sop-end-to-end` skill template is committed.
- [x] `scripts/install.sh` installs the CLI and links the skill.
- [x] `make install` runs the script; the README documents it.
- [x] The script is idempotent and backs up an existing non-symlink skill.
- [x] `make check` passes.

## Git

Branch: `task/T049-end-to-end-skill`
Commit: `task(T049): bundle the end-to-end skill and install script`
PR: `[Task T049] Bundle the end-to-end skill and install script`

## Out of Scope

Installing the skill into a target project automatically from `sop init`;
publishing the skill to a registry.

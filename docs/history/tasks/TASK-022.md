# T022 --- Documentation and Lessons

## Status

DONE

## Objective

Make completion leave the repository understandable: the docs reflect what is
done and what remains, and a lessons document captures the engineering decisions
worth reusing.

## Dependencies

- Stages 0–21 (the work being documented)

## Scope

- `README.md`: current status reflects the completed V1 surface; add the `resume`
  command; note that `sop run` is the remaining CLI wiring.
- `docs/architecture/OVERVIEW.md`: add the components built after the first draft (commit
  gate, CI generation/remediation, merge gate, completion loop, resume, bootstrap,
  parallelism, handoff).
- `LESSONS.md`: new, concise engineering lessons.
- `docs/PLAN.md`: mark this stage done.

## Rules

- Document only what exists; do not invent components or commands.
- Prefer concise, accurate statements over exhaustive prose.
- Reference authoritative artifacts by path instead of duplicating them.

## Tests

Documentation-only; validation is `make check` plus a read-through that every
named command and component exists.

## Acceptance Criteria

- [x] README current status matches the implemented surface.
- [x] `sop resume` is documented.
- [x] ARCHITECTURE lists the post-draft components.
- [x] `LESSONS.md` exists and is accurate.
- [x] PLAN marks the stage done.
- [x] `make check` passes.

## Git

Branch: `task/T022-documentation`
Commit: `task(T022): update documentation and add lessons`
PR: `[Task T022] Update documentation and add lessons`

## Out of Scope

Wiring `sop run`; generating a checked-in `TASKS.md`.

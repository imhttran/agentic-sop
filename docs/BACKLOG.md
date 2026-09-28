# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](PLAN.md); this file is
the running list in between.

## Bootstrap resilience: run the Ollama agent as a known-good binary

The bootstrap agent can break itself while editing `internal/ollamaagent`: a small
compile error in that very package removes the agent needed to repair it, because
SOP builds and runs the harness from the same tree under edit.

Install (and run) a pinned, prebuilt known-good `sop-ollama-agent` binary — outside
the tree under edit — so a broken working copy can still be repaired by the agent.
Recovery must not depend on the tool the change is allowed to break.

## Local network service (team mode)

Share SOP state and control across a team on the local network.

## Small-device dashboard

A read-only view of SOP state for small devices.

## Jev adapter and Jev-vs-deterministic evaluation

An adapter for the Jev planner, and an evaluation comparing it with the
deterministic planner.

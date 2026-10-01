# Example: implement

Use only when the operator **explicitly** asks to change the repository. Never
upgrade a read-only request into an implementation.

```bash
sop prompt \
  --capability implement \
  "Add caching to provider model discovery."
```

`implement` runs SOP's governed implementation lifecycle: plan → implement →
deterministic validation → review → quality gate → bounded fix → human approval
boundary. It writes the standard run artifacts under
`.agent-sdlc/runs/prompts/<run-id>/`.

It cannot run on a provider that does not declare `IMPLEMENT`; SOP fails clearly
rather than sending an edit-style request to a text-only model.

Do not satisfy this request by editing files or running shell/git yourself. The
skill's job is to invoke SOP and return SOP's result.

# Security Specification

**Type:** Normative specification

## Purpose

Defines the required security behavior of SOP Controller: local-first binding,
network mode, request safety, the discovery boundary, and secret handling.

## Related Specifications

- [WORKFLOW.md](WORKFLOW.md)
- [ACTIVITY.md](ACTIVITY.md)
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md)
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md)

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Binding and Network Mode

- The controller MUST be local-first: it MUST bind to a loopback address by
  default.
- The controller MUST refuse to bind a non-loopback address unless network mode
  (`SOP_CONTROLLER_ALLOW_NETWORK=true`) is explicitly enabled.
- Network mode MUST require an access token (`SOP_CONTROLLER_TOKEN`); the
  controller MUST refuse to start without one.

## Authentication and Sessions

- In network mode, every request except the sign-in page (`/login`), `/healthz`, and
  static assets MUST require an authenticated session.
- The token MUST be submitted through the sign-in form and validated server-side; the
  controller MUST NOT accept the token from a URL, and MUST use constant-time comparison.
- A successful sign-in MUST mint an opaque, server-side session delivered in a cookie
  that is `Secure` over TLS, `HttpOnly`, and `SameSite`, with a lifetime that MUST NOT
  exceed 30 days. The configured token MUST NOT appear in a URL, a response body, a log,
  or browser-readable storage (including `localStorage`).
- The controller MUST store only a hash of the session identifier — never the raw value —
  in memory and in any persisted store.
- A session MUST expire after its bounded lifetime, MUST be invalidated immediately by
  sign-out, and MUST NOT survive a change to the configured token.
- A remembered session MUST survive a normal controller restart: the controller MUST
  persist it to an owner-only local file (`SOP_CONTROLLER_SESSION_FILE`) containing only
  hashed identifiers and the token's fingerprint, and that file MUST NOT be committed. An
  operator MAY disable persistence by setting the path empty, in which case sessions end
  on restart.
- Repeated failed sign-ins MUST be throttled.

## Request Safety

- State-changing requests MUST be `POST` and MUST carry a CSRF token
  (double-submit cookie).
- Requesting an action MUST NOT bypass SOP's gates: the only actions the
  controller offers MUST delegate to SOP's own operations.

## Discovery Boundary

- Project discovery MUST be an allowlist: the controller MUST read only explicit
  projects and directories beneath explicitly configured workspace roots.
- The controller MUST NOT scan the wider filesystem and MUST NOT infer roots.
- Discovery MUST NOT follow symlinks, and MUST NOT traverse metadata or build
  directories (`.git`, `node_modules`, `vendor`, `dist`, `build`, `target`,
  `.cache`).
- Discovery MUST be depth-bounded (`SOP_CONTROLLER_DISCOVERY_DEPTH`).
- A conflict where two roots claim the same project identity MUST be refused and
  reported, not silently resolved.

## Data Access

- The browser MUST NOT reach SQLite. Every read MUST be server-side through
  `internal/sopclient`.
- The controller MUST NOT write SOP persistence from the web layer.
- The controller MUST interact with SOP through the supported boundary only. See
  [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md).

## Secret Handling

- The controller MUST NOT render secrets, environment variables, API keys, or
  agent credentials.
- The controller MUST NOT forward model/provider secrets to a SOP command. The
  only environment it adds is `SOP_ACTIVITY=on` (observer-only). See
  [ACTIVITY.md](ACTIVITY.md).
- Free-form text (blocking reasons, change summaries, activity detail, approval
  evidence) MUST be sanitized before display so it cannot leak prompts, secrets,
  or file contents.

## Related Documentation

- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — network mode
  variables.
- [../guides/PROJECT-DISCOVERY.md](../guides/PROJECT-DISCOVERY.md) — discovery in
  context.
- [../requirements/PRD.md](../requirements/PRD.md) — V1 security requirements.

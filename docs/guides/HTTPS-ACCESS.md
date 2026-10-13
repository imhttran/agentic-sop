# Secure HTTPS Access

**Type:** Guide

Serve the controller to an iPhone over certificate-validated HTTPS on the local network,
without exposing it publicly. Caddy is a local TLS reverse proxy in front of a
loopback-bound controller.

```text
iPhone  ──HTTPS──▶  Caddy (LAN, :8443)  ──HTTP──▶  127.0.0.1:8090  ──▶  SOP Controller
```

Only Caddy's HTTPS listener is reachable on the LAN; the controller stays on loopback and
keeps its access token and CSRF protection.

## Prerequisites

- **Caddy** on `PATH` (`brew install caddy`).
- The **Go** toolchain (the launcher builds the controller).
- Nothing else to prepare: the launcher keeps the access token in the **macOS
  Keychain** and creates one on first run (see below). An explicit
  `SOP_CONTROLLER_TOKEN` in the environment overrides the Keychain for a single run.

## Start

```bash
scripts/https-proxy.sh          # or: make https
```

The launcher builds the controller, resolves the access token, starts the controller on
`127.0.0.1:8090` with token auth enforced, starts Caddy, and prints the URLs. Ctrl+C stops
both.

```text
SOP Controller → Caddy HTTPS proxy

Controller (loopback): http://127.0.0.1:8090
HTTPS (LAN):           https://Hoangs-MacBook-Pro-2.local:8443
Approvals:             https://Hoangs-MacBook-Pro-2.local:8443/approvals
```

The hostname defaults to the Mac's Bonjour name (`<LocalHostName>.local`), which an
iPhone on the same Wi-Fi resolves over mDNS. Override it with
`SOP_CONTROLLER_HTTPS_HOST`, or the ports with `SOP_CONTROLLER_HTTPS_PORT` and
`SOP_CONTROLLER_UPSTREAM_PORT`. Stop a backgrounded proxy with
`scripts/https-proxy.sh stop`.

## Access token in the Keychain

The launcher loads the token from the login Keychain — service `sop-controller`, account
`controller-token` — and, when none exists, generates a strong random one and stores it
there (never overwriting an existing one). It is used only as the controller's own
environment variable: never a command argument, never printed, never logged, never
committed.

```bash
scripts/keychain-token.sh init     # create one if absent (idempotent; never overwrites)
scripts/keychain-token.sh rotate   # replace it with a new one (ends existing sessions)
make token-rotate                  # shortcut for the line above
```

To reveal the token so you can type it on the sign-in page (keep it out of logs):

```bash
security find-generic-password -s sop-controller -a controller-token -w   # or: make token-show
```

`rotate` stores a new token; the running controller keeps the old one until you restart
it, and any signed-in browser session ends. Setting `SOP_CONTROLLER_TOKEN` in the
environment overrides the Keychain for that run only.

## Trust the certificate on the iPhone

Caddy issues the certificate from its own **local CA**, so the iPhone must trust that CA
once. The CA is at `.caddy/caddy/pki/authorities/local/root.crt` (never committed).

1. **Install the CA.** AirDrop `root.crt` to the iPhone (or open it by any means) and
   tap it. Settings → **General → VPN & Device Management → Downloaded Profile →
   Install**.
2. **Enable full trust.** Settings → **General → About → Certificate Trust Settings** →
   turn on full trust for **Caddy Local Authority**.
3. **Open the dashboard** in Safari at `https://<host>:8443/`.
4. **Confirm** Safari shows a valid secure connection (padlock, no warning).

> Do not consider the iPhone trusted until you have verified step 3–4 **on the physical
> device**. Emulation (curl, a desktop browser) validates the certificate chain and
> hostname, but not iOS's trust store.

The first visit opens the sign-in page, where you enter your access token once (it sets a
session cookie; the token is never placed in the URL). Reveal the stored token with the
`security find-generic-password …` command above.

```text
https://<host>:8443/login
```

After that, open `https://<host>:8443/approvals` directly — the device is remembered for up
to **30 days**: the session persists across navigation, browser restarts, and controller
restarts until it expires, and **Sign out** in the top bar ends it. The token is never
logged (the controller logs only the request path, not the query or body).

To revoke every remembered device at once, run `make sessions-clear` and restart the
controller (a running controller keeps its in-memory sessions until then). The startup
summary reports the effective lifetime and whether sessions are persisted.

## Security

- **Loopback controller.** The controller binds `127.0.0.1` only; it is not reachable on
  the LAN directly. Caddy is the only LAN listener.
- **Certificate-validated TLS.** Caddy's local CA — no ACME, no public internet, and
  certificate verification is never disabled.
- **Authentication and CSRF preserved.** In network mode the controller requires a
  login; the token is entered once at `/login` and carried in a `Secure`, `HttpOnly`,
  `SameSite` session cookie, with the double-submit CSRF cookie on every state-changing
  request. The token is never placed in a URL or logged.
- **Proxy headers.** The controller honors `X-Forwarded-Proto` only from the loopback
  proxy (to mark cookies `Secure`) and otherwise ignores `X-Forwarded-*` and `Host`, so
  nothing the proxy adds affects authorization or CSRF.
- **Secrets.** Caddy's CA and certificate private keys live under the ignored `.caddy/`
  directory and are never committed. The access token lives in the macOS Keychain and is
  passed only as the controller's environment variable — never a command argument, and no
  token or key is printed, logged, or embedded in the documented URLs.

## Verify

```bash
CA=.caddy/caddy/pki/authorities/local/root.crt
HOST=$(scutil --get LocalHostName).local

# TLS validates against the local CA and the hostname matches.
curl --cacert "$CA" "https://$HOST:8443/healthz"

# The controller is loopback-only.
lsof -nP -iTCP:8090 -sTCP:LISTEN        # 127.0.0.1:8090

# It is not exposed on the LAN over HTTP.
curl "http://$(ipconfig getifaddr en0):8090/healthz"   # connection refused
```

## Limitations

- The **iPhone trust step is manual** and physical; the guide cannot verify it.
- `.local` resolution depends on mDNS/Bonjour working on the network.
- Port `8443` avoids needing root; adjust `SOP_CONTROLLER_HTTPS_PORT` if you prefer another.
- Deleting `.caddy/` regenerates the CA and requires re-trusting it on the iPhone.

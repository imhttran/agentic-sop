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
- **`SOP_CONTROLLER_TOKEN`** set in your environment — the access token the proxy
  preserves. It is never printed and never committed.
- The **Go** toolchain (the launcher builds the controller).

## Start

```bash
export SOP_CONTROLLER_TOKEN="choose-a-token"
scripts/https-proxy.sh          # or: make https
```

The launcher builds the controller, starts it on `127.0.0.1:8090` with token auth
enforced, starts Caddy, and prints the URLs. Ctrl+C stops both.

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

The first visit needs your access token once, which sets a cookie:

```text
https://<host>:8443/?token=<your-token>
```

After that, open `https://<host>:8443/approvals` directly. The token is never printed by
the launcher and never logged (the controller logs only the request path, not the query).

## Security

- **Loopback controller.** The controller binds `127.0.0.1` only; it is not reachable on
  the LAN directly. Caddy is the only LAN listener.
- **Certificate-validated TLS.** Caddy's local CA — no ACME, no public internet, and
  certificate verification is never disabled.
- **Authentication and CSRF preserved.** The controller keeps `SOP_CONTROLLER_TOKEN` and
  the double-submit CSRF cookie; requests through the proxy are authenticated exactly as
  on loopback.
- **Proxy headers.** The controller does not trust `X-Forwarded-*` or the `Host` header,
  so nothing the proxy adds affects authorization or CSRF.
- **Secrets.** Caddy's CA and certificate private keys live under the ignored `.caddy/`
  directory and are never committed; no token or key is printed or embedded in the
  documented URLs.

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

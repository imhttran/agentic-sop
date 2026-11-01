# HOWTO: Track and control a SOP run from a local dashboard (and your iPhone)

This guide wires the **`sop-controller`** web dashboard to an `agentic-sop`
project so you can watch a run and drive SOP commands from a browser — locally,
and from an iPhone on the same Wi‑Fi.

The dashboard lives in a separate repo:

```
~/agentic-workspace/projects/sop-controller
```

**SOP stays the workflow authority.** The dashboard only observes
(`.agent-sdlc/state.db`, read-only) and *commands* (`sop run`, `resume`,
`validate`, `review`, `retry`). It keeps no second source of truth, and it
cannot bypass SOP's human gates.

```text
iPhone / browser
      |
      v
sop-controller   (Go HTTP server: templates + HTMX; loopback by default)
      |
      v
sop CLI          (commands: run / resume / validate / review / retry)
      |
      v
<project>/.agent-sdlc/state.db   (read-only)  +  run artifacts
```

---

## 0. Prerequisites

| Need | Check | Notes |
| --- | --- | --- |
| Go | `go version` | 1.24+ required (verified here with go1.27.1) |
| `sop` CLI | `which sop` | On this machine: `/Users/imhttran/go/bin/sop` |
| A SOP project | `ls <project>/.agent-sdlc/state.db` | Example used below: `/Users/imhttran/agentic-workspace/agentic-sop` |
| Dashboard repo | `ls ~/agentic-workspace/projects/sop-controller` | Already present |

No Node, no npm, no PostgreSQL. Server-rendered HTML only.

The dashboard runs SOP commands through `SOP_BIN` (default `sop`) — so whatever
`sop` is on the dashboard process's `PATH` is what actually executes. Start the
dashboard from a shell where `which sop` resolves.

---

## 1. Build the dashboard

```sh
cd ~/agentic-workspace/projects/sop-controller
make build          # -> .run/sop-controller
```

Verified: `go build ./cmd/sop-controller` succeeds on this machine.

---

## Part A — Local use (Mac browser only)

This is the default and safest mode: loopback only, no token, nothing exposed to
the network.

## 2. Point it at your SOP project

The dashboard reads every directory listed in `SOP_CONTROLLER_PROJECTS`
(comma-separated). Each must contain `.agent-sdlc/state.db`.

```sh
cd ~/agentic-workspace/projects/sop-controller
export SOP_CONTROLLER_PROJECTS=/Users/imhttran/agentic-workspace/agentic-sop
```

Without this it defaults to `.` (the controller repo itself), which shows that
project, not the one you want to watch.

## 3. Run it

Foreground:

```sh
make run
# -> http://127.0.0.1:8080
```

Or background (writes pid + log under `.run/`):

```sh
make start     # build + run in the background
make status    # is it up? pings /healthz
make logs      # tail the log
make stop      # shut it down
```

Change the port with `make start ADDR=127.0.0.1:9000`.

Open <http://127.0.0.1:8080> on the Mac.

## 4. What you'll see

| View | URL | Shows |
| --- | --- | --- |
| Projects | `/projects` | progress + total / done / running / blocked counts |
| Project | `/projects/{id}` | task states, dependencies, why a task is blocked |
| Task | `/projects/{id}/tasks/{task}` | state, attempts, branch, deps, latest failure |
| Activity | (on task/project) | attempt events; raw transcripts are secondary |
| Review | (on task) | review findings + severity from run artifacts |
| CI / validation | (on task) | build / test / lint results and reasons |
| Handoff | (on task) | handoff status, compressor error, carry-forward facts |

The task list, activity, review, CI, and handoff panels refresh automatically
via HTMX polling (default every `3s`).

## 5. Commands you can trigger

Buttons map to real SOP CLI calls (via `SOP_BIN`):

| Button | Runs | Effect |
| --- | --- | --- |
| Run | `sop run` | continue / execute the active plan |
| Resume | `sop resume` | act on interrupted work |
| Validate | `sop validate` | run configured build / test / lint |
| Review | `sop review` | run the configured review engine |
| Report | `sop report` | summary of the latest run |
| Retry | `sop retry <task>` | requeue a single `BLOCKED` task |

These run on the Mac, not in the browser. The dashboard never mutates state
directly, is double-click safe, and is restart-safe.

`human.approval_before_commit: true` still holds: a run stops at the human gate
and nothing is committed from the dashboard.

---

## Part B — Control it from your iPhone (same Wi‑Fi)

The Mac and iPhone are on the same network (verified: Mac LAN IP
`192.168.86.249`). LAN exposure is an **explicit opt-in** and **requires an
access token** — the server refuses to start a network bind without one, and
every route except `/healthz` and `/static/` is gated.

## 6. Create a strong token

```sh
openssl rand -hex 32
```

Copy the value; you'll paste it into the URL once in step 9. Treat it like a
password — anyone on your Wi‑Fi with it can trigger SOP commands.

## 7. Start the dashboard in network mode

Bind to all interfaces (`0.0.0.0`) so the phone can reach it:

```sh
cd ~/agentic-workspace/projects/sop-controller
export SOP_CONTROLLER_PROJECTS=/Users/imhttran/agentic-workspace/agentic-sop
export SOP_CONTROLLER_ALLOW_NETWORK=true
export SOP_CONTROLLER_TOKEN='<paste-token-from-step-6>'
make start ADDR=0.0.0.0:8080
```

Why the flags matter:

- `SOP_CONTROLLER_ALLOW_NETWORK=true` — without it, a non-loopback `ADDR` is
  silently rewritten to `127.0.0.1` (so the phone could never connect).
- `SOP_CONTROLLER_TOKEN=...` — required whenever network mode is on.
- `ADDR=0.0.0.0:8080` — bind all interfaces. (On the `make` line this overrides
  any `SOP_CONTROLLER_ADDR` in `.env`, because `make` passes it as a real
  environment variable.)

Prefer a repeatable setup? Put the three variables in
`~/agentic-workspace/projects/sop-controller/.env` (the server auto-loads it),
then just run `make start ADDR=0.0.0.0:8080`. Keep the token out of version
control.

## 8. Find the address to use on the phone

```sh
ipconfig getifaddr en0        # Wi-Fi IPv4, e.g. 192.168.86.249
```

Two options:

- IP: `http://192.168.86.249:8080`
- Bonjour name (survives DHCP IP changes): `http://Hoangs-MacBook-Pro-2.local:8080`

If the IP came back empty, you're on Ethernet (`en1`/`en2`) or a VPN is
intercepting; check `ifconfig` or turn the VPN off for LAN access.

## 9. Open it on the iPhone

In Safari, open your URL **with the token as a query parameter**:

```
http://192.168.86.249:8080/?token=<paste-token-from-step-6>
```

On success this sets an access cookie, so you don't need the token again in that
browser. Then:

1. Tap the project to see task states.
2. Use **Add to Home Screen** (Share → Add to Home Screen) for an app-like
   launcher on the same URL.
3. Bookmark the plain `http://192.168.86.249:8080/` for later — the cookie is
   already set.

The UI is responsive: desktop tables collapse to cards on small screens.

## 10. First-connection checklist

- **macOS firewall prompt** — the first time, macOS may ask whether the binary
  may accept incoming connections. Choose **Allow**. If it was denied, re-enable
  it in System Settings → Network → Firewall → Options.
- **Same SSID** — the phone must be on the same Wi‑Fi (not a guest/isolated
  SSID, not cellular).
- **Port free** — only one process can own `8080` (`lsof -nP -iTCP:8080 -sTCP:LISTEN`).
- **Token accepted** — a wrong/missing token returns `401 access token required`.

---

## Troubleshooting

| Symptom | Cause | Fix |
| --- | --- | --- |
| `access token required` (401) | Opening without `?token=`, or cookie not set | Re-open the URL with `?token=<token>` |
| `invalid CSRF token` (403) | A state-changing POST without the CSRF cookie/token | Do it in the browser UI; don't hand-craft POSTs |
| Phone can't connect, Mac can | Bound to loopback only | Set `SOP_CONTROLLER_ALLOW_NETWORK=true` and `ADDR=0.0.0.0:8080` |
| Server exits: `ALLOW_NETWORK=true requires SOP_CONTROLLER_TOKEN` | Network mode without a token | Set `SOP_CONTROLLER_TOKEN` |
| Wrong project's tasks shown | `SOP_CONTROLLER_PROJECTS` unset (defaults to `.`) | Point it at the project root |
| A command fails as if no agent | Dashboard inherited a bad provider env (e.g. a stale `SOP_AGENT_PROVIDER` / `SOP_AGENT_COMMAND`) | Start the dashboard from a clean shell, or export the provider you intend |
| `run` says a different plan is active | Active plan still has unresolved work | Finish it, or use `sop reconcile <PLAN.md>` (SOP-side; not a dashboard action) |
| Worked yesterday, not today | DHCP changed the Mac's IP | Use the `.local` name, or reserve a static IP |

---

## Optional — control it from anywhere (not just home Wi‑Fi)

LAN mode only works on the same network. To reach the dashboard from anywhere
without exposing a port to the internet, put the phone and Mac on a private mesh
VPN such as Tailscale, then browse to the Mac's Tailscale address:

```sh
export SOP_CONTROLLER_ALLOW_NETWORK=true
export SOP_CONTROLLER_TOKEN='<token>'
make start ADDR=0.0.0.0:8080
# then, from the phone on the tailnet:
#   http://<mac-tailscale-name>:8080/?token=<token>
```

Still keep the token on. Do **not** forward port `8080` on your router — the
dashboard speaks plain HTTP and is meant for a trusted private network.

---

## Environment reference

| Variable | Default | Purpose |
| --- | --- | --- |
| `SOP_CONTROLLER_ADDR` | `127.0.0.1:8080` | Listen address (loopback enforced unless network mode is on) |
| `SOP_CONTROLLER_PROJECTS` | `.` | Comma-separated project roots containing `.agent-sdlc/state.db` |
| `SOP_BIN` | `sop` | SOP CLI used for run / resume / validate / review / report / retry |
| `SOP_CONTROLLER_POLL` | `3s` | HTMX polling cadence |
| `SOP_CONTROLLER_COMMAND_TIMEOUT` | `15m` | Upper bound for a single SOP command |
| `SOP_CONTROLLER_ALLOW_NETWORK` | `false` | Opt in to a non-loopback bind (requires a token) |
| `SOP_CONTROLLER_TOKEN` | — | Required access token when network mode is on |

## Security notes

- Loopback by default; network mode is explicit and token-gated.
- The browser never reaches SQLite — all reads go through the server's SOP
  boundary, and state is read-only.
- State-changing requests are `POST` with a double-submit CSRF cookie.
- No secrets or environment variables are rendered into pages.
- LAN traffic is **plain HTTP**. That's acceptable on a trusted home network;
  on a phone, prefer a private mesh VPN (above) over exposing the port.
- The access token is a bearer credential. Rotate it by restarting with a new
  value.

## Quick start (copy/paste)

Local only:

```sh
cd ~/agentic-workspace/projects/sop-controller
SOP_CONTROLLER_PROJECTS=/Users/imhttran/agentic-workspace/agentic-sop make run
# open http://127.0.0.1:8080
```

iPhone on the same Wi‑Fi:

```sh
cd ~/agentic-workspace/projects/sop-controller
export SOP_CONTROLLER_PROJECTS=/Users/imhttran/agentic-workspace/agentic-sop
export SOP_CONTROLLER_ALLOW_NETWORK=true
export SOP_CONTROLLER_TOKEN="$(openssl rand -hex 32)"
echo "token: $SOP_CONTROLLER_TOKEN"
make start ADDR=0.0.0.0:8080
ipconfig getifaddr en0   # then open http://<that-ip>:8080/?token=<token> in Safari
```

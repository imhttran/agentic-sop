#!/bin/sh
# Local HTTPS access for the SOP Controller, through Caddy.
#
#   iPhone → HTTPS (Caddy) → HTTP 127.0.0.1:$UPSTREAM → SOP Controller
#
# The controller binds to loopback and keeps its token authentication; only Caddy's
# HTTPS listener is reachable on the LAN. Caddy issues the certificate from its own local
# CA (no ACME, no public internet), so the certificate is valid once that CA is trusted on
# the iPhone — see docs/guides/HTTPS-ACCESS.md. No token, key, or credential is printed.
#
# Usage:
#   scripts/https-proxy.sh          start the controller + proxy (Ctrl+C stops both)
#   scripts/https-proxy.sh stop     stop a proxy started earlier
#
# Environment (all optional):
#   SOP_CONTROLLER_TOKEN          access token; overrides the Keychain when set
#   SOP_CONTROLLER_HTTPS_HOST     LAN hostname (default: <LocalHostName>.local)
#   SOP_CONTROLLER_HTTPS_PORT     Caddy HTTPS port (default: 8443)
#   SOP_CONTROLLER_UPSTREAM_PORT  controller loopback port (default: 8090)
#   SOP_CONTROLLER_PROJECTS       projects the controller observes (default: this repo)
#
# The access token is loaded from the macOS Keychain (service "sop-controller",
# account "controller-token") and created there on first run; see
# scripts/keychain-token.sh. The token is never printed, logged, or passed as a
# command argument.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
run_dir="$repo_dir/.run"
data_dir="$repo_dir/.caddy"
pid_file="$run_dir/https-proxy.pid"

mode=${1:-start}

local_name=$(scutil --get LocalHostName 2>/dev/null || hostname -s)
host=${SOP_CONTROLLER_HTTPS_HOST:-"$local_name.local"}
https_port=${SOP_CONTROLLER_HTTPS_PORT:-8443}
upstream_port=${SOP_CONTROLLER_UPSTREAM_PORT:-8090}
projects=${SOP_CONTROLLER_PROJECTS:-$repo_dir}

stop_proxy() {
	if [ -f "$pid_file" ]; then
		kill "$(cat "$pid_file")" 2>/dev/null || true
		rm -f "$pid_file"
	fi
}

case "$mode" in
stop)
	stop_proxy
	echo "https-proxy: stopped"
	exit 0
	;;
start) ;;
*)
	echo "usage: scripts/https-proxy.sh [start|stop]" >&2
	exit 2
	;;
esac

command -v caddy >/dev/null 2>&1 || {
	echo "https-proxy: caddy not found (install it: brew install caddy)" >&2
	exit 1
}

# Resolve the access token: an explicit SOP_CONTROLLER_TOKEN wins, then the macOS
# Keychain, then a freshly generated token stored in the Keychain (never overwriting
# an existing one). The value stays in a shell variable — never an argument, a log,
# or the printed banner — and is exported only into the controller's own environment.
token=${SOP_CONTROLLER_TOKEN:-}
if [ -z "$token" ]; then
	token=$("$repo_dir/scripts/keychain-token.sh" get 2>/dev/null || true)
fi
if [ -z "$token" ]; then
	"$repo_dir/scripts/keychain-token.sh" init
	token=$("$repo_dir/scripts/keychain-token.sh" get)
fi
if [ -z "$token" ]; then
	echo "https-proxy: no access token (set SOP_CONTROLLER_TOKEN or run scripts/keychain-token.sh init)" >&2
	exit 1
fi

mkdir -p "$run_dir" "$data_dir"

# Build the controller once (the Go toolchain is the only build dependency).
( cd "$repo_dir" && go build -o "$run_dir/sop-controller" ./cmd/sop-controller )

# Start the controller bound to loopback, with token authentication enforced, so the
# proxy preserves auth even though the listener is local.
(
	cd "$repo_dir"
	# Export into this subshell only: the token reaches the controller but not Caddy or
	# the parent shell, and it is never an argument to any command.
	export SOP_CONTROLLER_TOKEN="$token"
	export SOP_CONTROLLER_ADDR="127.0.0.1:$upstream_port"
	export SOP_CONTROLLER_ALLOW_NETWORK=true
	export SOP_CONTROLLER_PROJECTS="$projects"
	exec "$run_dir/sop-controller" >>"$run_dir/https-proxy.log" 2>&1
) &
controller_pid=$!
echo "$controller_pid" >"$pid_file"

# Wait for the controller to answer on loopback.
i=0
while [ "$i" -lt 60 ]; do
	if curl -fsS "http://127.0.0.1:$upstream_port/healthz" >/dev/null 2>&1; then break; fi
	sleep 0.25
	i=$((i + 1))
done

ca_path="$data_dir/caddy/pki/authorities/local/root.crt"
cat <<EOF

SOP Controller → Caddy HTTPS proxy

Controller (loopback): http://127.0.0.1:$upstream_port
HTTPS (LAN):           https://$host:$https_port
Approvals:             https://$host:$https_port/approvals

Sign in once with your access token (the form posts it to the server; it is never printed
here and never placed in a URL):
  https://$host:$https_port/login
The token is stored in the macOS Keychain. To reveal it (to type at /login):
  security find-generic-password -s sop-controller -a controller-token -w
Then open:
  https://$host:$https_port/approvals

Trust Caddy's local CA on the iPhone first (once per device):
  $ca_path
See docs/guides/HTTPS-ACCESS.md for the iPhone steps.
Press Ctrl+C to stop.

EOF

# Stop the controller when Caddy exits or the script is interrupted.
trap 'kill "$controller_pid" 2>/dev/null || true; rm -f "$pid_file"' INT TERM EXIT

SOP_CONTROLLER_HTTPS_SITE="$host:$https_port, localhost:$https_port" \
SOP_CONTROLLER_UPSTREAM_PORT="$upstream_port" \
XDG_DATA_HOME="$data_dir" \
XDG_CONFIG_HOME="$data_dir" \
	caddy run --config "$repo_dir/caddy/Caddyfile" --adapter caddyfile </dev/null

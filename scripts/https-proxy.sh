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
# Environment (all optional except SOP_CONTROLLER_TOKEN):
#   SOP_CONTROLLER_TOKEN          required: the access token the proxy preserves
#   SOP_CONTROLLER_HTTPS_HOST     LAN hostname (default: <LocalHostName>.local)
#   SOP_CONTROLLER_HTTPS_PORT     Caddy HTTPS port (default: 8443)
#   SOP_CONTROLLER_UPSTREAM_PORT  controller loopback port (default: 8090)
#   SOP_CONTROLLER_PROJECTS       projects the controller observes (default: this repo)
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

if [ -z "${SOP_CONTROLLER_TOKEN:-}" ]; then
	echo "https-proxy: set SOP_CONTROLLER_TOKEN first; the proxy preserves token authentication" >&2
	exit 1
fi
command -v caddy >/dev/null 2>&1 || {
	echo "https-proxy: caddy not found (install it: brew install caddy)" >&2
	exit 1
}

mkdir -p "$run_dir" "$data_dir"

# Build the controller once (the Go toolchain is the only build dependency).
( cd "$repo_dir" && go build -o "$run_dir/sop-controller" ./cmd/sop-controller )

# Start the controller bound to loopback, with token authentication enforced, so the
# proxy preserves auth even though the listener is local.
(
	cd "$repo_dir"
	exec env \
		SOP_CONTROLLER_ADDR="127.0.0.1:$upstream_port" \
		SOP_CONTROLLER_ALLOW_NETWORK=true \
		SOP_CONTROLLER_PROJECTS="$projects" \
		"$run_dir/sop-controller" >>"$run_dir/https-proxy.log" 2>&1
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

First visit needs your access token once (it sets a cookie; the token is never printed
here and never stored in this output):
  https://$host:$https_port/?token=<your-token>
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

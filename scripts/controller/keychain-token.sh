#!/bin/sh
# Store and load the SOP Controller access token in the macOS Keychain.
#
#   scripts/keychain-token.sh get      print the stored token (exits 1 if absent)
#   scripts/keychain-token.sh init     create a strong random token if none exists (never overwrites)
#   scripts/keychain-token.sh rotate   replace the token with a new random one (ends existing sessions)
#   scripts/keychain-token.sh set      store the token read from standard input (fails if one exists)
#
# The token lives in the login Keychain under service "sop-controller", account
# "controller-token". It is never taken from a command argument and never printed
# except by `get`, which is meant to be captured, never echoed. Nothing here writes
# the token to a log. Uses only the built-in `security` command.
#
# Typical use from another script:
#   token=$(scripts/keychain-token.sh get)
set -eu

service=${SOP_CONTROLLER_KEYCHAIN_SERVICE:-sop-controller}
account=${SOP_CONTROLLER_KEYCHAIN_ACCOUNT:-controller-token}

has_item() {
	security find-generic-password -s "$service" -a "$account" >/dev/null 2>&1
}

# read_token prints the stored token; reading an existing item never prompts.
read_token() {
	security find-generic-password -s "$service" -a "$account" -w
}

# generate prints a strong random token (128 bits, 32 characters; a trailing newline is
# fine, callers read one line).
generate() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 16
	else
		LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32
	fi
}

# run_detached runs `security` in a new session so it reads the password from
# standard input instead of prompting on the controlling terminal (its password
# reader prefers /dev/tty and hangs a non-interactive script otherwise). The secret
# stays on the pipe; only flags are passed as arguments.
run_detached() {
	if command -v perl >/dev/null 2>&1; then
		exec perl -e 'use POSIX (); POSIX::setsid(); exec @ARGV or die "exec: $!\n"' "$@"
	fi
	if command -v python3 >/dev/null 2>&1; then
		exec python3 -c 'import os, sys; os.setsid(); os.execvp(sys.argv[1], sys.argv[1:])' "$@"
	fi
	echo "keychain-token: need perl or python3 to store a token without prompting" >&2
	exit 1
}

# store reads the token from standard input (one line) and writes it. UPDATE=1
# replaces an existing token; otherwise the write fails rather than overwrite.
store() {
	token=""
	IFS= read -r token || true
	if [ -z "$token" ]; then
		echo "keychain-token: no token on standard input" >&2
		exit 1
	fi
	update_arg=""
	if [ "${UPDATE:-0}" = "1" ]; then
		update_arg="-U"
	fi
	# `security` asks for the password twice (entry + confirmation), both from stdin.
	# Its prompts go to stderr and are captured so a success stays quiet; a genuine
	# failure (for example a locked keychain) is still printed.
	if ! err=$(printf '%s\n%s\n' "$token" "$token" |
		run_detached security add-generic-password $update_arg -s "$service" -a "$account" -w 2>&1); then
		printf '%s\n' "$err" >&2
		exit 1
	fi
}

case "${1:-}" in
get)
	if ! has_item; then
		echo "keychain-token: no token stored (service $service, account $account)" >&2
		exit 1
	fi
	read_token
	;;
init)
	# Idempotent: never overwrite an existing token.
	if ! has_item; then
		generate | store
	fi
	;;
rotate)
	# Explicit replacement: a new token invalidates any existing sessions.
	# UPDATE is a shell variable, so the pipeline subshell running `store` sees it.
	UPDATE=1
	generate | store
	;;
set)
	if has_item; then
		echo "keychain-token: a token already exists; use 'rotate' to replace it" >&2
		exit 1
	fi
	store
	;;
*)
	echo "usage: scripts/keychain-token.sh [get|init|rotate|set]" >&2
	exit 2
	;;
esac

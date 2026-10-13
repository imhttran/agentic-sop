# SOP Controller — developer commands.
#
#   make run      foreground (Ctrl-C to stop)
#   make start    build + run in the background
#   make stop     stop the background server
#
# `make` with no target prints this list.

SHELL := /bin/sh

BIN  := .run/sop-controller
PID  := .run/sop-controller.pid
LOG  := .run/sop-controller.log
ADDR ?= 127.0.0.1:8080
# Remembered-device session store; override with SOP_CONTROLLER_SESSION_FILE (mirrors the controller).
SOP_CONTROLLER_SESSION_FILE ?= .run/sessions.json

.PHONY: help run start stop restart status logs https https-stop token-rotate token-show sessions-clear build test test-race fmt vet tidy clean

help:
	@echo "SOP Controller"
	@echo ""
	@echo "  make run       run in the foreground (Ctrl-C to stop)"
	@echo "  make start     build + run in the background"
	@echo "  make stop      stop the background server"
	@echo "  make restart   stop, then start"
	@echo "  make status    is it running? (also pings /healthz)"
	@echo "  make logs      tail the background log"
	@echo "  make https     run behind the Caddy HTTPS proxy (token from the macOS Keychain)"
	@echo "  make https-stop  stop the HTTPS proxy"
	@echo "  make token-rotate  replace the Keychain access token (ends existing sessions)"
	@echo "  make token-show    print the Keychain access token (to sign in on a phone)"
	@echo "  make sessions-clear  delete persisted remembered-device sessions"
	@echo ""
	@echo "  make build     build ./cmd/sop-controller to $(BIN)"
	@echo "  make test      go test ./..."
	@echo "  make test-race go test -race ./..."
	@echo "  make fmt       gofmt -w ."
	@echo "  make vet       go vet ./..."
	@echo "  make tidy      go mod tidy"
	@echo "  make clean     remove $(BIN), PID, and logs"
	@echo ""
	@echo "  ADDR=$(ADDR)  (override the listen address)"

run:
	SOP_CONTROLLER_ADDR=$(ADDR) go run ./cmd/sop-controller

# Start the controller on loopback behind the Caddy HTTPS proxy (see docs/guides/HTTPS-ACCESS.md).
https:
	./scripts/https-proxy.sh

https-stop:
	./scripts/https-proxy.sh stop

# Replace the access token stored in the macOS Keychain (see docs/guides/HTTPS-ACCESS.md).
token-rotate:
	./scripts/keychain-token.sh rotate

# Print the access token so it can be typed on the sign-in page. Keep it out of logs.
token-show:
	@./scripts/keychain-token.sh get

# Delete the persisted remembered-device sessions, so every remembered browser signs in
# again after the controller restarts. A running controller keeps its in-memory sessions
# until it restarts, so follow this with `make restart` (or re-run the HTTPS launcher).
sessions-clear:
	@rm -f "$(SOP_CONTROLLER_SESSION_FILE)"
	@echo "cleared remembered sessions ($(SOP_CONTROLLER_SESSION_FILE)) — restart the controller to apply"

build:
	@mkdir -p .run
	go build -o $(BIN) ./cmd/sop-controller

start: build
	@mkdir -p .run
	@if [ -f $(PID) ] && kill -0 "$$(cat $(PID))" 2>/dev/null; then \
		echo "already running (pid $$(cat $(PID))) → http://$(ADDR)"; \
	else \
		SOP_CONTROLLER_ADDR=$(ADDR) nohup $(BIN) >>$(LOG) 2>&1 & echo $$! >$(PID); \
		sleep 1; \
		if kill -0 "$$(cat $(PID))" 2>/dev/null; then \
			echo "started (pid $$(cat $(PID))) → http://$(ADDR)   (logs: make logs)"; \
		else \
			echo "failed to start — see $(LOG)"; rm -f $(PID); exit 1; \
		fi; \
	fi

stop:
	@if [ -f $(PID) ] && kill -0 "$$(cat $(PID))" 2>/dev/null; then \
		pid=$$(cat $(PID)); kill "$$pid"; \
		i=0; while kill -0 "$$pid" 2>/dev/null && [ $$i -lt 25 ]; do sleep 0.2; i=$$((i+1)); done; \
		kill -0 "$$pid" 2>/dev/null && kill -9 "$$pid"; \
		rm -f $(PID); echo "stopped (pid $$pid)"; \
	else \
		echo "not running"; rm -f $(PID); \
	fi

restart: stop start

status:
	@if [ -f $(PID) ] && kill -0 "$$(cat $(PID))" 2>/dev/null; then \
		echo "running (pid $$(cat $(PID))) → http://$(ADDR)"; \
		if curl -fsS "http://$(ADDR)/healthz" >/dev/null 2>&1; then echo "health: ok"; else echo "health: not responding yet"; fi; \
	else \
		echo "not running"; \
	fi

logs:
	@mkdir -p .run; touch $(LOG); tail -f $(LOG)

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	@rm -rf .run
	@echo "cleaned"

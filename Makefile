# Developer convenience targets. The documented installation entry point is
# ./install.sh (see docs/guides/INSTALLATION.md); these targets delegate to it so there
# is one installation implementation and Make is never a second one. The scripts under
# scripts/ are support tooling, grouped by responsibility (install/, packaging/,
# checks/, agents/).

.PHONY: fmt vet test test-fast build check check-plugin install install-all install-ollama-agent \
	plugin install-plugin \
	install-skills install-skills-zed install-skills-claude \
	uninstall-skills uninstall-skills-zed uninstall-skills-claude \
	model-claude model-deepseek model-status \
	controller-build controller-https controller-keychain-token

ENV_FILE := .env

# `make test-fast` runs the suite with TMPDIR on RAM-backed storage where the
# platform provides it (Linux /dev/shm), which removes the SQLite file-I/O
# bottleneck without changing any SQLite durability setting. `make test` below is
# unchanged and remains the ordinary, disk-backed run. Override any input on the
# command line, e.g. `make test-fast TEST_FAST_PARALLEL=4 TEST_FAST_TIMEOUT=5m`.
# See docs/guides/DEVELOPMENT.md for when to use it and how it works.
TEST_FAST_PARALLEL ?= 2
TEST_FAST_TIMEOUT ?= 3m
TEST_FAST_TMPDIR ?=
TEST_FAST_MIN_FREE_MB ?= 512

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

# Fast suite: same tests, with TMPDIR on RAM-backed storage when available. The
# wrapper creates and cleans up a private per-invocation directory, falls back to
# the system temp dir on macOS/other platforms, and propagates the test exit
# status. Overridable via the TEST_FAST_* variables above.
test-fast:
	SOP_TEST_PARALLEL='$(TEST_FAST_PARALLEL)' \
	SOP_TEST_TIMEOUT='$(TEST_FAST_TIMEOUT)' \
	SOP_TEST_TMPDIR='$(TEST_FAST_TMPDIR)' \
	SOP_TEST_MIN_FREE_MB='$(TEST_FAST_MIN_FREE_MB)' \
	./scripts/checks/test-fast.sh

build:
	go build ./...

# The repository's canonical pre-commit check.
check: fmt vet test build check-plugin

# Verify the packaged Claude Code plugin still mirrors the canonical skills/ tree.
check-plugin:
	./scripts/packaging/build-claude-plugin.sh --check

# Install the sop CLI (and nothing else). Add `./install.sh --all` for everything.
install:
	./install.sh

# Install the sop CLI plus every agent integration whose environment is present.
install-all:
	./install.sh --all

# Prepare the Claude Code plugin package and print the official install steps.
install-plugin:
	./install.sh --plugin claude

# Regenerate the packaged Claude Code plugin from skills/ (it is a generated mirror).
plugin:
	./scripts/packaging/build-claude-plugin.sh

# The skill roots are installed by scripts/install/install-skills.sh, which owns that
# logic; ./install.sh --skills <agent> delegates to it. Use the script directly for
# --project/--uninstall/--force.
install-skills:
	./scripts/install/install-skills.sh all

install-skills-zed:
	./scripts/install/install-skills.sh zed

install-skills-claude:
	./scripts/install/install-skills.sh claude

uninstall-skills:
	./scripts/install/install-skills.sh all --uninstall

uninstall-skills-zed:
	./scripts/install/install-skills.sh zed --uninstall

uninstall-skills-claude:
	./scripts/install/install-skills.sh claude --uninstall

# Install/update the pinned known-good sop-ollama-agent binary outside the tree
# under edit, so the bootstrap invocation never compiles candidate source.
install-ollama-agent: check
	./scripts/install/install-sop-ollama-agent.sh


model-claude:
	@python3 scripts/switch-large-model.py claude

.PHONY: model-deepseek

model-deepseek:
	@python3 scripts/switch-large-model.py deepseek
	@echo "DeepSeek configuration activated"
	@echo "unset SOP_AGENT_COMMAND ANTHROPIC_MODEL"

model-status:
	@python3 scripts/switch-large-model.py status

# Controller conveniences (namespaced; they never overwrite a core target). The
# unified `sop controller serve` command is the primary entry point; these build the
# legacy wrapper and drive its HTTPS proxy / Keychain token helpers under
# scripts/controller/ and integrations/controller/.
controller-build:
	go build -o "$(CURDIR)/bin/sop-controller" ./cmd/sop-controller

controller-https:
	./scripts/controller/https-proxy.sh

controller-keychain-token:
	./scripts/controller/keychain-token.sh

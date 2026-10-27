# Developer convenience targets. The documented installation entry point is
# ./install.sh (see docs/guides/INSTALLATION.md); these targets delegate to it so there
# is one installation implementation and Make is never a second one. The scripts under
# scripts/ are support tooling, grouped by responsibility (install/, packaging/,
# checks/, agents/).

.PHONY: fmt vet test test-fast test-planner-tunnel test-mcp-portability setup-planner build check check-fast check-plugin install install-all install-ollama-agent \
	security-integration \
	ollama-agent-pin-preflight ollama-agent-invalid-test-mode plugin install-plugin \
	install-skills install-skills-zed install-skills-claude \
	uninstall-skills uninstall-skills-zed uninstall-skills-claude \
	model-claude model-deepseek model-status \
	controller-build controller-https controller-keychain-token

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

# The test mode for the Ollama agent installation gate. `ordinary` keeps the
# established disk-backed suite; `fast` runs the same suite through
# scripts/checks/test-fast.sh. An unknown value fails the install before any
# gate or installation step runs.
TEST_MODE ?= ordinary

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

# Candidate lifecycle regression tests for the issue #90 planner tunnel workflow.
# Isolated mock processes and a temporary git checkout; never touches a live tunnel.
test-planner-tunnel:
	./scripts/checks/test-planner-tunnel-candidate.sh

# Point the planner MCP server at a SOP project, with sop_start_work, using a
# fresh sop from origin/main. PROJECT defaults to this checkout. Pass
# SETUP_PLANNER_ARGS for options, e.g. SETUP_PLANNER_ARGS=--register-claude.
PROJECT ?=
setup-planner:
	./scripts/setup-planner.sh $(SETUP_PLANNER_ARGS) $(PROJECT)

# Simulated cross-agent compatibility tests for the portable validation handoff.
# Exercises Claude -> ChatGPT -> second-agent transitions and invalid-artifact rejection.
test-mcp-portability:
	./scripts/checks/test-validation-handoff-portability.sh

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

# The same gates as check, with the test step using the fast runner. It is the
# `TEST_MODE=fast` form of the Ollama agent installation gate; `check` above
# remains the ordinary, disk-backed gate.
check-fast: fmt vet test-fast build check-plugin

# Verify the packaged Claude Code plugin still mirrors the canonical skills/ tree.
check-plugin:
	./scripts/packaging/build-claude-plugin.sh --check

# Real-CLI security integration gate: provisions pinned Semgrep, Gitleaks,
# cargo-audit, and OSV-Scanner, then runs the tagged suite. Missing prerequisites
# fail rather than skip. The OSV database is a controlled offline fixture.
security-integration:
	./scripts/checks/security-integration.sh

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
#
# The pin preflight runs before the verification gates, so a missing or
# unresolvable pinned revision fails immediately instead of after the full
# suite. TEST_MODE selects the test gate; every other gate (formatting, vet,
# build, and plugin verification) is unchanged. `make install-ollama-agent
# TEST_MODE=fast` is opt-in; the default remains the ordinary suite. An invalid
# mode fails before the gates and before the installer, so a typo never installs
# a binary.
ollama-agent-pin-preflight:
	./scripts/install/install-sop-ollama-agent.sh --check

# Prerequisite order keeps the cheap pin preflight ahead of the test gates in a
# serial make. An invalid TEST_MODE stops make before either runs.
ifneq ($(words $(TEST_MODE)),1)
install-ollama-agent: ollama-agent-invalid-test-mode ollama-agent-pin-preflight
else ifeq ($(filter $(TEST_MODE),ordinary fast),)
install-ollama-agent: ollama-agent-invalid-test-mode ollama-agent-pin-preflight
else ifeq ($(strip $(TEST_MODE)),fast)
install-ollama-agent: ollama-agent-pin-preflight check-fast
else
install-ollama-agent: ollama-agent-pin-preflight check
endif
	./scripts/install/install-sop-ollama-agent.sh

ollama-agent-invalid-test-mode:
	@echo "install-ollama-agent: invalid TEST_MODE '$(TEST_MODE)' (want ordinary or fast)" >&2
	@exit 2


model-claude:
	@python3 scripts/switch-large-model.py claude

model-deepseek:
	@python3 scripts/switch-large-model.py deepseek

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


show-ollama:
	ollama ps
	ollama list
	# nvidia-smi

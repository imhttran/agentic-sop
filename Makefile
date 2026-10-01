# Developer convenience targets. The documented installation entry point is
# ./install.sh (see docs/guides/INSTALLATION.md); these targets delegate to it so there
# is one installation implementation and Make is never a second one.

.PHONY: fmt vet test build check check-plugin install install-all install-ollama-agent \
	plugin install-plugin \
	install-skills install-skills-zed install-skills-claude \
	uninstall-skills uninstall-skills-zed uninstall-skills-claude

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build:
	go build ./...

# The repository's canonical pre-commit check.
check: fmt vet test build check-plugin

# Verify the packaged Claude Code plugin still mirrors the canonical skills/ tree.
check-plugin:
	./scripts/build-claude-plugin.sh --check

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
	./scripts/build-claude-plugin.sh

# The skill roots are installed by scripts/install-skills.sh, which owns that logic;
# ./install.sh --skills <agent> delegates to it. Use the script directly for
# --project/--uninstall/--force.
install-skills:
	./scripts/install-skills.sh all

install-skills-zed:
	./scripts/install-skills.sh zed

install-skills-claude:
	./scripts/install-skills.sh claude

uninstall-skills:
	./scripts/install-skills.sh all --uninstall

uninstall-skills-zed:
	./scripts/install-skills.sh zed --uninstall

uninstall-skills-claude:
	./scripts/install-skills.sh claude --uninstall

# Install/update the pinned known-good sop-ollama-agent binary outside the tree
# under edit, so the bootstrap invocation never compiles candidate source.
install-ollama-agent: check
	./scripts/install-sop-ollama-agent.sh

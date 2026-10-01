.PHONY: fmt vet test build check install install-ollama-agent \
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

check: fmt vet test build

install:
	./scripts/install.sh

# Link the SOP skills into the coding agents' skill roots so `/sop*` commands appear.
# `install-skills` installs every supported agent that is present; the per-agent
# targets install one. See docs/guides/ZED-SKILLS.md and docs/guides/CLAUDE-SKILLS.md;
# add `--project` to scripts/install-skills.sh for a project-local install.
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

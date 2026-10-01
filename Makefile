.PHONY: fmt vet test build check install install-ollama-agent install-skills uninstall-skills

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

# Link the SOP Zed skills into ~/.agents/skills so `/sop*` commands appear in Zed.
# See docs/guides/ZED-SKILLS.md; use scripts/install-zed-skills.sh --project for a
# project-local install.
install-skills:
	./scripts/install-zed-skills.sh

uninstall-skills:
	./scripts/install-zed-skills.sh --uninstall

# Install/update the pinned known-good sop-ollama-agent binary outside the tree
# under edit, so the bootstrap invocation never compiles candidate source.
install-ollama-agent: check
	./scripts/install-sop-ollama-agent.sh

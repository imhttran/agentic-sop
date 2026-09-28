.PHONY: fmt vet test build check install install-ollama-agent

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

# Install/update the pinned known-good sop-ollama-agent binary outside the tree
# under edit, so the bootstrap invocation never compiles candidate source.
install-ollama-agent: check
	./scripts/install-sop-ollama-agent.sh

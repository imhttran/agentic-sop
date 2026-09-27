.PHONY: fmt vet test build check install

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

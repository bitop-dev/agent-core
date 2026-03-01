BINARY := agent-core
MODULE := github.com/bitop-dev/agent-core
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X $(MODULE)/internal/config.Version=$(VERSION)"

.PHONY: build run test lint clean install

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/agent-core

run: build
	./bin/$(BINARY)

test:
	go test ./... -v -race

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

install: build
	cp bin/$(BINARY) $(GOPATH)/bin/$(BINARY)

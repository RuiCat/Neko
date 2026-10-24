# AstrBox Go — build/test entry points.
#
# The Go build cache lives under .local/ because the machine's default cache may
# be read-only in this environment.
GO ?= go
GOCACHE ?= $(CURDIR)/.local/go-cache
GOMODCACHE ?= $(CURDIR)/.local/gomodcache
export GOCACHE GOMODCACHE

BIN := bin/astrbox-mcp

.PHONY: all build test lint run clean

all: build

build: $(BIN)

$(BIN): $(shell find cmd internal -name '*.go') go.mod go.sum
	@mkdir -p bin
	$(GO) build -o $(BIN) ./cmd/astrbox-mcp

test:
	$(GO) test ./... -count=1

lint:
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed:"; gofmt -l cmd internal; exit 1)

run: build
	./$(BIN) -data-dir data -scripts-dir scripts -transport stdio

clean:
	rm -rf bin

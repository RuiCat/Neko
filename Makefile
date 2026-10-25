# AstrBox Go — build/test entry points.
#
# The Go build cache lives under .local/ because the machine's default cache may
# be read-only in this environment.

GO ?= go
GOCACHE ?= $(CURDIR)/.local/go-cache
GOMODCACHE ?= $(CURDIR)/.local/gomodcache
export GOCACHE GOMODCACHE

ENGINE_BIN := bin/astrbox-mcp
DSH_BIN := bin/dsh-mcp
GO_SOURCES := $(shell find cmd internal -name '*.go')
LINT_MODULARITY := $(HOME)/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh

.PHONY: all build test lint render-dsh doctor run clean

all: build

build: $(ENGINE_BIN) $(DSH_BIN)

$(ENGINE_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -o $(ENGINE_BIN) ./cmd/astrbox-mcp

$(DSH_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -o $(DSH_BIN) ./cmd/dsh-mcp

test:
	$(GO) test ./... -count=1
	@set -- dsh/lib/*Test.mjs; \
	if [ -e "$$1" ]; then \
		echo "node --test $$*"; \
		node --test "$$@"; \
	else \
		echo "SKIP: no dsh/lib/*Test.mjs tests"; \
	fi

lint:
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed:"; gofmt -l cmd internal; exit 1)
	@test -f "$(LINT_MODULARITY)" || (echo "缺少 $(LINT_MODULARITY)（code-modularity 技能未安装）" >&2; exit 1)
	bash "$(LINT_MODULARITY)" internal cmd dsh --naming --comments

# Render dsh/cordis.patch.yml from the read-only template.
render-dsh:
	bash scripts/renderDshPatch.sh

# Spawn bin/dsh-mcp, run the MCP handshake and check the frozen 17 tools.
doctor: $(DSH_BIN)
	node scripts/doctorDsh.mjs

run: build
	./$(ENGINE_BIN) -data-dir data -scripts-dir scripts -transport stdio

# Remove build outputs only; data/ holds local runtime state and is never cleaned.
clean:
	rm -rf bin

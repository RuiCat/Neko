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
CONTEXT_BIN := bin/neko-context
METRICS_BIN := bin/astrbox-metrics
REPLAY_BIN := bin/astrbox-replay
CHAT_BIN := bin/neko-chat
NIGHTLY_BIN := bin/astrbox-nightly
GO_SOURCES := $(shell find cmd internal -name '*.go')

# Design sources and the embedded page tree are build inputs: editing any of
# them must rebuild the binaries (K6 / C19).
DESIGN_SOURCES := design/tokens.json design/components.json design/composition.json
WEB_SOURCES := $(shell find internal/monitor/web -type f)
DESIGN_CSS := internal/monitor/web/tokens.css

# Build-time version; a source archive without .git falls back to "dev".
VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# code-modularity 技能脚本：默认全局安装；CI/换用户时用 MODULARITY_LINT=... 覆盖。
MODULARITY_LINT ?= $(HOME)/.dsh/skills/code-modularity/scripts/lintCodeModularity.sh

.PHONY: all build test lint design monitor render-dsh doctor run clean

all: build

build: $(ENGINE_BIN) $(DSH_BIN) $(CONTEXT_BIN) $(METRICS_BIN) $(REPLAY_BIN) $(CHAT_BIN) $(NIGHTLY_BIN)

$(ENGINE_BIN): $(GO_SOURCES) $(DESIGN_SOURCES) $(WEB_SOURCES) $(DESIGN_CSS) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(ENGINE_BIN) ./cmd/astrbox-mcp

$(DSH_BIN): $(GO_SOURCES) $(DESIGN_SOURCES) $(WEB_SOURCES) $(DESIGN_CSS) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(DSH_BIN) ./cmd/dsh-mcp

$(CONTEXT_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CONTEXT_BIN) ./cmd/neko-context

$(METRICS_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(METRICS_BIN) ./cmd/astrbox-metrics

$(REPLAY_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(REPLAY_BIN) ./cmd/astrbox-replay

$(CHAT_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(CHAT_BIN) ./cmd/neko-chat

$(NIGHTLY_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(NIGHTLY_BIN) ./cmd/astrbox-nightly

# Regenerate the CSS from design/tokens.json and copy it into the embedded page
# tree; both outputs are versioned so a clean checkout builds without node.
design: $(DESIGN_CSS)

$(DESIGN_CSS): $(DESIGN_SOURCES) design/dist/genTokens.py
	python3 design/dist/genTokens.py
	cp design/dist/tokens.css $(DESIGN_CSS)

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
	@test -f "$(MODULARITY_LINT)" || (echo "缺少 $(MODULARITY_LINT)（code-modularity 技能未安装；可用 MODULARITY_LINT=... 覆盖）" >&2; exit 1)
	bash "$(MODULARITY_LINT)" internal cmd dsh --naming --comments

# Render dsh/cordis.patch.yml from the read-only template.
render-dsh:
	bash scripts/tools/renderDshPatch.sh

# Spawn bin/dsh-mcp, run the MCP handshake and check the frozen 17 tools.
doctor: $(DSH_BIN)
	node scripts/tools/doctorDsh.mjs

# Start the read-only monitor UI (add http_token to config.json for writes).
monitor: $(DSH_BIN)
	./$(DSH_BIN) -monitor-addr 127.0.0.1:8765

run: build
	./$(ENGINE_BIN) -data-dir data -scripts-dir scripts -transport stdio

# Remove build outputs only; data/ holds local runtime state and is never cleaned.
clean:
	rm -rf bin

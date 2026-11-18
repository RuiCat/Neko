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
TRAIN_BIN := bin/astrbox-train
EMOTE_BIN := bin/astrbox-emote
DATA_BIN := bin/neko-data
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

.PHONY: all build test lint design monitor install-runtime install-plugins doctor run clean

all: build

build: $(ENGINE_BIN) $(DSH_BIN) $(CONTEXT_BIN) $(METRICS_BIN) $(REPLAY_BIN) $(CHAT_BIN) $(NIGHTLY_BIN) $(TRAIN_BIN) $(EMOTE_BIN) $(DATA_BIN)

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

$(TRAIN_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(TRAIN_BIN) ./cmd/astrbox-train

$(DATA_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(DATA_BIN) ./cmd/neko-data

$(EMOTE_BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p bin
	$(GO) build -ldflags "$(LDFLAGS)" -o $(EMOTE_BIN) ./cmd/astrbox-emote

# Regenerate the CSS from design/tokens.json and copy it into the embedded page
# tree; both outputs are versioned so a clean checkout builds without node.
design: $(DESIGN_CSS)

$(DESIGN_CSS): $(DESIGN_SOURCES) design/dist/genTokens.py
	python3 design/dist/genTokens.py
	cp design/dist/tokens.css $(DESIGN_CSS)

test:
	$(GO) test ./... -count=1
	@set -- plugins/dsh-astrbox/lib/*Test.mjs internal/monitor/web/*Test.mjs; \
	if [ -e "$$1" ]; then \
		echo "node --test $$*"; \
		node --test "$$@"; \
	else \
		echo "SKIP: no node tests"; \
	fi

lint:
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal)" || (echo "gofmt needed:"; gofmt -l cmd internal; exit 1)
	@test -f "$(MODULARITY_LINT)" || (echo "缺少 $(MODULARITY_LINT)（code-modularity 技能未安装；可用 MODULARITY_LINT=... 覆盖）" >&2; exit 1)
	bash "$(MODULARITY_LINT)" internal cmd plugins/dsh-astrbox --naming --comments

# 拷贝运行时（bin/scripts/config）到 ~/.dsh-neko/Neko。
# 用法：make install-runtime [NEKO_HOME=...] [PRUNE=1]；make -n install-runtime 为 dry-run。
NEKO_HOME ?= $(HOME)/.dsh-neko/Neko
PRUNE ?= 0

# 插件包安装（生产态）：按各包 files 清单打包后解到 ~/.dsh-neko/plugins。
# 用法：make install-plugins [PLUGIN_HOME=...]；make -n install-plugins 为 dry-run。
# 安装副本与仓库解耦：改完仓库代码要重跑本目标（DSH 侧用 link: 指向安装副本）。
PLUGIN_HOME ?= $(HOME)/.dsh-neko
PLUGINS_OUT ?= $(PLUGIN_HOME)/plugins
DSH_PROFILE_NODE_MODULES ?= $(HOME)/.dsh/profiles/node_modules
PLUGIN_PKGS := dsh-astrbox dsh-mcp-client dsh-qqbot dsh-plugin-diary dsh-plugin-bilibili

install-runtime: build
	@case "$(NEKO_HOME)" in "$(CURDIR)"|"$(CURDIR)"/*) echo "NEKO_HOME 不能位于仓库内: $(NEKO_HOME)" >&2; exit 1;; esac
	@mkdir -p "$(NEKO_HOME)/bin" "$(NEKO_HOME)/data" "$(NEKO_HOME)/scripts"
	install -m 0755 bin/dsh-mcp bin/astrbox-mcp "$(NEKO_HOME)/bin/"
	@if [ "$(PRUNE)" = "1" ]; then rm -rf "$(NEKO_HOME)/scripts"; mkdir -p "$(NEKO_HOME)/scripts"; fi
	@if command -v rsync >/dev/null 2>&1; then rsync -a --exclude '.git' scripts/ "$(NEKO_HOME)/scripts/"; else cp -a scripts/. "$(NEKO_HOME)/scripts/"; fi
	@if [ -f config.json ]; then cp -f config.json "$(NEKO_HOME)/config.json"; fi
	@if [ -d private ]; then mkdir -p "$(NEKO_HOME)/private"; rsync -a --exclude '.git' private/ "$(NEKO_HOME)/private/"; fi
	@if [ -f observation.example.json ] && [ ! -f "$(NEKO_HOME)/data/observation.json" ]; then \
		cp -f observation.example.json "$(NEKO_HOME)/data/observation.json"; \
	fi
	@# scripts.json 与 templates/ 都是派生数据：删掉让运行时下次启动从 scripts/ 重新引导（旧模板的组合会过期）。
	@rm -f "$(NEKO_HOME)/data/scripts.json"
	@rm -rf "$(NEKO_HOME)/data/templates"
	@if [ -f plugins/dsh-astrbox/bridge.config.example.json ] && [ ! -f "$(NEKO_HOME)/data/bridge.config.json" ]; then \
		cp -f plugins/dsh-astrbox/bridge.config.example.json "$(NEKO_HOME)/data/bridge.config.json"; \
		sed -i "s|__WORKSPACE_PREFIX__|$(NEKO_HOME)|" "$(NEKO_HOME)/data/bridge.config.json"; \
	fi
	@echo "runtime installed: $(NEKO_HOME)"

# 安装插件包到 $(PLUGINS_OUT)（默认 ~/.dsh-neko/plugins）：
#   1. 构建 qqbot 的 lib/（其余包的 lib/ 已随仓库签入）；
#   2. 按各包 package.json 的 files 清单 npm pack 后解包，避免把 src/、测试与 node_modules 带进安装根；
#   3. qqbot 的运行时依赖（含 peer）从仓库 node_modules 整体拷贝；
#   4. 重建安装根内的两条解析链（astrbox 相对入口 → 包装插件 → 宿主实现）。
install-plugins:
	@case "$(PLUGINS_OUT)" in "$(CURDIR)"|"$(CURDIR)"/*) echo "PLUGINS_OUT 不能位于仓库内: $(PLUGINS_OUT)" >&2; exit 1;; esac
	cd plugins/dsh-qqbot && npm run build
	@mkdir -p "$(PLUGINS_OUT)"
	@set -e; \
	for p in $(PLUGIN_PKGS); do \
		tmp=$$(mktemp -d); \
		printf 'pack %s\n' "$$p"; \
		(cd "plugins/$$p" && npm pack --silent --pack-destination "$$tmp" >/dev/null); \
		rm -rf "$(PLUGINS_OUT)/$$p"; mkdir -p "$(PLUGINS_OUT)/$$p"; \
		tar -xzf "$$tmp"/*.tgz -C "$(PLUGINS_OUT)/$$p" --strip-components=1; \
		rm -rf "$$tmp"; \
	done
	rsync -a --delete plugins/dsh-qqbot/node_modules/ "$(PLUGINS_OUT)/dsh-qqbot/node_modules/"
	@mkdir -p "$(PLUGINS_OUT)/dsh-mcp-client/node_modules/@deepseek-ai"
	ln -sfn "$(DSH_PROFILE_NODE_MODULES)/@deepseek-ai/dsh-mcp-client" "$(PLUGINS_OUT)/dsh-mcp-client/node_modules/@deepseek-ai/dsh-mcp-client"
	@echo "plugins installed: $(PLUGINS_OUT)"

# Spawn bin/dsh-mcp, run the MCP handshake and check the frozen tool set.
doctor: $(DSH_BIN)
	node tools/doctorDsh.mjs

# Start the read-only monitor UI (add http_token to config.json for writes).
monitor: $(DSH_BIN)
	./$(DSH_BIN) -monitor-addr 127.0.0.1:8765

run: build
	./$(ENGINE_BIN) -data-dir data -scripts-dir scripts -transport stdio

# Remove build outputs only; data/ holds local runtime state and is never cleaned.
clean:
	rm -rf bin

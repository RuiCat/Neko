// Command astrbox-mcp serves the AstrBox cat-girl engine as an MCP server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/events"
	"github.com/ruicat/astrbox/internal/function"
	"github.com/ruicat/astrbox/internal/mcpserver"
	"github.com/ruicat/astrbox/internal/registry"
)

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "state directory: entities.json, templates/, archives/, logs/")
	scriptsDir := flag.String("scripts-dir", defaultScriptsDir(), "scripts directory: scripts.json, *.neko, rules.json")
	transport := flag.String("transport", "stdio", "transport: stdio or http")
	addr := flag.String("addr", "127.0.0.1:8765", "listen address for -transport=http")
	configPath := flag.String("config", "config.json", "domain config JSON (missing file = defaults)")
	flag.Parse()
	domainConfig, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("astrbox: 配置加载失败: %v", err)
	}
	if err := config.Set(domainConfig); err != nil {
		log.Fatalf("astrbox: 配置无效: %v", err)
	}

	store := entity.NewStore(*dataDir)
	if err := store.Load(); err != nil {
		log.Fatalf("astrbox: 加载数据目录失败: %v", err)
	}
	if err := store.MigrateUserEntities(); err != nil {
		log.Printf("astrbox: 拆分用户实体失败: %v", err)
	}
	machine := engine.New(store)
	machine.SetDecisionLog(filepath.Join(*dataDir, "llm_decisions.json"))
	funcs := function.New(store)
	eventSystem := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	if rules, err := loadRules(filepath.Join(*scriptsDir, "rules.json")); err != nil {
		log.Printf("astrbox: rules.json 跳过: %v", err)
	} else if len(rules) > 0 {
		machine.AddRules(rules)
	}
	reg, err := registry.New(store, machine, filepath.Join(*dataDir, "scripts.json"), *scriptsDir)
	if err != nil {
		log.Fatalf("astrbox: 加载脚本注册表失败: %v", err)
	}
	reg.SetFunctionSink(eventSystem)
	diagStore := diagnostics.New(diagnostics.DEFAULT_CAPACITY)
	reg.SetDiagnostics(diagStore)
	replay := reg.ReplayAll()
	if len(reg.List()) == 0 {
		bootstrap, err := reg.BootstrapFromSources()
		if err != nil {
			log.Printf("astrbox: 从 scripts/ 引导失败: %v", err)
		} else if len(bootstrap.Replayed) > 0 || len(bootstrap.Failed) > 0 {
			log.Printf("astrbox: 注册表为空，已从 scripts/ 引导 %d 个脚本 (failed=%d)",
				len(bootstrap.Replayed), len(bootstrap.Failed))
		}
	}
	log.Printf("astrbox: 实体 %d, 模板 %d, 规则 %d, 脚本 %d (replayed=%d skipped=%d failed=%d)",
		store.Count(), len(store.ListTemplates()), len(machine.Rules()), len(reg.List()),
		len(replay.Replayed), len(replay.Skipped), len(replay.Failed))
	if len(replay.Failed) > 0 {
		log.Printf("astrbox: 重放失败脚本: %v", replay.Failed)
		for name, details := range replay.Details {
			log.Printf("  - %s: %v", name, details)
		}
	}
	if findings := reg.ScanConsistency(); findings > 0 {
		log.Printf("astrbox: 脚本一致性检查发现 %d 个问题（可通过 diagnostics_query 查看）", findings)
	}

	server := mcpserver.New(store, machine, reg)
	server.SetDataDir(*dataDir)
	server.SetEventBridge(eventSystem.Bridge())
	server.SetEventBus(eventSystem.Bus())
	server.SetDiagnostics(diagStore)
	server.SetStateLogger(engine.NewStateLogger(filepath.Join(*dataDir, "logs"), 7))
	ctx := context.Background()
	if *transport == "http" {
		if !isLoopbackAddr(*addr) && domainConfig.HttpToken == "" {
			log.Fatalf("astrbox: 拒绝在非回环地址 %s 上无 token 启动 HTTP；请配置 http_token 或绑定 127.0.0.1", *addr)
		}
		var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server.MCPServer()
		}, nil)
		if domainConfig.HttpToken != "" {
			handler = tokenMiddleware(handler, domainConfig.HttpToken)
		} else {
			log.Printf("astrbox: 警告：HTTP 无鉴权，仅建议本机使用")
		}
		log.Printf("astrbox: MCP HTTP listening on %s", *addr)
		if err := http.ListenAndServe(*addr, handler); err != nil {
			log.Fatalf("astrbox: HTTP 服务退出: %v", err)
		}
		return
	}
	if err := server.RunStdio(ctx); err != nil {
		log.Fatalf("astrbox: stdio 服务退出: %v", err)
	}
}

func defaultDataDir() string {
	if value := os.Getenv("ASTRBOX_DATA_DIR"); value != "" {
		return value
	}
	return "data"
}

func defaultScriptsDir() string {
	if value := os.Getenv("ASTRBOX_SCRIPTS_DIR"); value != "" {
		return value
	}
	return "scripts"
}

func loadRules(path string) ([]engine.Rule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return engine.ParseRules(raw)
}

// Command astrbox-mcp serves the AstrBox cat-girl engine as an MCP server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/bootstrap"
)

func main() {
	dataDir := flag.String("data-dir", defaultDataDir(), "state directory: entities.json, templates/, archives/, logs/")
	scriptsDir := flag.String("scripts-dir", defaultScriptsDir(), "scripts directory: scripts.json, *.neko, rules.json")
	transport := flag.String("transport", "stdio", "transport: stdio or http")
	addr := flag.String("addr", "127.0.0.1:8765", "listen address for -transport=http")
	configPath := flag.String("config", "config.json", "domain config JSON (missing file = defaults)")
	flag.Parse()

	runtime, err := bootstrap.New(bootstrap.Options{
		DataDir: *dataDir, ScriptsDir: *scriptsDir, ConfigPath: *configPath,
	})
	if err != nil {
		log.Fatalf("astrbox: %v", err)
	}
	server := runtime.NewMCPServer()
	ctx := context.Background()
	if *transport == "http" {
		if !isLoopbackAddr(*addr) && runtime.Config.HttpToken == "" {
			log.Fatalf("astrbox: 拒绝在非回环地址 %s 上无 token 启动 HTTP；请配置 http_token 或绑定 127.0.0.1", *addr)
		}
		var handler http.Handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
			return server.MCPServer()
		}, nil)
		if runtime.Config.HttpToken != "" {
			handler = tokenMiddleware(handler, runtime.Config.HttpToken)
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

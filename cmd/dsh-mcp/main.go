// Command dsh-mcp serves the AstrBox engine over MCP stdio and also exposes a
// local control plane on a Unix domain socket for the DSH plugin.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/control"
	"github.com/RuiCat/Neko/internal/driver"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/monitor"
	"github.com/RuiCat/Neko/internal/monitor/auth"
)

// version is injected with -ldflags "-X main.version=<git short HEAD>".
var version = "dev"

const (
	// EXIT_OK marks a normal EOF or signal-cleaned exit.
	EXIT_OK = 0
	// EXIT_LOAD_FAILED marks config/data failures and unexpected stdio errors.
	EXIT_LOAD_FAILED = 2
	// EXIT_ALREADY_RUNNING marks a held single-instance lock or busy socket.
	EXIT_ALREADY_RUNNING = 3
	// EXIT_SOCKET_FAILED marks a control listener that cannot be established.
	EXIT_SOCKET_FAILED = 4
	// SHUTDOWN_TIMEOUT bounds graceful control-plane shutdown.
	SHUTDOWN_TIMEOUT = 3 * time.Second
	// MONITOR_HEADER_TIMEOUT bounds reading one monitor request header.
	MONITOR_HEADER_TIMEOUT = 5 * time.Second
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	os.Exit(run(os.Args[1:]))
}

// run parses flags, assembles the runtime, starts the control listener before
// MCP stdio and returns the process exit code.
func run(args []string) int {
	flags := flag.NewFlagSet("dsh-mcp", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dataDir := flags.String("data-dir", defaultDataDir(),
		"状态目录：entities.json、templates/、archives/、logs/（默认 data，可用 ASTRBOX_DATA_DIR）")
	scriptsDir := flags.String("scripts-dir", defaultScriptsDir(),
		"脚本目录：scripts.json、*.neko、rules.json（默认 scripts，可用 ASTRBOX_SCRIPTS_DIR）")
	configPath := flags.String("config", "config.json", "领域配置 JSON（文件缺失时使用默认值）")
	controlSocket := flags.String("control-socket", "",
		"控制面 Unix socket 路径（默认 <data-dir>/dsh-control.sock）")
	monitorAddr := flags.String("monitor-addr", defaultMonitorAddr(),
		"监视器 HTTP 地址（默认 127.0.0.1:8765，可用 ASTRBOX_MONITOR_ADDR；留空禁用）")
	contextsConfig := flags.String("contexts-config", "",
		"角色上下文配置（contexts.json；留空禁用上下文驱动）")
	nightlyAt := flags.String("nightly-at", "",
		"夜间衰减时间（本地 HH:MM；留空禁用）")
	nightlyFactor := flags.Float64("nightly-factor", 0.5,
		"夜间衰减系数（0<factor<1）")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return EXIT_OK
		}
		return EXIT_LOAD_FAILED
	}
	socketPath := *controlSocket
	if socketPath == "" {
		socketPath = filepath.Join(*dataDir, "dsh-control.sock")
	}

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Printf("dsh-mcp: 无法创建数据目录 %s: %v", *dataDir, err)
		return EXIT_LOAD_FAILED
	}
	lock, err := acquireEngineLock(*dataDir)
	if err != nil {
		var held *lockHeldError
		if errors.As(err, &held) {
			log.Printf("dsh-mcp: 数据目录已被另一个进程占用（pid %d）：%s", held.PID, *dataDir)
			log.Printf("dsh-mcp: 如确认无进程使用，可执行 rm %s 后重试", held.Path)
			return EXIT_ALREADY_RUNNING
		}
		log.Printf("dsh-mcp: 单实例锁失败: %v", err)
		return EXIT_LOAD_FAILED
	}
	defer func() { _ = lock.Release() }()

	runtime, err := bootstrap.New(bootstrap.Options{
		DataDir: *dataDir, ScriptsDir: *scriptsDir, ConfigPath: *configPath,
		Journal: true, NightlyAt: *nightlyAt, NightlyFactor: *nightlyFactor,
	})
	if err != nil {
		log.Printf("dsh-mcp: 运行时装配失败: %v", err)
		return EXIT_LOAD_FAILED
	}
	contextDriver, driverNote := loadContextDriver(*contextsConfig, *dataDir)
	if driverNote != "" {
		log.Print(driverNote)
	}
	if contextDriver != nil {
		contextDriver.SetErrorHandler(func(deliverErr error) {
			log.Printf("dsh-mcp: 事件驱动的上下文投递失败: %v", deliverErr)
		})
		runtime.Events.Bus().SetObserver(func(name string, eventCtx events.Context, records []string) {
			report := contextDriver.OnEvent(driver.Event{
				Name:    name,
				Entity:  eventCtx.EntityID,
				Field:   eventCtx.FieldPath,
				Old:     eventCtx.OldValue,
				New:     eventCtx.NewValue,
				Delta:   eventCtx.Delta,
				Records: records,
			})
			for _, skipped := range report.Skipped {
				log.Printf("dsh-mcp: 事件 %s → %s 被冷却/配额跳过", name, skipped)
			}
		})
	}
	mcpServer := runtime.NewMCPServer()
	mcpServer.SetContextDriver(contextDriver)
	mcpServer.EnableContextTools()
	controlServer := control.New(runtime, mcpServer, control.Options{Driver: contextDriver})
	token := config.Current().HttpToken
	if !auth.ValidTokenCharset(token) {
		log.Printf("dsh-mcp: http_token 字符集不合法（要求 ^[A-Za-z0-9_-]{16,128}$）")
		return EXIT_LOAD_FAILED
	}
	monitorServer := monitor.New(runtime, mcpServer, monitor.Options{Token: token, Version: version})

	// The control listener must be ready before MCP stdio starts so the DSH
	// plugin can poll it immediately.
	listener, err := openControlListener(socketPath)
	if err != nil {
		if errors.Is(err, errSocketInUse) {
			log.Printf("dsh-mcp: 控制面 socket 已被占用：%s", socketPath)
			log.Printf("dsh-mcp: 如确认无进程使用，可执行 rm %s 后重试", socketPath)
			return EXIT_ALREADY_RUNNING
		}
		log.Printf("dsh-mcp: 无法建立控制面 socket %s: %v", socketPath, err)
		return EXIT_SOCKET_FAILED
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var signalled atomic.Bool
	var controlFailed atomic.Bool
	var monitorHTTP *http.Server
	if *monitorAddr != "" {
		monitorListener, listenErr := net.Listen("tcp", *monitorAddr)
		if listenErr != nil {
			log.Printf("dsh-mcp: 监视器监听 %s 失败，MCP 继续运行: %v", *monitorAddr, listenErr)
		} else {
			monitorHTTP = &http.Server{Handler: monitorServer, ReadHeaderTimeout: MONITOR_HEADER_TIMEOUT}
			go func() {
				if serveErr := monitorHTTP.Serve(monitorListener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					log.Printf("dsh-mcp: 监视器服务退出: %v", serveErr)
				}
			}()
			log.Printf("dsh-mcp: monitor 就绪 http://%s/monitor/ (readOnly=%t)",
				*monitorAddr, token == "")
		}
	}
	cleanup := newCleanup(controlServer, socketPath, func(shutdownCtx context.Context) {
		monitorServer.Close()
		if monitorHTTP != nil {
			_ = monitorHTTP.Shutdown(shutdownCtx)
		}
	})
	defer cleanup()

	go func() {
		if err := controlServer.Serve(listener); err != nil {
			controlFailed.Store(true)
			log.Printf("dsh-mcp: 控制面服务退出: %v", err)
			cancel()
		}
	}()

	// Pre-open every declared context; a DSH that is not up yet is only logged
	// because the driver opens lazily on the next invoke.
	if contextDriver != nil {
		go func() {
			for contextID, openErr := range contextDriver.OpenAll(ctx) {
				log.Printf("dsh-mcp: 打开上下文 %s 失败（稍后按需重试）: %v", contextID, openErr)
			}
		}()
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signalCh)
	go func() {
		sig, ok := <-signalCh
		if !ok {
			return
		}
		signalled.Store(true)
		log.Printf("dsh-mcp: 收到信号 %s，正在清理", sig)
		cancel()
	}()

	log.Printf("dsh-mcp: control 就绪 %s，MCP stdio 启动（data=%s scripts=%s）",
		socketPath, *dataDir, *scriptsDir)
	stdioErr := mcpServer.RunStdio(ctx)
	switch {
	case signalled.Load():
		log.Printf("dsh-mcp: 已按信号退出")
		return EXIT_OK
	case controlFailed.Load():
		log.Printf("dsh-mcp: 控制面异常退出")
		return EXIT_SOCKET_FAILED
	case stdioErr == nil:
		log.Printf("dsh-mcp: MCP stdio 正常结束")
		return EXIT_OK
	default:
		log.Printf("dsh-mcp: MCP stdio 异常结束: %v", stdioErr)
		return EXIT_LOAD_FAILED
	}
}

// newCleanup returns an idempotent shutdown that stops the control and monitor
// planes and removes the socket file.
func newCleanup(server *control.Server, socketPath string, extra func(context.Context)) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), SHUTDOWN_TIMEOUT)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				log.Printf("dsh-mcp: 控制面关闭: %v", err)
			}
			if extra != nil {
				extra(ctx)
			}
			if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("dsh-mcp: 清理 socket 失败: %v", err)
			}
		})
	}
}

// loadContextDriver builds the optional DSH context driver. An empty path or a
// missing file disables it; any other failure is reported and also disables it
// so the engine keeps serving.
func loadContextDriver(configPath, dataDir string) (*driver.Driver, string) {
	if strings.TrimSpace(configPath) == "" {
		return nil, "dsh-mcp: 上下文驱动未启用（未设置 -contexts-config）"
	}
	cfg, err := driver.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Sprintf("dsh-mcp: 上下文驱动未启用（找不到 %s）", configPath)
		}
		return nil, fmt.Sprintf("dsh-mcp: 上下文配置无效: %v", err)
	}
	cfg = cfg.WithDataDir(dataDir)
	contextDriver, err := driver.New(cfg)
	if err != nil {
		return nil, fmt.Sprintf("dsh-mcp: 上下文驱动初始化失败: %v", err)
	}
	return contextDriver, fmt.Sprintf("dsh-mcp: 上下文驱动就绪（%d 个上下文，registry=%s）",
		len(cfg.Contexts), cfg.RegistryPath)
}

// defaultDataDir follows ASTRBOX_DATA_DIR, falling back to ./data.
func defaultDataDir() string {
	if value := os.Getenv("ASTRBOX_DATA_DIR"); value != "" {
		return value
	}
	return "data"
}

// defaultScriptsDir follows ASTRBOX_SCRIPTS_DIR, falling back to ./scripts.
func defaultScriptsDir() string {
	if value := os.Getenv("ASTRBOX_SCRIPTS_DIR"); value != "" {
		return value
	}
	return "scripts"
}

// defaultMonitorAddr follows ASTRBOX_MONITOR_ADDR, falling back to the K1
// default 127.0.0.1:8765; an explicit empty value disables the listener.
func defaultMonitorAddr() string {
	if value, ok := os.LookupEnv("ASTRBOX_MONITOR_ADDR"); ok {
		return value
	}
	return "127.0.0.1:8765"
}

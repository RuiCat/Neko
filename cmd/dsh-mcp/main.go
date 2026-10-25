// Command dsh-mcp serves the AstrBox engine over MCP stdio and also exposes a
// local control plane on a Unix domain socket for the DSH plugin.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ruicat/astrbox/internal/bootstrap"
	"github.com/ruicat/astrbox/internal/control"
)

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
	})
	if err != nil {
		log.Printf("dsh-mcp: 运行时装配失败: %v", err)
		return EXIT_LOAD_FAILED
	}
	mcpServer := runtime.NewMCPServer()
	controlServer := control.New(runtime, mcpServer, control.Options{})

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
	cleanup := newCleanup(controlServer, socketPath)
	defer cleanup()

	go func() {
		if err := controlServer.Serve(listener); err != nil {
			controlFailed.Store(true)
			log.Printf("dsh-mcp: 控制面服务退出: %v", err)
			cancel()
		}
	}()

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

// newCleanup returns an idempotent shutdown that stops the control plane and
// removes the socket file.
func newCleanup(server *control.Server, socketPath string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), SHUTDOWN_TIMEOUT)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				log.Printf("dsh-mcp: 控制面关闭: %v", err)
			}
			if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("dsh-mcp: 清理 socket 失败: %v", err)
			}
		})
	}
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

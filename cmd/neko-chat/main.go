// Command neko-chat is the standalone conversation process: one line on stdin
// is one turn; the answer goes to stdout. It writes a complete TurnRecord per
// turn (input + package + answer) and never changes hard state itself.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/chat"
	"github.com/RuiCat/Neko/internal/llm"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// MAX_LINE_BYTES bounds one stdin turn.
const MAX_LINE_BYTES = 1 << 20

func main() {
	dataDir := flag.String("data-dir", "data", "数据目录")
	scriptsDir := flag.String("scripts-dir", "scripts", "脚本目录")
	configPath := flag.String("config", "config.json", "域配置 JSON（缺失即默认）")
	session := flag.String("session", "neko-chat", "会话 ID（写入 TurnRecord）")
	personaPath := flag.String("persona", "", "人格文件路径（留空用内置人格）")
	dryRun := flag.Bool("dry-run", false, "不调用远程模型，只跑补偿包与日志")
	flag.Parse()

	runtime, err := bootstrap.New(bootstrap.Options{
		DataDir: *dataDir, ScriptsDir: *scriptsDir, ConfigPath: *configPath, Journal: true,
	})
	if err != nil {
		log.Fatalf("neko-chat: 运行时装配失败: %v", err)
	}
	defer func() {
		if err := runtime.Close(); err != nil {
			log.Printf("neko-chat: 关闭日志失败: %v", err)
		}
	}()

	provider, providerErr := llm.FromEnv("NEKO_LLM_ENDPOINT", "NEKO_LLM_API_KEY", "NEKO_LLM_MODEL")
	if providerErr != nil && !*dryRun {
		log.Fatalf("neko-chat: %v（或加 -dry-run）", providerErr)
	}
	persona := ""
	if *personaPath != "" {
		raw, err := os.ReadFile(*personaPath)
		if err != nil {
			log.Fatalf("neko-chat: 读取人格文件失败: %v", err)
		}
		persona = string(raw)
	}
	conversation, err := chat.New(chat.Options{
		Store: runtime.Store, Schema: runtime.HardState, Sequence: runtime.JournalSequence,
		Board: runtime.Board, Assembler: runtime.Assembler, Journal: runtime.Journal,
		Provider: provider, Persona: persona, SessionID: *session, DryRun: *dryRun,
	})
	if err != nil {
		log.Fatalf("neko-chat: %v", err)
	}
	server := runtime.NewMCPServer()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), MAX_LINE_BYTES)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}
		tick, err := server.RunTick("", mcpserver.TickOptions{SkipJournal: true, SkipSaveWhenUnchanged: true})
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 状态机执行失败: %v\n", err)
			continue
		}
		result, err := conversation.Turn(context.Background(), line, tick.TickID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 本轮失败（已记录）: %v\n", err)
			continue
		}
		if *dryRun {
			fmt.Printf("[dry-run] %s active=%v state_fields=%d\n", result.TurnID, result.Active, len(result.Package.State))
			continue
		}
		fmt.Println(result.Answer)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 读取输入失败: %v\n", err)
		os.Exit(1)
	}
}

// Command neko-context drives one character context through the DSH HTTP API.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/RuiCat/Neko/internal/driver"
)

// EXIT_USAGE marks invalid flags, a bad config or a rejected invocation.
const EXIT_USAGE = 2

// REFRESH_TIMEOUT bounds the session/list reconciliation in -list.
const REFRESH_TIMEOUT = 3 * time.Second

func main() { os.Exit(run(os.Args[1:])) }

// run parses flags and performs one list or invoke operation.
func run(args []string) int {
	flags := flag.NewFlagSet("neko-context", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	configPath := flags.String("config", "contexts.json", "角色上下文配置")
	contextID := flags.String("context", "", "目标 context_id")
	text := flags.String("text", "", "投递正文")
	eventID := flags.String("event", "", "事件 id（幂等键；留空自动生成）")
	priority := flags.String("priority", driver.PRIORITY_NORMAL, "normal 或 urgent")
	list := flags.Bool("list", false, "只打印上下文注册表")
	if err := flags.Parse(args); err != nil {
		return EXIT_USAGE
	}
	cfg, err := driver.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "neko-context:", err)
		return EXIT_USAGE
	}
	contextDriver, err := driver.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "neko-context:", err)
		return EXIT_USAGE
	}
	if *list {
		refreshCtx, cancel := context.WithTimeout(context.Background(), REFRESH_TIMEOUT)
		refreshErr := contextDriver.RefreshStatus(refreshCtx)
		cancel()
		body := map[string]any{"ok": true, "contexts": contextDriver.Snapshot()}
		body["refreshed"] = refreshErr == nil
		return printJSON(body)
	}
	if *contextID == "" || *text == "" {
		fmt.Fprintln(os.Stderr, "neko-context: 需要 -context 与 -text（或 -list）")
		return EXIT_USAGE
	}
	result, err := contextDriver.Invoke(context.Background(), driver.InvokeRequest{
		ContextID: *contextID,
		EventID:   *eventID,
		Text:      *text,
		Priority:  *priority,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "neko-context:", err)
		return EXIT_USAGE
	}
	return printJSON(map[string]any{"ok": true, "result": result})
}

// printJSON writes one indented document and returns 0 on success.
func printJSON(value any) int {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "neko-context:", err)
		return EXIT_USAGE
	}
	fmt.Println(string(raw))
	return 0
}

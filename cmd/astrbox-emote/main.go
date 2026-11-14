// Command astrbox-emote is the offline companion of the emote_find MCP tool:
// it queries the emote library from a shell and rebuilds its vectors and
// cluster database without going through the MCP server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/RuiCat/Neko/internal/emote"
)

// EXIT_OK and friends follow the repository's CLI convention.
const (
	EXIT_OK          = 0
	EXIT_USAGE       = 2
	EXIT_LOOKUP_FAIL = 3
	EXIT_BUILD_FAIL  = 4
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run dispatches one subcommand and returns the process exit code.
func run(args []string) int {
	if len(args) == 0 {
		usage()
		return EXIT_USAGE
	}
	switch args[0] {
	case "find":
		return runFind(args[1:])
	case "build":
		return runBuild(args[1:])
	case "info":
		return runInfo(args[1:])
	case "-h", "--help", "help":
		usage()
		return EXIT_OK
	default:
		fmt.Fprintf(os.Stderr, "未知子命令 %q\n", args[0])
		usage()
		return EXIT_USAGE
	}
}

// usage prints the subcommand summary.
func usage() {
	fmt.Fprint(os.Stderr, `用法:
  astrbox-emote find  [-dir DIR] [-top N] [-model M] [-url U] [-json] <文本...>
  astrbox-emote build [-dir DIR] [-out DIR] [-model M] [-url U] [-workers N]
                      [-limit N] [-clusters K] [-iterations N] [-seed N] [-dry-run]
  astrbox-emote info  [-dir DIR]

默认表情目录: $ASTRBOX_EMOTE_DIR 或 ~/data/catworkspace/emotes
`)
}

// runFind implements the lookup subcommand.
func runFind(args []string) int {
	flags := flag.NewFlagSet("find", flag.ContinueOnError)
	dir := flags.String("dir", emote.DefaultDir(), "表情目录")
	top := flags.Int("top", emote.DEFAULT_TOP, "返回数量（1-20）")
	model := flags.String("model", emote.DEFAULT_MODEL, "嵌入模型")
	embedURL := flags.String("url", emote.DEFAULT_EMBED_URL, "Ollama embeddings 地址")
	asJSON := flags.Bool("json", false, "输出 JSON")
	timeout := flags.Duration("timeout", emote.DEFAULT_TIMEOUT, "单次嵌入超时")
	if err := flags.Parse(args); err != nil {
		return EXIT_USAGE
	}
	text := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if text == "" {
		fmt.Fprintln(os.Stderr, "缺少查询文本")
		return EXIT_USAGE
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := emote.NewClient(emote.Config{Dir: *dir, Model: *model, EmbedURL: *embedURL, Timeout: *timeout})
	ctx, cancel := context.WithTimeout(ctx, *timeout+5*time.Second)
	defer cancel()
	result, err := client.Find(ctx, text, *top)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 表情匹配失败: %v\n", err)
		return EXIT_LOOKUP_FAIL
	}
	if *asJSON {
		raw, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ JSON 序列化失败: %v\n", err)
			return EXIT_LOOKUP_FAIL
		}
		fmt.Println(string(raw))
		return EXIT_OK
	}
	fmt.Printf("source: %s\n", result.Source)
	for _, item := range result.Results {
		fmt.Printf("  [%.4f] %s | %s | path=%s\n", item.Score, item.File, item.Desc, item.Path)
	}
	return EXIT_OK
}

// runBuild implements the rebuild subcommand.
func runBuild(args []string) int {
	flags := flag.NewFlagSet("build", flag.ContinueOnError)
	dir := flags.String("dir", emote.DefaultDir(), "输入目录（emote_index.json）")
	outDir := flags.String("out", "", "输出目录（默认同 -dir）")
	model := flags.String("model", emote.DEFAULT_MODEL, "嵌入模型")
	embedURL := flags.String("url", emote.DEFAULT_EMBED_URL, "Ollama embeddings 地址")
	workers := flags.Int("workers", emote.DEFAULT_WORKERS, "并发嵌入数")
	limit := flags.Int("limit", 0, "只处理前 N 条（0=全部，烟测用）")
	clusters := flags.Int("clusters", emote.DEFAULT_CLUSTERS, "聚类簇数")
	iterations := flags.Int("iterations", emote.DEFAULT_ITERATIONS, "KMeans 轮数上限")
	seed := flags.Int64("seed", emote.DEFAULT_SEED, "KMeans 随机种子")
	dryRun := flags.Bool("dry-run", false, "只打印计划，不写文件")
	timeout := flags.Duration("timeout", emote.DEFAULT_TIMEOUT, "单次嵌入超时")
	if err := flags.Parse(args); err != nil {
		return EXIT_USAGE
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	options := emote.BuildOptions{
		Dir: *dir, OutDir: *outDir, Model: *model, EmbedURL: *embedURL,
		Timeout: *timeout, Workers: *workers, Limit: *limit, DryRun: *dryRun,
		Clusters: *clusters, Iterations: *iterations, Seed: *seed,
		Progress: func(done, total int) {
			fmt.Fprintf(os.Stderr, "  进度: %d/%d (%d%%)\n", done, total, done*100/max(total, 1))
		},
	}
	report, err := emote.RunBuild(ctx, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 构建失败: %v\n", err)
		return EXIT_BUILD_FAIL
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 报告序列化失败: %v\n", err)
		return EXIT_BUILD_FAIL
	}
	fmt.Println(string(raw))
	return EXIT_OK
}

// runInfo prints what the client loaded.
func runInfo(args []string) int {
	flags := flag.NewFlagSet("info", flag.ContinueOnError)
	dir := flags.String("dir", emote.DefaultDir(), "表情目录")
	if err := flags.Parse(args); err != nil {
		return EXIT_USAGE
	}
	summary, err := emote.NewClient(emote.Config{Dir: *dir}).Summary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		return EXIT_LOOKUP_FAIL
	}
	fmt.Print(summary)
	return EXIT_OK
}

// max keeps the progress percentage division safe.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

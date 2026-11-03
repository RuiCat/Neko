// Command astrbox-replay audits journal compensation packages against their
// snapshots and exits non-zero on any finding.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/replay"
)

type report struct {
	GeneratedAt time.Time     `json:"generated_at"`
	JournalDir  string        `json:"journal_dir"`
	Result      replay.Result `json:"result"`
}

func main() {
	journalDir := flag.String("journal", "data/journal", "交互日志目录")
	outPath := flag.String("out", "", "报告输出路径（留空只打印摘要）")
	flag.Parse()

	records, err := journal.ReadDir(*journalDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	result := replay.Audit(records)
	payload := report{GeneratedAt: time.Now().UTC(), JournalDir: *journalDir, Result: result}
	fmt.Printf("records=%d with_compensation=%d valid=%d invalid=%d\n",
		result.Records, result.WithCompensation, result.Valid, result.Invalid)
	for _, issue := range result.Issues {
		fmt.Printf("  - %s [%s] %s\n", issue.TurnID, issue.Field, issue.Message)
	}
	if *outPath != "" {
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ 序列化报告失败: %v\n", err)
			os.Exit(1)
		}
		if err := writeReport(*outPath, raw); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
	}
	if result.Invalid > 0 {
		os.Exit(1)
	}
}

// writeReport writes the report atomically (temp file + rename).
func writeReport(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("替换报告失败: %w", err)
	}
	return nil
}

// Command astrbox-metrics computes the offline metrics report from a journal
// directory and writes it as JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/metrics"
)

func main() {
	journalDir := flag.String("journal", "data/journal", "交互日志目录")
	annotationsDir := flag.String("annotations", "data/annotations", "人工标注目录（缺失=无标注）")
	outPath := flag.String("out", "data/metrics/report.json", "指标报告输出路径（- 表示 stdout）")
	flag.Parse()

	records, err := journal.ReadDir(*journalDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	labels, err := annotations.LoadDir(*annotationsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	report := metrics.ComputeWithAnnotations(records, labels, *journalDir, time.Now())
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 序列化报告失败: %v\n", err)
		os.Exit(1)
	}
	if *outPath == "-" {
		fmt.Println(string(raw))
	} else if err := writeReport(*outPath, raw); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	for _, metric := range report.Metrics {
		if metric.Status == metrics.STATUS_COMPUTED {
			fmt.Printf("%s = %.4f (%d/%d)\n", metric.Name, metric.Value, metric.Numerator, metric.Denominator)
		} else {
			fmt.Printf("%s = %s (%s)\n", metric.Name, metric.Status, metric.Reason)
		}
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

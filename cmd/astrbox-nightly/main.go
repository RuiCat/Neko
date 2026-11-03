// Command astrbox-nightly runs the P3 nightly offline pass from the command
// line. The pipeline itself lives in internal/nightly so the in-process timer
// shares the same implementation.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/nightly"
	"github.com/RuiCat/Neko/internal/repair"
)

func main() {
	dataDir := flag.String("data-dir", "data", "运行数据目录（journal / experts.json / candidates.jsonl）")
	journalFlag := flag.String("journal", "", "journal 目录（默认 <data-dir>/journal）")
	annotationsFlag := flag.String("annotations", "", "标注目录（默认 <data-dir>/annotations）")
	referenceFlag := flag.String("reference", "", "参照集 JSONL（默认 <data-dir>/reference-set/v0.jsonl；缺失则跳过）")
	triageFlag := flag.String("triage", "", "triage.jsonl 输出（默认 <data-dir>/metrics/triage.jsonl）")
	repairsFlag := flag.String("repairs", "", "修复版本文件（默认 <data-dir>/repairs/versions.json）")
	outFlag := flag.String("out", "", "夜间报告（默认 <data-dir>/metrics/nightly.json）")
	decayFactor := flag.Float64("decay-factor", nightly.DEFAULT_DECAY_FACTOR, "专家上下文衰减系数 (0,1)")
	candidatesTTL := flag.Duration("candidates-ttl", candidates.DEFAULT_TTL, "候选队列 TTL（默认 7 天）")
	dryRun := flag.Bool("dry-run", false, "只扫描与判定，不落修复版本、不衰减")
	rollbackFlag := flag.Bool("rollback", false, "回滚最后一个未回滚的修复版本后退出")
	flag.Parse()

	if *rollbackFlag {
		store := repair.NewStore(resolve(*repairsFlag, filepath.Join(*dataDir, "repairs", "versions.json")))
		version, err := store.Rollback()
		if err != nil {
			fail(err)
		}
		fmt.Printf("↩️ 已回滚修复版本 %s（%d 条）\n", version.Version, version.Count)
		return
	}

	report, err := nightly.Run(nightly.Options{
		DataDir: *dataDir, JournalDir: *journalFlag, AnnotationsDir: *annotationsFlag,
		ReferencePath: *referenceFlag, TriagePath: *triageFlag, RepairsPath: *repairsFlag,
		OutPath: *outFlag, DecayFactor: *decayFactor, CandidatesTTL: *candidatesTTL, DryRun: *dryRun,
	})
	if err != nil {
		fail(err)
	}
	printReport(report)
	if !report.Gate.Passed {
		fmt.Fprintln(os.Stderr, "❌ 回归闸门未通过：本次不应用修复")
		os.Exit(1)
	}
}

// resolve returns the explicit flag value or its derived fallback.
func resolve(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// printReport echoes the pass summary to stderr's sibling stdout.
func printReport(report nightly.Report) {
	fmt.Printf("📋 扫描 %d 条记录，findings=%d，候选=%d，拒收=%d\n",
		report.Records, report.Findings, report.Candidates, len(report.Rejections))
	for _, check := range report.Gate.Checks {
		mark := "✅"
		if !check.Passed {
			mark = "❌"
		}
		fmt.Printf("%s %s: %s\n", mark, check.Name, check.Detail)
	}
	if report.Version != nil {
		fmt.Printf("🚀 已上线修复版本 %s（%d 条）\n", report.Version.Version, report.Version.Count)
	}
}

// fail prints the error and exits non-zero.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "❌ %v\n", err)
	os.Exit(1)
}

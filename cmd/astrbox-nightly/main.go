// Command astrbox-nightly runs the P3 nightly offline pass: triage one journal
// window, build repair candidates, gate them against replay + the frozen
// reference set, promote or roll back, then decay the learned contexts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/annotations"
	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/referenceset"
	"github.com/RuiCat/Neko/internal/regression"
	"github.com/RuiCat/Neko/internal/repair"
	"github.com/RuiCat/Neko/internal/triage"
)

// nightlyReport is the JSON artifact one nightly pass writes.
type nightlyReport struct {
	GeneratedAt time.Time          `json:"generated_at"`
	JournalDir  string             `json:"journal_dir"`
	Records     int                `json:"records"`
	Findings    int                `json:"findings"`
	ByClass     map[string]int     `json:"by_class"`
	Candidates  int                `json:"candidates"`
	Rejections  []repair.Rejection `json:"rejections,omitempty"`
	Gate        regression.Result  `json:"gate"`
	Version     *repair.Version    `json:"version,omitempty"`
	RolledBack  *repair.Version    `json:"rolled_back,omitempty"`
	Decay       map[string]int     `json:"decay,omitempty"`
	Expired     int                `json:"expired,omitempty"`
	DryRun      bool               `json:"dry_run"`
}

func main() {
	dataDir := flag.String("data-dir", "data", "运行数据目录（journal / experts.json / candidates.jsonl）")
	journalFlag := flag.String("journal", "", "journal 目录（默认 <data-dir>/journal）")
	annotationsFlag := flag.String("annotations", "", "标注目录（默认 <data-dir>/annotations）")
	referenceFlag := flag.String("reference", "", "参照集 JSONL（默认 <data-dir>/reference-set/v0.jsonl；缺失则跳过）")
	triageFlag := flag.String("triage", "", "triage.jsonl 输出（默认 <data-dir>/metrics/triage.jsonl）")
	repairsFlag := flag.String("repairs", "", "修复版本文件（默认 <data-dir>/repairs/versions.json）")
	outFlag := flag.String("out", "", "夜间报告（默认 <data-dir>/metrics/nightly.json）")
	decayFactor := flag.Float64("decay-factor", 0.5, "专家上下文衰减系数 (0,1)")
	dryRun := flag.Bool("dry-run", false, "只扫描与判定，不落修复版本、不衰减")
	rollbackFlag := flag.Bool("rollback", false, "回滚最后一个未回滚的修复版本后退出")
	flag.Parse()

	resolve := func(flagValue, fallback string) string {
		if flagValue != "" {
			return flagValue
		}
		return fallback
	}
	journalDir := resolve(*journalFlag, filepath.Join(*dataDir, "journal"))
	annotationsDir := resolve(*annotationsFlag, filepath.Join(*dataDir, "annotations"))
	referencePath := resolve(*referenceFlag, filepath.Join(*dataDir, "reference-set", "v0.jsonl"))
	triagePath := resolve(*triageFlag, filepath.Join(*dataDir, "metrics", "triage.jsonl"))
	repairsPath := resolve(*repairsFlag, filepath.Join(*dataDir, "repairs", "versions.json"))
	outPath := resolve(*outFlag, filepath.Join(*dataDir, "metrics", "nightly.json"))

	store := repair.NewStore(repairsPath)
	if *rollbackFlag {
		version, err := store.Rollback()
		if err != nil {
			fail(err)
		}
		fmt.Printf("↩️ 已回滚修复版本 %s（%d 条）\n", version.Version, version.Count)
		return
	}

	now := time.Now()
	records, err := journal.ReadDir(journalDir)
	if err != nil {
		fail(err)
	}
	labels, err := annotations.LoadDir(annotationsDir)
	if err != nil {
		fail(err)
	}
	findings := triage.Scan(records, labels)
	if err := triage.WriteFile(triagePath, findings); err != nil {
		fail(err)
	}
	repairCandidates, rejections := repair.Build(findings, now)
	references := loadReferences(referencePath)
	gate := regression.Gate(records, references)

	report := nightlyReport{
		GeneratedAt: now, JournalDir: journalDir, Records: len(records),
		Findings: len(findings), ByClass: countClasses(findings),
		Candidates: len(repairCandidates), Rejections: rejections, Gate: gate, DryRun: *dryRun,
	}
	if !*dryRun {
		if gate.Passed {
			version, applyErr := store.Apply(repairCandidates, now)
			if applyErr != nil {
				fail(applyErr)
			}
			if version.Version != "" {
				report.Version = &version
			}
		}
		removed, err := decayExperts(filepath.Join(*dataDir, "experts.json"), *decayFactor)
		if err != nil {
			fail(err)
		}
		report.Decay = removed
		expired, err := candidates.NewStore(filepath.Join(*dataDir, "candidates.jsonl")).
			Expire(now.Add(-candidates.DEFAULT_TTL))
		if err != nil {
			fail(err)
		}
		report.Expired = expired
	}
	if err := writeJSON(outPath, report); err != nil {
		fail(err)
	}

	fmt.Printf("📋 扫描 %d 条记录，findings=%d，候选=%d，拒收=%d\n",
		report.Records, report.Findings, report.Candidates, len(report.Rejections))
	for _, check := range gate.Checks {
		mark := "✅"
		if !check.Passed {
			mark = "❌"
		}
		fmt.Printf("%s %s: %s\n", mark, check.Name, check.Detail)
	}
	if report.Version != nil {
		fmt.Printf("🚀 已上线修复版本 %s（%d 条）\n", report.Version.Version, report.Version.Count)
	}
	if !gate.Passed {
		fmt.Fprintln(os.Stderr, "❌ 回归闸门未通过：本次不应用修复")
		os.Exit(1)
	}
}

// loadReferences reads the frozen reference set; a missing file is not fatal
// (the gate reports the check as skipped).
func loadReferences(path string) []referenceset.Entry {
	entries, err := referenceset.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 参照集不可用（%v），回归闸门跳过结构校验\n", err)
		return nil
	}
	return entries
}

// decayExperts loads the persisted expert contexts, decays them and saves them
// back. A missing store is treated as empty.
func decayExperts(path string, factor float64) (map[string]int, error) {
	registry := expert.NewDefaultRegistry(expert.DEFAULT_CONTEXT_CAPACITY)
	if err := expert.LoadFile(path, registry); err != nil {
		return nil, err
	}
	removed := registry.DecayAll(factor)
	if err := expert.SaveFile(path, registry); err != nil {
		return nil, err
	}
	return removed, nil
}

// countClasses tallies findings per triage queue.
func countClasses(findings []triage.Finding) map[string]int {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[string(finding.Class)]++
	}
	return counts
}

// writeJSON writes the report atomically.
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化报告失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("替换报告失败: %w", err)
	}
	return nil
}

// fail prints the error and exits non-zero.
func fail(err error) {
	fmt.Fprintf(os.Stderr, "❌ %v\n", err)
	os.Exit(1)
}

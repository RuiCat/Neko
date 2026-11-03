// Package nightly is the reusable P3 offline pipeline: scan the journal, build
// repair candidates, gate them, promote or roll back, and decay the learned
// contexts. The CLI and the in-process timer share this one implementation.
package nightly

import (
	"encoding/json"
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

// DEFAULT_DECAY_FACTOR halves every expert weight per nightly pass.
const DEFAULT_DECAY_FACTOR = 0.5

// Options configures one nightly pass. Empty path fields derive from DataDir.
type Options struct {
	DataDir        string
	JournalDir     string
	AnnotationsDir string
	ReferencePath  string
	TriagePath     string
	RepairsPath    string
	OutPath        string
	DecayFactor    float64
	CandidatesTTL  time.Duration
	DryRun         bool
	// SkipDecay lets an in-process caller decay its live registry first and
	// then run only the triage/repair/regression part of the pass.
	SkipDecay bool
}

// Report is the JSON artifact one nightly pass produces.
type Report struct {
	GeneratedAt time.Time          `json:"generated_at"`
	JournalDir  string             `json:"journal_dir"`
	Records     int                `json:"records"`
	Findings    int                `json:"findings"`
	ByClass     map[string]int     `json:"by_class"`
	Candidates  int                `json:"candidates"`
	Rejections  []repair.Rejection `json:"rejections,omitempty"`
	Gate        regression.Result  `json:"gate"`
	Version     *repair.Version    `json:"version,omitempty"`
	Decay       map[string]int     `json:"decay,omitempty"`
	Expired     int                `json:"expired,omitempty"`
	DryRun      bool               `json:"dry_run"`
}

// Run executes one full pass. A failing gate never promotes a version, so the
// caller may treat a non-nil error as "nothing was applied".
func Run(options Options) (Report, error) {
	if options.DataDir == "" {
		return Report{}, fmt.Errorf("nightly: 缺少 DataDir")
	}
	resolve := func(value, fallback string) string {
		if value != "" {
			return value
		}
		return fallback
	}
	journalDir := resolve(options.JournalDir, filepath.Join(options.DataDir, "journal"))
	annotationsDir := resolve(options.AnnotationsDir, filepath.Join(options.DataDir, "annotations"))
	referencePath := resolve(options.ReferencePath, filepath.Join(options.DataDir, "reference-set", "v0.jsonl"))
	triagePath := resolve(options.TriagePath, filepath.Join(options.DataDir, "metrics", "triage.jsonl"))
	repairsPath := resolve(options.RepairsPath, filepath.Join(options.DataDir, "repairs", "versions.json"))
	outPath := resolve(options.OutPath, filepath.Join(options.DataDir, "metrics", "nightly.json"))
	decayFactor := options.DecayFactor
	if decayFactor <= 0 || decayFactor >= 1 {
		decayFactor = DEFAULT_DECAY_FACTOR
	}
	candidatesTTL := options.CandidatesTTL
	if candidatesTTL <= 0 {
		candidatesTTL = candidates.DEFAULT_TTL
	}

	now := time.Now()
	records, err := journal.ReadDir(journalDir)
	if err != nil {
		return Report{}, err
	}
	labels, err := annotations.LoadDir(annotationsDir)
	if err != nil {
		return Report{}, err
	}
	findings := triage.Scan(records, labels)
	if err := triage.WriteFile(triagePath, findings); err != nil {
		return Report{}, err
	}
	repairCandidates, rejections := repair.Build(findings, now)
	references := loadReferences(referencePath)
	gate := regression.Gate(records, references)

	report := Report{
		GeneratedAt: now, JournalDir: journalDir, Records: len(records),
		Findings: len(findings), ByClass: countClasses(findings),
		Candidates: len(repairCandidates), Rejections: rejections, Gate: gate, DryRun: options.DryRun,
	}
	if !options.DryRun {
		if gate.Passed {
			version, applyErr := repair.NewStore(repairsPath).Apply(repairCandidates, now)
			if applyErr != nil {
				return report, applyErr
			}
			if version.Version != "" {
				report.Version = &version
			}
		}
		removed := map[string]int{}
		if !options.SkipDecay {
			var decayErr error
			removed, decayErr = decayExperts(filepath.Join(options.DataDir, "experts.json"), decayFactor)
			if decayErr != nil {
				return report, decayErr
			}
		}
		report.Decay = removed
		expired, expireErr := candidates.NewStore(filepath.Join(options.DataDir, "candidates.jsonl")).
			Expire(now.Add(-candidatesTTL))
		if expireErr != nil {
			return report, expireErr
		}
		report.Expired = expired
	}
	if err := WriteReport(outPath, report); err != nil {
		return report, err
	}
	return report, nil
}

// WriteReport writes the report atomically.
func WriteReport(path string, report Report) error {
	raw, err := json.MarshalIndent(report, "", "  ")
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

// loadReferences reads the frozen reference set; a missing file is not fatal.
func loadReferences(path string) []referenceset.Entry {
	entries, err := referenceset.LoadFile(path)
	if err != nil {
		return nil
	}
	return entries
}

// decayExperts loads, decays and saves the persisted expert contexts.
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

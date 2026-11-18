// generate.go is the single entry that reads the five read-only legacy corpora
// and returns the validated K1 artifacts. Parse issues are recorded, not fatal;
// file read errors and invariant violations are fatal.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"fmt"
	"os"
	"strings"
)

// Corpus lists the corpus files in the fixed order used by the snapshot hash.
// An empty Files slice asks Generate to use DefaultCorpus.
type Corpus struct {
	Files []string
}

// DefaultCorpus returns the five P4 source corpora in snapshot order.
func DefaultCorpus() Corpus {
	return Corpus{Files: []string{
		"/home/ruicat/Desktop/备份/scripts/behavior/cat_behavior.neko",
		"/home/ruicat/Desktop/备份/scripts/behavior/cat_behavior_r18.neko",
		"/home/ruicat/Desktop/备份/scripts/behavior/cat_quest_library.neko",
		"/home/ruicat/Desktop/备份/scripts/behavior/cat_quest.neko",
		"/home/ruicat/Desktop/备份/scripts/r18/cat_r18_flows.neko",
	}}
}

// Generate parses the corpus, extracts flows/effects/anchors and validates every
// K1 invariant before returning. The result is independent of previous runs.
func Generate(corpus Corpus) (Artifacts, error) {
	files := corpus.Files
	if len(files) == 0 {
		files = DefaultCorpus().Files
	}
	if len(files) != 5 {
		return Artifacts{}, fmt.Errorf("K1 snapshot expects 5 corpus files, got %d", len(files))
	}
	hashes, combined, manifest, err := snapshotOf(files)
	if err != nil {
		return Artifacts{}, fmt.Errorf("hash corpus: %w", err)
	}
	flows := []FlowSpec{}
	taskTemplates := []TaskTemplateSpec{}
	libraryTemplates := map[string]TaskTemplateSpec{}
	records := []MigrationRecord{}
	containers := []Container{}
	skipped := []SkippedBlock{}
	issues := []ParseIssue{}
	unclassified := []UnclassifiedEffect{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return Artifacts{}, fmt.Errorf("read corpus %s: %w", file, err)
		}
		extracted := extractDocument(file, ParseDocument(data))
		flows = append(flows, extracted.Flows...)
		taskTemplates = append(taskTemplates, extracted.TaskTemplates...)
		if strings.HasSuffix(file, "cat_quest_library.neko") {
			for _, template := range extracted.TaskTemplates {
				libraryTemplates[template.ID] = template
			}
		}
		records = append(records, extracted.Records...)
		containers = append(containers, extracted.Containers...)
		skipped = append(skipped, extracted.Skipped...)
		issues = append(issues, extracted.Issues...)
		unclassified = append(unclassified, extracted.Unclassified...)
	}
	fillTaskTemplateGaps(taskTemplates, libraryTemplates)
	artifacts := Artifacts{
		Flows: FlowSpecFile{
			SchemaVersion: SCHEMA_VERSION,
			Flows:         flows,
			TaskTemplates: taskTemplates,
		},
		Effects: buildEffectMap(flows, unclassified),
		Report:  buildMigrationReport(hashes, combined, manifest, records, containers, skipped, issues),
	}
	if err := ValidateArtifacts(artifacts); err != nil {
		return Artifacts{}, fmt.Errorf("generated artifacts violate K1: %w", err)
	}
	return artifacts, nil
}

// fillTaskTemplateGaps joins cat_quest's partial templates with the same-id
// library template for the fields the partial shape omits (条件.示例/对话). It
// copies existing corpus text only; it never authors new strings.
func fillTaskTemplateGaps(templates []TaskTemplateSpec, library map[string]TaskTemplateSpec) {
	for i := range templates {
		reference, ok := library[templates[i].ID]
		if !ok {
			continue
		}
		if templates[i].Condition.Example == "" {
			templates[i].Condition.Example = reference.Condition.Example
		}
		if !hasTaskDialog(templates[i].Dialog) {
			templates[i].Dialog = reference.Dialog
		}
	}
}

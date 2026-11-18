// migration_test.go pins the real-corpus counts and the C1 invariants: five
// snapshot hashes, count == len(records), and every record reopening to a line
// that contains its key.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func requireCorpus(t *testing.T) Corpus {
	t.Helper()
	corpus := DefaultCorpus()
	for _, file := range corpus.Files {
		if _, err := os.Stat(file); err != nil {
			t.Skipf("corpus unavailable: %v", err)
		}
	}
	return corpus
}

func TestGenerateCorpusInvariants(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := len(artifacts.Flows.Flows); got != 44 {
		t.Fatalf("flows = %d, want 44 (27+5 behavior, 12 r18)", got)
	}
	if got := len(artifacts.Flows.TaskTemplates); got != 11 {
		t.Fatalf("taskTemplates = %d, want 11 (6+5)", got)
	}
	if got := len(artifacts.Effects.Effects); got != 55 {
		t.Fatalf("effects = %d, want 55", got)
	}
	if artifacts.Effects.Unclassified != 0 {
		t.Fatalf("unclassified = %d, want 0: %+v", artifacts.Effects.Unclassified, artifacts.Effects.UnclassifiedItems)
	}
	report := artifacts.Report
	if report.Count != len(report.Records) {
		t.Fatalf("count = %d, records = %d", report.Count, len(report.Records))
	}
	if report.Count != 55 {
		t.Fatalf("records = %d, want 55", report.Count)
	}
	if report.ErrorCount != 0 {
		t.Fatalf("parse errors = %+v", report.Errors)
	}
	if report.ContainerCount != 6 {
		t.Fatalf("containers = %d, want 6", report.ContainerCount)
	}
	if report.SkippedCount != 13 {
		t.Fatalf("skipped = %d, want 13 (NPC schema + 12 legacy rules)", report.SkippedCount)
	}
	kinds := map[string]int{}
	for _, record := range report.Records {
		if record.Kind == "" {
			t.Fatalf("record %s:%d has empty kind", record.File, record.Line)
		}
		kinds[record.Kind]++
	}
	if kinds["flow"] != 32 || kinds["r18Flow"] != 12 || kinds["taskTemplate"] != 11 {
		t.Fatalf("record kinds = %v", kinds)
	}
	wantCounts := []int{27, 5, 6, 5, 12}
	for i, file := range corpus.Files {
		count := 0
		for _, record := range report.Records {
			if record.File == file {
				count++
			}
		}
		if count != wantCounts[i] {
			t.Fatalf("%s: records = %d, want %d", filepath.Base(file), count, wantCounts[i])
		}
	}
	if err := ValidateAnchors(report); err != nil {
		t.Fatalf("anchors: %v", err)
	}
	if err := ValidateSnapshotHashes(report); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(report.SnapshotFiles) != 5 {
		t.Fatalf("snapshotFiles = %d, want 5", len(report.SnapshotFiles))
	}
	manifestLines := strings.Split(strings.TrimSuffix(report.SnapshotSha256Compat, "\n"), "\n")
	if len(manifestLines) != 5 {
		t.Fatalf("snapshot_sha256 lines = %d, want 5", len(manifestLines))
	}
	for i, line := range manifestLines {
		if !strings.Contains(line, "  "+corpus.Files[i]) {
			t.Fatalf("manifest line %d = %q, want corpus file %s", i, line, corpus.Files[i])
		}
	}
}

func TestGenerateEffectKindsAndR18Steps(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	vitalityKinds := map[EffectKind]bool{}
	vitalityValues := []string{}
	for _, effect := range artifacts.Effects.Effects {
		if effect.Path != "身体.活力" {
			continue
		}
		vitalityKinds[effect.Kind] = true
		vitalityValues = append(vitalityValues, fmt.Sprint(effect.Value))
	}
	if !vitalityKinds[EFFECT_KIND_SET] || !vitalityKinds[EFFECT_KIND_ADD] {
		t.Fatalf("身体.活力 kinds = %v, want set and add", vitalityKinds)
	}
	if len(vitalityValues) != 3 {
		t.Fatalf("身体.活力 occurrences = %v, want 3", vitalityValues)
	}
	foundFloat := false
	foundText := false
	for _, effect := range artifacts.Effects.Effects {
		if effect.Path == "身体.阴道_有效开口" && fmt.Sprint(effect.Value) == "3.5" && effect.Kind == EFFECT_KIND_SET {
			foundFloat = true
		}
		if effect.Path == "状态.触发" && effect.Kind == EFFECT_KIND_TEXT {
			foundText = true
		}
	}
	if !foundFloat {
		t.Fatal("missing 身体.阴道_有效开口 = 3.5 set")
	}
	if !foundText {
		t.Fatal("missing 状态.触发 text effect")
	}
	r18Flows := 0
	for _, flow := range artifacts.Flows.Flows {
		if flow.ID == "" || len(flow.Steps) == 0 {
			t.Fatalf("flow %q has no steps", flow.ID)
		}
		if flow.Source.File == corpus.Files[4] {
			r18Flows++
		}
	}
	if r18Flows != 12 {
		t.Fatalf("r18 flows = %d, want 12", r18Flows)
	}
}

func TestGenerateTaskTemplates(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	templates := artifacts.Flows.TaskTemplates
	if len(templates) != 11 {
		t.Fatalf("taskTemplates = %d, want 11", len(templates))
	}
	partialCount := 0
	for _, template := range templates {
		if len(template.Stages) == 0 {
			t.Fatalf("%s: empty stages", template.ID)
		}
		for _, stage := range template.Stages {
			if stage == "已激活" {
				t.Fatalf("%s: 已激活 was not mapped to 未激活", template.ID)
			}
		}
		if template.Condition.Type == "" || template.Condition.Example == "" {
			t.Fatalf("%s: incomplete condition %+v", template.ID, template.Condition)
		}
		if template.Reward.Experience == 0 && template.Reward.Affinity == 0 {
			t.Fatalf("%s: empty reward %+v", template.ID, template.Reward)
		}
		if !hasTaskDialog(template.Dialog) {
			t.Fatalf("%s: empty dialog %+v", template.ID, template.Dialog)
		}
		if strings.HasSuffix(template.Source.File, "cat_quest.neko") {
			partialCount++
		}
	}
	if partialCount != 5 {
		t.Fatalf("cat_quest templates = %d, want 5", partialCount)
	}
	var library, partial *TaskTemplateSpec
	for i := range templates {
		if templates[i].ID != "收集任务" {
			continue
		}
		if strings.HasSuffix(templates[i].Source.File, "cat_quest_library.neko") {
			library = &templates[i]
		}
		if strings.HasSuffix(templates[i].Source.File, "cat_quest.neko") {
			partial = &templates[i]
		}
	}
	if library == nil || partial == nil {
		t.Fatal("missing 收集任务 library/partial template")
	}
	if len(library.Stages) != 3 || library.Stages[0] != "未激活" || library.Condition.Example != "小鱼干" {
		t.Fatalf("library 收集任务 = %+v", library)
	}
	if library.Reward.Experience != 20 || library.Reward.Affinity != 5 || library.Dialog.Activate != "去帮我找小鱼干吧" {
		t.Fatalf("library 收集任务 reward/dialog = %+v", library)
	}
	if len(partial.Stages) != 1 || partial.Stages[0] != "未激活" || partial.Condition.Example != "小鱼干" {
		t.Fatalf("partial 收集任务 = %+v", partial)
	}
	if partial.Reward.Experience != 20 || partial.Reward.Affinity != 5 || partial.Dialog.Activate != "去帮我找小鱼干吧" {
		t.Fatalf("partial 收集任务 reward/dialog = %+v", partial)
	}
}

func TestValidateRejectsBrokenInvariants(t *testing.T) {
	corpus := requireCorpus(t)
	artifacts, err := Generate(corpus)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	brokenCount := artifacts.Report
	brokenCount.Count++
	if err := ValidateMigrationReport(brokenCount); err == nil {
		t.Fatal("count mismatch was accepted")
	}

	brokenAnchor := artifacts.Report
	brokenAnchor.Records = append(append([]MigrationRecord{}, artifacts.Report.Records...), MigrationRecord{
		File: corpus.Files[0], Line: 1, Key: "不存在的键", Kind: "flow",
	})
	if err := ValidateAnchors(brokenAnchor); err == nil {
		t.Fatal("bad anchor was accepted")
	}

	brokenSnapshot := artifacts.Report
	brokenSnapshot.SnapshotSha256 = strings.Repeat("0", 64)
	if err := ValidateSnapshotHashes(brokenSnapshot); err == nil {
		t.Fatal("bad snapshot hash was accepted")
	}

	brokenEffects := artifacts.Effects
	brokenEffects.Effects = append([]EffectSpec{}, artifacts.Effects.Effects...)
	brokenEffects.Effects[0].Kind = "guess"
	if err := ValidateEffectMap(brokenEffects); err == nil {
		t.Fatal("unknown effect kind was accepted")
	}

	brokenFlows := artifacts.Flows
	brokenFlows.Flows = append([]FlowSpec{}, artifacts.Flows.Flows...)
	if len(brokenFlows.Flows) > 1 {
		brokenFlows.Flows[1].ID = brokenFlows.Flows[0].ID
		if err := ValidateFlowSpecFile(brokenFlows); err == nil {
			t.Fatal("duplicate flow id was accepted")
		}
	}

	brokenTasks := artifacts.Flows
	brokenTasks.TaskTemplates = append([]TaskTemplateSpec{}, artifacts.Flows.TaskTemplates...)
	brokenTasks.TaskTemplates[0].Stages = nil
	if err := ValidateFlowSpecFile(brokenTasks); err == nil {
		t.Fatal("empty task stages was accepted")
	}
}

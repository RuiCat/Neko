// rules_test.go pins the repair-rule persistence used by the apply executor.
package repair

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
)

func strPtr(value string) *string { return &value }

func sampleRule() engine.Rule {
	return engine.Rule{
		ID: "repair_abc", Match: "猫猫", When: strPtr("1 < 2"), Source: "repair", Enabled: true,
		Branches: []engine.Branch{{Else: true, Then: []engine.Op{{Op: "set", Path: "状态.信任值", Value: 0.3}}}},
	}
}

func TestUpsertAndRemoveRule(t *testing.T) {
	path := t.TempDir() + "/rules.json"
	if err := UpsertRule(path, sampleRule()); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	rules, err := LoadRules(path)
	if err != nil || len(rules) != 1 || rules[0].ID != "repair_abc" {
		t.Fatalf("load after upsert: %+v, %v", rules, err)
	}
	if !rules[0].Enabled || rules[0].When == nil || *rules[0].When != "1 < 2" {
		t.Fatalf("规则字段丢失: %+v", rules[0])
	}
	if err := UpsertRule(path, sampleRule()); err != nil {
		t.Fatalf("upsert again: %v", err)
	}
	rules, _ = LoadRules(path)
	if len(rules) != 1 {
		t.Fatalf("重复 upsert 应只留一条: %d", len(rules))
	}
	removed, err := RemoveRule(path, "repair_abc")
	if err != nil || !removed {
		t.Fatalf("remove: %v, %v", removed, err)
	}
	rules, _ = LoadRules(path)
	if len(rules) != 0 {
		t.Fatalf("删除后应为空: %+v", rules)
	}
}

func TestLoadRulesMissingIsEmpty(t *testing.T) {
	rules, err := LoadRules(t.TempDir() + "/missing.json")
	if err != nil || len(rules) != 0 {
		t.Fatalf("缺失文件应为空: %+v, %v", rules, err)
	}
}

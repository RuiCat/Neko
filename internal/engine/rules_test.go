// Compatibility test for the frozen rule parser (rule.go), ported from
// astrbox-plugin/tests/test_engine.py::TestEngine::test_rules_parse.
package engine

import "testing"

func TestParseRulesPythonShape(t *testing.T) {
	rules, err := ParseRules([]byte(`{
		"version": 1,
		"rules": [
			{"id": "a", "match": "NPC[*]", "enabled": true,
			 "branches": [{"if": "体力<20", "then": []}]},
			{"id": "b", "match": "NPC[*]", "enabled": true,
			 "branches": [{"else": true, "then": []}]}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseRules 失败: %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("应解析 2 条规则, 实际 %d", len(rules))
	}
	if rules[0].ID != "a" {
		t.Errorf("首条规则 id = %s, 期望 a", rules[0].ID)
	}
}

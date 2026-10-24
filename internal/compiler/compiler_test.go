package compiler

import (
	"testing"

	"github.com/ruicat/astrbox/internal/neko"
)

func TestRewriteCondition(t *testing.T) {
	primary := []neko.Subject{{Kind: "primary", TypeName: "NPC"}}
	cases := []struct {
		name, input, want string
		subjects          []neko.Subject
	}{
		{"typed subject", "$[NPC].身体.体力 < 20", "{e}.身体.体力 < 20", primary},
		{"bare field", "体力 < 10", "{e}.身体.体力 < 10", primary},
		{"mapped field", "主人行为 in {抚摸}", "{e}.行为.主人行为 in {抚摸}", primary},
		{"global unchanged", "场景.事件 = 1", "场景.事件 = 1", primary},
		{"multi subject kept", "$1.亲密 > 5", "$1.亲密 > 5",
			[]neko.Subject{{Kind: "multi", Var: "$1", TypeName: "NPC"}}},
		{"logical split", "疼痛>95 ∨ 出血>70", "{e}.状态.疼痛 > 95 ∨ {e}.伤害.出血 > 70", primary},
		{"dollar path", "$.状态.身体 = \"活跃\"", "{e}.状态.身体 = \"活跃\"", primary},
		{"function arg", "危险度($[NPC].身体.体力) > 50", "危险度({e}.身体.体力) > 50", primary},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, _ := RewriteCondition(testCase.input, testCase.subjects)
			if got != testCase.want {
				t.Fatalf("RewriteCondition(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

func TestCompileSimpleRule(t *testing.T) {
	def := neko.RuleDef{
		Subjects:   []neko.Subject{{Kind: "primary", TypeName: "NPC"}},
		HeaderRaw:  "@规则[$[NPC]]",
		Conditions: []string{"$[NPC].身体.体力 < 20"},
		Actions: []neko.ActionOp{
			{Op: "set", Target: "状态.身体", Value: "疲惫"},
		},
	}
	rule, err := CompileRule(def)
	if err != nil {
		t.Fatalf("compile rule: %v", err)
	}
	if rule.Match != "NPC[*]" {
		t.Fatalf("match = %q", rule.Match)
	}
	if rule.When == nil || *rule.When != "{e}.身体.体力 < 20" {
		t.Fatalf("when = %v", rule.When)
	}
	if len(rule.Branches) != 1 || len(rule.Branches[0].Then) != 1 {
		t.Fatalf("branches = %+v", rule.Branches)
	}
	if rule.Branches[0].Then[0].Path != "{e}.状态.身体" {
		t.Fatalf("op path = %q", rule.Branches[0].Then[0].Path)
	}
	if !rule.Enabled || rule.Source != "script" {
		t.Fatalf("rule flags: enabled=%v source=%q", rule.Enabled, rule.Source)
	}
}

func TestCompileScriptText(t *testing.T) {
	text := `@Data NPC:猫娘 {
  "身体": {"体力": 60}
}

@规则[$[NPC]] {
  条件: 身体.体力 < 20
  动作: 状态.身体 = "疲惫"
}
`
	ast := neko.Parse(text)
	if len(ast.Errors) != 0 {
		t.Fatalf("parse errors: %v", ast.Errors)
	}
	result := CompileScript(ast)
	if !result.OK() {
		t.Fatalf("compile errors: %v", result.Errors)
	}
	template := result.Templates["NPC:猫娘"]
	if template == nil {
		t.Fatalf("template missing: %v", result.Templates)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("rules = %d", len(result.Rules))
	}
}

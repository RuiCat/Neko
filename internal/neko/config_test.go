package neko

import (
	"testing"

	"github.com/ruicat/astrbox/internal/config"
)

func TestConfiguredStateSugar(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状况", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	text := `@规则[$[NPC]] {
  条件: 体力 < 10
  分支: {
    体力 < 5 → 状况 = "昏厥"
    体力 >= 5 → 状况.情绪 = "难过"
  }
}`
	ast := Parse(text)
	if len(ast.RuleDefs) != 1 {
		t.Fatalf("rules = %d", len(ast.RuleDefs))
	}
	branches := ast.RuleDefs[0].Branches
	if len(branches) != 2 {
		t.Fatalf("branches = %d", len(branches))
	}
	if branches[0].State["身体"] != "昏厥" {
		t.Fatalf("default subgroup = %v", branches[0].State)
	}
	if branches[1].State["情绪"] != "难过" {
		t.Fatalf("explicit subgroup = %v", branches[1].State)
	}
}

func TestConfiguredGlobalActionTarget(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
		GlobalEntities: []string{"宇宙"}, DefaultScene: "场景",
		SceneEventField: "事件", ClassificationField: "分类",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	ast := Parse(`@规则[$] {
  动作: 宇宙.事件 = 1
}`)
	action := ast.RuleDefs[0].Actions[0]
	if action.Entity != "宇宙" || action.Target != "事件" {
		t.Fatalf("configured global action = %+v", action)
	}

	local := Parse(`@规则[$] {
  动作: 场景.事件 = 1
}`)
	action = local.RuleDefs[0].Actions[0]
	if action.Entity != "" || action.Target != "场景.事件" {
		t.Fatalf("old global should be local: %+v", action)
	}
}

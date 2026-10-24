package compiler

import (
	"testing"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/neko"
)

func TestConfiguredStateVocabulary(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状况", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	stateWhen := `$1.状况 = "活跃"`
	rewritten := RewriteStateWhen(&stateWhen)
	if rewritten == nil || *rewritten != `{e}.状况.身体 = "活跃"` {
		t.Fatalf("state when = %v", rewritten)
	}

	primary := []neko.Subject{{Kind: "primary", TypeName: "NPC"}}
	condition, _ := RewriteCondition("疼痛 > 95", primary)
	if condition != "{e}.状况.疼痛 > 95" {
		t.Fatalf("mapped state field = %q", condition)
	}
	bare, _ := RewriteCondition("体力 < 10", primary)
	if bare != "{e}.身体.体力 < 10" {
		t.Fatalf("bare field = %q", bare)
	}
}

func TestConfiguredGlobalVocabulary(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
		GlobalEntities: []string{"宇宙"}, DefaultScene: "基地",
		SceneEventField: "日志", ClassificationField: "标签",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	primary := []neko.Subject{{Kind: "primary", TypeName: "NPC"}}
	global, _ := RewriteCondition("宇宙.天气 > 1", primary)
	if global != "宇宙.天气 > 1" {
		t.Fatalf("configured global = %q", global)
	}
	nonGlobal, _ := RewriteCondition("场景.事件 = 1", primary)
	if nonGlobal != "{e}.场景.事件 = 1" {
		t.Fatalf("old global should be local now: %q", nonGlobal)
	}

	rule, err := CompileRule(neko.RuleDef{
		Subjects: []neko.Subject{{Kind: "primary", TypeName: "NPC"}},
		Actions: []neko.ActionOp{
			{Op: "emit", Value: "信号"},
			{Op: "append", Entity: "基地", Value: "雨"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	then := rule.Branches[0].Then
	if then[0].Op != "emit" || then[0].Entity != "基地" {
		t.Fatalf("emit op = %+v", then[0])
	}
	if then[1].Path != "日志" {
		t.Fatalf("scene event path = %q", then[1].Path)
	}
}

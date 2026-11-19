// extract_test.go pins the three corpus shapes (behavior, task templates, R18
// flow index) and the C2 effect classification including unclassified values.
package extract

import . "github.com/RuiCat/Neko/internal/flow/spec"

import (
	"encoding/json"
	"testing"
)

func TestExtractBehaviorFlow(t *testing.T) {
	src := []byte(`@定义 行为模板 {
  "吃饭": {
    "场景": "S001-KT",
    "需要物品": ["食物"],
    "花费": 3,
    "消耗": ["食物"],
    "产出": {"类型": "素材", "名称": "食物", "描述": "饭菜"},
    "步骤": ["摆碗", "吃饭"],
    "效果": {"测试.数值": 80, "测试.开关": false, "测试.增量": "+2", "测试.文本": "文本"}
  },
  "打工": {
    "场景": "S008",
    "收入": 20,
    "步骤": ["到店"],
    "效果": {"身体.活力": "-15"}
  }
}`)
	extracted := extractDocument("/tmp/corpus.neko", ParseDocument(src))
	if len(extracted.Issues) != 0 {
		t.Fatalf("issues: %v", extracted.Issues)
	}
	if len(extracted.Flows) != 2 || len(extracted.Records) != 2 || len(extracted.Containers) != 1 {
		t.Fatalf("flows=%d records=%d containers=%d", len(extracted.Flows), len(extracted.Records), len(extracted.Containers))
	}
	eat := extracted.Flows[0]
	if eat.ID != "吃饭" || eat.Source.Line != 2 || eat.Scene != "S001-KT" || eat.Cost != 3 {
		t.Fatalf("eat flow = %+v", eat)
	}
	if len(eat.Requires) != 1 || eat.Requires[0] != "食物" {
		t.Fatalf("requires = %v", eat.Requires)
	}
	if len(eat.Consume) != 1 || eat.Consume[0] != "食物" {
		t.Fatalf("consume = %v", eat.Consume)
	}
	if len(eat.Produce) != 1 || eat.Produce[0].Name != "食物" || eat.Produce[0].Type != "素材" {
		t.Fatalf("produce = %v", eat.Produce)
	}
	if len(eat.Steps) != 2 || eat.Steps[1] != "吃饭" {
		t.Fatalf("steps = %v", eat.Steps)
	}
	if len(eat.Effects) != 4 {
		t.Fatalf("effects = %v", eat.Effects)
	}
	kinds := map[string]EffectSpec{}
	for _, effect := range eat.Effects {
		kinds[effect.Path] = effect
	}
	if got := kinds["测试.数值"]; got.Kind != EFFECT_KIND_SET || got.Value != json.Number("80") {
		t.Fatalf("测试.数值 effect = %+v", got)
	}
	if got := kinds["测试.开关"]; got.Kind != EFFECT_KIND_BOOL || got.Value != false {
		t.Fatalf("测试.开关 effect = %+v", got)
	}
	if got := kinds["测试.增量"]; got.Kind != EFFECT_KIND_ADD || got.Value != json.Number("2") {
		t.Fatalf("测试.增量 effect = %+v", got)
	}
	if got := kinds["测试.文本"]; got.Kind != EFFECT_KIND_TEXT || got.Value != "文本" {
		t.Fatalf("测试.文本 effect = %+v", got)
	}
	if extracted.Records[0].Key != "吃饭" || extracted.Records[0].Line != 2 || extracted.Records[0].Kind != "flow" {
		t.Fatalf("record = %+v", extracted.Records[0])
	}
	if extracted.Flows[1].Effects[0].Value != json.Number("-15") {
		t.Fatalf("打工 effect = %+v", extracted.Flows[1].Effects[0])
	}
}

func TestExtractTaskTemplatesAndSkippedBlocks(t *testing.T) {
	src := []byte(`@定义 任务模板库 {
  "收集任务": {"阶段": ["已激活", "进行中", "完成"], "条件": {"类型": "需要物品", "示例": "小鱼干"}, "奖励": {"经验": 20, "好感度": 5}, "对话": {"激活": "去找", "进行中": "找到没", "完成": "谢谢"}},
  "地点任务": {"阶段": ["已激活", "进行中", "完成"], "条件": {"类型": "需要地点", "示例": "S001-LR"}, "奖励": {"经验": 15}, "对话": {"激活": "去看看", "进行中": "到了吗", "完成": "做得好"}}
}
@定义 任务模板 {
  "收集任务": {"阶段": "已激活", "条件": "需要物品", "奖励": "经验20+好感5"}
}
@定义 角色基础 {
  "任务": {"阶段": "无"}
}
@规则[$[角色基础:猫猫]] {
  优先级: 200
  $.任务.阶段 == "已激活" { $.测试.字段 <- "x" }
}`)
	extracted := extractDocument("/tmp/corpus.neko", ParseDocument(src))
	if len(extracted.Flows) != 0 {
		t.Fatalf("task templates must not become flows: %v", extracted.Flows)
	}
	if len(extracted.Records) != 3 {
		t.Fatalf("records = %d, want 3", len(extracted.Records))
	}
	for _, record := range extracted.Records {
		if record.Kind != "taskTemplate" {
			t.Fatalf("record kind = %q", record.Kind)
		}
	}
	if len(extracted.Containers) != 3 {
		t.Fatalf("containers = %d, want 3", len(extracted.Containers))
	}
	if len(extracted.TaskTemplates) != 3 {
		t.Fatalf("taskTemplates = %d, want 3", len(extracted.TaskTemplates))
	}
	library := extracted.TaskTemplates[0]
	if library.ID != "收集任务" || len(library.Stages) != 3 || library.Stages[0] != "未激活" {
		t.Fatalf("library stages = %+v", library)
	}
	if library.Condition.Type != "需要物品" || library.Condition.Example != "小鱼干" {
		t.Fatalf("library condition = %+v", library.Condition)
	}
	if library.Reward.Experience != 20 || library.Reward.Affinity != 5 {
		t.Fatalf("library reward = %+v", library.Reward)
	}
	if !hasTaskDialog(library.Dialog) {
		t.Fatalf("library dialog = %+v", library.Dialog)
	}
	partial := extracted.TaskTemplates[2]
	if partial.ID != "收集任务" || len(partial.Stages) != 1 || partial.Stages[0] != "未激活" {
		t.Fatalf("partial stages = %+v", partial)
	}
	if partial.Condition.Type != "需要物品" || partial.Condition.Example != "" {
		t.Fatalf("partial condition = %+v", partial.Condition)
	}
	if partial.Reward.Experience != 20 || partial.Reward.Affinity != 5 {
		t.Fatalf("partial reward = %+v", partial.Reward)
	}
	if hasTaskDialog(partial.Dialog) {
		t.Fatalf("partial dialog should be empty before Generate join: %+v", partial.Dialog)
	}
	if len(extracted.Skipped) != 2 {
		t.Fatalf("skipped = %+v, want 角色基础 and one rule", extracted.Skipped)
	}
	if extracted.Skipped[0].Directive != "@定义 NPC" || extracted.Skipped[1].Directive != "@规则" {
		t.Fatalf("skipped order = %+v", extracted.Skipped)
	}
}

func TestExtractR18Flows(t *testing.T) {
	src := []byte(`@定义 性行为流程库 {
  "流程索引": {
    "f1": {"步骤": 84, "类型": {"对话": 66}},
    "f2": {"步骤": 40, "类型": {"对话": 34}}
  },
  "流程对话摘要": {
    "f1": ["a", "b"],
    "f2": ["c"]
  }
}`)
	extracted := extractDocument("/tmp/corpus.neko", ParseDocument(src))
	if len(extracted.Issues) != 0 {
		t.Fatalf("issues: %v", extracted.Issues)
	}
	if len(extracted.Flows) != 2 || len(extracted.Records) != 2 {
		t.Fatalf("flows=%d records=%d", len(extracted.Flows), len(extracted.Records))
	}
	if got := extracted.Flows[0]; got.ID != "f1" || len(got.Steps) != 2 || got.Steps[1] != "b" {
		t.Fatalf("flow f1 = %+v", got)
	}
	if got := extracted.Flows[1]; got.ID != "f2" || len(got.Steps) != 1 {
		t.Fatalf("flow f2 = %+v", got)
	}
	if extracted.Records[0].Kind != "r18Flow" || extracted.Records[0].Line != 3 {
		t.Fatalf("record = %+v", extracted.Records[0])
	}
}

func TestExtractUnclassifiedEffectIsCounted(t *testing.T) {
	src := []byte(`@定义 行为模板 {
  "怪效果": {
    "场景": "S001",
    "步骤": ["a"],
    "效果": {"怪.路径": {"a": 1}}
  }
}`)
	extracted := extractDocument("/tmp/corpus.neko", ParseDocument(src))
	if len(extracted.Flows) != 1 || len(extracted.Flows[0].Effects) != 0 {
		t.Fatalf("classified effects = %+v", extracted.Flows)
	}
	if len(extracted.Unclassified) != 1 {
		t.Fatalf("unclassified = %+v", extracted.Unclassified)
	}
	if extracted.Unclassified[0].Path != "怪.路径" || extracted.Unclassified[0].Reason == "" {
		t.Fatalf("unclassified detail = %+v", extracted.Unclassified[0])
	}
	effectMap := buildEffectMap(extracted.Flows, extracted.Unclassified)
	if effectMap.Unclassified != 1 || len(effectMap.UnclassifiedItems) != 1 || len(effectMap.Effects) != 0 {
		t.Fatalf("effect map = %+v", effectMap)
	}
}

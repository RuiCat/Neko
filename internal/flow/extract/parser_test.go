// parser_test.go pins the .neko data subset: comments, CJK identifiers, nested
// objects/arrays, literal line anchors and balanced skipping of @规则 blocks.
package extract

import (
	"strings"
	"testing"
)

func TestParseDocumentBehaviorShape(t *testing.T) {
	src := []byte(`// header
@定义 行为模板 {
  "购买食材": {
    "场景": "S008",
    "花费": 10,
    "产出": {"类型": "素材", "名称": "食材", "描述": "新鲜蔬菜和肉"},
    "步骤": ["出门", "挑选", "付钱"],
    "效果": {}
  },
  "吃饭": {
    "需要物品": ["食物"],
    "步骤": ["吃饭"],
    "效果": {"测试.数值": 80, "测试.开关": false, "测试.增量": "+2", "测试.文本": "文本"}
  }
}

@规则[$[NPC:猫猫]] {
  优先级: 200
  $.任务.阶段 == "已激活" {
    $.测试.字段 <- "x"
  }
}`)
	doc := ParseDocument(src)
	if len(doc.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", doc.Issues)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(doc.Blocks))
	}
	behavior := doc.Blocks[0]
	if behavior.Directive != "定义" || behavior.Name != "行为模板" || behavior.Line != 2 || behavior.Skipped {
		t.Fatalf("unexpected behavior block: %+v", behavior)
	}
	if len(behavior.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(behavior.Entries))
	}
	purchase := behavior.Entries[0]
	if purchase.Key != "购买食材" || purchase.KeyLine != 3 {
		t.Fatalf("purchase anchor = %d %q", purchase.KeyLine, purchase.Key)
	}
	if purchase.Value.Kind != VALUE_OBJECT {
		t.Fatalf("purchase body kind = %v", purchase.Value.Kind)
	}
	scene, ok := objectEntry(purchase.Value.Entries, "场景")
	if !ok || scene.Value.Text != "S008" || scene.Value.Line != 4 {
		t.Fatalf("scene = %+v ok=%v", scene, ok)
	}
	cost, ok := objectEntry(purchase.Value.Entries, "花费")
	if !ok || cost.Value.Kind != VALUE_NUMBER || cost.Value.Text != "10" || cost.Value.Line != 5 {
		t.Fatalf("cost = %+v ok=%v", cost, ok)
	}
	produce, ok := objectEntry(purchase.Value.Entries, "产出")
	if !ok || produce.Value.Kind != VALUE_OBJECT || produce.Value.Line != 6 {
		t.Fatalf("produce = %+v ok=%v", produce, ok)
	}
	produceName, ok := objectEntry(produce.Value.Entries, "名称")
	if !ok || produceName.Value.Text != "食材" {
		t.Fatalf("produce name = %+v ok=%v", produceName, ok)
	}
	eat := behavior.Entries[1]
	if eat.Key != "吃饭" || eat.KeyLine != 10 {
		t.Fatalf("eat anchor = %d %q", eat.KeyLine, eat.Key)
	}
	effects, ok := objectEntry(eat.Value.Entries, "效果")
	if !ok || effects.Value.Kind != VALUE_OBJECT || len(effects.Value.Entries) != 4 {
		t.Fatalf("effects = %+v ok=%v", effects, ok)
	}
	for _, effect := range effects.Value.Entries {
		if effect.KeyLine != 13 {
			t.Fatalf("inline effect %q line = %d, want 13", effect.Key, effect.KeyLine)
		}
	}
	rule := doc.Blocks[1]
	if rule.Directive != "规则" || rule.Line != 17 || !rule.Skipped {
		t.Fatalf("unexpected rule block: %+v", rule)
	}
}

func TestParseDocumentNestedArraysAndNumbers(t *testing.T) {
	src := []byte(`@定义 性行为流程库 {
  "流程索引": {
    "a": {"步骤": 84, "类型": {"对话": 66}},
    "b": {"步骤": 3.5, "类型": {"对话": -1}}
  },
  "流程对话摘要": {
    "a": ["x", "y"],
    "b": []
  }
}`)
	doc := ParseDocument(src)
	if len(doc.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", doc.Issues)
	}
	if len(doc.Blocks) != 1 || len(doc.Blocks[0].Entries) != 2 {
		t.Fatalf("unexpected blocks: %+v", doc.Blocks)
	}
	index, ok := objectEntry(doc.Blocks[0].Entries, "流程索引")
	if !ok {
		t.Fatal("missing 流程索引")
	}
	b, ok := objectEntry(index.Value.Entries, "b")
	if !ok || b.Value.Kind != VALUE_OBJECT {
		t.Fatalf("b = %+v ok=%v", b, ok)
	}
	step, ok := objectEntry(b.Value.Entries, "步骤")
	if !ok || step.Value.Text != "3.5" {
		t.Fatalf("step = %+v ok=%v", step, ok)
	}
	types, ok := objectEntry(b.Value.Entries, "类型")
	if !ok {
		t.Fatal("missing 类型")
	}
	dialog, ok := objectEntry(types.Value.Entries, "对话")
	if !ok || dialog.Value.Text != "-1" {
		t.Fatalf("dialog = %+v ok=%v", dialog, ok)
	}
	summary, ok := objectEntry(doc.Blocks[0].Entries, "流程对话摘要")
	if !ok {
		t.Fatal("missing 流程对话摘要")
	}
	a, ok := objectEntry(summary.Value.Entries, "a")
	if !ok || a.Value.Kind != VALUE_ARRAY || len(a.Value.Items) != 2 || a.Value.Items[1].Text != "y" {
		t.Fatalf("a = %+v ok=%v", a, ok)
	}
}

func TestParseDocumentStringEscapes(t *testing.T) {
	src := []byte(`@定义 X { "k": "a\"b\\c\n" }`)
	doc := ParseDocument(src)
	if len(doc.Issues) != 0 {
		t.Fatalf("unexpected issues: %v", doc.Issues)
	}
	entry, ok := objectEntry(doc.Blocks[0].Entries, "k")
	if !ok {
		t.Fatal("missing k")
	}
	if got, want := entry.Value.Text, "a\"b\\c\n"; got != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

func TestParseDocumentMalformedIsReported(t *testing.T) {
	src := []byte(`@定义 行为模板 { "a": }`)
	doc := ParseDocument(src)
	if len(doc.Issues) == 0 {
		t.Fatal("expected an issue for a missing value")
	}
	if !strings.Contains(doc.Issues[0].Message, "unexpected token") {
		t.Fatalf("issue = %+v", doc.Issues[0])
	}
	if len(doc.Blocks) != 1 || len(doc.Blocks[0].Entries) != 0 {
		t.Fatalf("partial extraction = %+v", doc.Blocks)
	}
}

func objectEntry(entries []Entry, key string) (Entry, bool) {
	for _, entry := range entries {
		if entry.Key == key {
			return entry, true
		}
	}
	return Entry{}, false
}

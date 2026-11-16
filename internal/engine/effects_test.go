// Text effect parsing/application tests. The parser keeps no word table: the
// word is resolved against the target entity's own leaf paths.
package engine

import (
	"testing"

	"github.com/RuiCat/Neko/internal/entity"
)

func hasEffect(effects []Effect, word string, delta float64) bool {
	for _, effect := range effects {
		if effect.Word == word && effect.Delta == delta {
			return true
		}
	}
	return false
}

func expectEffect(t *testing.T, effects []Effect, word string, delta float64) {
	t.Helper()
	if !hasEffect(effects, word, delta) {
		t.Errorf("缺少效果 %s %+v, 实际 %+v", word, delta, effects)
	}
}

func newEffectEntity(t *testing.T, data map[string]any) *entity.Entity {
	t.Helper()
	ent, err := entity.CreateEntity("猫猫", "NPC", nil, data, nil)
	if err != nil {
		t.Fatalf("创建实体失败: %v", err)
	}
	return ent
}

func entityNumber(t *testing.T, ent *entity.Entity, path string) float64 {
	t.Helper()
	value, err := ent.Get(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	number, ok := toFloatValue(value)
	if !ok {
		t.Fatalf("路径 %s 不是数字: %#v", path, value)
	}
	return number
}

func TestParseEffectsSimple(t *testing.T) {
	effects := ParseEffects("情境修正:体力+15,疼痛-20")
	if len(effects) != 2 {
		t.Fatalf("应解析 2 条效果, 实际 %+v", effects)
	}
	expectEffect(t, effects, "体力", 15)
	expectEffect(t, effects, "疼痛", -20)
}

func TestParseEffectsMultiple(t *testing.T) {
	effects := ParseEffects("主人出门:口渴+30,体力+20,水分-15")
	if len(effects) != 3 {
		t.Fatalf("应解析 3 条效果, 实际 %+v", effects)
	}
	expectEffect(t, effects, "口渴", 30)
	expectEffect(t, effects, "体力", 20)
	expectEffect(t, effects, "水分", -15)
}

func TestParseEffectsNoEffect(t *testing.T) {
	if effects := ParseEffects("普通文本"); len(effects) != 0 {
		t.Fatalf("普通文本不应有效果: %+v", effects)
	}
	if effects := ParseEffects(""); len(effects) != 0 {
		t.Fatalf("空文本不应有效果: %+v", effects)
	}
}

func TestParseEffectsRanges(t *testing.T) {
	effects := ParseEffects("体力5-15,疼痛-10~-5,恐惧-10-(-5)")
	expectEffect(t, effects, "体力", 5)
	expectEffect(t, effects, "疼痛", -10)
	expectEffect(t, effects, "恐惧", -10)
}

func TestParseEffectsKeepsRawWord(t *testing.T) {
	effects := ParseEffects("快感+10")
	if len(effects) != 1 || effects[0].Word != "快感" {
		t.Fatalf("应保留原始词: %+v", effects)
	}
}

func TestApplyEffectsResolvesLeafName(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"身体": map[string]any{"体力": 75, "疼痛": 10},
		"代谢": map[string]any{"胃内容": 100},
	})
	applied := ApplyEffects(ent, "抚摸:体力+15,疼痛-5")
	if len(applied) != 2 {
		t.Fatalf("应应用 2 条效果: %+v", applied)
	}
	if got := entityNumber(t, ent, "身体.体力"); got != 90 {
		t.Fatalf("身体.体力 = %v，期望 90", got)
	}
	if got := entityNumber(t, ent, "身体.疼痛"); got != 5 {
		t.Fatalf("身体.疼痛 = %v，期望 5", got)
	}
}

func TestApplyEffectsExactDottedPath(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"代谢": map[string]any{"胃内容": 100},
	})
	applied := ApplyEffects(ent, "代谢.胃内容+50")
	if len(applied) != 1 || applied[0].Path != "代谢.胃内容" {
		t.Fatalf("apply = %+v", applied)
	}
	if got := entityNumber(t, ent, "代谢.胃内容"); got != 150 {
		t.Fatalf("代谢.胃内容 = %v，期望 150", got)
	}
}

func TestApplyEffectsRawDelta(t *testing.T) {
	// 引擎不再替字段猜 0..100：范围交给模板注解，这里只验证原样加增量。
	ent := newEffectEntity(t, map[string]any{
		"身体": map[string]any{"体力": 98, "饱食": 12},
	})
	ApplyEffects(ent, "体力+15,饱食-10")
	if got := entityNumber(t, ent, "身体.体力"); got != 113 {
		t.Fatalf("身体.体力 = %v，期望 113", got)
	}
	if got := entityNumber(t, ent, "身体.饱食"); got != 2 {
		t.Fatalf("身体.饱食 = %v，期望 2", got)
	}
}

func TestApplyEffectsUnknownWordSkipped(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"身体": map[string]any{"体力": 50},
	})
	applied := ApplyEffects(ent, "神秘感+99,体力+5")
	if len(applied) != 1 || applied[0].Path != "身体.体力" {
		t.Fatalf("未知词应跳过: %+v", applied)
	}
}

func TestApplyEffectsShortestPathWins(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"身体":   map[string]any{"体力": 50},
		"器官状态": map[string]any{"基础生理": map[string]any{"体力": 10}},
	})
	applied := ApplyEffects(ent, "体力+5")
	if len(applied) != 1 || applied[0].Path != "身体.体力" {
		t.Fatalf("应选最短路径 身体.体力: %+v", applied)
	}
}

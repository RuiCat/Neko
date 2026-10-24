// Ported effect-parser assertions from astrbox-plugin/tests/test_effect_parser.py.
package engine

import (
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func hasEffect(effects []Effect, path string, delta float64) bool {
	for _, effect := range effects {
		if effect.Path == path && effect.Delta == delta {
			return true
		}
	}
	return false
}

func expectEffect(t *testing.T, effects []Effect, path string, delta float64) {
	t.Helper()
	if !hasEffect(effects, path, delta) {
		t.Errorf("缺少效果 %s %+v, 实际 %+v", path, delta, effects)
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
	effects := ParseEffects("情境修正:撒娇+15,傲娇-20")
	if len(effects) != 2 {
		t.Fatalf("应解析 2 条效果, 实际 %+v", effects)
	}
	expectEffect(t, effects, "关系.撒娇欲", 15)
	expectEffect(t, effects, "行为.傲娇倾向", -20)
}

func TestParseEffectsMultiple(t *testing.T) {
	effects := ParseEffects("主人出门:黏人+30,撒娇+20,安全感-15")
	if len(effects) != 3 {
		t.Fatalf("应解析 3 条效果, 实际 %+v", effects)
	}
	expectEffect(t, effects, "行为.黏人", 30)
	expectEffect(t, effects, "关系.撒娇欲", 20)
	expectEffect(t, effects, "关系.安全感", -15)
}

func TestParseEffectsNoEffect(t *testing.T) {
	if effects := ParseEffects("普通文本"); len(effects) != 0 {
		t.Errorf("普通文本不应产生效果: %+v", effects)
	}
	if effects := ParseEffects(""); len(effects) != 0 {
		t.Errorf("空文本不应产生效果: %+v", effects)
	}
}

func TestParseEffectsPathMap(t *testing.T) {
	effects := ParseEffects("快感+10,兴奋+5,害羞度-10")
	expectEffect(t, effects, "器官状态.基础生理.快感程度", 10)
	expectEffect(t, effects, "器官状态.扩展心理.兴奋度", 5)
	expectEffect(t, effects, "关系.害羞", -10)
}

func TestParseEffectsUnknownWordSkipped(t *testing.T) {
	effects := ParseEffects("神秘感+99,撒娇+5")
	if len(effects) != 1 {
		t.Fatalf("未知词应跳过, 实际 %+v", effects)
	}
	if effects[0].Path != "关系.撒娇欲" {
		t.Errorf("保留的已知词路径错误: %s", effects[0].Path)
	}
}

func TestApplyEffectsToEntity(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"关系": map[string]any{"撒娇欲": 85.0},
		"行为": map[string]any{"傲娇倾向": 40.0},
	})
	applied := ApplyEffects(ent, "抚摸:撒娇+15,傲娇-20")
	if got := entityNumber(t, ent, "关系.撒娇欲"); got != 100 {
		t.Errorf("撒娇欲应饱和到 100, 实际 %v", got)
	}
	if got := entityNumber(t, ent, "行为.傲娇倾向"); got != 20 {
		t.Errorf("傲娇倾向应变为 20, 实际 %v", got)
	}
	if len(applied) != 2 {
		t.Errorf("应报告 2 条应用效果, 实际 %+v", applied)
	}
}

func TestApplyEffectsClamp(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{"关系": map[string]any{"撒娇欲": 95.0}})
	ApplyEffects(ent, "撒娇+15")
	if got := entityNumber(t, ent, "关系.撒娇欲"); got != 100 {
		t.Errorf("上限应为 100, 实际 %v", got)
	}
	lower := newEffectEntity(t, map[string]any{"关系": map[string]any{"安全感": 10.0}})
	ApplyEffects(lower, "安全感-15")
	if got := entityNumber(t, lower, "关系.安全感"); got != 0 {
		t.Errorf("下限应为 0, 实际 %v", got)
	}
}

func TestEffectMappingsSupplement(t *testing.T) {
	hunger := ParseEffects("体力-10,饱食+20,水分-5")
	expectEffect(t, hunger, "身体.体力", -10)
	expectEffect(t, hunger, "身体.饱食", 20)
	expectEffect(t, hunger, "身体.水分", -5)

	damage := ParseEffects("伤害+3,创伤+10,恐惧+5")
	expectEffect(t, damage, "器官状态.R18G伤害.伤害程度", 3)
	expectEffect(t, damage, "伤害.创伤", 10)
	expectEffect(t, damage, "状态.恐惧", 5)

	fresh := ParseEffects("成瘾+5,期待+10,信任+15,出血+8,羞耻感+20")
	expectEffect(t, fresh, "器官状态.扩展心理.成瘾度", 5)
	expectEffect(t, fresh, "器官状态.扩展心理.期待度", 10)
	expectEffect(t, fresh, "器官状态.扩展心理.信任度", 15)
	expectEffect(t, fresh, "伤害.出血", 8)
	expectEffect(t, fresh, "器官状态.扩展心理.羞耻感", 20)
}

func TestFourBehaviorEffects(t *testing.T) {
	touchHand := ParseEffects("亲密+5,信赖+3,害羞+2,撒娇+4")
	expectEffect(t, touchHand, "关系.亲密", 5)
	expectEffect(t, touchHand, "关系.信赖", 3)
	expectEffect(t, touchHand, "关系.害羞", 2)
	expectEffect(t, touchHand, "关系.撒娇欲", 4)

	rubBelly := ParseEffects("舒适+8,亲密+3,满足+5,信任+2")
	expectEffect(t, rubBelly, "身体.舒适", 8)
	expectEffect(t, rubBelly, "器官状态.扩展心理.满足度", 5)
	expectEffect(t, rubBelly, "器官状态.扩展心理.信任度", 2)

	pinchFace := ParseEffects("害羞+10,亲密+2,撒娇+3")
	expectEffect(t, pinchFace, "关系.害羞", 10)
	expectEffect(t, pinchFace, "关系.撒娇欲", 3)

	poke := ParseEffects("快感+15,湿润+10,收缩+2,亲密+3")
	expectEffect(t, poke, "器官状态.基础生理.快感程度", 15)
	expectEffect(t, poke, "器官状态.基础生理.湿润度", 10)
	expectEffect(t, poke, "器官状态.基础生理.收缩状态", 2)
	expectEffect(t, poke, "关系.亲密", 3)
}

func fullExecutionEntity(t *testing.T) *entity.Entity {
	return newEffectEntity(t, map[string]any{
		"关系": map[string]any{"亲密": 50.0, "信赖": 50.0, "害羞": 30.0, "撒娇欲": 30.0},
		"身体": map[string]any{"舒适": 50.0},
		"行为": map[string]any{"好奇心": 50.0, "服从": 50.0, "主动": 50.0, "傲娇倾向": 30.0, "黏人": 30.0},
		"器官状态": map[string]any{
			"基础生理": map[string]any{"快感程度": 30.0, "湿润度": 50.0, "收缩状态": 50.0},
			"扩展心理": map[string]any{"满足度": 40.0, "信任度": 50.0, "幸福感": 50.0, "兴奋度": 30.0},
		},
	})
}

func TestFourBehaviorFullExecution(t *testing.T) {
	ent := fullExecutionEntity(t)
	ApplyEffects(ent, "亲密+5,信赖+3,害羞+2,撒娇+4")
	if got := entityNumber(t, ent, "关系.亲密"); got != 55 {
		t.Errorf("亲密应为 55, 实际 %v", got)
	}
	if got := entityNumber(t, ent, "关系.信赖"); got != 53 {
		t.Errorf("信赖应为 53, 实际 %v", got)
	}
	if got := entityNumber(t, ent, "关系.害羞"); got != 32 {
		t.Errorf("害羞应为 32, 实际 %v", got)
	}
	if got := entityNumber(t, ent, "关系.撒娇欲"); got != 34 {
		t.Errorf("撒娇欲应为 34, 实际 %v", got)
	}

	belly := fullExecutionEntity(t)
	ApplyEffects(belly, "舒适+8,亲密+3,满足+5,信任+2")
	if got := entityNumber(t, belly, "身体.舒适"); got != 58 {
		t.Errorf("舒适应为 58, 实际 %v", got)
	}
	if got := entityNumber(t, belly, "关系.亲密"); got != 53 {
		t.Errorf("亲密应为 53, 实际 %v", got)
	}
	if got := entityNumber(t, belly, "器官状态.扩展心理.满足度"); got != 45 {
		t.Errorf("满足度应为 45, 实际 %v", got)
	}
	if got := entityNumber(t, belly, "器官状态.扩展心理.信任度"); got != 52 {
		t.Errorf("信任度应为 52, 实际 %v", got)
	}

	face := fullExecutionEntity(t)
	ApplyEffects(face, "害羞+10,亲密+2,撒娇+3")
	if got := entityNumber(t, face, "关系.害羞"); got != 40 {
		t.Errorf("害羞应为 40, 实际 %v", got)
	}
	if got := entityNumber(t, face, "关系.亲密"); got != 52 {
		t.Errorf("亲密应为 52, 实际 %v", got)
	}
	if got := entityNumber(t, face, "关系.撒娇欲"); got != 33 {
		t.Errorf("撒娇欲应为 33, 实际 %v", got)
	}

	poke := fullExecutionEntity(t)
	ApplyEffects(poke, "快感+15,湿润+10,收缩+2,亲密+3")
	if got := entityNumber(t, poke, "器官状态.基础生理.快感程度"); got != 45 {
		t.Errorf("快感程度应为 45, 实际 %v", got)
	}
	if got := entityNumber(t, poke, "器官状态.基础生理.湿润度"); got != 60 {
		t.Errorf("湿润度应为 60, 实际 %v", got)
	}
	if got := entityNumber(t, poke, "器官状态.基础生理.收缩状态"); got != 52 {
		t.Errorf("收缩状态应为 52, 实际 %v", got)
	}
	if got := entityNumber(t, poke, "关系.亲密"); got != 53 {
		t.Errorf("亲密应为 53, 实际 %v", got)
	}
}

func TestEffectAliases(t *testing.T) {
	expectEffect(t, ParseEffects("满足+5"), "器官状态.扩展心理.满足度", 5)
	expectEffect(t, ParseEffects("满足度+5"), "器官状态.扩展心理.满足度", 5)
	expectEffect(t, ParseEffects("傲娇倾向-20"), "行为.傲娇倾向", -20)
	expectEffect(t, ParseEffects("撒娇倾向+10"), "关系.撒娇欲", 10)
	expectEffect(t, ParseEffects("好奇心+15"), "行为.好奇心", 15)
	expectEffect(t, ParseEffects("幸福+20"), "器官状态.扩展心理.幸福感", 20)
	expectEffect(t, ParseEffects("服从倾向+5"), "行为.服从", 5)
	expectEffect(t, ParseEffects("主动程度+8"), "行为.主动", 8)
	expectEffect(t, ParseEffects("黏人程度+6"), "行为.黏人", 6)
}

func TestDictStateFieldPreserved(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"关系": map[string]any{
			"撒娇欲": map[string]any{"value": 97.0, "unit": "%", "desc": "想蹭蹭主人"},
		},
	})
	applied := ApplyEffects(ent, "撒娇+15")
	wrapper, ok := ent.GetOrNone("关系.撒娇欲").(map[string]any)
	if !ok {
		t.Fatalf("dict 结构被破坏: %#v", ent.GetOrNone("关系.撒娇欲"))
	}
	if number, _ := toFloatValue(wrapper["value"]); number != 100 {
		t.Errorf("value 应限幅到 100, 实际 %v", wrapper["value"])
	}
	if wrapper["unit"] != "%" {
		t.Errorf("unit 应保留, 实际 %v", wrapper["unit"])
	}
	if wrapper["desc"] != "想蹭蹭主人" {
		t.Errorf("desc 应保留, 实际 %v", wrapper["desc"])
	}
	if len(applied) != 1 {
		t.Errorf("应报告 1 条应用效果, 实际 %d", len(applied))
	}

	lower := newEffectEntity(t, map[string]any{
		"关系": map[string]any{
			"安全感": map[string]any{"value": 10.0, "unit": "%", "desc": "有点不安"},
		},
	})
	ApplyEffects(lower, "安全感-15")
	lowerWrapper := lower.GetOrNone("关系.安全感").(map[string]any)
	if number, _ := toFloatValue(lowerWrapper["value"]); number != 0 {
		t.Errorf("value 应限幅到 0, 实际 %v", lowerWrapper["value"])
	}
	if lowerWrapper["desc"] != "有点不安" {
		t.Errorf("desc 应保留, 实际 %v", lowerWrapper["desc"])
	}
}

func TestClampHugeValues(t *testing.T) {
	high := newEffectEntity(t, map[string]any{"关系": map[string]any{"撒娇欲": 50.0}})
	ApplyEffects(high, "撒娇+999")
	if got := entityNumber(t, high, "关系.撒娇欲"); got != 100 {
		t.Errorf("+999 应限幅 100, 实际 %v", got)
	}
	low := newEffectEntity(t, map[string]any{"关系": map[string]any{"撒娇欲": 50.0}})
	ApplyEffects(low, "撒娇-999")
	if got := entityNumber(t, low, "关系.撒娇欲"); got != 0 {
		t.Errorf("-999 应限幅 0, 实际 %v", got)
	}
}

func TestApplyNoneAndEmpty(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{"关系": map[string]any{"撒娇欲": 50.0}})
	if applied := ApplyEffects(ent, ""); len(applied) != 0 {
		t.Errorf("空效果文本应返回空: %+v", applied)
	}
	if effects := ParseEffects(""); len(effects) != 0 {
		t.Errorf("空效果文本应解析为空: %+v", effects)
	}
}

func TestEffectsWithPunctuation(t *testing.T) {
	effects := ParseEffects("满足+50,幸福感+35,爱意+25,期待+30,体力-25,成瘾度+5")
	expectEffect(t, effects, "器官状态.扩展心理.满足度", 50)
	expectEffect(t, effects, "器官状态.扩展心理.幸福感", 35)
	expectEffect(t, effects, "关系.爱意", 25)
	expectEffect(t, effects, "器官状态.扩展心理.期待度", 30)
	expectEffect(t, effects, "身体.体力", -25)
	expectEffect(t, effects, "器官状态.扩展心理.成瘾度", 5)
}

func TestContextPrefixEffects(t *testing.T) {
	effects := ParseEffects("情境:雨天·黏人+20撒娇+15好奇-20")
	if len(effects) != 3 {
		t.Fatalf("应解析 3 条效果, 实际 %+v", effects)
	}
	expectEffect(t, effects, "行为.黏人", 20)
	expectEffect(t, effects, "关系.撒娇欲", 15)
	expectEffect(t, effects, "行为.好奇心", -20)
}

func TestContextPrefixNoTriggerForOther(t *testing.T) {
	if effects := ParseEffects("情境:晴天·晒太阳舒服"); len(effects) != 0 {
		t.Errorf("晴天情境不应产生效果: %+v", effects)
	}
}

func TestFloatEffects(t *testing.T) {
	floatEffects := ParseEffects("快感+5.5,湿润+2.5")
	if len(floatEffects) != 2 || floatEffects[0].Delta != 5.5 || floatEffects[1].Delta != 2.5 {
		t.Errorf("小数增量错误: %+v", floatEffects)
	}
	intEffects := ParseEffects("撒娇+15,傲娇-20")
	if len(intEffects) != 2 || intEffects[0].Delta != 15 {
		t.Errorf("整数增量错误: %+v", intEffects)
	}
	half := ParseEffects("快感+0.5")
	if len(half) != 1 || half[0].Delta != 0.5 {
		t.Errorf("+0.5 解析错误: %+v", half)
	}
	if got := ParseAssign("清洁状态:良好"); len(got) != 1 || got[0].Value != "良好" {
		t.Errorf("字符串赋值错误: %+v", got)
	}
}

func TestFullwidthSupport(t *testing.T) {
	if got := ParseAssign("安全等级＝red"); len(got) != 1 || got[0].Path != "行为.安全等级" || got[0].Value != "red" {
		t.Errorf("全角等号赋值错误: %+v", got)
	}
	if got := ParseAssign("状态.身体＝疲惫"); len(got) != 1 || got[0].Value != "疲惫" {
		t.Errorf("全角等号嵌套赋值错误: %+v", got)
	}
	if got := ParseEffects("体力＋１０"); len(got) != 1 || got[0].Delta != 10 {
		t.Errorf("全角加号数字错误: %+v", got)
	}
	fullwidth := ParseEffects("体力－１０,饱食＋２０.５")
	if len(fullwidth) != 2 || fullwidth[0].Delta != -10 || fullwidth[1].Delta != 20.5 {
		t.Errorf("全角减号/小数错误: %+v", fullwidth)
	}
	if got := ParseEffects("体力+5.5"); len(got) != 1 || got[0].Delta != 5.5 {
		t.Errorf("半角回归失败: %+v", got)
	}
}

func TestRangeFormat(t *testing.T) {
	positive := ParseEffects("行为修饰:湿润5-15")
	if len(positive) != 1 || positive[0].Path != "器官状态.基础生理.湿润度" || positive[0].Delta != 5 {
		t.Errorf("正区间应取起点 +5: %+v", positive)
	}
	negative := ParseEffects("行为修饰:体力-1-(-0.5)")
	if len(negative) != 1 || negative[0].Path != "身体.体力" || negative[0].Delta != -1 {
		t.Errorf("负区间应取起点 -1: %+v", negative)
	}
	tilded := ParseEffects("体力-10~-5")
	if len(tilded) != 1 || tilded[0].Delta != -10 {
		t.Errorf("~ 区间应取起点 -10: %+v", tilded)
	}
	mixed := ParseEffects("行为修饰:幸福感10-25,满足5-15,恐惧-10-(-5)")
	if len(mixed) != 3 {
		t.Fatalf("混合区间应解析 3 条: %+v", mixed)
	}
	expectEffect(t, mixed, "器官状态.扩展心理.幸福感", 10)
	expectEffect(t, mixed, "器官状态.扩展心理.满足度", 5)
	expectEffect(t, mixed, "状态.恐惧", -10)
}

func TestAssignParsing(t *testing.T) {
	cases := []struct {
		text string
		path string
		any  any
	}{
		{"安全等级=red", "行为.安全等级", "red"},
		{"安全等级:red", "行为.安全等级", "red"},
		{"阶段=睡眠", "作息.阶段", "睡眠"},
		{"清洁状态:脏", "清洁.状态", "脏"},
		{"身体状态=疲惫", "状态.身体", "疲惫"},
		{"情绪=开心", "状态.情绪", "开心"},
	}
	for _, item := range cases {
		assignments := ParseAssign(item.text)
		if len(assignments) != 1 || assignments[0].Path != item.path || assignments[0].Value != item.any {
			t.Errorf("解析 %q 错误: %+v", item.text, assignments)
		}
	}
	nested := []struct {
		text string
		path string
		any  any
	}{
		{"状态.身体=疲惫", "状态.身体", "疲惫"},
		{"状态.情绪:难过", "状态.情绪", "难过"},
		{"状态.身体 = 疲惫", "状态.身体", "疲惫"},
	}
	for _, item := range nested {
		assignments := ParseAssign(item.text)
		if len(assignments) != 1 || assignments[0].Path != item.path || assignments[0].Value != item.any {
			t.Errorf("嵌套路径解析 %q 错误: %+v", item.text, assignments)
		}
	}
	if got := ParseAssign(""); len(got) != 0 {
		t.Errorf("空文本应无赋值: %+v", got)
	}
	if got := ParseAssign("无等号也无冒号"); len(got) != 0 {
		t.Errorf("无运算符应无赋值: %+v", got)
	}
}

func TestAssignGreedyFix(t *testing.T) {
	mixed := ParseAssign("安全等级=red 撒娇+15")
	if len(mixed) != 1 || mixed[0].Path != "行为.安全等级" || mixed[0].Value != "red" {
		t.Errorf("赋值值不应吞掉后续效果: %+v", mixed)
	}
	expectEffect(t, ParseEffects("安全等级=red 撒娇+15"), "关系.撒娇欲", 15)
	spaced := ParseAssign("安全等级 = orange")
	if len(spaced) != 1 || spaced[0].Value != "orange" {
		t.Errorf("带空格赋值错误: %+v", spaced)
	}
	multi := ParseAssign("安全等级=red,状态.身体=疲惫 撒娇+15")
	if len(multi) != 2 || multi[0].Value != "red" || multi[1].Value != "疲惫" {
		t.Errorf("多赋值混合错误: %+v", multi)
	}
}

func TestApplyAssign(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"行为": map[string]any{"安全等级": "green"},
		"状态": map[string]any{"身体": "活跃"},
	})
	applied := ApplyAssign(ent, "安全等级=red,状态.身体=疲惫")
	if len(applied) != 2 {
		t.Fatalf("应应用 2 条赋值, 实际 %+v", applied)
	}
	if got := ent.GetOrNone("行为.安全等级"); got != "red" {
		t.Errorf("安全等级应为 red, 实际 %v", got)
	}
	if got := ent.GetOrNone("状态.身体"); got != "疲惫" {
		t.Errorf("身体状态应为 疲惫, 实际 %v", got)
	}
}

func TestOrganStateMappingDictWrappers(t *testing.T) {
	ent := newEffectEntity(t, map[string]any{
		"器官状态": map[string]any{
			"基础生理": map[string]any{
				"湿润度":  map[string]any{"value": 80.0, "unit": "%", "level": "湿润"},
				"快感程度": map[string]any{"value": 20.0, "unit": "%", "level": "轻微"},
			},
			"扩展心理":   map[string]any{},
			"R18G伤害": map[string]any{},
		},
		"伤害": map[string]any{"出血": 5.0, "创伤": 10.0},
		"状态": map[string]any{"恐惧": 20.0},
		"行为": map[string]any{"服从": 5.0},
	})
	ApplyEffects(ent, "湿润+10")
	wet := ent.GetOrNone("器官状态.基础生理.湿润度").(map[string]any)
	if number, _ := toFloatValue(wet["value"]); number != 90 {
		t.Errorf("湿润度 value 应为 90, 实际 %v", wet["value"])
	}
	if wet["unit"] != "%" {
		t.Errorf("湿润度 unit 应保留 %%, 实际 %v", wet["unit"])
	}
	ApplyEffects(ent, "快感+5")
	pleasure := ent.GetOrNone("器官状态.基础生理.快感程度").(map[string]any)
	if number, _ := toFloatValue(pleasure["value"]); number != 25 {
		t.Errorf("快感程度 value 应为 25, 实际 %v", pleasure["value"])
	}
	ApplyEffects(ent, "湿润-999")
	wet = ent.GetOrNone("器官状态.基础生理.湿润度").(map[string]any)
	if number, _ := toFloatValue(wet["value"]); number != 0 {
		t.Errorf("湿润度 value 应限幅到 0, 实际 %v", wet["value"])
	}
}

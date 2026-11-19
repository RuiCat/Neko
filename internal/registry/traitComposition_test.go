// 回归：修饰模板模型（角色基础 + 若干修饰）。
//
// 模型要点：
//
//	`角色基础` 组合全部内核，自身不带修饰时就是人族；
//	种族 / 生命阶段都是**修饰模板**（猫娘、幼年…），只声明自己的标识字段，
//	由同名规则把状态写到自身——它们不依赖人族基类，只要求自身满足 角色基础 的字段定义；
//	修饰可任意叠加（角色基础:猫娘:幼年），互不覆盖对方字段。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

func TestTraitComposition(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	// NPC 这个名字必须彻底消失，避免两种叫法并存。
	if store.HasTemplate("NPC") {
		t.Error("NPC 模板仍存在，应已全部替换为 角色基础")
	}
	// 五种族必须结构对称：种族类型 + 对应幼年类型都要存在。
	races := []string{"人族", "猫娘", "犬娘", "狐娘", "精灵"}
	if !store.HasTemplate("角色基础") {
		t.Error("模板 角色基础 缺失")
	}
	for _, race := range races {
		for _, name := range []string{"角色基础:" + race, "角色基础:" + race + ":幼年"} {
			if !store.HasTemplate(name) {
				t.Errorf("模板 %s 缺失（种族 %s 不对称）", name, race)
			}
		}
	}
	// 修饰模板自身必须存在（组合语法引用它们）。
	for _, name := range []string{"人族", "猫娘", "犬娘", "狐娘", "精灵", "幼年", "成年", "猫耳", "猫尾", "肉垫"} {
		if !store.HasTemplate(name) {
			t.Errorf("修饰模板 %s 缺失", name)
		}
	}

	type tc struct {
		id, typ    string
		wantRace   string
		wantFrom   string
		wantStage  string
		wantTraits int
		wantPregWk int
	}
	cases := []tc{
		// 裸 角色基础：没有任何修饰 → 人族默认。
		{"裸基础", "角色基础", "人族", "default", "成年", 0, 40},
		// 显式人族修饰。
		{"显式人族", "角色基础:人族", "人族", "trait", "成年", 0, 40},
		// 五种族对称：每个种族修饰都把自身状态写到同一组字段。
		{"猫娘", "角色基础:猫娘", "猫娘", "trait", "成年", 3, 40},
		{"犬娘", "角色基础:犬娘", "犬娘", "trait", "成年", 3, 9},
		{"狐娘", "角色基础:狐娘", "狐娘", "trait", "成年", 3, 7},
		{"精灵", "角色基础:精灵", "精灵", "trait", "成年", 2, 60},
		// 种族 + 阶段叠加：阶段不改变种族，种族也不清掉阶段。
		{"幼年猫娘", "角色基础:猫娘:幼年", "猫娘", "trait", "幼年", 3, 40},
		{"幼年人族", "角色基础:人族:幼年", "人族", "trait", "幼年", 0, 40},
		{"幼年犬娘", "角色基础:犬娘:幼年", "犬娘", "trait", "幼年", 3, 9},
		{"幼年狐娘", "角色基础:狐娘:幼年", "狐娘", "trait", "幼年", 3, 7},
		{"幼年精灵", "角色基础:精灵:幼年", "精灵", "trait", "幼年", 2, 60},
		// 只加外形特征、不加种族修饰：种族走默认人族。
		{"只加猫耳", "角色基础:猫耳", "人族", "default", "成年", 0, 40},
	}

	for _, c := range cases {
		if _, err := store.Create(c.id, c.typ, map[string]any{
			"基础": map[string]any{"名字": c.id, "性别": "女", "年龄": 20},
		}, nil); err != nil {
			t.Fatalf("create %s (%s): %v", c.id, c.typ, err)
		}
	}
	machine.Tick("")

	for _, c := range cases {
		ent := store.Get(c.id)
		if got := ent.GetOrNone("基础.种族"); got != c.wantRace {
			t.Errorf("%s (%s) 种族 = %v, want %s", c.id, c.typ, got, c.wantRace)
		}
		if got := ent.GetOrNone("基础.种族来源"); got != c.wantFrom {
			t.Errorf("%s (%s) 种族来源 = %v, want %s", c.id, c.typ, got, c.wantFrom)
		}
		if got := ent.GetOrNone("基础.生命阶段"); got != c.wantStage {
			t.Errorf("%s (%s) 生命阶段 = %v, want %s", c.id, c.typ, got, c.wantStage)
		}
		traits, _ := ent.GetOrNone("基础.特性").([]any)
		if len(traits) != c.wantTraits {
			t.Errorf("%s (%s) 特性数 = %d, want %d (%v)", c.id, c.typ, len(traits), c.wantTraits, traits)
		}
		if got := asIntValue(t, ent, "种族参数.孕期周"); got != c.wantPregWk {
			t.Errorf("%s (%s) 孕期周 = %d, want %d", c.id, c.typ, got, c.wantPregWk)
		}
	}

	// 种族修饰必须真正影响派生属性（否则说明规则没读到修饰写入的数据）。
	catAgility := asIntValue(t, store.Get("猫娘"), "基础.属性.敏捷")
	humanAgility := asIntValue(t, store.Get("显式人族"), "基础.属性.敏捷")
	if catAgility <= humanAgility {
		t.Errorf("猫娘敏捷 %d 应高于人族 %d", catAgility, humanAgility)
	}

	// 阶段修饰不应影响种族与种族参数。
	if got := asIntValue(t, store.Get("幼年猫娘"), "种族参数.周期天"); got != 28 {
		t.Errorf("幼年修饰改动了周期天: %d", got)
	}

	// 选择器：$[角色基础] 覆盖全部角色（没有并存的 NPC 分支了）。
	sel := selectorOf(t, "角色基础[*]")
	if got := len(store.QuerySelector(sel)); got != len(store.ListAll()) {
		t.Errorf("$[角色基础] 命中 %d，期望全部 %d", got, len(store.ListAll()))
	}
}

func asIntValue(t *testing.T, ent *entity.Entity, path string) int {
	t.Helper()
	value, err := ent.Get(path)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	}
	t.Fatalf("%s = %T", path, value)
	return 0
}

func selectorOf(t *testing.T, ref string) entity.Selector {
	t.Helper()
	sel, err := entity.ParseEntityRef(ref)
	if err != nil {
		t.Fatalf("parse %s: %v", ref, err)
	}
	return sel
}

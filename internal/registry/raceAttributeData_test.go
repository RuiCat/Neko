// 回归：种族属性修正由数据驱动（P1）。
// 旧版 baseFunctions 里有 35 处 `if 基础.种族 == "猫娘"` 的 if 链，加一个种族要改 7 个函数。
// 现在修正值写在 种族参数.属性修正.*（baseInitTick 按种族写入），属性函数只读字段。
// 本测试锁定「数据驱动的结果与原 if 链完全一致」，防止重构时数值漂移。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

func TestRaceAttributesFromData(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	attrs := []string{"体质", "感知", "敏捷", "智力", "魅力", "意志", "幸运"}

	cases := []struct {
		id   string
		typ  string
		race string
		age  int
		want []int
		note string
	}{
		// 成年：种族修正 + 特征加成（猫耳 50/10=5、猫尾 10/2=5）。
		{"猫", "角色基础:猫娘", "猫娘", 25, []int{45, 65, 70, 50, 60, 50, 50}, "猫耳/猫尾加成"},
		{"犬", "角色基础:犬娘", "犬娘", 25, []int{55, 67, 62, 50, 55, 55, 50}, "犬耳 55/10、犬尾 8/2"},
		{"狐", "角色基础:狐娘", "狐娘", 25, []int{50, 63, 68, 55, 62, 50, 55}, "狐耳 55/10、狐尾 12/2"},
		{"人", "角色基础:人族", "人族", 25, []int{50, 50, 50, 50, 50, 50, 50}, "无修正基准"},
		// 精灵成熟年龄 40：25 岁仍算未成年，吃 −10 年龄惩罚。
		{"精", "角色基础:精灵", "精灵", 25, []int{32, 75, 50, 50, 70, 45, 55}, "尖耳 15 + 未成年（体质/敏捷/智力 −10、魅力 +5）"},
	}

	for _, tc := range cases {
		if _, err := store.Create(tc.id, tc.typ, map[string]any{
			"基础": map[string]any{"名字": tc.id, "性别": "女", "年龄": tc.age, "种族": tc.race},
		}, nil); err != nil {
			t.Fatalf("create %s: %v", tc.id, err)
		}
		machine.Tick(tc.id)
		ent := store.Get(tc.id)
		for i, key := range attrs {
			value, err := ent.Get("基础.属性." + key)
			if err != nil {
				t.Fatalf("%s get %s: %v", tc.id, key, err)
			}
			got, ok := value.(int)
			if !ok {
				t.Fatalf("%s %s = %T", tc.id, key, value)
			}
			if got != tc.want[i] {
				t.Errorf("%s %s = %d, want %d (%s)", tc.id, key, got, tc.want[i], tc.note)
			}
		}
		// 未初始化种族的实体应保持 0 修正（模板默认），确认不是靠 if 链兜底。
		if tc.race == "人族" {
			raw, _ := ent.Get("种族参数.属性修正.体质")
			var got float64
			switch typed := raw.(type) {
			case int:
				got = float64(typed)
			case int64:
				got = float64(typed)
			case float64:
				got = typed
			default:
				t.Fatalf("人族 属性修正.体质 = %T (%v)", raw, raw)
			}
			if got != 0 {
				t.Errorf("人族 属性修正.体质 = %v, want 0", raw)
			}
		}
	}
}

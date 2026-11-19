// 回归：水平衡口径 + MODS 语义分离（C+D+E）。
// 旧版尿量未按分钟折算（800/tick ≈ 19 L/天），体液单向下滑、灌注被拖到 47，
// 多器官衰竭作为「100 − 灌注」的快照被被动推高。本测试把三件事钉住：
// 1）不喝水的健康实体在 12 个 60 分钟 tick 内体液不崩、MODS 保持 0；
// 2）没有真实驱动（器官应力/感染/休克/灌注<40）时 MODS 每 tick −1 回落；
// 3）出现结构性器官应力后 MODS 开始累积。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

func TestWaterBalanceAndModsSemantics(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	if _, err := store.Create("猫猫", "角色基础", map[string]any{"基础": map[string]any{"名字": "猫猫", "年龄": 18}}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	ent := store.Get("猫猫")
	set := func(path string, value any) {
		t.Helper()
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	asInt := func(path string) int {
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
	tick := func(count int) {
		t.Helper()
		for i := 0; i < count; i++ {
			set("代谢.流逝分钟", 60)
			machine.Tick("")
		}
	}

	// 1) 12 小时不喝水：旧口径会掉到 32000（<40000），新口径应稳在 41000 以上。
	tick(12)
	if fluid := asInt("代谢.体液"); fluid < 40000 {
		t.Fatalf("体液 dehydration too fast: %d, want >=40000 (尿量未按分钟折算?)", fluid)
	}
	if mods := asInt("伤害.多器官衰竭"); mods != 0 {
		t.Fatalf("MODS rose without any injury driver: %d, want 0", mods)
	}
	if sodium := asInt("代谢.血钠"); sodium != 140 {
		t.Fatalf("血钠 drifted without osmotic deviation: %d, want 140", sodium)
	}

	// 2) 无驱动 → 每 tick −1 回落。
	set("伤害.多器官衰竭", 30)
	tick(2)
	if mods := asInt("伤害.多器官衰竭"); mods != 28 {
		t.Fatalf("MODS recovery = %d, want 28 (每 tick −1)", mods)
	}

	// 3) 结构性器官应力（肛路径应力 85 → 直肠屏障）→ MODS 累积。
	set("伤害.多器官衰竭", 0)
	set("路径.类型", "肛")
	set("路径.应力", 85)
	tick(2)
	if mods := asInt("伤害.多器官衰竭"); mods < 1 {
		t.Fatalf("MODS did not accumulate under organ stress: %d", mods)
	}
}

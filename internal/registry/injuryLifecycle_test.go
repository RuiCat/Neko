// 回归：M4 路径破裂→污染→脓毒；M5 失血→休克→死亡（脚本内核验收）。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

// TestPathSepsisAndShockDeathScripts pins two script-side loops:
// a breached route accumulates contamination and fires 脓毒 at 感染>=60;
// sustained bleeding raises 休克 to 100 and flips 死亡.
func TestPathSepsisAndShockDeathScripts(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	create := func(id string) *entity.Entity {
		t.Helper()
		if _, err := store.Create(id, "角色基础", map[string]any{"基础": map[string]any{"名字": id, "年龄": 20}}, nil); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
		return store.Get(id)
	}
	set := func(ent *entity.Entity, path string, value any) {
		t.Helper()
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	get := func(ent *entity.Entity, path string) any {
		t.Helper()
		value, err := ent.Get(path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		return value
	}
	asInt := func(value any) int {
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		}
		return -1
	}

	// M4：创口路径应力超阈值 → 破口 → 污染累积 → 感染 >=60 触发脓毒事件。
	path := create("猫猫")
	set(path, "路径.类型", "创口")
	set(path, "路径.应力", 50)
	set(path, "伤害.出血速率", 0)
	sepsis := false
	for i := 0; i < 30; i++ {
		set(path, "代谢.流逝分钟", 5)
		machine.Tick("")
		if contam := asInt(get(path, "路径.污染")); contam < 10 {
			t.Fatalf("tick %d: contamination = %d, want >=10", i, contam)
		}
		if get(path, "角色状态.最近事件") == "脓毒" {
			sepsis = true
			break
		}
	}
	if !sepsis {
		t.Fatalf("sepsis not triggered: 感染=%v 污染=%v 破口=%v",
			get(path, "伤害.感染"), get(path, "路径.污染"), get(path, "路径.破口"))
	}
	if breached, _ := get(path, "路径.破口").(bool); !breached {
		t.Fatalf("path not breached: 应力=%v 阈值=%v", get(path, "路径.应力"), get(path, "路径.屏障阈值"))
	}

	// M5：持续出血 → 休克分级上升 → 休克 100 → 死亡。
	bleed := create("梦梦")
	set(bleed, "伤害.出血速率", 40)
	set(bleed, "代谢.流逝分钟", 60)
	machine.Tick("")
	if shock := asInt(get(bleed, "伤害.休克")); shock != 50 {
		t.Fatalf("first tick shock = %d, want 50", shock)
	}
	if dead, _ := get(bleed, "伤害.死亡").(bool); dead {
		t.Fatalf("died too early: 失血=%v", get(bleed, "伤害.失血量"))
	}
	set(bleed, "代谢.流逝分钟", 60)
	machine.Tick("")
	if shock := asInt(get(bleed, "伤害.休克")); shock != 100 {
		t.Fatalf("second tick shock = %d, want 100", shock)
	}
	if dead, _ := get(bleed, "伤害.死亡").(bool); !dead {
		t.Fatalf("death not set: 失血=%v 休克=%v 灌注=%v",
			get(bleed, "伤害.失血量"), get(bleed, "伤害.休克"), get(bleed, "循环.灌注"))
	}
}

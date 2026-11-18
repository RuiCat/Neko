// 回归：脚本生殖内核的受孕→妊娠→分娩闭环（M3 验收）。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

// TestPregnancyLifecycleScripts pins the script-side reproductive loop:
// conception -> 40 weeks of pregnancy -> postpartum delivery bleeding and
// energy demand, with no unbounded state machine.
func TestPregnancyLifecycleScripts(t *testing.T) {
	dataDir := t.TempDir()
	store := entity.NewStore(dataDir)
	machine := engine.New(store)
	funcs := function.New(store)
	ev := events.NewSystem(store, funcs)
	machine.SetFuncCaller(funcs)
	loadRepoScripts(t, store, machine, funcs, ev, dataDir)

	if _, err := store.Create("猫猫", "NPC", map[string]any{"基础": map[string]any{"名字": "猫猫", "年龄": 20}}, nil); err != nil {
		t.Fatalf("create: %v", err)
	}
	ent := store.Get("猫猫")
	set := func(path string, value any) {
		t.Helper()
		if err := ent.Set(path, value); err != nil {
			t.Fatalf("set %s: %v", path, err)
		}
	}
	get := func(path string) any {
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

	set("生殖.受孕概率", 100)
	set("生殖.受孕窗口", true)
	set("生殖.避孕", false)
	set("生殖.怀孕状态", "未孕")
	set("行为.体内暴露", true)
	set("基础.可孕", true)

	// 受孕判定由 @函数 收口（规则条件不能直接调随机）；这里直接验证谓词。
	set("生殖.受孕概率", 100)
	set("生殖.受孕窗口", true)
	set("生殖.避孕", false)
	set("生殖.怀孕状态", "未孕")
	set("行为.体内暴露", true)
	set("基础.可孕", true)
	hit, err := funcs.Call("生殖受孕命中", nil, ent)
	if err != nil {
		t.Fatalf("conception call: %v", err)
	}
	if ok, _ := hit.(bool); !ok {
		t.Fatalf("conception predicate = %v, want true", hit)
	}
	// 进入早孕后按周推进；初始化字段模拟规则命中后的状态。
	set("生殖.怀孕状态", "早孕")
	set("生殖.孕周", 0)
	set("生殖.孕期分钟", 0)

	// 每周一个 tick，推进到产后；记录经过的状态与孕周。
	states := map[string]bool{"早孕": true}
	energyMid := 0
	volumeLate := 0
	delivered := false
	deliveryBleed := -1
	for week := 0; week < 45 && !delivered; week++ {
		set("代谢.流逝分钟", 10080)
		machine.Tick("")
		state, _ := get("生殖.怀孕状态").(string)
		states[state] = true
		switch state {
		case "中孕":
			energyMid = asInt(get("生殖.能量需求"))
		case "晚孕":
			volumeLate = asInt(get("生殖.血容量系数"))
		case "产后":
			delivered = true
			deliveryBleed = asInt(get("伤害.出血速率"))
		}
	}
	if !delivered {
		t.Fatalf("no delivery within 45 weeks: states=%v week=%v", states, get("生殖.孕周"))
	}
	if week := asInt(get("生殖.孕周")); week < 40 || week > 42 {
		t.Fatalf("delivered at week %d, want 40..42", week)
	}
	if !states["中孕"] || !states["晚孕"] {
		t.Fatalf("pregnancy stages missing: %v", states)
	}
	if energyMid != 340 {
		t.Fatalf("mid-pregnancy energy demand = %d, want 340", energyMid)
	}
	if volumeLate < 130 {
		t.Fatalf("late-pregnancy blood volume factor = %d, want >=130", volumeLate)
	}
	if deliveryBleed != 20 {
		t.Fatalf("delivery bleed rate = %d, want 20", deliveryBleed)
	}
	if settled, _ := get("生殖.分娩已结算").(bool); !settled {
		t.Fatalf("delivery settlement flag = %v, want true", get("生殖.分娩已结算"))
	}
}

// 回归：伤口愈合由 .neko 伤害内核结算（污染折减、严重度降速、关闭清零）。
package registry

import (
	"testing"

	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/function"
)

// TestWoundHealingScripts pins the script-side wound settlement: 25%/day at
// severity 50, contamination slows it, and a full progress closes the wound.
func TestWoundHealingScripts(t *testing.T) {
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
	tick := func() {
		t.Helper()
		set("代谢.流逝分钟", 1440)
		machine.Tick("")
	}
	getInt := func(path string) int {
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
		default:
			t.Fatalf("%s = %T", path, value)
			return 0
		}
	}

	set("伤害.伤口.伤口数", 1)
	set("伤害.伤口.严重度", 50)
	set("伤害.伤口.污染", 0)
	set("伤害.伤口.愈合进度", 0)

	tick()
	if got := getInt("伤害.伤口.愈合进度"); got != 25 {
		t.Fatalf("day1 progress = %d, want 25", got)
	}
	tick()
	if got := getInt("伤害.伤口.愈合进度"); got != 50 {
		t.Fatalf("day2 progress = %d, want 50", got)
	}
	tick()
	tick()
	if got := getInt("伤害.伤口.伤口数"); got != 0 {
		t.Fatalf("wound not closed after 4 days: count=%d progress=%d", got, getInt("伤害.伤口.愈合进度"))
	}

	// 污染 100 → 速率折半（12%/天）；严重度 80 → 基准 15%/天。
	set("伤害.伤口.伤口数", 1)
	set("伤害.伤口.严重度", 50)
	set("伤害.伤口.污染", 100)
	set("伤害.伤口.愈合进度", 0)
	tick()
	if got := getInt("伤害.伤口.愈合进度"); got != 12 {
		t.Fatalf("contaminated progress = %d, want 12", got)
	}
	set("伤害.伤口.伤口数", 1)
	set("伤害.伤口.严重度", 80)
	set("伤害.伤口.污染", 0)
	set("伤害.伤口.愈合进度", 0)
	tick()
	if got := getInt("伤害.伤口.愈合进度"); got != 15 {
		t.Fatalf("severe wound progress = %d, want 15", got)
	}
}

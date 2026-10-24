package engine

import (
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/entity"
)

func TestConfiguredStateGroup(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状况", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	dir := t.TempDir()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	ent, err := store.Create("猫猫", "NPC", map[string]any{
		"状况":   map[string]any{"身体": "活跃"},
		"状态定义": map[string]any{"身体": []any{"活跃", "疲惫", "昏厥"}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	machine.SetDecisionLog(filepath.Join(dir, "llm_decisions.json"))
	empty := ""
	machine.AddRule(Rule{
		ID: "rA", Match: "猫猫", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状况.身体", Value: "疲惫"}}}},
	})
	machine.AddRule(Rule{
		ID: "rB", Match: "猫猫", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状况.身体", Value: "昏厥"}}}},
	})
	tick := machine.Tick("*")
	if len(tick.Conflicts) == 0 {
		t.Fatalf("expected conflict under configured state group, got %+v", tick)
	}
	if !machine.ResolveConflict(tick.Conflicts[0].ConflictID, map[string]any{"身体": "昏厥"}, "test") {
		t.Fatal("resolve failed")
	}
	if got := ent.GetOrNone("状况.身体"); got != "昏厥" {
		t.Fatalf("状况.身体 = %v", got)
	}
}

func TestConfiguredEmitTarget(t *testing.T) {
	if err := config.Set(config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
		GlobalEntities: []string{"基地"}, DefaultScene: "基地",
		SceneEventField: "日志", ClassificationField: "标签",
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)

	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("基地", "场景", map[string]any{"日志": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	empty := ""
	machine.AddRule(Rule{
		ID: "rEmit", Match: "猫猫", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "emit", Value: "信号"}}}},
	})
	machine.Tick("*")
	events, _ := store.Get("基地").GetOrNone("日志").([]any)
	if len(events) != 1 || events[0] != "信号" {
		t.Fatalf("基地.日志 = %v", events)
	}
}

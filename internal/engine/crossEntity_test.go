package engine

import (
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
	nekoFunction "github.com/ruicat/astrbox/internal/function"
	"github.com/ruicat/astrbox/internal/neko"
)

func TestBranchRaceAcrossEntities(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"状态":   map[string]any{"身体": "平静"},
		"状态定义": map[string]any{"身体": []any{"平静", "活跃", "疲惫"}},
	}
	if _, err := store.Create("A", "NPC", state, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("B", "NPC", map[string]any{"状态": map[string]any{"身体": "平静"}}, nil); err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	empty := ""
	machine.AddRule(Rule{
		ID: "rA", Match: "A", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状态.身体", Value: "活跃"}}}},
	})
	machine.AddRule(Rule{
		ID: "rB", Match: "B", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Entity: "A", Path: "状态.身体", Value: "疲惫"}}}},
	})
	result := machine.Tick("*")
	if len(result.Conflicts) == 0 {
		t.Fatalf("expected cross-entity branch_race, got %+v", result.Transitions)
	}
	conflict := result.Conflicts[0]
	if conflict.Type != "branch_race" || conflict.EntityID != "A" {
		t.Fatalf("conflict = %+v", conflict)
	}
}

func TestBranchRaceFromFunctionWrite(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{
		"状态":   map[string]any{"身体": "平静", "触发": ""},
		"状态定义": map[string]any{"身体": []any{"平静", "活跃", "疲惫"}},
	}
	if _, err := store.Create("A", "NPC", state, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("B", "NPC", map[string]any{"状态": map[string]any{"身体": "平静", "触发": ""}}, nil); err != nil {
		t.Fatal(err)
	}
	executor := nekoFunction.New(store)
	if err := executor.Register(neko.FuncDef{
		Name: "转移", Params: []neko.Param{{Name: "目标"}},
		Body: "目标.状态.身体 = \"疲惫\"",
	}); err != nil {
		t.Fatal(err)
	}
	machine := New(store)
	machine.SetFuncCaller(executor)
	empty := ""
	machine.AddRule(Rule{
		ID: "rA", Match: "A", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状态.身体", Value: "活跃"}}}},
	})
	machine.AddRule(Rule{
		ID: "rB", Match: "B", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "状态.触发", Value: `转移("A")`}}}},
	})
	result := machine.Tick("*")
	found := false
	for _, conflict := range result.Conflicts {
		if conflict.Type == "branch_race" && conflict.EntityID == "A" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected branch_race from function write, conflicts=%+v", result.Conflicts)
	}
}

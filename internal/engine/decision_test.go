package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func TestResolveConflictLogsAndBackfills(t *testing.T) {
	dir := t.TempDir()
	store := entity.NewStore(dir)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	ent, err := store.Create("猫猫", "NPC", map[string]any{
		"状态":   map[string]any{"身体": "活跃"},
		"状态定义": map[string]any{"身体": []any{"活跃", "疲惫", "昏厥"}},
	}, nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	machine := New(store)
	decisionPath := filepath.Join(dir, "llm_decisions.json")
	machine.SetDecisionLog(decisionPath)

	empty := ""
	machine.AddRule(Rule{
		ID: "rA", Match: "猫猫", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状态.身体", Value: "疲惫"}}}},
	})
	machine.AddRule(Rule{
		ID: "rB", Match: "猫猫", Enabled: true,
		Branches: []Branch{{If: &empty, Then: []Op{{Op: "set", Path: "{e}.状态.身体", Value: "昏厥"}}}},
	})

	tick := machine.Tick("*")
	if len(tick.Conflicts) == 0 {
		t.Fatalf("expected a conflict, got %+v", tick)
	}
	conflict := tick.Conflicts[0]
	resolved := machine.ResolveConflictWithEffects(conflict.ConflictID,
		map[string]any{"身体": "昏厥"}, "test reason",
		[]Op{{Op: "set", Path: "{e}.标记.测试", Value: true}})
	if !resolved {
		t.Fatal("resolve failed")
	}

	if got := ent.GetOrNone("状态.身体"); got != "昏厥" {
		t.Fatalf("state = %v", got)
	}
	learned := machine.GetRule("learned_" + conflict.ConflictID)
	if learned == nil {
		t.Fatal("learned rule not back-filled")
	}
	if learned.Source != "llm_backfill" || learned.Priority < 100 {
		t.Fatalf("learned rule = %+v", learned)
	}
	raw, err := os.ReadFile(decisionPath)
	if err != nil {
		t.Fatalf("decision log: %v", err)
	}
	var records []map[string]any
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatalf("decision log is not json: %v", err)
	}
	if len(records) != 1 || records[0]["conflict_id"] != conflict.ConflictID {
		t.Fatalf("records = %v", records)
	}
	if !strings.Contains(rawGet(records), "test reason") {
		t.Fatalf("reason missing: %s", raw)
	}
}

func rawGet(records []map[string]any) string {
	raw, _ := json.Marshal(records)
	return string(raw)
}

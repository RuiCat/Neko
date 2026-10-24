// Integration test against the real data/entities.json snapshot.
//
// It proves the engine works on production-shaped data and that Tick never
// writes entities.json back to disk (state changes stay in memory).
package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruicat/astrbox/internal/entity"
)

func TestIntegrationRealEntities(t *testing.T) {
	dataDir := filepath.Join("..", "..", "data")
	entitiesPath := filepath.Join(dataDir, "entities.json")
	before, err := os.ReadFile(entitiesPath)
	if err != nil {
		t.Fatalf("读取真实数据失败: %v", err)
	}

	store := entity.NewStore(dataDir)
	if err := store.Load(); err != nil {
		t.Fatalf("载入真实数据失败: %v", err)
	}
	cat := store.Get("猫猫")
	if cat == nil {
		t.Fatal("真实数据里缺少实体 猫猫")
	}
	if err := cat.Set("身体.体力", 15.0); err != nil {
		t.Fatalf("设置体力失败: %v", err)
	}

	machine := New(store)
	machine.AddRule(Rule{
		ID:       "r_real_fatigue",
		Desc:     "体力<20 → 状态.身体=疲惫",
		Match:    "NPC[*]",
		Priority: 10,
		Enabled:  true,
		Branches: []Branch{
			{
				If:    stringPtr("{e}.身体.体力 < 20"),
				State: map[string]any{"身体": "疲惫"},
			},
			{Else: true},
		},
	})

	result := machine.Tick("NPC[*]")
	if result.Status != "ok" {
		t.Errorf("status = %s, 期望 ok (errors=%v)", result.Status, result.Errors)
	}
	if got := cat.GetOrNone("状态.身体"); got != "疲惫" {
		t.Errorf("猫猫.状态.身体 = %v, 期望 疲惫", got)
	}
	if len(result.Transitions) != 1 {
		t.Fatalf("应只有猫猫一个转移, 实际 %d: %+v", len(result.Transitions), result.Transitions)
	}
	transition := result.Transitions[0]
	if transition.EntityID != "猫猫" || transition.RuleID != "r_real_fatigue" {
		t.Errorf("转移记录错误: %+v", transition)
	}
	if transition.FromState["身体"] != "活跃" || transition.ToState["身体"] != "疲惫" {
		t.Errorf("转移前后状态错误: from=%+v to=%+v", transition.FromState, transition.ToState)
	}
	if len(transition.Operations) != 1 {
		t.Fatalf("应记录一个 set 操作: %+v", transition.Operations)
	}
	operation := transition.Operations[0]
	if operation["op"] != "set" || operation["path"] != "状态.身体" || operation["value"] != "疲惫" {
		t.Errorf("操作记录错误: %+v", operation)
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("TickResult 序列化失败: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("TickResult 反序列化失败: %v", err)
	}
	if tickID, ok := payload["tick_id"].(string); !ok || tickID == "" {
		t.Errorf("JSON tick_id 缺失或为空: %s", raw)
	}
	if _, ok := payload["hint"].(string); !ok {
		t.Errorf("JSON hint 缺失: %s", raw)
	}

	after, err := os.ReadFile(entitiesPath)
	if err != nil {
		t.Fatalf("回读真实数据失败: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("Tick 不应改写 data/entities.json")
	}
}

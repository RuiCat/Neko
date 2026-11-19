// sceneOps_test.go covers scene-target resolution (default-scene token ->
// actor's current scene) and the bounded scene event stream.
package engine

import (
	"testing"

	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/entity"
)

// newSceneStore builds a store with one NPC in scene S001-LR plus that scene.
func newSceneStore(t *testing.T) *entity.Store {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	store.AddTemplate("场景", map[string]any{"事件": []any{}, "连接": []any{}})
	if _, err := store.Create("S001-LR", "场景", map[string]any{"事件": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "角色基础", map[string]any{
		"位置": map[string]any{"场景": "S001-LR"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return store
}

// sceneEvents reads one scene's event list.
func sceneEvents(t *testing.T, store *entity.Store, sceneID string) []any {
	t.Helper()
	ent := store.Get(sceneID)
	if ent == nil {
		t.Fatalf("场景 %s 不存在", sceneID)
	}
	events, _ := ent.GetOrNone("事件").([]any)
	return events
}

// applySceneOp runs one unconditional rule action against 猫猫.
func applySceneOp(t *testing.T, store *entity.Store, op Op) TickResult {
	t.Helper()
	machine := New(store)
	machine.AddRule(Rule{
		ID: "r_scene", Match: "猫猫", Enabled: true,
		Branches: []Branch{{Else: true, Then: []Op{op}}},
	})
	result := machine.Tick("猫猫")
	if result.Status != "ok" {
		t.Fatalf("tick status = %s", result.Status)
	}
	return result
}

// findWrite returns the first observed write for one entity/path pair.
func findWrite(result TickResult, entityID, path string) *EntityWrite {
	for index := range result.Writes {
		write := &result.Writes[index]
		if write.EntityID == entityID && write.Path == path {
			return write
		}
	}
	return nil
}

// TestAppendDefaultSceneWritesCurrentScene covers `场景.事件 += ...`: the token
// is not an entity, so it resolves to the actor's current scene and the tick
// reports the write with both list values (the board needs them).
func TestAppendDefaultSceneWritesCurrentScene(t *testing.T) {
	store := newSceneStore(t)
	result := applySceneOp(t, store, Op{Op: "append", Entity: "场景", Path: "事件", Value: "进入客厅"})
	if got := sceneEvents(t, store, "S001-LR"); len(got) != 1 || got[0] != "进入客厅" {
		t.Fatalf("场景事件 = %#v, want [进入客厅]", got)
	}
	write := findWrite(result, "S001-LR", "事件")
	if write == nil {
		t.Fatalf("缺少场景事件写入记录：%+v", result.Writes)
	}
	if from, _ := write.From.([]any); len(from) != 0 {
		t.Fatalf("write.From = %#v, want empty list", write.From)
	}
	if to, _ := write.To.([]any); len(to) != 1 || to[0] != "进入客厅" {
		t.Fatalf("write.To = %#v, want [进入客厅]", write.To)
	}
}

// TestEmitWithoutSceneWritesCurrentScene covers `emit` with no explicit scene.
func TestEmitWithoutSceneWritesCurrentScene(t *testing.T) {
	store := newSceneStore(t)
	applySceneOp(t, store, Op{Op: "emit", Value: "远处打雷"})
	if got := sceneEvents(t, store, "S001-LR"); len(got) != 1 || got[0] != "远处打雷" {
		t.Fatalf("场景事件 = %#v, want [远处打雷]", got)
	}
}

// TestEmitDefaultSceneTokenWritesCurrentScene covers the compiler's emit form,
// which always carries the configured default scene as the explicit entity.
func TestEmitDefaultSceneTokenWritesCurrentScene(t *testing.T) {
	store := newSceneStore(t)
	applySceneOp(t, store, Op{Op: "emit", Entity: config.Default().DefaultScene, Value: "窗外下雨"})
	if got := sceneEvents(t, store, "S001-LR"); len(got) != 1 || got[0] != "窗外下雨" {
		t.Fatalf("场景事件 = %#v, want [窗外下雨]", got)
	}
}

// TestRealSceneEntityWinsOverCurrentScene keeps the old meaning: a real entity
// named 场景 is still the target, so existing data/scripts keep working.
func TestRealSceneEntityWinsOverCurrentScene(t *testing.T) {
	store := newSceneStore(t)
	if _, err := store.Create("场景", "场景", map[string]any{"事件": []any{}}, nil); err != nil {
		t.Fatal(err)
	}
	applySceneOp(t, store, Op{Op: "append", Entity: "场景", Path: "事件", Value: "全局事件"})
	if got := sceneEvents(t, store, "场景"); len(got) != 1 || got[0] != "全局事件" {
		t.Fatalf("全局场景事件 = %#v, want [全局事件]", got)
	}
	if got := sceneEvents(t, store, "S001-LR"); len(got) != 0 {
		t.Fatalf("当前场景不应收到写入：%#v", got)
	}
}

// TestSceneEventStreamIsTrimmedToLimit covers scene_event_limit: only the
// newest entries survive, so the stream and its pushes stay bounded.
func TestSceneEventStreamIsTrimmedToLimit(t *testing.T) {
	bounded := config.Config{
		StateGroup: "状态", StateDefinitionField: "状态定义", DefaultStateSubgroup: "身体",
		DefaultScene: "场景", SceneEventField: "事件", SceneEventLimit: 2,
	}
	if err := config.Set(bounded); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.Reset)
	store := newSceneStore(t)
	for _, event := range []string{"一", "二", "三"} {
		applySceneOp(t, store, Op{Op: "append", Entity: "场景", Path: "事件", Value: event})
	}
	got := sceneEvents(t, store, "S001-LR")
	if len(got) != 2 || got[0] != "二" || got[1] != "三" {
		t.Fatalf("场景事件 = %#v, want [二 三]", got)
	}
}

// TestSceneOpWithoutCurrentSceneIsSkipped covers an NPC with no position: the
// op is a no-op instead of a panic or a write to a random entity.
func TestSceneOpWithoutCurrentSceneIsSkipped(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "角色基础", map[string]any{"状态": map[string]any{"触发": ""}}, nil); err != nil {
		t.Fatal(err)
	}
	result := applySceneOp(t, store, Op{Op: "append", Entity: "场景", Path: "事件", Value: "孤儿"})
	if len(result.Writes) != 0 {
		t.Fatalf("writes = %+v, want none", result.Writes)
	}
	if scene := store.Get("S001-LR"); scene != nil {
		t.Fatalf("不应凭空创建场景实体：%+v", scene)
	}
}

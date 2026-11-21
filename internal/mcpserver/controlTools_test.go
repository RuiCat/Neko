// controlTools_test.go covers the K20 MCP control tools: batch state writes,
// entity delete, the event board, the world clock and the operational tools.
package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/control/ops"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/observation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// callToolText invokes one tool and returns its text content.
func callToolText(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s returned no content", name)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s content = %T", name, result.Content[0])
	}
	return text.Text
}

// callToolJSON invokes one tool and decodes its JSON success body.
func callToolJSON(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) map[string]any {
	t.Helper()
	text := callToolText(t, session, name, arguments)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("%s did not answer JSON: %v (%s)", name, err, text)
	}
	return payload
}

// TestEntityStateSetBatch covers the values form and the backward-compatible
// single path/value form.
func TestEntityStateSetBatch(t *testing.T) {
	fx := newTickFixture(t)
	session, cancel := connect(t, fx.server)
	defer cancel()

	batch := callToolJSON(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫",
		"values": map[string]any{"状态.情绪": "开心", "身体.体力": 90},
	})
	if batch["ok"] != true || batch["entity"] != "猫猫" {
		t.Fatalf("batch = %v", batch)
	}
	paths, ok := batch["paths"].([]any)
	if !ok || len(paths) != 2 || paths[0] != "状态.情绪" || paths[1] != "身体.体力" {
		t.Fatalf("paths = %v", batch["paths"])
	}
	if value, err := fx.entity.Get("状态.情绪"); err != nil || value != "开心" {
		t.Fatalf("情绪 = %v (%v)", value, err)
	}
	if value, err := fx.entity.Get("身体.体力"); err != nil || value != float64(90) {
		t.Fatalf("体力 = %v (%v)", value, err)
	}

	single := callToolJSON(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫", "path": "身体.体力", "value": "77",
	})
	if single["ok"] != true {
		t.Fatalf("single = %v", single)
	}
	if value, err := fx.entity.Get("身体.体力"); err != nil || value != float64(77) {
		t.Fatalf("体力 after single = %v (%v)", value, err)
	}
}

// TestEntityStateSetRejectsBadInput covers unknown entities and mixed forms.
func TestEntityStateSetRejectsBadInput(t *testing.T) {
	fx := newTickFixture(t)
	session, cancel := connect(t, fx.server)
	defer cancel()

	missing := callToolText(t, session, "entity_state_set", map[string]any{
		"entity": "不存在", "path": "状态.情绪", "value": "开心",
	})
	if !strings.HasPrefix(missing, "❌") || !strings.Contains(missing, "不存在") {
		t.Fatalf("unknown entity answer = %q", missing)
	}
	mixed := callToolText(t, session, "entity_state_set", map[string]any{
		"entity": "猫猫", "path": "状态.情绪", "value": "开心",
		"values": map[string]any{"状态.情绪": "开心"},
	})
	if !strings.HasPrefix(mixed, "❌") || !strings.Contains(mixed, "不能同时使用") {
		t.Fatalf("mixed forms answer = %q", mixed)
	}
	noPath := callToolText(t, session, "entity_state_set", map[string]any{"entity": "猫猫"})
	if !strings.HasPrefix(noPath, "❌") {
		t.Fatalf("missing path answer = %q", noPath)
	}
}

// TestEntityDeleteTool covers deleting an entity with its observation stream,
// then a second delete of the same id.
func TestEntityDeleteTool(t *testing.T) {
	fx := newTickFixture(t)
	bus := wireObservations(t, fx)
	if _, err := fx.server.store.Create("月月", "角色基础", map[string]any{"状态": map[string]any{"情绪": "平静"}}, nil); err != nil {
		t.Fatalf("create 月月: %v", err)
	}
	if _, err := fx.server.WriteEntityPaths("月月", map[string]any{"状态.情绪": "开心"}, "test"); err != nil {
		t.Fatalf("write: %v", err)
	}
	session, cancel := connect(t, fx.server)
	defer cancel()

	deleted := callToolJSON(t, session, "entity_delete", map[string]any{"entity": "月月"})
	if deleted["ok"] != true || deleted["deleted"] != "月月" {
		t.Fatalf("delete = %v", deleted)
	}
	if fx.server.store.Get("月月") != nil {
		t.Fatal("entity still in the store")
	}
	if version := bus.Version(observation.ScopeForEntity("月月")); version != 0 {
		t.Fatalf("观察流应随实体删除：version=%d", version)
	}
	again := callToolText(t, session, "entity_delete", map[string]any{"entity": "月月"})
	if !strings.HasPrefix(again, "❌") || !strings.Contains(again, "不存在") {
		t.Fatalf("second delete answer = %q", again)
	}
}

// TestObservationGetTool covers both read modes of the observation tool.
func TestObservationGetTool(t *testing.T) {
	fx := newTickFixture(t)
	wireObservations(t, fx)
	if _, err := fx.server.WriteEntityPaths("猫猫", map[string]any{"状态.情绪": "开心"}, "test"); err != nil {
		t.Fatalf("write: %v", err)
	}
	session, cancel := connect(t, fx.server)
	defer cancel()

	delta := callToolJSON(t, session, "observation_get", map[string]any{"entity": "猫猫"})
	events, ok := delta["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("delta events = %v", delta["events"])
	}
	event, _ := events[0].(map[string]any)
	if event["path"] != "状态.情绪" || event["to"] != "开心" || event["tier"] != "normal" {
		t.Fatalf("event = %v", event)
	}
	if delta["scope"] != "entity:猫猫" || delta["cursor"] != float64(1) {
		t.Fatalf("delta envelope = %v", delta)
	}

	state := callToolJSON(t, session, "observation_get", map[string]any{"entity": "猫猫", "mode": "state"})
	entries, ok := state["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("state entries = %v", state["entries"])
	}
	entry, _ := entries[0].(map[string]any)
	if entry["path"] != "状态.情绪" || entry["value"] != "开心" {
		t.Fatalf("entry = %v", entry)
	}

	missing := callToolText(t, session, "observation_get", map[string]any{})
	if !strings.HasPrefix(missing, "❌") {
		t.Fatalf("missing scope answer = %q", missing)
	}
}

// createWorldEntity seeds the 世界 entity for clock tool tests.
func createWorldEntity(t *testing.T, fx *tickFixture, timestamp string) {
	t.Helper()
	if _, err := fx.server.store.Create("世界", "世界", map[string]any{
		"环境": map[string]any{"当前时间": timestamp, "当前月份": 11},
		"状态": map[string]any{"阶段": "傍晚"},
	}, nil); err != nil {
		t.Fatalf("create world: %v", err)
	}
}

// TestWorldClockTool covers read, advance and set.
func TestWorldClockTool(t *testing.T) {
	fx := newTickFixture(t)
	createWorldEntity(t, fx, "2026-11-07T18:30:00+08:00")
	session, cancel := connect(t, fx.server)
	defer cancel()

	read := callToolJSON(t, session, "world_clock", map[string]any{})
	if read["ok"] != true || read["worldTime"] != "2026-11-07T18:30:00+08:00" {
		t.Fatalf("read = %v", read)
	}
	advanced := callToolJSON(t, session, "world_clock", map[string]any{"minutes": 60})
	if advanced["worldTime"] != "2026-11-07T19:30:00+08:00" || advanced["phase"] != "傍晚" {
		t.Fatalf("advanced = %v", advanced)
	}
	if _, err := os.Stat(filepath.Join(fx.dataDir, "entities.json")); err != nil {
		t.Fatalf("clock not saved: %v", err)
	}
	set := callToolJSON(t, session, "world_clock", map[string]any{"time": "2026-12-25T08:15:00+08:00"})
	// 阶段由脚本在 tick 时派生；工具只回显存储值。
	if set["worldTime"] != "2026-12-25T08:15:00+08:00" || set["month"] != float64(12) || set["phase"] != "傍晚" {
		t.Fatalf("set = %v", set)
	}
}

// TestWorldClockToolRejectsBadInput covers the missing world entity and the
// invalid argument combinations.
func TestWorldClockToolRejectsBadInput(t *testing.T) {
	fx := newTickFixture(t)
	session, cancel := connect(t, fx.server)
	defer cancel()
	if text := callToolText(t, session, "world_clock", map[string]any{}); !strings.HasPrefix(text, "❌") {
		t.Fatalf("missing world answer = %q", text)
	}

	createWorldEntity(t, fx, "2026-11-07T18:30:00+08:00")
	for name, arguments := range map[string]map[string]any{
		"both":   {"minutes": 5, "time": "2026-11-07T18:30:00+08:00"},
		"range":  {"minutes": 2000},
		"format": {"time": "不是时间"},
	} {
		text := callToolText(t, session, "world_clock", arguments)
		if !strings.HasPrefix(text, "❌") {
			t.Fatalf("%s answer = %q", name, text)
		}
	}
}

// TestOpsTools covers the six operational tools end to end on a temp runtime:
// nightly decay, judge -> queue -> resolve, and the apply/rollback failures.
func TestOpsTools(t *testing.T) {
	fx := newTickFixture(t)
	fx.server.SetOpsDeps(ops.Deps{
		Store:   fx.server.store,
		Machine: fx.machine,
		Experts: expert.NewDefaultSet(expert.DEFAULT_CONTEXT_CAPACITY),
		DataDir: fx.dataDir,
	})
	session, cancel := connect(t, fx.server)
	defer cancel()

	nightly := callToolJSON(t, session, "nightly_run", map[string]any{})
	if nightly["ok"] != true || nightly["factor"] != 0.5 {
		t.Fatalf("nightly = %v", nightly)
	}
	if _, err := os.Stat(filepath.Join(fx.dataDir, "experts.json")); err != nil {
		t.Fatalf("experts.json not persisted: %v", err)
	}

	judged := callToolJSON(t, session, "judge_run", map[string]any{
		"entity_id": "猫猫", "path": "状态.信任值", "proposed": "0.3", "source": "teacher",
	})
	if judged["queued"] != true {
		t.Fatalf("judge = %v", judged)
	}
	id, _ := judged["candidate_id"].(string)
	if id == "" {
		t.Fatalf("candidate_id missing: %v", judged)
	}

	listed := callToolJSON(t, session, "candidates_list", map[string]any{})
	if listed["count"] != float64(1) {
		t.Fatalf("candidates = %v", listed)
	}
	resolved := callToolJSON(t, session, "candidate_resolve", map[string]any{"id": id, "status": "approved"})
	if resolved["ok"] != true {
		t.Fatalf("resolve = %v", resolved)
	}
	if text := callToolText(t, session, "candidate_apply", map[string]any{"id": "cand_missing"}); !strings.HasPrefix(text, "❌") {
		t.Fatalf("candidate_apply answer = %q", text)
	}
	if text := callToolText(t, session, "repair_rollback", map[string]any{}); !strings.HasPrefix(text, "❌") {
		t.Fatalf("repair_rollback answer = %q", text)
	}
}

// TestOpsToolsWithoutDeps covers a server assembled without runtime wiring: the
// operational tools report a configuration failure instead of panicking.
func TestOpsToolsWithoutDeps(t *testing.T) {
	fx := newTickFixture(t)
	session, cancel := connect(t, fx.server)
	defer cancel()
	text := callToolText(t, session, "nightly_run", map[string]any{})
	if !strings.HasPrefix(text, "❌") || !strings.Contains(text, "依赖未配置") {
		t.Fatalf("nightly without deps answer = %q", text)
	}
	if text := callToolText(t, session, "judge_run", map[string]any{"entity_id": "猫猫", "path": "状态.情绪"}); !strings.HasPrefix(text, "❌") {
		t.Fatalf("judge without deps answer = %q", text)
	}
}

// TestDeleteEntityRemovesObservations covers the helper directly: a changed
// observation stream disappears with the entity and the store is saved.
func TestDeleteEntityRemovesObservations(t *testing.T) {
	fx := newTickFixture(t)
	bus := wireObservations(t, fx)
	if _, err := fx.server.WriteEntityPaths("猫猫", map[string]any{"状态.情绪": "开心"}, "test"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := fx.server.DeleteEntity("猫猫"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if fx.server.store.Get("猫猫") != nil || bus.Version(observation.ScopeForEntity("猫猫")) != 0 {
		t.Fatal("entity or observation stream survived the delete")
	}
	if err := fx.server.DeleteEntity("猫猫"); err == nil {
		t.Fatal("second delete must report the missing entity")
	}
}

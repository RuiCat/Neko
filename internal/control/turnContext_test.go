// turnContext_test.go pins the /turn-context package shape and its read-only
// projection semantics.
package control

import (
	"net/http"
	"testing"

	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
)

func TestTurnContextReturnsPackage(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodGet, "/turn-context?entity=猫猫", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true {
		t.Fatalf("ok 字段错误：%v", payload)
	}
	compensation, ok := payload["compensation"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 compensation：%v", payload)
	}
	if compensation["state_source"] != "state_machine_snapshot" {
		t.Fatalf("state_source 错误：%v", compensation["state_source"])
	}
	state, ok := compensation["state"].(map[string]any)
	if !ok || state["猫猫.状态.情绪"] != "平静" {
		t.Fatalf("状态投影错误：%v", compensation["state"])
	}
	snapshot, ok := payload["snapshot"].(map[string]any)
	if !ok || snapshot["snapshot_id"] == "" {
		t.Fatalf("缺少快照：%v", payload)
	}
	gate, ok := payload["gate"].(map[string]any)
	if !ok || gate["version"] != "gate-v1" {
		t.Fatalf("门控信息错误：%v", payload["gate"])
	}
}

func TestTurnContextRejectsMissingEntity(t *testing.T) {
	env := newTestEnv(t, Options{})
	if recorder := env.call(t, http.MethodGet, "/turn-context", nil); recorder.Code != http.StatusBadRequest {
		t.Fatalf("缺少 entity 应 400，得到 %d", recorder.Code)
	}
	if recorder := env.call(t, http.MethodGet, "/turn-context?entity=梦梦", nil); recorder.Code != http.StatusNotFound {
		t.Fatalf("未知实体应 404，得到 %d", recorder.Code)
	}
}

// TestTurnContextFiltersBySession proves the optional session parameter keeps a
// turn's own memory out of its package while sibling data stays recallable.
func TestTurnContextFiltersBySession(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.runtime.Experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{
		SnapshotID: "evt_self", SessionID: "s-self", Content: "用户: 本会话 | 助手: 喵", Weight: 0.9,
	})
	env.runtime.Experts.Remember("猫猫", gate.EXPERT_EVENT, expert.Entry{
		SnapshotID: "evt_other", SessionID: "s-other", Content: "用户: 别的会话 | 助手: 喵", Weight: 0.9,
	})
	own := decodeBody(t, env.call(t, http.MethodGet, "/turn-context?entity=猫猫&session=s-self", nil))
	if memory := memoryOf(t, own); len(memory) != 1 || memory[0]["source_snapshot_id"] != "evt_other" {
		t.Fatalf("本会话条目应被剔除：%v", memory)
	}
	other := decodeBody(t, env.call(t, http.MethodGet, "/turn-context?entity=猫猫&session=s-other", nil))
	if memory := memoryOf(t, other); len(memory) != 1 || memory[0]["source_snapshot_id"] != "evt_self" {
		t.Fatalf("别的会话条目应保留：%v", memory)
	}
	anonymous := decodeBody(t, env.call(t, http.MethodGet, "/turn-context?entity=猫猫", nil))
	if memory := memoryOf(t, anonymous); len(memory) != 2 {
		t.Fatalf("无 session 参数时历史条目都应召回：%v", memory)
	}
}

// memoryOf extracts the package memory list from a /turn-context response.
func memoryOf(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	compensation, ok := payload["compensation"].(map[string]any)
	if !ok {
		t.Fatalf("缺少补偿包：%v", payload)
	}
	items, ok := compensation["memory"].([]any)
	if !ok {
		return nil
	}
	memory := make([]map[string]any, 0, len(items))
	for _, item := range items {
		fragment, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("记忆片段形状错误：%v", item)
		}
		memory = append(memory, fragment)
	}
	return memory
}

func TestTurnContextDoesNotWriteJournal(t *testing.T) {
	env := newTestEnv(t, Options{})
	before := len(env.runtime.Store.ListAll())
	env.call(t, http.MethodGet, "/turn-context?entity=猫猫", nil)
	if after := len(env.runtime.Store.ListAll()); after != before {
		t.Fatalf("实体数量变化：%d -> %d", before, after)
	}
}

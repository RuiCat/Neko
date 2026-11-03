// turnContext_test.go pins the /turn-context package shape and its read-only
// projection semantics.
package control

import (
	"net/http"
	"testing"
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

func TestTurnContextDoesNotWriteJournal(t *testing.T) {
	env := newTestEnv(t, Options{})
	before := len(env.runtime.Store.ListAll())
	env.call(t, http.MethodGet, "/turn-context?entity=猫猫", nil)
	if after := len(env.runtime.Store.ListAll()); after != before {
		t.Fatalf("实体数量变化：%d -> %d", before, after)
	}
}

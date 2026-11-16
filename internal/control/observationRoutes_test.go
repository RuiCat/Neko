// observationRoutes_test.go covers the /v2/observations read surface: deltas,
// the folded state projection, stream discovery and the error vocabulary.
package control

import (
	"net/http"
	"testing"

	"github.com/RuiCat/Neko/internal/observation"
)

// writeObservedState writes one entity path so the bus carries one event.
func (env *testEnv) writeObservedState(t *testing.T, entity, path string, value any) {
	t.Helper()
	if _, err := env.mcp.WriteEntityPaths(entity, map[string]any{path: value}, "test"); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// 默认策略没有业务路径规则（tier 全为 normal）；带规则的策略由
// observation.json 提供，见 internal/observation 的策略测试。
func TestObservationsDelta(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.writeObservedState(t, "猫猫", "状态.情绪", "开心")
	recorder := env.call(t, http.MethodGet, "/v2/observations?scope=entity:%E7%8C%AB%E7%8C%AB&since=0", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true || payload["scope"] != "entity:猫猫" {
		t.Fatalf("envelope = %v", payload)
	}
	events, ok := payload["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events = %v", payload["events"])
	}
	event, _ := events[0].(map[string]any)
	if event["path"] != "状态.情绪" || event["from"] != "平静" || event["to"] != "开心" || event["tier"] != "normal" {
		t.Fatalf("event = %v", event)
	}
	if payload["cursor"] != float64(1) || payload["version"] != float64(1) {
		t.Fatalf("cursor/version = %v/%v", payload["cursor"], payload["version"])
	}
	budget, _ := payload["budget"].(map[string]any)
	if budget["maxItems"] != float64(observation.DEFAULT_MAX_ITEMS) {
		t.Fatalf("budget = %v", budget)
	}
}

func TestObservationsDeltaCursorSkipsPushed(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.writeObservedState(t, "猫猫", "状态.情绪", "开心")
	recorder := env.call(t, http.MethodGet, "/v2/observations?scope=entity:%E7%8C%AB%E7%8C%AB&since=1", nil)
	payload := decodeBody(t, recorder)
	events, _ := payload["events"].([]any)
	if len(events) != 0 {
		t.Fatalf("cursor 之后不应有事件：%v", events)
	}
	if payload["cursor"] != float64(1) {
		t.Fatalf("无事件时 cursor 应推进到 version=1，得到 %v", payload["cursor"])
	}
}

func TestObservationState(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.writeObservedState(t, "猫猫", "状态.身体", "虚弱")
	recorder := env.call(t, http.MethodGet, "/v2/observations/state?scope=entity:%E7%8C%AB%E7%8C%AB", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	entries, ok := payload["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %v", payload["entries"])
	}
	entry, _ := entries[0].(map[string]any)
	if entry["path"] != "状态.身体" || entry["value"] != "虚弱" || entry["tier"] != "normal" {
		t.Fatalf("entry = %v", entry)
	}
}

func TestObservationStreams(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodGet, "/v2/observations/streams?entity=%E7%8C%AB%E7%8C%AB", nil)
	payload := decodeBody(t, recorder)
	scopes, ok := payload["scopes"].([]any)
	if !ok || len(scopes) != 1 || scopes[0] != "entity:猫猫" {
		t.Fatalf("scopes = %v", payload["scopes"])
	}
}

func TestObservationRoutesRejectBadRequests(t *testing.T) {
	env := newTestEnv(t, Options{})
	for _, target := range []string{
		"/v2/observations",
		"/v2/observations?since=abc&scope=entity:x",
		"/v2/observations?limit=0&scope=entity:x",
		"/v2/observations/state",
		"/v2/observations/streams",
	} {
		recorder := env.call(t, http.MethodGet, target, nil)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", target, recorder.Code)
		}
		if code := errorCode(t, decodeBody(t, recorder)); code != CODE_BAD_REQUEST {
			t.Fatalf("%s code = %q", target, code)
		}
	}
	recorder := env.call(t, http.MethodPost, "/v2/observations?scope=entity:x", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", recorder.Code)
	}
}

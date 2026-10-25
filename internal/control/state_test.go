package control

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestHealthEndpoint checks the K1 /health payload.
func TestHealthEndpoint(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodGet, "/health", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["ok"] != true {
		t.Fatalf("ok = %v", payload["ok"])
	}
	if payload["pid"] != float64(os.Getpid()) {
		t.Fatalf("pid = %v", payload["pid"])
	}
	if payload["dataDir"] != env.dataDir || payload["scriptsDir"] != env.scriptsDir {
		t.Fatalf("dirs = %v / %v", payload["dataDir"], payload["scriptsDir"])
	}
	if payload["entityCount"] != float64(1) {
		t.Fatalf("entityCount = %v", payload["entityCount"])
	}
	if payload["version"] != CONTROL_VERSION {
		t.Fatalf("version = %v", payload["version"])
	}
	if _, ok := payload["uptimeMs"].(float64); !ok {
		t.Fatalf("uptimeMs = %v", payload["uptimeMs"])
	}
}

// TestStateSelectedPaths checks values and missing for repeated path params.
func TestStateSelectedPaths(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodGet,
		"/state?entity=猫猫&path=状态.情绪&path=不存在.路径", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	values, _ := payload["values"].(map[string]any)
	if values["状态.情绪"] != "平静" {
		t.Fatalf("values = %v", values)
	}
	missing, _ := payload["missing"].([]any)
	if len(missing) != 1 || missing[0] != "不存在.路径" {
		t.Fatalf("missing = %v", missing)
	}
	if payload["entity"] != "猫猫" {
		t.Fatalf("entity = %v", payload["entity"])
	}
	if bytes, _ := payload["bytes"].(float64); bytes <= 0 {
		t.Fatalf("bytes = %v", payload["bytes"])
	}
}

// TestStateWholeSnapshot checks the no-path payload and truncation boundary.
func TestStateWholeSnapshot(t *testing.T) {
	env := newTestEnv(t, Options{})
	recorder := env.call(t, http.MethodGet, "/state?entity=猫猫", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["truncated"] != false {
		t.Fatalf("truncated = %v", payload["truncated"])
	}
	data, _ := payload["data"].(map[string]any)
	if data["备注"] != "测试实体" {
		t.Fatalf("data = %v", data)
	}

	bigData := map[string]any{"大字段": strings.Repeat("x", 4096)}
	if _, err := env.runtime.Store.Create("大猫", "NPC", bigData, nil); err != nil {
		t.Fatalf("create big entity: %v", err)
	}
	truncated := decodeBody(t, env.call(t, http.MethodGet, "/state?entity=大猫&maxBytes=1024", nil))
	if truncated["truncated"] != true {
		t.Fatalf("truncated = %v", truncated["truncated"])
	}
	if truncated["data"] != nil {
		t.Fatalf("data should be omitted when truncated: %v", truncated["data"])
	}
	if bytes, _ := truncated["bytes"].(float64); bytes <= 1024 {
		t.Fatalf("bytes = %v", truncated["bytes"])
	}
}

// TestStateErrors covers the 400/404/405 paths of /state.
func TestStateErrors(t *testing.T) {
	env := newTestEnv(t, Options{})

	missingEntity := env.call(t, http.MethodGet, "/state", nil)
	if missingEntity.Code != http.StatusBadRequest {
		t.Fatalf("missing entity status = %d", missingEntity.Code)
	}
	if code := errorCode(t, decodeBody(t, missingEntity)); code != CODE_BAD_REQUEST {
		t.Fatalf("missing entity code = %s", code)
	}

	badMaxBytes := env.call(t, http.MethodGet, "/state?entity=猫猫&maxBytes=1", nil)
	if badMaxBytes.Code != http.StatusBadRequest {
		t.Fatalf("bad maxBytes status = %d", badMaxBytes.Code)
	}

	unknownEntity := env.call(t, http.MethodGet, "/state?entity=不存在", nil)
	if unknownEntity.Code != http.StatusNotFound {
		t.Fatalf("unknown entity status = %d", unknownEntity.Code)
	}
	if code := errorCode(t, decodeBody(t, unknownEntity)); code != CODE_NOT_FOUND {
		t.Fatalf("unknown entity code = %s", code)
	}

	wrongMethod := env.call(t, http.MethodPost, "/state?entity=猫猫", []byte(`{}`))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("allow = %q", allow)
	}
	if code := errorCode(t, decodeBody(t, wrongMethod)); code != CODE_METHOD_NOT_ALLOWED {
		t.Fatalf("wrong method code = %s", code)
	}

	unknownEndpoint := env.call(t, http.MethodGet, "/nope", nil)
	if unknownEndpoint.Code != http.StatusNotFound {
		t.Fatalf("unknown endpoint status = %d", unknownEndpoint.Code)
	}
}

// TestStateReadsSnapshotOnly verifies /state reflects values written through
// the entity lock (Snapshot), not stale template copies.
func TestStateReadsSnapshotOnly(t *testing.T) {
	env := newTestEnv(t, Options{})
	entity := env.runtime.Store.Get("猫猫")
	if err := entity.Set("状态.情绪", "开心"); err != nil {
		t.Fatalf("set: %v", err)
	}
	payload := decodeBody(t, env.call(t, http.MethodGet, "/state?entity=猫猫&path=状态.情绪", nil))
	values, _ := payload["values"].(map[string]any)
	if values["状态.情绪"] != "开心" {
		t.Fatalf("values = %v", values)
	}
}

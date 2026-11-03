// Data-plane contract tests: path parity with the control plane, deterministic
// maxBytes truncation and the single tick payload constructor.
package monitor

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/control"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// TestPathParityWithControl covers K1/C14: the monitor entity endpoint and the
// control /state endpoint agree on values and missing, including template
// fallback, and the monitor endpoint rejects empty paths and dedupes repeats.
func TestPathParityWithControl(t *testing.T) {
	env := newTestEnv(t, Options{})
	controlServer := control.New(env.runtime, env.mcp, control.Options{})
	paths := []string{"备注", "身体.模板专有", "状态.情绪", "不存在.路径"}
	for _, path := range paths {
		controlRecorder := httptest.NewRecorder()
		controlRequest := httptest.NewRequest(http.MethodGet,
			"/state?entity=猫猫&path="+url.QueryEscape(path), nil)
		controlServer.ServeHTTP(controlRecorder, controlRequest)
		controlPayload := decode(t, controlRecorder)
		controlValues, _ := controlPayload["values"].(map[string]any)

		monitorRecorder := env.request(t, http.MethodGet,
			"/monitor/api/entities/猫猫?path="+url.QueryEscape(path), nil, nil)
		monitorPayload := decode(t, monitorRecorder)
		monitorValues, _ := monitorPayload["values"].(map[string]any)

		controlValue, controlOK := controlValues[path]
		monitorValue, monitorOK := monitorValues[path]
		if controlOK != monitorOK {
			t.Fatalf("path %q 存在性不一致: control=%v monitor=%v", path, controlOK, monitorOK)
		}
		if controlOK && !reflect.DeepEqual(controlValue, monitorValue) {
			t.Fatalf("path %q 值不一致: control=%v monitor=%v", path, controlValue, monitorValue)
		}
	}

	emptyPath := env.request(t, http.MethodGet, "/monitor/api/entities/猫猫?path=", nil, nil)
	if emptyPath.Code != http.StatusBadRequest {
		t.Fatalf("空 path status = %d", emptyPath.Code)
	}
	deduped := env.request(t, http.MethodGet,
		"/monitor/api/entities/猫猫?path="+url.QueryEscape("备注")+"&path="+url.QueryEscape("备注"), nil, nil)
	values, _ := decode(t, deduped)["values"].(map[string]any)
	if len(values) != 1 {
		t.Fatalf("重复 path 未去重: %v", values)
	}

	unknown := env.request(t, http.MethodGet, "/monitor/api/entities/不存在", nil, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("未知实体 status = %d", unknown.Code)
	}
}

// TestMaxBytesDeterministic covers C15: bounded listings stay parseable, bytes
// is the payload size and the kept items are a sorted prefix.
func TestMaxBytesDeterministic(t *testing.T) {
	env := newTestEnv(t, Options{})
	keys := map[string]string{"k": strings.Repeat("x", 1024)}
	for index := 0; index < 100; index++ {
		id := fmt.Sprintf("e%03d", index)
		if _, err := env.runtime.Store.Create(id, "NPC", nil, keys); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	recorder := env.request(t, http.MethodGet, "/monitor/api/entities?maxBytes=4096", nil, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, bodyText(recorder))
	}
	payload := decode(t, recorder)
	bytesField, _ := payload["bytes"].(float64)
	if bytesField > 4096 {
		t.Fatalf("bytes = %v 超过 maxBytes", bytesField)
	}
	if payload["truncated"] != true {
		t.Fatalf("truncated = %v", payload["truncated"])
	}
	items, _ := payload["items"].([]any)
	if len(items) == 0 {
		t.Fatal("截断后 items 为空")
	}
	previous := ""
	for _, item := range items {
		row, _ := item.(map[string]any)
		id, _ := row["id"].(string)
		if id <= previous {
			t.Fatalf("items 不是 id 字典序前缀: %q after %q", id, previous)
		}
		previous = id
	}
	second := env.request(t, http.MethodGet, "/monitor/api/entities?maxBytes=4096", nil, nil)
	if !bytes.Equal(recorder.Body.Bytes(), second.Body.Bytes()) {
		t.Fatal("maxBytes 截断结果不确定")
	}
	for _, bad := range []string{"1023", "1048577"} {
		recorder := env.request(t, http.MethodGet, "/monitor/api/entities?maxBytes="+bad, nil, nil)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("maxBytes=%s status = %d", bad, recorder.Code)
		}
	}
}

// TestTickPayloadParity covers C24: the control /tick body, the monitor /tick
// body and the WS tick frame share one field set built by mcpserver.TickPayload.
func TestTickPayloadParity(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN})
	controlServer := control.New(env.runtime, env.mcp, control.Options{})
	controlRecorder := httptest.NewRecorder()
	controlServer.ServeHTTP(controlRecorder,
		httptest.NewRequest(http.MethodPost, "/tick", strings.NewReader(`{}`)))
	controlBody := decode(t, controlRecorder)

	monitorRecorder := env.request(t, http.MethodPost, "/monitor/api/tick", []byte(`{}`), authed())
	if monitorRecorder.Code != http.StatusOK {
		t.Fatalf("monitor tick status = %d (%s)", monitorRecorder.Code, bodyText(monitorRecorder))
	}
	monitorBody := decode(t, monitorRecorder)
	if !sameKeys(controlBody, monitorBody) {
		t.Fatalf("tick 字段集不一致: control=%v monitor=%v", keysOf(controlBody), keysOf(monitorBody))
	}
	for _, key := range []string{"tick_id", "tickId"} {
		if _, ok := monitorBody[key]; !ok {
			t.Fatalf("monitor tick 缺少 %s", key)
		}
	}

	server := env.liveServer(t)
	conn := dial(t, server, "/monitor/ws?topics=tick",
		[]string{"astrbox.token." + TEST_TOKEN})
	readFrame(t, conn)
	readUntil(t, conn, "snapshot")
	if _, err := env.mcp.RunTick("", mcpserver.TickOptions{SkipSaveWhenUnchanged: true}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
	frame := readUntil(t, conn, "tick")
	result, _ := frame["result"].(map[string]any)
	withOK := map[string]any{"ok": true}
	for key, value := range result {
		withOK[key] = value
	}
	if !sameKeys(controlBody, withOK) {
		t.Fatalf("WS tick 字段集不一致: control=%v ws=%v", keysOf(controlBody), keysOf(withOK))
	}
}

// sameKeys reports whether both maps have exactly the same key set.
func sameKeys(left, right map[string]any) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

// keysOf renders a map's keys for failure messages.
func keysOf(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

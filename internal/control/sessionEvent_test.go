package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/neko"
)

// TestSessionEventLoggedOnly covers the no-callback path: only a log line.
func TestSessionEventLoggedOnly(t *testing.T) {
	env := newTestEnv(t, Options{})
	body := []byte(`{"kind":"session-created","sessionId":"s-1","cwd":"/tmp/work"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["applied"] != "logged" {
		t.Fatalf("applied = %v", payload["applied"])
	}
	logText := env.controlLogText(t)
	if !containsAll(logText, "POST /session-event", "callback=missing", "session=s-1") {
		t.Fatalf("control log = %s", logText)
	}
}

// TestSessionEventFunction covers the registered callback path: the .neko
// function receives positional args and the following Save writes entities.json.
func TestSessionEventFunction(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.registerSessionFunction(t, DEFAULT_FUNCTION_NAME)
	body := []byte(`{"kind":"turn-end","sessionId":"s-2","cwd":"/tmp/work","turn":7}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if payload["applied"] != "function" {
		t.Fatalf("applied = %v", payload["applied"])
	}
	entity := env.runtime.Store.Get("猫猫")
	if value, err := entity.Get("状态.最后事件"); err != nil || value != "turn-end" {
		t.Fatalf("状态.最后事件 = %v (%v)", value, err)
	}
	if value, err := entity.Get("状态.最后会话"); err != nil || value != "s-2" {
		t.Fatalf("状态.最后会话 = %v (%v)", value, err)
	}
	if value, err := entity.Get("状态.最后轮次"); err != nil || value != 7 {
		t.Fatalf("状态.最后轮次 = %v (%v)", value, err)
	}
	if _, err := os.Stat(filepath.Join(env.dataDir, "entities.json")); err != nil {
		t.Fatalf("callback save missing: %v", err)
	}
	logText := env.controlLogText(t)
	if !containsAll(logText, "POST /session-event", "callback=ok") {
		t.Fatalf("control log = %s", logText)
	}
}

// TestSessionEventSummaryArgument covers the optional fifth positional argument:
// the bridge's turn digest arrives as a JSON string.
func TestSessionEventSummaryArgument(t *testing.T) {
	env := newTestEnv(t, Options{})
	definition := neko.FuncDef{
		Name: "summaryEvent",
		Params: []neko.Param{
			{Name: "kind"}, {Name: "session_id"}, {Name: "cwd"}, {Name: "turn"}, {Name: "summary"},
		},
		Body: "状态.摘要 = summary\n返回 summary",
	}
	if err := env.runtime.Funcs.Register(definition); err != nil {
		t.Fatalf("register summaryEvent: %v", err)
	}
	body := []byte(`{"kind":"turn-end","sessionId":"s-6","cwd":"/tmp/work","turn":2,` +
		`"function":"summaryEvent","summary":{"assistantDigest":"你好","toolCalls":3,"steps":1}}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	entity := env.runtime.Store.Get("猫猫")
	value, err := entity.Get("状态.摘要")
	if err != nil {
		t.Fatalf("状态.摘要: %v", err)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("状态.摘要 = %T, want string", value)
	}
	if !containsAll(text, `"assistantDigest":"你好"`, `"toolCalls":3`, `"steps":1`) {
		t.Fatalf("摘要 = %s", text)
	}
	if logText := env.controlLogText(t); !containsAll(logText, "summary=tools:3,steps:1") {
		t.Fatalf("control log = %s", logText)
	}
}

// TestSessionEventNamedFunction covers the optional function override: the
// named callback runs instead of the server default.
func TestSessionEventNamedFunction(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.registerCallback(t, DEFAULT_FUNCTION_NAME, "状态.默认事件 = kind\n返回 kind")
	env.registerCallback(t, "customEvent", "状态.自定义事件 = kind\n返回 kind")

	body := []byte(`{"kind":"turn-end","sessionId":"s-3","cwd":"/tmp/work","turn":1,"function":"customEvent"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if payload := decodeBody(t, recorder); payload["applied"] != "function" {
		t.Fatalf("applied = %v", payload["applied"])
	}
	entity := env.runtime.Store.Get("猫猫")
	if value, err := entity.Get("状态.自定义事件"); err != nil || value != "turn-end" {
		t.Fatalf("状态.自定义事件 = %v (%v)", value, err)
	}
	if _, err := entity.Get("状态.默认事件"); err == nil {
		t.Fatalf("默认回调不应在指定 function 时被调用")
	}
	if logText := env.controlLogText(t); !containsAll(logText, "function=customEvent") {
		t.Fatalf("control log = %s", logText)
	}

	// Without the optional field the server default callback runs.
	recorder = env.call(t, http.MethodPost, "/session-event",
		[]byte(`{"kind":"session-created","sessionId":"s-4","cwd":"/tmp/work"}`))
	if payload := decodeBody(t, recorder); payload["applied"] != "function" {
		t.Fatalf("default applied = %v", payload["applied"])
	}
	if value, err := entity.Get("状态.默认事件"); err != nil || value != "session-created" {
		t.Fatalf("状态.默认事件 = %v (%v)", value, err)
	}
}

// TestSessionEventUnknownNamedFunction logs only and never falls back to the
// server default callback.
func TestSessionEventUnknownNamedFunction(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.registerCallback(t, DEFAULT_FUNCTION_NAME, "状态.默认事件 = kind\n返回 kind")
	body := []byte(`{"kind":"session-created","sessionId":"s-5","cwd":"/tmp/work","function":"missingEvent"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if payload := decodeBody(t, recorder); payload["applied"] != "logged" {
		t.Fatalf("applied = %v", payload["applied"])
	}
	if _, err := env.runtime.Store.Get("猫猫").Get("状态.默认事件"); err == nil {
		t.Fatalf("未注册的 function 不应回落到默认回调")
	}
	if logText := env.controlLogText(t); !containsAll(logText, "function=missingEvent", "callback=missing") {
		t.Fatalf("control log = %s", logText)
	}
}

// TestSessionEventDeadlineReturns504 blocks the shared state gate and expects
// the server-side deadline error instead of an open socket.
func TestSessionEventDeadlineReturns504(t *testing.T) {
	env := newTestEnv(t, Options{Deadline: 100 * time.Millisecond})
	env.registerSessionFunction(t, DEFAULT_FUNCTION_NAME)
	release := holdStateGate(t, env)

	started := time.Now()
	recorder := env.call(t, http.MethodPost, "/session-event",
		[]byte(`{"kind":"session-created","sessionId":"s-slow","cwd":"/tmp/work"}`))
	elapsed := time.Since(started)
	release()

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, decodeBody(t, recorder)); code != CODE_DEADLINE_EXCEEDED {
		t.Fatalf("code = %s", code)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("504 took %s", elapsed)
	}
	env.waitForUserEntitiesContain(t, "最后事件")
}

// TestSessionEventValidation covers 400/405 for malformed events.
func TestSessionEventValidation(t *testing.T) {
	env := newTestEnv(t, Options{})
	cases := []struct {
		name string
		body string
	}{
		{"unknown kind", `{"kind":"other","sessionId":"s","cwd":"/tmp"}`},
		{"missing session", `{"kind":"session-created","cwd":"/tmp"}`},
		{"missing cwd", `{"kind":"session-created","sessionId":"s"}`},
		{"turn-end without turn", `{"kind":"turn-end","sessionId":"s","cwd":"/tmp"}`},
		{"negative turn", `{"kind":"turn-end","sessionId":"s","cwd":"/tmp","turn":-1}`},
		{"blank function", `{"kind":"session-created","sessionId":"s","cwd":"/tmp","function":"   "}`},
		{"bad json", `{`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := env.call(t, http.MethodPost, "/session-event", []byte(testCase.body))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
			}
			if code := errorCode(t, decodeBody(t, recorder)); code != CODE_BAD_REQUEST {
				t.Fatalf("code = %s", code)
			}
		})
	}

	wrongMethod := env.call(t, http.MethodGet, "/session-event", nil)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method status = %d", wrongMethod.Code)
	}
	if allow := wrongMethod.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("allow = %q", allow)
	}
}

// TestSessionEventExplicitEntity covers K3: an explicit request entity makes
// the callback and the following Save act on that entity, not on the server
// default.
func TestSessionEventExplicitEntity(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.registerSessionFunction(t, DEFAULT_FUNCTION_NAME)
	if _, err := env.runtime.Store.Create("梦梦", "NPC", nil, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	body := []byte(`{"kind":"turn-end","sessionId":"s-entity","cwd":"/tmp/work","turn":4,"entity":"梦梦"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if payload := decodeBody(t, recorder); payload["applied"] != "function" {
		t.Fatalf("applied = %v", payload["applied"])
	}
	entity := env.runtime.Store.Get("梦梦")
	if value, err := entity.Get("状态.最后会话"); err != nil || value != "s-entity" {
		t.Fatalf("梦梦.状态.最后会话 = %v (%v)", value, err)
	}
	if value, err := entity.Get("状态.最后轮次"); err != nil || value != 4 {
		t.Fatalf("梦梦.状态.最后轮次 = %v (%v)", value, err)
	}
	if _, err := env.runtime.Store.Get("猫猫").Get("状态.最后事件"); err == nil {
		t.Fatalf("默认实体不应被回写")
	}
	raw, err := os.ReadFile(filepath.Join(env.dataDir, "user", "entities.json"))
	if err != nil || !strings.Contains(string(raw), "s-entity") {
		t.Fatalf("保存文件未包含绑定实体回写: %v (%s)", err, raw)
	}
}

// TestSessionEventUnknownEntityRejected covers K3: an explicit unknown entity
// answers 404 entity_not_found and never reaches the callback or the disk.
func TestSessionEventUnknownEntityRejected(t *testing.T) {
	env := newTestEnv(t, Options{})
	env.registerSessionFunction(t, DEFAULT_FUNCTION_NAME)
	body := []byte(`{"kind":"turn-end","sessionId":"s-ghost","cwd":"/tmp/work","turn":1,"entity":"不存在"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	payload := decodeBody(t, recorder)
	if code := errorCode(t, payload); code != CODE_ENTITY_NOT_FOUND {
		t.Fatalf("code = %s", code)
	}
	errorObject, _ := payload["error"].(map[string]any)
	if message, _ := errorObject["message"].(string); !strings.Contains(message, "不存在") {
		t.Fatalf("message = %q", message)
	}
	if _, err := env.runtime.Store.Get("猫猫").Get("状态.最后事件"); err == nil {
		t.Fatalf("未知实体不应触发回调")
	}
	env.assertEntityFilesDoNotContain(t, "s-ghost")
}

// TestSessionEventUnknownEntityBeforeCallbackMissing is the ordering
// regression: entity validation runs before the callback-missing early return,
// so an unknown entity is still 404 when no .neko callback is registered.
func TestSessionEventUnknownEntityBeforeCallbackMissing(t *testing.T) {
	env := newTestEnv(t, Options{})
	body := []byte(`{"kind":"session-created","sessionId":"s-order","cwd":"/tmp/work","entity":"幽灵"}`)
	recorder := env.call(t, http.MethodPost, "/session-event", body)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if code := errorCode(t, decodeBody(t, recorder)); code != CODE_ENTITY_NOT_FOUND {
		t.Fatalf("code = %s", code)
	}
	logText := env.controlLogText(t)
	if !strings.Contains(logText, "POST /session-event 404") {
		t.Fatalf("控制日志未记录 404: %s", logText)
	}
	if strings.Contains(logText, "callback=missing") {
		t.Fatalf("未知实体不应走到 callback=missing 早退: %s", logText)
	}
	env.assertEntityFilesDoNotContain(t, "s-order")
}

// TestSessionEventDefaultEntityFallback covers K3's compatibility half: an
// absent or blank entity keeps the server default entity and the legacy
// applied=function behaviour.
func TestSessionEventDefaultEntityFallback(t *testing.T) {
	env := newTestEnv(t, Options{Entity: "梦梦"})
	env.registerSessionFunction(t, DEFAULT_FUNCTION_NAME)
	if _, err := env.runtime.Store.Create("梦梦", "NPC", nil, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	cases := []struct {
		name      string
		sessionID string
		body      string
	}{
		{"absent", "s-default", `{"kind":"turn-end","sessionId":"s-default","cwd":"/tmp/work","turn":2}`},
		{"blank", "s-blank", `{"kind":"turn-end","sessionId":"s-blank","cwd":"/tmp/work","turn":3,"entity":""}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := env.call(t, http.MethodPost, "/session-event", []byte(testCase.body))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
			}
			if payload := decodeBody(t, recorder); payload["applied"] != "function" {
				t.Fatalf("applied = %v", payload["applied"])
			}
			entity := env.runtime.Store.Get("梦梦")
			if value, err := entity.Get("状态.最后会话"); err != nil || value != testCase.sessionID {
				t.Fatalf("默认实体.状态.最后会话 = %v (%v)", value, err)
			}
		})
	}
	if _, err := env.runtime.Store.Get("猫猫").Get("状态.最后事件"); err == nil {
		t.Fatalf("非默认实体不应被回写")
	}
}

// TestSessionEventExplicitEntityDrivesTurnRecord covers the recordDSHTurn half
// of K3: the journaled snapshot belongs to the request entity, not the default.
func TestSessionEventExplicitEntityDrivesTurnRecord(t *testing.T) {
	runtime, server := newJournalEnv(t)
	if _, err := runtime.Store.Create("梦梦", "NPC", nil, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	body, err := json.Marshal(map[string]any{
		"kind": "turn-end", "sessionId": "s-record", "cwd": "/tmp/x", "turn": 2,
		"entity":  "梦梦",
		"summary": map[string]any{"assistantDigest": "喵，我在。", "userDigest": "在吗？"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/session-event", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	records, err := journal.ReadDir(filepath.Join(runtime.DataDir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if record.Source != journal.SOURCE_DSH_TURN {
			continue
		}
		found = true
		if record.Compensation.StateSource != "state_machine_snapshot" {
			t.Fatalf("补偿包来源 %q", record.Compensation.StateSource)
		}
		hasEntity := false
		for key := range record.Snapshot.State {
			if strings.HasPrefix(key, "梦梦.") {
				hasEntity = true
			}
			if strings.HasPrefix(key, "猫猫.") {
				t.Fatalf("快照应属于 梦梦，却出现 %s", key)
			}
		}
		if !hasEntity {
			t.Fatalf("快照缺少 梦梦 状态: %v", record.Snapshot.State)
		}
	}
	if !found {
		t.Fatalf("缺少 DSH 轮记录（全部 %d 条）", len(records))
	}
}

// assertEntityFilesDoNotContain fails when either persisted entities file
// carries needle; it is how "the rejected event never reached Save" is checked
// without depending on whether bootstrap pre-created the files.
func (env *testEnv) assertEntityFilesDoNotContain(t *testing.T, needle string) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(env.dataDir, "entities.json"),
		filepath.Join(env.dataDir, "user", "entities.json"),
	} {
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("读取 %s: %v", path, err)
		}
		if strings.Contains(string(raw), needle) {
			t.Fatalf("%s 不应包含 %q: %s", path, needle, raw)
		}
	}
}

// containsAll reports whether every substring is present.
func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

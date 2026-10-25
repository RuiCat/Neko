package control

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// containsAll reports whether every substring is present.
func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}

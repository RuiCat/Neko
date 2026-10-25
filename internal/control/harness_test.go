package control

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ruicat/astrbox/internal/bootstrap"
	"github.com/ruicat/astrbox/internal/mcpserver"
	"github.com/ruicat/astrbox/internal/neko"
)

// testEnv is one control server backed by a real runtime in temp directories.
type testEnv struct {
	runtime    *bootstrap.Runtime
	mcp        *mcpserver.Server
	server     *Server
	dataDir    string
	scriptsDir string
}

// newTestEnv assembles a real bootstrap runtime with one NPC entity.
func newTestEnv(t *testing.T, options Options) *testEnv {
	t.Helper()
	dataDir := t.TempDir()
	scriptsDir := t.TempDir()
	runtime, err := bootstrap.New(bootstrap.Options{DataDir: dataDir, ScriptsDir: scriptsDir})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	runtime.Store.AddTemplate("NPC", map[string]any{
		"状态": map[string]any{"情绪": "平静", "身体": "健康"},
		"身体": map[string]any{"体力": 80},
	})
	if _, err := runtime.Store.Create("猫猫", "NPC", map[string]any{"备注": "测试实体"}, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	mcp := runtime.NewMCPServer()
	return &testEnv{
		runtime: runtime, mcp: mcp, server: New(runtime, mcp, options),
		dataDir: dataDir, scriptsDir: scriptsDir,
	}
}

// call serves one request in-process and returns the recorded response.
func (env *testEnv) call(t *testing.T, method, target string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	env.server.ServeHTTP(recorder, request)
	return recorder
}

// decodeBody parses a JSON response body.
func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, recorder.Body.String())
	}
	return payload
}

// errorCode extracts error.code from a failure envelope.
func errorCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	errorObject, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 error 对象: %v", payload)
	}
	code, _ := errorObject["code"].(string)
	return code
}

// registerCallback registers a .neko function with the four session-event
// positional parameters and the given body.
func (env *testEnv) registerCallback(t *testing.T, name, body string) {
	t.Helper()
	definition := neko.FuncDef{
		Name: name,
		Params: []neko.Param{
			{Name: "kind"}, {Name: "session_id"}, {Name: "cwd"}, {Name: "turn"},
		},
		Body: body,
	}
	if err := env.runtime.Funcs.Register(definition); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// registerSessionFunction registers a callback that records its positional args.
func (env *testEnv) registerSessionFunction(t *testing.T, name string) {
	t.Helper()
	env.registerCallback(t, name,
		"状态.最后事件 = kind\n状态.最后会话 = session_id\n状态.最后轮次 = turn\n返回 kind")
}

// holdStateGate occupies the shared state-write gate until the returned release
// function runs; it is the deterministic stand-in for an in-flight MCP tick.
func holdStateGate(t *testing.T, env *testEnv) func() {
	t.Helper()
	entered := make(chan struct{})
	releaseChannel := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = env.mcp.WithStateWrite(func() error {
			close(entered)
			<-releaseChannel
			return nil
		})
	}()
	<-entered
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(releaseChannel)
			<-finished
		})
	}
	t.Cleanup(release)
	return release
}

// waitForUserEntitiesContain polls the saved user entities file for a value so
// an asynchronous write started by a 504 test is known to be on disk.
func (env *testEnv) waitForUserEntitiesContain(t *testing.T, needle string) {
	t.Helper()
	path := filepath.Join(env.dataDir, "user", "entities.json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(raw), needle) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s 未在期限内落盘 %q", path, needle)
}

// controlLogText reads today's control log for assertions.
func (env *testEnv) controlLogText(t *testing.T) string {
	t.Helper()
	path := filepath.Join(env.dataDir, "logs",
		CONTROL_LOG_PREFIX+time.Now().Format(CONTROL_LOG_DATE_FORMAT)+CONTROL_LOG_SUFFIX)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取控制日志 %s: %v", path, err)
	}
	return string(raw)
}

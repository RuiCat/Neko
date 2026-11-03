// Shared fixture for the internal/monitor tests: one real runtime, one monitor
// server and helpers for in-process HTTP and real WebSocket calls.
package monitor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/mcpserver"
	"github.com/coder/websocket"
)

// TEST_TOKEN is a charset-valid token used by the auth tests.
const TEST_TOKEN = "test-token-0123456789"

// testEnv is one assembled monitor server over temp directories.
type testEnv struct {
	runtime    *bootstrap.Runtime
	mcp        *mcpserver.Server
	server     *Server
	dataDir    string
	scriptsDir string
}

// newTestEnv assembles a real runtime with one NPC template and one entity.
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
		"身体": map[string]any{"体力": 80, "模板专有": "模板值"},
	})
	if _, err := runtime.Store.Create("猫猫", "NPC", map[string]any{"备注": "测试实体"}, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	mcp := runtime.NewMCPServer()
	server := New(runtime, mcp, options)
	t.Cleanup(server.Close)
	return &testEnv{runtime: runtime, mcp: mcp, server: server, dataDir: dataDir, scriptsDir: scriptsDir}
}

// request serves one in-process request with the loopback Host header.
func (e *testEnv) request(t *testing.T, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	return e.requestWithHost(t, "127.0.0.1:8765", method, target, body, headers)
}

// requestWithHost serves one in-process request under an explicit Host header.
func (e *testEnv) requestWithHost(t *testing.T, host, method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Host = host
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	e.server.ServeHTTP(recorder, request)
	return recorder
}

// liveServer starts the monitor handler on a real loopback listener.
func (e *testEnv) liveServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(e.server)
	t.Cleanup(server.Close)
	return server
}

// decode parses one JSON response body.
func decode(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	payload := map[string]any{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, recorder.Body.String())
	}
	return payload
}

// failureCode extracts error.code from a failure envelope.
func failureCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	object, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 error 对象: %v", payload)
	}
	code, _ := object["code"].(string)
	return code
}

// dial opens one real WebSocket connection to the test server.
func dial(t *testing.T, server *httptest.Server, target string, protocols []string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + target
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: protocols})
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

// readFrame reads and decodes one server frame.
func readFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	frame := map[string]any{}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("frame is not JSON: %v (%s)", err, data)
	}
	return frame
}

// readUntil reads frames until one has the wanted type.
func readUntil(t *testing.T, conn *websocket.Conn, frameType string) map[string]any {
	t.Helper()
	for attempt := 0; attempt < 50; attempt++ {
		frame := readFrame(t, conn)
		if frame["type"] == frameType {
			return frame
		}
	}
	t.Fatalf("没有收到 %s 帧", frameType)
	return nil
}

// frameSeq extracts the numeric seq of one frame.
func frameSeq(t *testing.T, frame map[string]any) float64 {
	t.Helper()
	seq, ok := frame["seq"].(float64)
	if !ok {
		t.Fatalf("frame 缺少 seq: %v", frame)
	}
	return seq
}

// bodyText returns the raw response body as text.
func bodyText(recorder *httptest.ResponseRecorder) string {
	return recorder.Body.String()
}

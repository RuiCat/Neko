// WebSocket contract tests: hello/snapshot ordering on every connection, no
// resumeSeq, per-connection monotonic seq, deltas only after a snapshot and
// added/updated/removed diagnostic frames.
package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/coder/websocket"
)

// TestWSSubprotocolEcho covers K3/C7: the handshake selects and echoes the
// astrbox.token.<token> subprotocol the browser offered.
func TestWSSubprotocolEcho(t *testing.T) {
	env := newTestEnv(t, Options{Token: TEST_TOKEN})
	server := env.liveServer(t)
	protocol := "astrbox.token." + TEST_TOKEN
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/monitor/ws?topics=tick"
	conn, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{Subprotocols: []string{protocol}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.CloseNow() }()
	if conn.Subprotocol() != protocol {
		t.Fatalf("选中子协议 = %q", conn.Subprotocol())
	}
	if echoed := response.Header.Get("Sec-WebSocket-Protocol"); echoed != protocol {
		t.Fatalf("响应头回显 = %q", echoed)
	}
}

// TestWSSeqContract covers K2/C9: every connection (including one carrying
// resumeSeq) starts with hello then snapshot; deltas follow the snapshot with a
// larger per-connection seq.
func TestWSSeqContract(t *testing.T) {
	env := newTestEnv(t, Options{Version: "test"})
	server := env.liveServer(t)
	conn := dial(t, server, "/monitor/ws?topics=state", nil)
	hello := readFrame(t, conn)
	if hello["type"] != "hello" {
		t.Fatalf("首帧不是 hello: %v", hello)
	}
	if hello["version"] != "test" {
		t.Fatalf("hello version = %v", hello["version"])
	}
	snapshot := readUntil(t, conn, "snapshot")
	if snapshot["topic"] != "state" {
		t.Fatalf("snapshot topic = %v", snapshot["topic"])
	}
	snapshotSeq := frameSeq(t, snapshot)

	entity := env.runtime.Store.Get("猫猫")
	if err := entity.Set("备注", "ws-delta"); err != nil {
		t.Fatalf("set: %v", err)
	}
	delta := readUntil(t, conn, "delta")
	if delta["entity"] != "猫猫" || delta["path"] != "备注" || delta["newValue"] != "ws-delta" {
		t.Fatalf("delta = %v", delta)
	}
	if frameSeq(t, delta) <= snapshotSeq {
		t.Fatalf("delta seq %v 不大于 snapshot seq %v", frameSeq(t, delta), snapshotSeq)
	}

	if err := conn.Close(websocket.StatusNormalClosure, ""); err != nil {
		t.Fatalf("close: %v", err)
	}
	reconnect := dial(t, server, "/monitor/ws?topics=state&resumeSeq=1", nil)
	first := readFrame(t, reconnect)
	if first["type"] != "hello" {
		t.Fatalf("重连首帧不是 hello: %v", first)
	}
	again := readUntil(t, reconnect, "snapshot")
	if again["topic"] != "state" {
		t.Fatalf("重连 snapshot topic = %v", again["topic"])
	}
	if frameSeq(t, again) <= 0 {
		t.Fatalf("重连 snapshot seq = %v", frameSeq(t, again))
	}
	if err := env.runtime.Store.Get("猫猫").Set("备注", "after-reconnect"); err != nil {
		t.Fatalf("set after reconnect: %v", err)
	}
	after := readUntil(t, reconnect, "delta")
	if frameSeq(t, after) <= frameSeq(t, again) {
		t.Fatalf("重连后 delta seq %v 不大于 snapshot seq %v", frameSeq(t, after), frameSeq(t, again))
	}
}

// TestDiagnosticsFrameSemantics covers C11: added/updated/removed are distinct
// and one deduplicated record keeps one id with count=2.
func TestDiagnosticsFrameSemantics(t *testing.T) {
	env := newTestEnv(t, Options{})
	server := env.liveServer(t)
	conn := dial(t, server, "/monitor/ws?topics=diagnostics", nil)
	readFrame(t, conn)
	readUntil(t, conn, "snapshot")

	record := diagnostics.Record{
		Code: "test_code", Script: "s", Entity: "猫猫",
		Message: "诊断测试", Severity: diagnostics.SEVERITY_WARNING,
	}
	env.runtime.Diagnostics.Add(record)
	added := readUntil(t, conn, "diagnostics")
	if added["op"] != "added" {
		t.Fatalf("op = %v", added["op"])
	}
	stored, _ := added["record"].(map[string]any)
	id, _ := stored["id"].(string)
	if id == "" {
		t.Fatalf("record id 缺失: %v", added)
	}

	env.runtime.Diagnostics.Add(record)
	updated := readUntil(t, conn, "diagnostics")
	if updated["op"] != "updated" {
		t.Fatalf("op = %v", updated["op"])
	}
	updatedRecord, _ := updated["record"].(map[string]any)
	if updatedRecord["id"] != id || updatedRecord["count"] != float64(2) {
		t.Fatalf("更新记录 = %v", updatedRecord)
	}

	env.runtime.Diagnostics.ClearByScript("s")
	removed := readUntil(t, conn, "diagnostics")
	if removed["op"] != "removed" || removed["id"] != id {
		t.Fatalf("removed = %v", removed)
	}
}

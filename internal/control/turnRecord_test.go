// turnRecord_test.go proves a DSH turn-end digest becomes a complete journal
// record, and that tick records stay separate.
package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// newJournalEnv assembles a runtime with journaling enabled so turn-end can
// append records; the writer is closed before the test reads the file.
func newJournalEnv(t *testing.T) (*bootstrap.Runtime, *Server) {
	t.Helper()
	runtime, err := bootstrap.New(bootstrap.Options{DataDir: t.TempDir(), ScriptsDir: t.TempDir(), Journal: true})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	runtime.Store.AddTemplate("NPC", map[string]any{"状态": map[string]any{"情绪": "平静"}})
	if _, err := runtime.Store.Create("猫猫", "NPC", nil, nil); err != nil {
		t.Fatalf("create entity: %v", err)
	}
	return runtime, New(runtime, runtime.NewMCPServer(), Options{})
}

func TestTurnEndWritesCompleteRecord(t *testing.T) {
	runtime, server := newJournalEnv(t)
	body, _ := json.Marshal(map[string]any{
		"kind": "turn-end", "sessionId": "s-1", "cwd": "/tmp/x", "turn": 3,
		"summary": map[string]any{"assistantDigest": "喵，我在。", "userDigest": "在吗？", "toolCalls": 1, "steps": 2},
	})
	request := httptest.NewRequest(http.MethodPost, "/session-event", bytes.NewReader(body))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("状态码 %d：%s", recorder.Code, recorder.Body.String())
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	records, err := journal.ReadDir(runtime.DataDir + "/journal")
	if err != nil {
		t.Fatal(err)
	}
	complete := 0
	turnID := ""
	for _, record := range records {
		if record.Source != journal.SOURCE_DSH_TURN {
			continue
		}
		complete++
		turnID = record.TurnID
		if record.Status != journal.STATUS_COMPLETE {
			t.Fatalf("状态 %q", record.Status)
		}
		if !strings.Contains(record.Answer, "喵") {
			t.Fatalf("回答为空：%+v", record)
		}
		if record.UserInput != "在吗？" {
			t.Fatalf("用户输入为空：%+v", record)
		}
		if record.Compensation.StateSource != "state_machine_snapshot" {
			t.Fatalf("补偿包来源 %q", record.Compensation.StateSource)
		}
		if record.SessionID != "s-1" {
			t.Fatalf("会话 %q", record.SessionID)
		}
	}
	if complete != 1 || turnID == "" {
		t.Fatalf("complete DSH 记录数 %d，期望 1（全部 %d 条）", complete, len(records))
	}
	// The turn is now in the experts' private context, so the next package
	// recalls it with the turn's snapshot as the source anchor.
	contextRequest := httptest.NewRequest(http.MethodGet, "/turn-context?entity=猫猫", nil)
	contextRecorder := httptest.NewRecorder()
	server.ServeHTTP(contextRecorder, contextRequest)
	payload := decodeBody(t, contextRecorder)
	compensation, ok := payload["compensation"].(map[string]any)
	if !ok {
		t.Fatalf("缺少补偿包：%v", payload)
	}
	memory, ok := compensation["memory"].([]any)
	if !ok || len(memory) == 0 {
		t.Fatalf("补偿包没有召回上一轮记忆：%v", compensation)
	}
	first, ok := memory[0].(map[string]any)
	if !ok || first["source_snapshot_id"] != turnID {
		t.Fatalf("记忆来源错误：%v（期望 %s）", memory[0], turnID)
	}
}

func TestTickRecordsAreMarkedAsEngineTicks(t *testing.T) {
	runtime, _ := newJournalEnv(t)
	if _, err := runtime.NewMCPServer().RunTick("", mcpserver.TickOptions{SkipSaveWhenUnchanged: true}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := journal.ReadDir(runtime.DataDir + "/journal")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Source != journal.SOURCE_TICK {
		t.Fatalf("tick 记录来源错误：%+v", records)
	}
}

// chat_test.go pins the turn contract: complete record, package in the prompt,
// bounded recent history, dry-run and provider-failure handling.
package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/expert"
	"github.com/RuiCat/Neko/internal/gate"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/llm"
)

type chatFixture struct {
	chat   *Chat
	writer *journal.Writer
	dir    string
}

func newChatFixture(t *testing.T, status int, answer string) *chatFixture {
	t.Helper()
	store := entity.NewStore(t.TempDir())
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("猫猫", "NPC", map[string]any{"状态": map[string]any{"情绪": "平静"}}, nil); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "test-model",
			"choices": []any{map[string]any{"message": map[string]any{"content": answer}}},
			"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3},
		})
	}))
	t.Cleanup(server.Close)
	provider, err := llm.NewHTTPProvider(server.URL, "test-key", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	provider.Client = server.Client()
	provider.RetryBackoff = 0
	dir := t.TempDir()
	writer := journal.NewWriter(dir)
	t.Cleanup(func() { _ = writer.Close() })
	chat, err := New(Options{
		Store: store, Schema: hardstate.DefaultSchema(), Sequence: hardstate.NewSequence(),
		Board:     blackboard.New(),
		Assembler: compensation.NewAssembler(gate.NewRuleGate(), expert.NewDefaultRegistry(4)),
		Journal:   writer, Provider: provider, SessionID: "test-session", MaxRecent: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &chatFixture{chat: chat, writer: writer, dir: dir}
}

func TestTurnWritesCompleteRecordWithPackage(t *testing.T) {
	fixture := newChatFixture(t, http.StatusOK, "喵")
	result, err := fixture.chat.Turn(context.Background(), "在吗", "tick-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "喵" || result.Record.Status != journal.STATUS_COMPLETE {
		t.Fatalf("结果错误：%+v", result.Record)
	}
	if result.Record.Compensation.StateSource != compensation.STATE_SOURCE {
		t.Fatalf("补偿包来源 %q", result.Record.Compensation.StateSource)
	}
	if result.Record.Snapshot.TickID != "tick-1" {
		t.Fatalf("tick id %q", result.Record.Snapshot.TickID)
	}
	if err := fixture.writer.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := journal.ReadDir(fixture.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Answer != "喵" {
		t.Fatalf("日志记录错误：%+v", records)
	}
}

func TestDryRunSkipsProvider(t *testing.T) {
	store := entity.NewStore(t.TempDir())
	_ = store.Load()
	_, _ = store.Create("猫猫", "NPC", map[string]any{"状态": map[string]any{"情绪": "平静"}}, nil)
	writer := journal.NewWriter(t.TempDir())
	fixture, err := New(Options{
		Store: store, Schema: hardstate.DefaultSchema(), Sequence: hardstate.NewSequence(),
		Board:     blackboard.New(),
		Assembler: compensation.NewAssembler(gate.NewRuleGate(), expert.NewDefaultRegistry(4)),
		Journal:   writer, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.Turn(context.Background(), "在吗", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Status != journal.STATUS_INCOMPLETE || result.Record.Answer != "" {
		t.Fatalf("dry-run 记录错误：%+v", result.Record)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderFailureWritesErrorRecord(t *testing.T) {
	fixture := newChatFixture(t, http.StatusBadRequest, "")
	if _, err := fixture.chat.Turn(context.Background(), "在吗", ""); err == nil {
		t.Fatal("provider 失败应返回错误")
	}
	if err := fixture.writer.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := journal.ReadDir(fixture.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != journal.STATUS_ERROR {
		t.Fatalf("失败记录错误：%+v", records)
	}
}

func TestRecentHistoryIsBounded(t *testing.T) {
	fixture := newChatFixture(t, http.StatusOK, "喵")
	for index := 0; index < 3; index++ {
		if _, err := fixture.chat.Turn(context.Background(), "第"+string(rune('一'+index))+"轮", ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(fixture.chat.Recent()); got > DEFAULT_MAX_RECENT {
		t.Fatalf("历史未受控：%d", got)
	}
	if fixture.chat.Recent()[len(fixture.chat.Recent())-1].Role != "assistant" {
		t.Fatal("历史末尾应为助手回复")
	}
}

func TestPromptCarriesPersonaTwoSystemMessagesAndInput(t *testing.T) {
	fixture := newChatFixture(t, http.StatusOK, "喵")
	messages := fixture.chat.buildMessages(journal.Compensation{SnapshotID: "evt_x"}, "在吗")
	if len(messages) != 3 {
		t.Fatalf("消息数 %d", len(messages))
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "猫猫") {
		t.Fatal("缺少人格设定")
	}
	if !strings.Contains(messages[1].Content, "evt_x") {
		t.Fatal("补偿包未进入提示")
	}
	if messages[2].Content != "在吗" {
		t.Fatal("用户输入未进入提示")
	}
}

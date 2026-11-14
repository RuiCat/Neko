package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/emote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestEmoteToolRegistered verifies emote_find is listed with the text argument
// and answers with the project's ❌/JSON conventions (A1).
func TestEmoteToolRegistered(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	server.SetEmoteClient(emote.NewClient(emote.Config{Dir: t.TempDir()}))
	session, cancel := connect(t, server)
	defer cancel()
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var definition *mcp.Tool
	for i := range tools.Tools {
		if tools.Tools[i].Name == "emote_find" {
			definition = tools.Tools[i]
		}
	}
	if definition == nil {
		t.Fatal("emote_find not registered")
	}
	raw, err := json.Marshal(definition.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if !strings.Contains(string(raw), `"text"`) || !strings.Contains(string(raw), `"required"`) {
		t.Fatalf("schema missing text/required: %s", raw)
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "emote_find", Arguments: map[string]any{"text": "  "}})
	if err != nil {
		t.Fatalf("call emote_find: %v", err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "text 不能为空") {
		t.Fatalf("empty text answer = %q", text)
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "emote_find", Arguments: map[string]any{"text": "主人我好想你呀"}})
	if err != nil {
		t.Fatalf("call emote_find: %v", err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !strings.HasPrefix(text, "❌") || !strings.Contains(text, "表情库索引缺失") {
		t.Fatalf("missing-data answer = %q", text)
	}
}

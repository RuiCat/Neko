package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/events"
	"github.com/ruicat/astrbox/internal/function"
	"github.com/ruicat/astrbox/internal/registry"
)

func newTestServer(t *testing.T) (*Server, func()) {
	t.Helper()
	store := entity.NewStore("../../data")
	if err := store.Load(); err != nil {
		t.Fatalf("load store: %v", err)
	}
	machine := engine.New(store)
	reg, err := registry.New(store, machine, filepath.Join(t.TempDir(), "scripts.json"), filepath.Join(t.TempDir(), "scripts"))
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	server := New(store, machine, reg)
	server.SetDataDir(t.TempDir())
	server.SetEventBridge(events.NewSystem(store, function.New(store)).Bridge())
	return server, func() {}
}

func connect(t *testing.T, server *Server) (*mcp.ClientSession, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.MCPServer().Run(ctx, serverTransport) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("connect: %v", err)
	}
	return session, cancel
}

func TestToolListAndInventory(t *testing.T) {
	server, cleanup := newTestServer(t)
	defer cleanup()
	session, cancel := connect(t, server)
	defer cancel()
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := map[string]bool{
		"entity_state_get": false, "entity_state_set": false, "entity_query": false,
		"entity_create": false, "state_machine_tick": false, "state_machine_resolve": false,
		"neko_script_apply": false, "neko_script_remove": false, "neko_script_list": false,
		"inventory": false,
	}
	for _, tool := range tools.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %s not registered", name)
		}
	}

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "entity_state_get", Arguments: map[string]any{
		"entity": "猫猫", "path": "身体.体力",
	}})
	if err != nil {
		t.Fatalf("call entity_state_get: %v", err)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result is not json: %v (%s)", err, text)
	}
	if payload["值"] == nil {
		t.Fatalf("missing 值: %s", text)
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "inventory", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call inventory: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty inventory result")
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "event_log", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call event_log: %v", err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !json.Valid([]byte(text)) {
		t.Fatalf("event_log is not json: %s", text)
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "stats", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call stats: %v", err)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !json.Valid([]byte(text)) {
		t.Fatalf("stats is not json: %s", text)
	}

	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "entity_snapshot", Arguments: map[string]any{
		"name": "test_snapshot",
	}})
	if err != nil {
		t.Fatalf("call entity_snapshot: %v", err)
	}
	var snapshot map[string]any
	snapshotText := result.Content[0].(*mcp.TextContent).Text
	if err := json.Unmarshal([]byte(snapshotText), &snapshot); err != nil {
		t.Fatalf("snapshot result is not json: %v (%s)", err, snapshotText)
	}
	path, _ := snapshot["path"].(string)
	if path == "" {
		t.Fatalf("snapshot path missing: %s", snapshotText)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
}

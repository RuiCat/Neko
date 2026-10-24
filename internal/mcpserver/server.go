// Package mcpserver exposes the AstrBox engine as MCP tools over stdio/HTTP.
package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
	"github.com/ruicat/astrbox/internal/events"
	"github.com/ruicat/astrbox/internal/registry"
)

// Server bundles the engine components behind one MCP server.
type Server struct {
	store       *entity.Store
	machine     *engine.StateMachine
	registry    *registry.Registry
	logger      *engine.StateLogger
	dataDir     string
	bridge      *events.Bridge
	eventBus    *events.Bus
	diagnostics *diagnostics.Store
}

// New builds an MCP server over an initialized store, engine and registry.
func New(store *entity.Store, machine *engine.StateMachine, reg *registry.Registry) *Server {
	return &Server{store: store, machine: machine, registry: reg}
}

// SetStateLogger enables daily tick logging (optional).
func (s *Server) SetStateLogger(logger *engine.StateLogger) { s.logger = logger }

// SetDataDir sets the directory used for manual snapshots.
func (s *Server) SetDataDir(path string) { s.dataDir = path }

// SetEventBridge exposes validation/event logs through the event_log tool.
func (s *Server) SetEventBridge(bridge *events.Bridge) { s.bridge = bridge }

// SetDiagnostics wires the shared diagnostic store (optional).
func (s *Server) SetDiagnostics(store *diagnostics.Store) { s.diagnostics = store }

// SetEventBus exposes the structured event dispatch history (optional).
func (s *Server) SetEventBus(bus *events.Bus) { s.eventBus = bus }

// MCPServer constructs and registers every tool.
func (s *Server) MCPServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "astrbox", Version: "1.0.0"}, nil)
	s.addEntityTools(server)
	s.addStateMachineTools(server)
	s.addScriptTools(server)
	s.addScriptFileTools(server)
	s.addDiagnosticsTools(server)
	s.addInventoryTool(server)
	s.addStatsTool(server)
	s.addSnapshotTool(server)
	s.addEventLogTool(server)
	return server
}

// RunStdio serves the MCP protocol over stdin/stdout.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.MCPServer().Run(ctx, &mcp.StdioTransport{})
}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}

func jsonResult(value any) (*mcp.CallToolResult, any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return textResult("❌ 序列化失败: " + err.Error())
	}
	return textResult(string(raw))
}

func fail(format string, args ...any) (*mcp.CallToolResult, any, error) {
	return textResult("❌ " + fmt.Sprintf(format, args...))
}

type entityGetArgs struct {
	Entity      string `json:"entity" jsonschema:"实体ID，如 猫猫/梦梦/场景"`
	Path        string `json:"path,omitempty" jsonschema:"点分路径，如 身体.体力；留空返回完整数据"`
	Diagnostics string `json:"diagnostics,omitempty" jsonschema:"诊断附加内容：none/summary/full，默认 summary"`
}

type entitySetArgs struct {
	Entity string `json:"entity" jsonschema:"实体ID"`
	Path   string `json:"path" jsonschema:"点分路径"`
	Value  string `json:"value" jsonschema:"值（JSON 或纯文本）"`
}

type entityQueryArgs struct {
	Query string `json:"query" jsonschema:"查询表达式：NPC[*] / 猫猫 / key:场景=客厅 / 状态.身体=疲惫"`
}

type entityCreateArgs struct {
	Type string `json:"type" jsonschema:"实体类型（需存在模板）"`
	Name string `json:"name" jsonschema:"实体ID（唯一）"`
	Data string `json:"data,omitempty" jsonschema:"初始数据（JSON）"`
}

func (s *Server) addEntityTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_state_get",
		Description: "读取实体状态值；不填 path 返回完整数据。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entityGetArgs) (*mcp.CallToolResult, any, error) {
		ent := s.store.Get(args.Entity)
		if ent == nil {
			return fail("实体 '%s' 不存在。可用: %s", args.Entity, jsonString(s.entityIDs()))
		}
		if args.Path == "" {
			payload := map[string]any{"实体": ent.ID, "类型": ent.Type, "数据": ent.Data}
			s.attachDiagnostics(payload, ent, args.Diagnostics)
			return jsonResult(payload)
		}
		value, err := ent.Get(args.Path)
		if err != nil {
			return fail("路径 '%s' 不存在（实例和模板都没有）", args.Path)
		}
		return jsonResult(map[string]any{"实体": ent.ID, "路径": args.Path, "值": value})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_state_set",
		Description: "写入实体状态值并落盘。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entitySetArgs) (*mcp.CallToolResult, any, error) {
		ent := s.store.Get(args.Entity)
		if ent == nil {
			return fail("实体 '%s' 不存在", args.Entity)
		}
		value := parseValue(args.Value)
		if err := ent.Set(args.Path, value); err != nil {
			return fail("写入失败: %s", err)
		}
		if err := s.store.Save(); err != nil {
			return fail("保存失败: %s", err)
		}
		return textResult(fmt.Sprintf("✅ 已设置 %s.%s = %s", args.Entity, args.Path, jsonString(value)))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_query",
		Description: "查询实体：类型通配 / by id / key:查询 / 字段值查询。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entityQueryArgs) (*mcp.CallToolResult, any, error) {
		entities, err := s.store.Query(args.Query)
		if err != nil {
			return fail("查询失败: %s", err)
		}
		rows := make([]map[string]any, 0, len(entities))
		cfg := config.Current()
		for _, ent := range entities {
			rows = append(rows, map[string]any{
				"id": ent.ID, "type": ent.Type, "keys": ent.Keys,
				cfg.StateGroup:           ent.GetOrNone(cfg.StateGroup),
				cfg.DefaultStateSubgroup: ent.GetOrNone(cfg.DefaultStateSubgroup),
			})
		}
		return jsonResult(map[string]any{"query": args.Query, "count": len(rows), "entities": rows})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_create",
		Description: "创建实体（深拷贝模板链 + 数据覆盖）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entityCreateArgs) (*mcp.CallToolResult, any, error) {
		data := map[string]any{}
		if strings.TrimSpace(args.Data) != "" {
			parsed := parseValue(args.Data)
			object, ok := parsed.(map[string]any)
			if !ok {
				object = map[string]any{"值": parsed}
			}
			data = object
		}
		if _, err := s.store.Create(args.Name, args.Type, data, nil); err != nil {
			return fail("创建失败: %s", err)
		}
		if err := s.store.Save(); err != nil {
			return fail("保存失败: %s", err)
		}
		return textResult(fmt.Sprintf("✅ 已创建实体 %s (类型: %s)", args.Name, args.Type))
	})
}

func (s *Server) entityIDs() []string {
	ids := make([]string, 0, s.store.Count())
	for _, ent := range s.store.ListAll() {
		ids = append(ids, ent.ID)
	}
	sort.Strings(ids)
	return ids
}

func parseValue(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

func jsonString(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(raw)
}

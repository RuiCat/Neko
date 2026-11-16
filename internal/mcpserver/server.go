// Package mcpserver exposes the AstrBox engine as MCP tools over stdio/HTTP.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/RuiCat/Neko/internal/blackboard"
	"github.com/RuiCat/Neko/internal/compensation"
	"github.com/RuiCat/Neko/internal/config"
	"github.com/RuiCat/Neko/internal/control/ops"
	"github.com/RuiCat/Neko/internal/diagnostics"
	"github.com/RuiCat/Neko/internal/driver"
	"github.com/RuiCat/Neko/internal/emote"
	"github.com/RuiCat/Neko/internal/engine"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/events"
	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/journal"
	"github.com/RuiCat/Neko/internal/observation"
	"github.com/RuiCat/Neko/internal/registry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	// contextDriver enables the context_list/context_send tools when set.
	contextDriver *driver.Driver
	// contextTools registers the cross-context tools; the dsh-mcp entry sets it.
	contextTools bool
	// tickMu serializes RunTick so MCP tools and the control plane share one
	// in-flight tick per process.
	tickMu sync.Mutex
	// tickObserverMu guards tickObserver, which is set once at wiring time.
	tickObserverMu sync.RWMutex
	// tickObserver receives every finished tick outside tickMu.
	tickObserver func(engine.TickResult)
	// journal writes one TurnRecord per tick when enabled; the snapshot
	// sequence and schema are fixed at wiring time.
	journal         *journal.Writer
	journalSequence *hardstate.Sequence
	hardStateSchema hardstate.Schema
	// sessionMu guards sessionID, updated by the control plane.
	sessionMu sync.RWMutex
	sessionID string
	// board and assembler enable the compensation dry-run per tick.
	board     *blackboard.Board
	assembler *compensation.Assembler
	// teachMu serializes teacher-sample appends; trainMu keeps one training run
	// at a time (the pipeline writes shared work dirs and model files).
	teachMu sync.Mutex
	trainMu sync.Mutex
	// observations records every tick's net changes and direct writes; nil
	// disables the observation side effect without changing tick behavior.
	observations      *observation.Bus
	observationPolicy *observation.Policy
	// boardPublisher forwards script push messages to the session board; nil
	// disables the board side effect without changing tick behavior.
	boardPublisherMu sync.RWMutex
	boardPublisher   func(entityID, text string)
	// opsDeps backs the operational tools (nightly/judge/candidates/repair);
	// bootstrap wires it from the runtime.
	opsDeps ops.Deps
	// emoteClient answers emote_find; bootstrap wires it from the runtime.
	emoteClient *emote.Client
}

// New builds an MCP server over an initialized store, engine and registry.
func New(store *entity.Store, machine *engine.StateMachine, reg *registry.Registry) *Server {
	return &Server{store: store, machine: machine, registry: reg}
}

// SetStateLogger enables daily tick logging (optional).
func (s *Server) SetStateLogger(logger *engine.StateLogger) { s.logger = logger }

// SetJournal enables one TurnRecord per finished tick. The sequence and schema
// are fixed at wiring time; appends never block the tick gate.
func (s *Server) SetJournal(writer *journal.Writer, sequence *hardstate.Sequence, schema hardstate.Schema) {
	s.journal = writer
	s.journalSequence = sequence
	s.hardStateSchema = schema
}

// SetSessionID records the active DSH session for later turn records. The
// control plane calls it on every session event.
func (s *Server) SetSessionID(id string) {
	s.sessionMu.Lock()
	s.sessionID = id
	s.sessionMu.Unlock()
}

// SetLearningLoop enables the compensation dry-run: every journaled tick
// builds a package from the board and attaches the activated experts. No
// remote model is called here; that arrives with the LLM adapter.
func (s *Server) SetLearningLoop(board *blackboard.Board, assembler *compensation.Assembler) {
	s.board = board
	s.assembler = assembler
}

// currentSessionID returns the last session seen, or an empty string.
func (s *Server) currentSessionID() string {
	s.sessionMu.RLock()
	defer s.sessionMu.RUnlock()
	return s.sessionID
}

// SetDataDir sets the directory used for manual snapshots.
func (s *Server) SetDataDir(path string) { s.dataDir = path }

// SetEventBridge exposes validation/event logs through the event_log tool.
func (s *Server) SetEventBridge(bridge *events.Bridge) { s.bridge = bridge }

// SetDiagnostics wires the shared diagnostic store (optional).
func (s *Server) SetDiagnostics(store *diagnostics.Store) { s.diagnostics = store }

// SetTickObserver registers the tick observer. The callback runs outside
// tickMu and must not call RunTick/TryRunTick/WithStateWrite/Store.Save; see
// notifyTick for the non-blocking delivery contract.
func (s *Server) SetTickObserver(observer func(engine.TickResult)) {
	s.tickObserverMu.Lock()
	s.tickObserver = observer
	s.tickObserverMu.Unlock()
}

// SetEventBus exposes the structured event dispatch history (optional).
func (s *Server) SetEventBus(bus *events.Bus) { s.eventBus = bus }

// SetContextDriver enables the cross-context tools (optional).
func (s *Server) SetContextDriver(contextDriver *driver.Driver) { s.contextDriver = contextDriver }

// EnableContextTools registers context_list/context_send in this server.
func (s *Server) EnableContextTools() { s.contextTools = true }

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
	s.addObservationTool(server)
	s.addWorldClockTool(server)
	s.addOpsTools(server)
	s.addTrainingTools(server)
	s.addEmoteTools(server)
	if s.contextTools {
		s.addContextTools(server)
	}
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
	Entity      string `json:"entity" jsonschema:"实体ID（例如某个角色或场景的 id）"`
	Path        string `json:"path,omitempty" jsonschema:"点分路径，如 身体.体力；留空返回完整数据"`
	Diagnostics string `json:"diagnostics,omitempty" jsonschema:"诊断附加内容：none/summary/full，默认 summary"`
}

type entitySetArgs struct {
	Entity string         `json:"entity" jsonschema:"实体ID"`
	Path   string         `json:"path,omitempty" jsonschema:"点分路径；与 values 二选一"`
	Value  string         `json:"value,omitempty" jsonschema:"值（JSON 或纯文本）；与 path 配对"`
	Values map[string]any `json:"values,omitempty" jsonschema:"批量写入：path → 值（JSON 或纯文本）；与 path/value 二选一"`
}

type entityQueryArgs struct {
	Query string `json:"query" jsonschema:"查询表达式：类型通配 / 实体ID / key:字段=值 / 路径=值"`
}

type entityCreateArgs struct {
	Type string `json:"type" jsonschema:"实体类型（需存在模板）"`
	Name string `json:"name" jsonschema:"实体ID（唯一）"`
	Data string `json:"data,omitempty" jsonschema:"初始数据（JSON）"`
}

type entityDeleteArgs struct {
	Entity string `json:"entity" jsonschema:"实体ID（不可撤销：同时删除该实体的事件黑板）"`
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
			payload := map[string]any{"实体": ent.ID, "类型": ent.Type, "数据": ent.Snapshot()}
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
		Description: "写入实体状态值并落盘：path/value 单条，或 values 批量（path → 值）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entitySetArgs) (*mcp.CallToolResult, any, error) {
		values, err := args.setValues()
		if err != nil {
			return fail("%s", err)
		}
		paths, err := s.WriteEntityPaths(args.Entity, values, hardstate.SOURCE_MCP)
		if err != nil {
			if errors.Is(err, ErrEntityMissing) {
				return fail("实体 '%s' 不存在。可用: %s", args.Entity, jsonString(s.entityIDs()))
			}
			return fail("%s", err)
		}
		return jsonResult(map[string]any{"ok": true, "entity": args.Entity, "paths": paths})
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
		if err := s.WithStateWrite(func() error {
			if _, err := s.store.Create(args.Name, args.Type, data, nil); err != nil {
				return fmt.Errorf("创建失败: %s", err)
			}
			if err := s.store.Save(); err != nil {
				return fmt.Errorf("保存失败: %s", err)
			}
			if created := s.store.Get(args.Name); created != nil {
				s.seedObservations(created.ID, created.Snapshot())
			}
			return nil
		}); err != nil {
			return fail("%s", err)
		}
		return textResult(fmt.Sprintf("✅ 已创建实体 %s (类型: %s)", args.Name, args.Type))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_delete",
		Description: "删除实体及其观察流并落盘（不可撤销）。实体不存在时返回错误。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args entityDeleteArgs) (*mcp.CallToolResult, any, error) {
		entityID := strings.TrimSpace(args.Entity)
		if entityID == "" {
			return fail("缺少 entity")
		}
		if err := s.DeleteEntity(entityID); err != nil {
			if errors.Is(err, ErrEntityMissing) {
				return fail("实体 '%s' 不存在。可用: %s", entityID, jsonString(s.entityIDs()))
			}
			return fail("%s", err)
		}
		return jsonResult(map[string]any{"ok": true, "deleted": entityID})
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

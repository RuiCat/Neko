package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/config"
	"github.com/ruicat/astrbox/internal/engine"
)

type tickArgs struct {
	Scope string `json:"scope,omitempty" jsonschema:"作用范围：* 全部 / NPC[*] 只处理 NPC"`
}

type resolveArgs struct {
	ConflictID  string `json:"conflict_id" jsonschema:"冲突ID（来自 tick 的 conflicts）"`
	TargetState string `json:"target_state" jsonschema:"目标状态（JSON 对象，如 {\"身体\":\"疲惫\"}）"`
	Reason      string `json:"reason,omitempty" jsonschema:"决策理由"`
	SideEffects string `json:"side_effects,omitempty" jsonschema:"附加操作（JSON 数组）"`
}

func (s *Server) addStateMachineTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "state_machine_tick",
		Description: "推进状态机一个周期；返回 transitions 与 conflicts。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args tickArgs) (*mcp.CallToolResult, any, error) {
		result, err := s.RunTick(args.Scope, TickOptions{})
		if err != nil {
			return fail("状态机执行后保存失败: %s", err)
		}
		return jsonResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "state_machine_resolve",
		Description: "解决 tick 返回的状态冲突（人工/LLM 仲裁）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args resolveArgs) (*mcp.CallToolResult, any, error) {
		target := map[string]any{}
		if err := json.Unmarshal([]byte(args.TargetState), &target); err != nil {
			return fail("target_state 必须是 JSON 对象: %s", err)
		}
		var effects []engine.Op
		if args.SideEffects != "" {
			if err := json.Unmarshal([]byte(args.SideEffects), &effects); err != nil {
				return fail("side_effects 必须是 JSON 数组: %s", err)
			}
		}
		var entityID string
		for _, conflict := range s.machine.PendingConflicts() {
			if conflict.ConflictID == args.ConflictID {
				entityID = conflict.EntityID
				break
			}
		}
		if !s.machine.ResolveConflictWithEffects(args.ConflictID, target, args.Reason, effects) {
			return fail("冲突 %s 不存在或已解决", args.ConflictID)
		}
		if s.diagnostics != nil && entityID != "" {
			s.diagnostics.ClearByCodeEntity("tick.conflict.pending", entityID)
		}
		if err := s.store.Save(); err != nil {
			return fail("保存失败: %s", err)
		}
		return textResult("✅ 冲突 " + args.ConflictID + " 已解决")
	})
}

// snapshotStates captures the key states of entities touched by this tick,
// mirroring the Python state logger's snapshot section.
func (s *Server) snapshotStates(result engine.TickResult) map[string]map[string]any {
	cfg := config.Current()
	snapshots := map[string]map[string]any{}
	for _, transition := range result.Transitions {
		if transition.EntityID == "" {
			continue
		}
		if _, seen := snapshots[transition.EntityID]; seen {
			continue
		}
		ent := s.store.Get(transition.EntityID)
		if ent == nil {
			continue
		}
		snapshots[transition.EntityID] = map[string]any{
			"体力":   ent.GetOrNone(cfg.DefaultStateSubgroup + ".体力"),
			"身体状态": ent.GetOrNone(cfg.StateGroup + ".身体"),
			"情绪":   ent.GetOrNone(cfg.StateGroup + ".情绪"),
		}
	}
	return snapshots
}

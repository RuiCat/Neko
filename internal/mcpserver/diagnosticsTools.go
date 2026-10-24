package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/diagnostics"
	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/entity"
)

// attachDiagnostics adds a bounded diagnostic block to one entity response.
func (s *Server) attachDiagnostics(payload map[string]any, ent *entity.Entity, mode string) {
	if s.diagnostics == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "none":
		return
	}
	s.collectDiagnostics()
	top := 3
	if strings.EqualFold(strings.TrimSpace(mode), "full") {
		top = 50
	}
	scope := s.diagnostics.ForEntity(ent.ID, ent.Type, top)
	if scope.Summary.Total == 0 {
		return
	}
	payload["diagnostics"] = scope
}

// collectDiagnostics pulls bridge and pending-conflict signals into the store.
// Duplicate entries are merged by the store instead of growing the ring.
func (s *Server) collectDiagnostics() {
	if s.diagnostics == nil {
		return
	}
	if s.bridge != nil {
		for _, validation := range s.bridge.LastValidations(20) {
			entityID, _ := validation["entity"].(string)
			path, _ := validation["path"].(string)
			message, _ := validation["error"].(string)
			s.diagnostics.Add(diagnostics.Record{
				Code: "schema.validation.failed", Severity: diagnostics.SEVERITY_WARNING,
				Source: "bridge", Entity: entityID,
				Message: path + ": " + message, Hint: "写入未回滚，请检查实体数据",
			})
		}
	}
	if s.eventBus != nil {
		for _, record := range s.eventBus.History(20) {
			if record.Status != "warning" && record.Status != "skipped" {
				continue
			}
			code := "event.handler.failed"
			if record.Status == "skipped" {
				code = "event.skipped"
			}
			s.diagnostics.Add(diagnostics.Record{
				Code: code, Severity: diagnostics.SEVERITY_WARNING,
				Source: "bridge", Entity: record.Entity,
				Message: record.Message, Hint: "检查 @事件 定义与 @函数 处理器",
			})
		}
	}
	if s.machine != nil {
		for _, conflict := range s.machine.PendingConflicts() {
			s.diagnostics.Add(diagnostics.Record{
				Code: "tick.conflict.pending", Severity: diagnostics.SEVERITY_WARNING,
				Source: "engine", Entity: conflict.EntityID,
				Message: "待仲裁冲突 " + conflict.ConflictID + ": " + conflict.Reason,
				Hint:    "调用 state_machine_resolve 仲裁",
			})
		}
	}
}

// recordTick mirrors one tick outcome into diagnostics.
func (s *Server) recordTick(result engine.TickResult) {
	if s.diagnostics == nil {
		return
	}
	for _, message := range result.Errors {
		s.diagnostics.Add(diagnostics.Record{
			Code: "tick.rule.error", Severity: diagnostics.SEVERITY_ERROR,
			Source: "engine", Message: message,
		})
	}
	for _, conflict := range result.Conflicts {
		message := "冲突 " + conflict.ConflictID
		if conflict.Reason != "" {
			message += ": " + conflict.Reason
		}
		s.diagnostics.Add(diagnostics.Record{
			Code: "tick.conflict.pending", Severity: diagnostics.SEVERITY_WARNING,
			Source: "engine", Entity: conflict.EntityID, Message: message,
			Hint: "调用 state_machine_resolve 仲裁",
		})
	}
	if result.RulesEvaluated >= engine.MAX_RULE_EVAL {
		s.diagnostics.Add(diagnostics.Record{
			Code: "tick.budget.exceeded", Severity: diagnostics.SEVERITY_WARNING,
			Source: "engine", Message: "本次 tick 达到规则求值上限",
			Hint: "检查规则条件是否过宽",
		})
	}
}

type diagnosticsQueryArgs struct {
	Severity string `json:"severity,omitempty" jsonschema:"error/warning/info"`
	Source   string `json:"source,omitempty" jsonschema:"registry/engine/bridge/consistency"`
	Entity   string `json:"entity,omitempty" jsonschema:"按实体过滤"`
	Script   string `json:"script,omitempty" jsonschema:"按脚本过滤"`
	Code     string `json:"code,omitempty" jsonschema:"按错误码过滤"`
	Since    string `json:"since,omitempty" jsonschema:"只返回该时间之后出现的（RFC3339）"`
	Limit    int    `json:"limit,omitempty" jsonschema:"最多返回条数，默认 20，上限 200"`
}

// addDiagnosticsTools registers the diagnostics_query tool.
func (s *Server) addDiagnosticsTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "diagnostics_query",
		Description: "查询引擎警告与异常（脚本、tick、schema 校验、一致性）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args diagnosticsQueryArgs) (*mcp.CallToolResult, any, error) {
		if s.diagnostics == nil {
			return jsonResult(map[string]any{
				"generation": 0, "summary": diagnostics.Summary{}, "items": []any{},
			})
		}
		s.collectDiagnostics()
		filter := diagnostics.Filter{
			Severity: diagnostics.Severity(args.Severity),
			Source:   args.Source, Entity: args.Entity, Script: args.Script, Code: args.Code,
		}
		if args.Since != "" {
			if parsed, err := time.Parse(time.RFC3339, args.Since); err == nil {
				filter.Since = parsed
			}
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}
		if limit > 200 {
			limit = 200
		}
		items := s.diagnostics.Snapshot(filter, limit)
		return jsonResult(map[string]any{
			"generation": s.diagnostics.Generation(),
			"summary":    s.diagnostics.Summary(filter),
			"items":      items,
			"truncated":  len(items) == limit,
		})
	})
}

package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ruicat/astrbox/internal/diagnostics"
)

type statsArgs struct{}

func (s *Server) addStatsTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "stats",
		Description: "系统统计：实体（按类型）、规则（总数/启用/优先级）、模板、脚本。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args statsArgs) (*mcp.CallToolResult, any, error) {
		entities := s.store.ListAll()
		typeCounts := map[string]int{}
		for _, ent := range entities {
			typeCounts[ent.Type]++
		}
		typeRows := make([]map[string]any, 0, len(typeCounts))
		for typeName, count := range typeCounts {
			typeRows = append(typeRows, map[string]any{"type": typeName, "count": count})
		}
		sort.Slice(typeRows, func(i, j int) bool {
			return typeRows[i]["count"].(int) > typeRows[j]["count"].(int)
		})
		rules := s.machine.Rules()
		enabled := 0
		priorities := map[int]int{}
		for _, rule := range rules {
			if rule.Enabled {
				enabled++
			}
			priorities[rule.Priority]++
		}
		priorityRows := make([]map[string]any, 0, len(priorities))
		for priority, count := range priorities {
			priorityRows = append(priorityRows, map[string]any{"priority": priority, "count": count})
		}
		sort.Slice(priorityRows, func(i, j int) bool {
			return priorityRows[i]["count"].(int) > priorityRows[j]["count"].(int)
		})
		if len(priorityRows) > 5 {
			priorityRows = priorityRows[:5]
		}
		payload := map[string]any{
			"entities": len(entities), "entity_types": typeRows,
			"rules": len(rules), "rules_enabled": enabled, "rule_priorities": priorityRows,
			"templates": len(s.store.ListTemplates()), "scripts": len(s.registry.List()),
		}
		if s.diagnostics != nil {
			payload["diagnostics"] = s.diagnostics.Summary(diagnostics.Filter{})
		}
		return jsonResult(payload)
	})
}

type snapshotArgs struct {
	Name string `json:"name,omitempty" jsonschema:"快照文件名（不含路径与扩展名），留空用时间戳"`
}

func (s *Server) addSnapshotTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "entity_snapshot",
		Description: "把当前全部实体写入 <dataDir>/archives/<name>.json，返回文件路径。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args snapshotArgs) (*mcp.CallToolResult, any, error) {
		if s.dataDir == "" {
			return fail("未设置 data-dir，无法写快照")
		}
		name := args.Name
		if name == "" {
			name = "entity_snapshot_" + time.Now().UTC().Format("20060102T150405Z")
		}
		if err := validateSnapshotName(name); err != nil {
			return fail("非法快照名: %s", err)
		}
		entities := map[string]any{}
		for _, ent := range s.store.ListAll() {
			entities[ent.ID] = map[string]any{
				"id": ent.ID, "type": ent.Type, "keys": ent.Keys,
				"data": ent.Data, "_meta": ent.Meta,
			}
		}
		payload := map[string]any{
			"version": 1, "saved_at": time.Now().UTC().Format(time.RFC3339),
			"entities": entities,
		}
		raw, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fail("序列化失败: %s", err)
		}
		dir := filepath.Join(s.dataDir, "archives")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail("创建 archives 失败: %s", err)
		}
		path := filepath.Join(dir, name+".json")
		if err := atomicWriteFile(path, raw); err != nil {
			return fail("写快照失败: %s", err)
		}
		return jsonResult(map[string]any{
			"path": path, "entities": len(entities), "bytes": len(raw),
		})
	})
}

func validateSnapshotName(name string) error {
	for _, ch := range name {
		if ch == '/' || ch == '\\' || ch == '.' || ch == ' ' || ch == '\u0000' {
			return fmt.Errorf("名称不能包含路径分隔符、点或空格")
		}
	}
	if name == "" {
		return fmt.Errorf("名称不能为空")
	}
	return nil
}

func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type eventLogArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"返回最近多少条，默认 20"`
}

func (s *Server) addEventLogTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "event_log",
		Description: "查看最近的事件派发记录与 schema 校验失败（未接事件桥时为空）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args eventLogArgs) (*mcp.CallToolResult, any, error) {
		limit := args.Limit
		if limit <= 0 {
			limit = 20
		}
		payload := map[string]any{"validations": []any{}, "events": []any{}}
		if s.bridge != nil {
			payload["validations"] = s.bridge.LastValidations(limit)
			payload["events"] = s.bridge.LastRecords()
		}
		if s.eventBus != nil {
			payload["history"] = s.eventBus.History(limit)
		}
		return jsonResult(payload)
	})
}

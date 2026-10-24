package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type scriptApplyArgs struct {
	Name   string `json:"name" jsonschema:"脚本名（字母/数字/下划线/中文/连字符）"`
	Script string `json:"script" jsonschema:"完整的 .neko 脚本原文"`
}

type scriptNameArgs struct {
	Name string `json:"name" jsonschema:"脚本名"`
}

func (s *Server) addScriptTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_apply",
		Description: "解析并应用一段 .neko 脚本（模板 + 规则 + 函数 + 事件），增量注册。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args scriptApplyArgs) (*mcp.CallToolResult, any, error) {
		result := s.registry.Apply(args.Name, args.Script, false, false)
		return jsonResult(result)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_remove",
		Description: "移除已注册脚本并释放其模板与规则引用。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args scriptNameArgs) (*mcp.CallToolResult, any, error) {
		return jsonResult(s.registry.Remove(args.Name))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_list",
		Description: "列出已注册的脚本名。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return jsonResult(map[string]any{
			"scripts": s.registry.List(),
			"items":   s.registry.ListScripts(),
		})
	})
}

type inventoryArgs struct{}

func (s *Server) addInventoryTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "inventory",
		Description: "查看实体/模板/规则/脚本的库存统计。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args inventoryArgs) (*mcp.CallToolResult, any, error) {
		entities := make([]map[string]any, 0, s.store.Count())
		for _, ent := range s.store.ListAll() {
			entities = append(entities, map[string]any{"id": ent.ID, "type": ent.Type})
		}
		return jsonResult(map[string]any{
			"entities": entities, "entity_count": s.store.Count(),
			"templates": s.store.ListTemplates(), "template_count": len(s.store.ListTemplates()),
			"rules": s.machine.Rules(), "rule_count": len(s.machine.Rules()),
			"scripts": s.registry.List(), "script_count": len(s.registry.List()),
		})
	})
}

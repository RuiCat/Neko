package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type scriptReadArgs struct {
	Name string `json:"name" jsonschema:"脚本名"`
}

type scriptValidateArgs struct {
	Name   string `json:"name,omitempty" jsonschema:"脚本名（可选，用于安全检查上下文）"`
	Script string `json:"script" jsonschema:"完整的 .neko 脚本原文"`
}

type scriptWriteArgs struct {
	Name         string `json:"name" jsonschema:"脚本名"`
	Script       string `json:"script" jsonschema:"完整的 .neko 脚本原文"`
	ExpectedHash string `json:"expected_hash,omitempty" jsonschema:"当前脚本 hash；覆盖已有脚本时必填"`
}

// addScriptFileTools registers the transactional script-file tools.
func (s *Server) addScriptFileTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_read",
		Description: "读取脚本源码与注册元数据（只读）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args scriptReadArgs) (*mcp.CallToolResult, any, error) {
		view, err := s.registry.ReadScript(args.Name)
		if err != nil {
			return fail("%s", err.Error())
		}
		return jsonResult(view)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_validate",
		Description: "编译校验脚本但不注册、不落盘（dry-run）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args scriptValidateArgs) (*mcp.CallToolResult, any, error) {
		return jsonResult(s.registry.ValidateScript(args.Name, args.Script, false))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "neko_script_write",
		Description: "带 hash 校验地写入并注册脚本；编译失败不落盘。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args scriptWriteArgs) (*mcp.CallToolResult, any, error) {
		result := s.registry.WriteScript(args.Name, args.Script, args.ExpectedHash)
		s.registry.CheckScript(args.Name)
		return jsonResult(result)
	})
}

// context_list/context_send: model-facing cross-context messaging.
package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/driver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// CONTEXT_REFRESH_TIMEOUT bounds the session/list reconciliation.
const CONTEXT_REFRESH_TIMEOUT = 2 * time.Second

// contextSendArgs is the context_send tool input.
type contextSendArgs struct {
	// TargetContextID selects the receiving context.
	TargetContextID string `json:"target_context_id" jsonschema:"目标上下文 id（先调 context_list 查看）"`
	// Text is the message body.
	Text string `json:"text" jsonschema:"要投递的正文"`
	// Priority is normal (default) or urgent.
	Priority string `json:"priority,omitempty" jsonschema:"normal（默认）或 urgent；urgent 在对方思考中插入"`
	// SourceContextID names the sender when known.
	SourceContextID string `json:"source_context_id,omitempty" jsonschema:"来源上下文 id（可选；用于循环防护）"`
}

// addContextTools registers the cross-context tools; they answer a
// disabled error when the driver is not configured.
func (s *Server) addContextTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_list",
		Description: "列出 Neko 的角色上下文（contextId、角色、状态）；跨上下文发送前先看它。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		if s.contextDriver == nil {
			return fail("上下文驱动未启用（dsh-mcp 未配置 -contexts-config）")
		}
		refreshCtx, cancel := context.WithTimeout(ctx, CONTEXT_REFRESH_TIMEOUT)
		_ = s.contextDriver.RefreshStatus(refreshCtx)
		cancel()
		return jsonResult(map[string]any{"contexts": s.contextDriver.Snapshot()})
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_send",
		Description: "把一条消息投递到另一个角色上下文；有 60s 去重与每分钟 10 条上限。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args contextSendArgs) (*mcp.CallToolResult, any, error) {
		if s.contextDriver == nil {
			return fail("上下文驱动未启用（dsh-mcp 未配置 -contexts-config）")
		}
		result, err := s.contextDriver.Send(ctx, driver.SendRequest{
			SourceContextID: strings.TrimSpace(args.SourceContextID),
			TargetContextID: strings.TrimSpace(args.TargetContextID),
			Text:            args.Text,
			Priority:        strings.TrimSpace(args.Priority),
		})
		if err != nil {
			return fail("%s", err)
		}
		return jsonResult(result)
	})
}

// emoteTools.go exposes the emote lookup as the emote_find MCP tool. The tool
// always registers; a missing data directory becomes a ❌ answer on the first
// call instead of a startup failure.
package mcpserver

import (
	"context"
	"strings"

	"github.com/RuiCat/Neko/internal/emote"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// emoteFindArgs is the emote_find schema.
type emoteFindArgs struct {
	Text string `json:"text" jsonschema:"想表达的内容/情感，如 主人我好想你呀"`
	Top  int    `json:"top,omitempty" jsonschema:"返回数量（默认 3，最多 20）"`
}

// addEmoteTools registers emote_find.
func (s *Server) addEmoteTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "emote_find",
		Description: "表情查找工具：按想表达的内容语义匹配 QQ 表情库（向量检索 + 意图融合），" +
			"返回表情图片路径，可直接交给 qqbot_send_file 发送。" +
			`用法: emote_find(text="主人我好想你呀", top=3)`,
	}, func(ctx context.Context, req *mcp.CallToolRequest, args emoteFindArgs) (*mcp.CallToolResult, any, error) {
		text := strings.TrimSpace(args.Text)
		if text == "" {
			return fail("text 不能为空")
		}
		if s.emoteClient == nil {
			return fail("表情库未配置")
		}
		result, err := s.emoteClient.Find(ctx, text, args.Top)
		if err != nil {
			return fail("表情匹配失败: %s", err)
		}
		return jsonResult(result)
	})
}

// SetEmoteClient wires the emote lookup backend.
func (s *Server) SetEmoteClient(client *emote.Client) {
	s.emoteClient = client
}

// blackboardTool.go registers the MCP blackboard_get tool (K20).
package mcpserver

import (
	"context"
	"strings"

	"github.com/RuiCat/Neko/internal/eventboard"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type blackboardArgs struct {
	Entity string `json:"entity,omitempty" jsonschema:"实体ID；留空返回全部黑板"`
}

// addBlackboardTool registers the read-only event board tool.
func (s *Server) addBlackboardTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "blackboard_get",
		Description: "读取事件黑板：指定 entity 返回该实体黑板（无则 null）；留空返回全部（entity 升序）。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args blackboardArgs) (*mcp.CallToolResult, any, error) {
		entityID := strings.TrimSpace(args.Entity)
		if entityID != "" {
			var board *eventboard.Board
			if s.eventBoards != nil {
				board = s.eventBoards.Board(entityID)
			}
			return jsonResult(map[string]any{"ok": true, "board": board})
		}
		boards := []*eventboard.Board{}
		if s.eventBoards != nil {
			boards = s.eventBoards.Boards()
		}
		return jsonResult(map[string]any{"ok": true, "boards": boards})
	})
}

// observationTool.go registers the read-only observation_get tool.
package mcpserver

import (
	"context"
	"strings"

	"github.com/RuiCat/Neko/internal/observation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type observationArgs struct {
	Entity string `json:"entity,omitempty" jsonschema:"实体ID"`
	Scope  string `json:"scope,omitempty" jsonschema:"scope（优先于 entity），如 entity:<实体ID>"`
	Since  int64  `json:"since,omitempty" jsonschema:"只返回 seq > since 的变化；delta 模式使用"`
	Mode   string `json:"mode,omitempty" jsonschema:"delta（默认，变化流）或 state（当前值投影）"`
}

// addObservationTool registers the read-only observation reader.
func (s *Server) addObservationTool(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "observation_get",
		Description: "读取观察流：mode=delta 返回 since 之后值得推送的变化（服务端已按策略过滤），mode=state 返回各路径当前值。",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args observationArgs) (*mcp.CallToolResult, any, error) {
		scope := strings.TrimSpace(args.Scope)
		if scope == "" {
			entityID := strings.TrimSpace(args.Entity)
			if entityID == "" {
				return fail("需要 entity 或 scope")
			}
			scope = observation.ScopeForEntity(entityID)
		}
		bus := s.observations
		policy := s.ObservationPolicy()
		if strings.TrimSpace(args.Mode) == "state" {
			entries, version, truncated := observation.State(bus, scope, policy)
			return jsonResult(map[string]any{
				"ok": true, "scope": scope, "version": version,
				"entries": entries, "truncated": truncated,
			})
		}
		selection := observation.Select(bus, scope, args.Since, 0, policy)
		return jsonResult(map[string]any{
			"ok": true, "scope": scope, "version": selection.Version,
			"since": args.Since, "cursor": selection.Cursor,
			"events": selection.Events, "truncated": selection.Truncated,
			"dropped": selection.Dropped,
		})
	})
}

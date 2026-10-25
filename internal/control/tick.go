// POST /tick handler: one shared state-machine tick, skipping unchanged saves.
package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ruicat/astrbox/internal/engine"
	"github.com/ruicat/astrbox/internal/mcpserver"
)

// tickRequest is the POST /tick body; an absent body means an empty scope.
type tickRequest struct {
	Scope string `json:"scope"`
}

// handleTick runs one tick through the shared TryRunTick implementation so a
// tick already in flight (MCP tool or another control request) is refused with
// 409 instead of queueing. Unchanged ticks skip the store save; save failures
// answer 500 with dirty:true.
func (s *Server) handleTick(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := tickRequest{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return badRequest("请求体不是合法 JSON: " + err.Error())
		}
	}
	result, acquired, err := s.mcp.TryRunTick(request.Scope,
		mcpserver.TickOptions{SkipSaveWhenUnchanged: true})
	if !acquired {
		return tickBusy()
	}
	body := tickBody(result)
	if err != nil {
		body["ok"] = false
		body["dirty"] = true
		body["error"] = map[string]any{"code": CODE_INTERNAL, "message": err.Error()}
		return response{status: http.StatusInternalServerError, body: body,
			note: fmt.Sprintf("tickId=%s status=%s save_failed", result.TickID, result.Status)}
	}
	body["ok"] = true
	return response{status: http.StatusOK, body: body,
		note: fmt.Sprintf("tickId=%s status=%s transitions=%d conflicts=%d",
			result.TickID, result.Status, len(result.Transitions), len(result.Conflicts))}
}

// tickBody turns a TickResult into the K1 payload (tickId plus the unchanged
// MCP field names).
func tickBody(result engine.TickResult) map[string]any {
	return map[string]any{
		"tickId":          result.TickID,
		"tick_id":         result.TickID,
		"status":          result.Status,
		"transitions":     nonNil(result.Transitions),
		"conflicts":       nonNil(result.Conflicts),
		"errors":          nonNil(result.Errors),
		"hint":            result.ReplyHint,
		"rules_evaluated": result.RulesEvaluated,
	}
}

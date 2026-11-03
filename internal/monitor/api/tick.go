// POST /monitor/api/tick and GET /monitor/api/tick/latest: one shared state
// machine tick payload built by the single mcpserver.TickPayload constructor.
package api

import (
	"net/http"

	"github.com/RuiCat/Neko/internal/mcpserver"
)

// tickRequest is the POST /tick body; an absent body means an empty scope.
type tickRequest struct {
	Scope string `json:"scope"`
}

// handleTick runs one tick through the shared TryRunTick gate and never queues.
func (h *Handler) handleTick(w http.ResponseWriter, r *http.Request) {
	if !h.requireWritable(w) {
		return
	}
	request := tickRequest{}
	if err := DecodeBody(r, &request); err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	result, acquired, err := h.deps.MCP.TryRunTick(request.Scope,
		mcpserver.TickOptions{SkipSaveWhenUnchanged: true})
	if !acquired {
		WriteFailure(w, http.StatusConflict, CODE_TICK_BUSY, "已有 tick 正在执行，请稍后重试")
		return
	}
	body := mcpserver.TickPayload(result)
	body["ok"] = true
	if h.deps.Audit != nil {
		h.deps.Audit.Record("tick", "", request.Scope, auditResult(err), r.RemoteAddr)
	}
	if err != nil {
		body["ok"] = false
		body["dirty"] = true
		body["error"] = map[string]any{"code": CODE_INTERNAL, "message": err.Error()}
		WriteJSON(w, http.StatusInternalServerError, body)
		return
	}
	WriteJSON(w, http.StatusOK, body)
}

// handleLatestTick answers the most recent tick payload, if any.
func (h *Handler) handleLatestTick(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if h.deps.Tick != nil {
		if latest, ok := h.deps.Tick.LatestTickPayload(); ok {
			payload = latest
		}
	}
	WriteOK(w, map[string]any{"result": payload})
}

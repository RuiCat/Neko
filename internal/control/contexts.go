// Context routes: list driver records and deliver one Neko event into a
// character context.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/RuiCat/Neko/internal/driver"
)

const (
	// CODE_CONTEXTS_DISABLED marks context routes without a configured driver.
	CODE_CONTEXTS_DISABLED = "contexts_disabled"
	// CODE_DSH_FAILED marks a DSH call that failed or was rejected.
	CODE_DSH_FAILED = "dsh_failed"
	// CONTEXT_REFRESH_TIMEOUT bounds the session/list reconciliation.
	CONTEXT_REFRESH_TIMEOUT = 2 * time.Second
)

// contextInvokeRequest is the POST /context-invoke body.
type contextInvokeRequest struct {
	// ContextID selects the target context.
	ContextID string `json:"contextId"`
	// EventID is the idempotency key; empty generates one.
	EventID string `json:"eventId"`
	// Text is the event card delivered as the prompt body.
	Text string `json:"text"`
	// Priority is normal (default) or urgent.
	Priority string `json:"priority"`
	// Source labels the event origin.
	Source string `json:"source"`
}

// handleContextsList answers every context record after reconciling status
// from DSH; a failed reconciliation still answers the last known records.
func (s *Server) handleContextsList() response {
	if s.driver == nil {
		return failure(http.StatusServiceUnavailable, CODE_CONTEXTS_DISABLED,
			"上下文驱动未启用（启动时未配置 contexts.json）")
	}
	refreshed := true
	refreshCtx, cancel := context.WithTimeout(context.Background(), CONTEXT_REFRESH_TIMEOUT)
	if err := s.driver.RefreshStatus(refreshCtx); err != nil {
		refreshed = false
	}
	cancel()
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "refreshed": refreshed, "contexts": nonNil(s.driver.Snapshot()),
	}}
}

// handleContextInvoke delivers one event into a context.
func (s *Server) handleContextInvoke(r *http.Request) response {
	if s.driver == nil {
		return failure(http.StatusServiceUnavailable, CODE_CONTEXTS_DISABLED,
			"上下文驱动未启用（启动时未配置 contexts.json）")
	}
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := contextInvokeRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	result, err := s.driver.Invoke(r.Context(), driver.InvokeRequest{
		ContextID: request.ContextID,
		EventID:   request.EventID,
		Text:      request.Text,
		Priority:  request.Priority,
		Source:    request.Source,
	})
	if err != nil {
		var requestError *driver.RequestError
		if errors.As(err, &requestError) {
			return badRequest(requestError.Message)
		}
		return failure(http.StatusBadGateway, CODE_DSH_FAILED, err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "result": result,
	}}
}

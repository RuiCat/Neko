// Unified JSON response and error-code vocabulary of the control plane.
package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RuiCat/Neko/internal/sessionboard"
)

const (
	// CODE_BAD_REQUEST marks malformed JSON, parameters or event kinds.
	CODE_BAD_REQUEST = "bad_request"
	// CODE_NOT_FOUND marks unknown endpoints and unknown entities.
	CODE_NOT_FOUND = "not_found"
	// CODE_METHOD_NOT_ALLOWED marks a wrong HTTP method on a known endpoint.
	CODE_METHOD_NOT_ALLOWED = "method_not_allowed"
	// CODE_TICK_BUSY marks a second concurrent tick.
	CODE_TICK_BUSY = "tick_busy"
	// CODE_INTERNAL marks a failed write or save.
	CODE_INTERNAL = "internal"
	// CODE_CONFLICT marks an operation refused by a gate (repair regression).
	CODE_CONFLICT = "conflict"
	// CODE_SHUTTING_DOWN marks requests that arrive while the server exits.
	CODE_SHUTTING_DOWN = "shutting_down"
	// CODE_DEADLINE_EXCEEDED marks a request past the server-side deadline.
	CODE_DEADLINE_EXCEEDED = "deadline_exceeded"
	// CODE_FORBIDDEN marks a cross-workspace session board request (K9).
	CODE_FORBIDDEN = sessionboard.CODE_FORBIDDEN
	// CODE_BOARD_TOO_LARGE marks one session board past 256 KiB (K9).
	CODE_BOARD_TOO_LARGE = sessionboard.CODE_BOARD_TOO_LARGE
	// CODE_WORKSPACE_LIMIT marks the 129th workspace session board (K9).
	CODE_WORKSPACE_LIMIT = sessionboard.CODE_WORKSPACE_LIMIT
	// CODE_REGION_LIMIT is registered for a no-eviction store variant (K9).
	CODE_REGION_LIMIT = sessionboard.CODE_REGION_LIMIT
	// CODE_STORE_TOO_LARGE marks the session board store past 4 MiB (K9).
	CODE_STORE_TOO_LARGE = sessionboard.CODE_STORE_TOO_LARGE
	// CODE_PERSIST_FAILED marks a failed session board save (K9).
	CODE_PERSIST_FAILED = sessionboard.CODE_PERSIST_FAILED
	// CODE_INVALID_SCOPE, CODE_INVALID_OPS, CODE_INVALID_KEY, CODE_INVALID_VALUE
	// and CODE_TOO_MANY_OPS are the K5 subcodes; they travel as a message prefix
	// under CODE_BAD_REQUEST (K5 allows either form).
	CODE_INVALID_SCOPE = "invalid_scope"
	CODE_INVALID_OPS   = "invalid_ops"
	CODE_INVALID_KEY   = "invalid_key"
	CODE_INVALID_VALUE = "invalid_value"
	CODE_TOO_MANY_OPS  = "too_many_ops"
)

// response is one fully buffered reply so a late handler can never write twice.
type response struct {
	status  int
	body    map[string]any
	headers map[string]string
	note    string
}

// failure builds the {ok:false,error:{code,message}} envelope.
func failure(status int, code, message string) response {
	return response{status: status, body: map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message},
	}}
}

// badRequest reports malformed JSON, parameters or event kinds.
func badRequest(message string) response {
	return failure(http.StatusBadRequest, CODE_BAD_REQUEST, message)
}

// unknownEndpoint reports a path the control plane does not serve.
func unknownEndpoint(path string) response {
	return failure(http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+path)
}

// notFound reports a missing entity.
func notFound(message string) response {
	return failure(http.StatusNotFound, CODE_NOT_FOUND, message)
}

// methodNotAllowed reports a wrong method and advertises the allowed one.
func methodNotAllowed(allowed string) response {
	resp := failure(http.StatusMethodNotAllowed, CODE_METHOD_NOT_ALLOWED, "只支持 "+allowed)
	resp.headers = map[string]string{"Allow": allowed}
	return resp
}

// tickBusy reports a tick that is already in flight; ticks are never queued.
func tickBusy() response {
	return failure(http.StatusConflict, CODE_TICK_BUSY, "已有 tick 正在执行，请稍后重试")
}

// internalError reports a failed function call or store save.
func internalError(message string) response {
	return failure(http.StatusInternalServerError, CODE_INTERNAL, message)
}

// unavailable reports that the process is shutting down.
func unavailable() response {
	return failure(http.StatusServiceUnavailable, CODE_SHUTTING_DOWN, "服务正在退出")
}

// deadlineExceeded reports that the server-side processing limit was reached.
func deadlineExceeded(limit time.Duration) response {
	return failure(http.StatusGatewayTimeout, CODE_DEADLINE_EXCEEDED,
		fmt.Sprintf("处理超过服务端期限 %s", limit))
}

// writeResponse writes one buffered reply with its headers.
func writeResponse(w http.ResponseWriter, resp response) {
	for key, value := range resp.headers {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.status)
	_ = json.NewEncoder(w).Encode(resp.body)
}

// nonNil keeps JSON arrays as [] instead of null when nothing was produced.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

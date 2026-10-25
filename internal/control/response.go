// Unified JSON response and error-code vocabulary of the control plane.
package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
	// CODE_SHUTTING_DOWN marks requests that arrive while the server exits.
	CODE_SHUTTING_DOWN = "shutting_down"
	// CODE_DEADLINE_EXCEEDED marks a request past the server-side deadline.
	CODE_DEADLINE_EXCEEDED = "deadline_exceeded"
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

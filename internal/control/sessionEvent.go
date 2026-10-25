// POST /session-event handler: forwards DSH session events into .neko.
package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	// KIND_SESSION_CREATED marks a newly created DSH session.
	KIND_SESSION_CREATED = "session-created"
	// KIND_TURN_END marks the end of one agent turn.
	KIND_TURN_END = "turn-end"
)

// sessionEventRequest is the POST /session-event body (K1).
type sessionEventRequest struct {
	Kind      string `json:"kind"`
	SessionID string `json:"sessionId"`
	Cwd       string `json:"cwd"`
	Turn      *int   `json:"turn"`
	// Function optionally overrides the configured callback name; empty keeps
	// the server default (dsh_on_event by default).
	Function string `json:"function"`
}

// callbackName resolves the optional per-request function override.
func (request sessionEventRequest) callbackName(fallback string) (string, error) {
	if request.Function == "" {
		return fallback, nil
	}
	name := strings.TrimSpace(request.Function)
	if name == "" {
		return "", fmt.Errorf("function 不能为空白")
	}
	return name, nil
}

// validate rejects unknown kinds and missing required fields.
func (request sessionEventRequest) validate() error {
	if request.Kind != KIND_SESSION_CREATED && request.Kind != KIND_TURN_END {
		return fmt.Errorf("kind 必须是 %s 或 %s", KIND_SESSION_CREATED, KIND_TURN_END)
	}
	if strings.TrimSpace(request.SessionID) == "" {
		return fmt.Errorf("sessionId 不能为空")
	}
	if strings.TrimSpace(request.Cwd) == "" {
		return fmt.Errorf("cwd 不能为空")
	}
	if request.Kind == KIND_TURN_END && request.Turn == nil {
		return fmt.Errorf("turn-end 必须带 turn")
	}
	if request.Turn != nil && *request.Turn < 0 {
		return fmt.Errorf("turn 不能为负数")
	}
	return nil
}

// handleSessionEvent calls the registered .neko callback and saves under
// Server.WithStateWrite so it serializes with ticks and state sets. When the
// callback is not registered the event is only logged.
func (s *Server) handleSessionEvent(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := sessionEventRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	if err := request.validate(); err != nil {
		return badRequest(err.Error())
	}
	turn := 0
	if request.Turn != nil {
		turn = *request.Turn
	}
	functionName, err := request.callbackName(s.functionName)
	if err != nil {
		return badRequest(err.Error())
	}
	note := fmt.Sprintf("kind=%s session=%s function=%s", request.Kind, request.SessionID, functionName)
	if !s.runtime.Funcs.Has(functionName) {
		return response{status: http.StatusOK, body: map[string]any{
			"ok": true, "applied": "logged",
		}, note: note + " callback=missing"}
	}

	entity := s.runtime.Store.Get(s.entityID)
	writeErr := s.mcp.WithStateWrite(func() error {
		args := []any{request.Kind, request.SessionID, request.Cwd, turn}
		if _, err := s.runtime.Funcs.Call(functionName, args, entity); err != nil {
			return err
		}
		return s.runtime.Store.Save()
	})
	if writeErr != nil {
		return response{status: http.StatusInternalServerError,
			body: map[string]any{"ok": false, "applied": "function",
				"error": map[string]any{"code": CODE_INTERNAL, "message": writeErr.Error()}},
			note: note + " callback=failed"}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "applied": "function",
	}, note: note + " callback=ok"}
}

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
	// CODE_ENTITY_NOT_FOUND marks a session-event entity that does not exist;
	// the event is rejected before any session or state side effect.
	CODE_ENTITY_NOT_FOUND = "entity_not_found"
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
	// Entity optionally selects the entity the event is applied to; empty or
	// blank keeps the server default entity, so old clients keep the old
	// behaviour (K3).
	Entity string `json:"entity"`
	// Summary is the optional turn digest the bridge reports with turn-end.
	Summary *sessionEventSummary `json:"summary"`
}

// sessionEventSummary is the optional turn digest from the bridge.
type sessionEventSummary struct {
	// AssistantDigest is the last assistant text of the turn, truncated.
	AssistantDigest string `json:"assistantDigest"`
	// UserDigest is the last user text of the turn, truncated.
	UserDigest string `json:"userDigest"`
	// ToolCalls counts the turn's tool calls.
	ToolCalls int `json:"toolCalls"`
	// Steps counts the turn's steps.
	Steps int `json:"steps"`
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

// effectiveEntityID resolves the entity one event applies to. An explicit id
// must exist, so an unknown one is rejected here — after request validation,
// before the callback-missing early return and before any session or driver
// side effect — because the event must not reach callbacks or disk for an
// entity that cannot be resolved (K3). An absent or blank id keeps the server
// default and its legacy existence check at the callback path, so old clients
// keep the old behaviour.
func (s *Server) effectiveEntityID(request sessionEventRequest) (string, *response) {
	requested := strings.TrimSpace(request.Entity)
	if requested == "" {
		return s.entityID, nil
	}
	if s.runtime.Store.Get(requested) == nil {
		rejected := failure(http.StatusNotFound, CODE_ENTITY_NOT_FOUND,
			fmt.Sprintf("实体 '%s' 不存在", requested))
		return "", &rejected
	}
	return requested, nil
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
	effectiveEntity, rejected := s.effectiveEntityID(request)
	if rejected != nil {
		return *rejected
	}
	// Remember the session so turn records written by later ticks carry it.
	s.mcp.SetSessionID(request.SessionID)
	turn := 0
	if request.Turn != nil {
		turn = *request.Turn
	}
	functionName, err := request.callbackName(s.functionName)
	if err != nil {
		return badRequest(err.Error())
	}
	note := fmt.Sprintf("kind=%s session=%s function=%s", request.Kind, request.SessionID, functionName)
	if request.Summary != nil {
		note += fmt.Sprintf(" summary=tools:%d,steps:%d", request.Summary.ToolCalls, request.Summary.Steps)
	}
	// The driver owns context status; update it even when no .neko callback
	// is registered so a finished turn always releases the context.
	if s.driver != nil && request.Kind == KIND_TURN_END {
		if driverErr := s.driver.HandleTurnEnd(request.SessionID, turn); driverErr != nil {
			note += " driver=failed"
		} else {
			note += " driver=ok"
		}
	}
	if !s.runtime.Funcs.Has(functionName) {
		s.recordDSHTurn(request, effectiveEntity)
		return response{status: http.StatusOK, body: map[string]any{
			"ok": true, "applied": "logged",
		}, note: note + " callback=missing"}
	}

	entity := s.runtime.Store.Get(effectiveEntity)
	if entity == nil {
		return notFound(fmt.Sprintf("实体 '%s' 不存在", effectiveEntity))
	}
	writeErr := s.mcp.WithStateWrite(func() error {
		args := []any{request.Kind, request.SessionID, request.Cwd, turn}
		// The summary travels as a JSON string so existing 4-parameter
		// callbacks keep working and scripts can parse it when they want it.
		if request.Summary != nil {
			if raw, marshalErr := json.Marshal(request.Summary); marshalErr == nil {
				args = append(args, string(raw))
			}
		}
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
	s.recordDSHTurn(request, effectiveEntity)
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "applied": "function",
	}, note: note + " callback=ok"}
}

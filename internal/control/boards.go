// boards.go serves GET/POST /boards: the read and write endpoints of the
// session boards (K4/K5). Both reuse the control-plane envelope and error-code
// vocabulary registered in response.go (K9).
package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/RuiCat/Neko/internal/sessionboard"
)

// boardWriteRequest is the POST /boards body (K5). Session and workspace are
// filled by the bridge from exec.agent; the model cannot set them through the
// tool, and no region field exists at all (K2/K7).
type boardWriteRequest struct {
	Scope     string            `json:"scope"`
	Workspace string            `json:"workspace"`
	Session   string            `json:"session"`
	Ops       []sessionboard.Op `json:"ops"`
}

// handleBoardsGet serves one board snapshot plus the deltas after a cursor
// (K4). It is read-only and never 404s: an unknown workspace is an empty board.
func (s *Server) handleBoardsGet(r *http.Request) response {
	query := r.URL.Query()
	scope := strings.TrimSpace(query.Get("scope"))
	if scope == "" {
		return badRequest("缺少 scope（workspace 或 global）")
	}
	if scope != sessionboard.SCOPE_WORKSPACE && scope != sessionboard.SCOPE_GLOBAL {
		return badRequest("scope 必须是 workspace 或 global")
	}
	workspace := query.Get("workspace")
	if scope == sessionboard.SCOPE_WORKSPACE && workspace == "" {
		return badRequest("workspace 板读取缺少 workspace")
	}
	since, err := parseBoardSince(query.Get("since"))
	if err != nil {
		return badRequest(err.Error())
	}
	store := s.sessionBoardStore()
	if store == nil {
		return emptyBoardResponse(scope, workspace, since)
	}
	result, err := store.Read(sessionboard.ReadQuery{
		Scope:     scope,
		Workspace: workspace,
		// The bridge's workspace claim is trusted here (C8/D2); the store still
		// enforces it against the board's own scope id.
		Caller: sessionboard.Caller{WorkspaceKey: workspace},
		Since:  since,
	})
	if err != nil {
		return boardErrorResponse(err)
	}
	body := map[string]any{"ok": true, "board": result.Board, "truncated": result.Truncated}
	if since != nil {
		body["since"] = *since
		body["deltas"] = nonNil(result.Deltas)
	}
	return response{status: http.StatusOK, body: body,
		note: boardReadNote(scope, workspace, since, result)}
}

// handleBoardsWrite applies one write batch to the caller's own region (K5).
func (s *Server) handleBoardsWrite(r *http.Request) response {
	request := boardWriteRequest{}
	if err := decodeBoardBody(r, &request); err != nil {
		return badRequest(err.Error())
	}
	store := s.sessionBoardStore()
	if store == nil {
		return internalError("会话黑板未初始化")
	}
	result, err := store.Write(sessionboard.WriteRequest{
		Scope:     request.Scope,
		Workspace: request.Workspace,
		Caller:    sessionboard.Caller{SessionID: request.Session, WorkspaceKey: request.Workspace},
		Ops:       request.Ops,
	})
	if err != nil {
		return boardErrorResponse(err)
	}
	return response{status: http.StatusOK, body: boardWriteBody(result),
		note: boardWriteNote(request, result)}
}

// panelWriteRequest is the POST /boards/panel body (K13): the panel names the
// board and the region explicitly instead of deriving them from a caller, and
// its writes are always attributed to the admin surface ("面板").
type panelWriteRequest struct {
	Scope     string            `json:"scope"`
	Workspace string            `json:"workspace"`
	Region    string            `json:"region"`
	Ops       []sessionboard.Op `json:"ops"`
}

// handleBoardsPanelWrite applies one admin write to the named region (K13). The
// region is created on first write, created entries carry by="面板", and an
// invalid request never changes state.
func (s *Server) handleBoardsPanelWrite(r *http.Request) response {
	request := panelWriteRequest{}
	if err := decodeBoardBody(r, &request); err != nil {
		return badRequest(err.Error())
	}
	store := s.sessionBoardStore()
	if store == nil {
		return internalError("会话黑板未初始化")
	}
	result, err := store.WritePanel(sessionboard.PanelWriteRequest{Scope: request.Scope,
		Workspace: request.Workspace, Region: request.Region, Ops: request.Ops})
	if err != nil {
		return boardErrorResponse(err)
	}
	return response{status: http.StatusOK, body: boardWriteBody(result),
		note: panelWriteNote(request, result)}
}

// handleBoardsWorkspaces lists every workspace board as a summary (K14). It is
// read-only, exposes no entry contents and never 404s: without a store it
// answers the empty list.
func (s *Server) handleBoardsWorkspaces() response {
	store := s.sessionBoardStore()
	if store == nil {
		return response{status: http.StatusOK, body: map[string]any{"ok": true,
			"workspaces": []sessionboard.WorkspaceSummary{}}, note: "boards-workspaces=no-store"}
	}
	workspaces := store.ListWorkspaces()
	return response{status: http.StatusOK,
		body: map[string]any{"ok": true, "workspaces": workspaces},
		note: "boards-workspaces count=" + strconv.Itoa(len(workspaces))}
}

// decodeBoardBody reads one /boards JSON body (K5/K13) into target: an empty
// body and invalid JSON both become bad-request messages.
func decodeBoardBody(r *http.Request, target any) error {
	raw, err := readBody(r)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("请求体不能为空")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return errors.New("请求体不是合法 JSON: " + err.Error())
	}
	return nil
}

// sessionBoardStore returns the runtime's session board store, or nil when the
// runtime was assembled without one (tests and embedders).
func (s *Server) sessionBoardStore() *sessionboard.Store {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.SessionBoards
}

// parseBoardSince parses the optional non-negative since cursor (K4).
func parseBoardSince(raw string) (*int, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return nil, errors.New("since 必须是非负整数")
	}
	return &value, nil
}

// boardErrorResponse maps one sessionboard error onto the control-plane
// envelope and HTTP status (K9).
func boardErrorResponse(err error) response {
	var boardErr *sessionboard.Error
	if !errors.As(err, &boardErr) {
		return internalError(err.Error())
	}
	switch boardErr.Code {
	case sessionboard.CODE_FORBIDDEN:
		return failure(http.StatusForbidden, CODE_FORBIDDEN, boardErr.Message)
	case sessionboard.CODE_PERSIST_FAILED:
		return failure(http.StatusInternalServerError, CODE_PERSIST_FAILED, boardErr.Message)
	default:
		return failure(http.StatusBadRequest, boardErr.Code, boardErr.Message)
	}
}

// emptyBoardResponse is the version-0 answer used when no store is wired (K4).
func emptyBoardResponse(scope, workspace string, since *int) response {
	body := map[string]any{"ok": true, "board": sessionboard.EmptyBoard(scope, workspace),
		"truncated": false}
	if since != nil {
		body["since"] = *since
		body["deltas"] = []sessionboard.Delta{}
	}
	return response{status: http.StatusOK, body: body, note: "boards=no-store"}
}

// boardReadNote summarizes one read for the control log.
func boardReadNote(scope, workspace string, since *int, result sessionboard.ReadResult) string {
	note := "boards scope=" + scope
	if workspace != "" {
		note += " workspace=" + workspace
	}
	note += " version=" + strconv.Itoa(result.Board.Version)
	if since != nil {
		note += " since=" + strconv.Itoa(*since) + " deltas=" + strconv.Itoa(len(result.Deltas))
	}
	return note
}

// boardWriteBody is the K5/K13 write answer: changed, the resulting version, the
// trimmed/evicted reports and, only when something changed, the update stamp.
func boardWriteBody(result sessionboard.WriteResult) map[string]any {
	body := map[string]any{"ok": true, "changed": result.Changed, "version": result.Version,
		"trimmed": nonNil(result.Trimmed), "evicted": nonNil(result.Evicted)}
	if result.Changed {
		body["updated_at"] = result.UpdatedAt
	}
	return body
}

// boardWriteNote summarizes one write for the control log.
func boardWriteNote(request boardWriteRequest, result sessionboard.WriteResult) string {
	return "boards-write scope=" + request.Scope + " session=" + request.Session +
		" changed=" + strconv.FormatBool(result.Changed) + " version=" + strconv.Itoa(result.Version)
}

// panelWriteNote summarizes one panel write for the control log.
func panelWriteNote(request panelWriteRequest, result sessionboard.WriteResult) string {
	return "boards-panel scope=" + request.Scope + " workspace=" + request.Workspace +
		" region=" + request.Region + " changed=" + strconv.FormatBool(result.Changed) +
		" version=" + strconv.Itoa(result.Version)
}

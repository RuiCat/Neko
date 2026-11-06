// GET /blackboard handler: one entity's event board or every board (K3).
package control

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/eventboard"
)

// handleBlackboard serves the read-only change boards. An unknown entity is an
// empty result with 200 and board:null (never 404), because the bridge and the
// panel both treat "no board" as an empty card. The whole list is sorted by
// entity, which the registry already guarantees.
func (s *Server) handleBlackboard(r *http.Request) response {
	entityID := strings.TrimSpace(r.URL.Query().Get("entity"))
	registry := s.eventBoardRegistry()
	if entityID != "" {
		var board *eventboard.Board
		if registry != nil {
			board = registry.Board(entityID)
		}
		if board == nil {
			return response{status: http.StatusOK, body: map[string]any{"ok": true, "board": nil},
				note: fmt.Sprintf("entity=%s board=none", entityID)}
		}
		return response{status: http.StatusOK, body: map[string]any{"ok": true, "board": board},
			note: fmt.Sprintf("entity=%s version=%d entries=%d", board.Entity, board.Version, len(board.Entries))}
	}
	boards := []*eventboard.Board{}
	if registry != nil {
		boards = registry.Boards()
	}
	return response{status: http.StatusOK, body: map[string]any{"ok": true, "boards": boards},
		note: fmt.Sprintf("boards=%d", len(boards))}
}

// eventBoardRegistry returns the runtime's shared board registry, or nil when
// the runtime was assembled without one (tests and embedders).
func (s *Server) eventBoardRegistry() *eventboard.Registry {
	if s.runtime == nil {
		return nil
	}
	return s.runtime.EventBoards
}

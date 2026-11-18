// Endpoint routing for the control plane; each handler lives in its own file.
package control

import "net/http"

// dispatch maps one HTTP request to its handler and enforces the method.
func (s *Server) dispatch(r *http.Request) response {
	switch r.URL.Path {
	case "/health":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleHealth()
	case "/state":
		switch r.Method {
		case http.MethodGet:
			return s.handleState(r)
		case http.MethodPost:
			return s.handleStateWrite(r)
		default:
			return methodNotAllowed(http.MethodGet + ", " + http.MethodPost)
		}
	case "/clock":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleClock(r)
	case "/entities/delete":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleEntityDelete(r)
	case "/entities/create":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleEntityCreate(r)
	case "/entities":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleEntities()
	case "/v2/observations":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleObservations(r)
	case "/v2/observations/state":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleObservationState(r)
	case "/v2/observations/streams":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleObservationStreams(r)
	case "/boards":
		switch r.Method {
		case http.MethodGet:
			return s.handleBoardsGet(r)
		case http.MethodPost:
			return s.handleBoardsWrite(r)
		default:
			return methodNotAllowed(http.MethodGet + ", " + http.MethodPost)
		}
	case "/boards/panel":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleBoardsPanelWrite(r)
	case "/boards/workspaces":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleBoardsWorkspaces()
	case "/tick":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleTick(r)
	case "/turn-context":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleTurnContext(r)
	case "/nightly":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleNightly(r)
	case "/judge":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleJudge(r)
	case "/candidates":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleCandidates(r)
	case "/candidates/resolve":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleResolveCandidate(r)
	case "/candidates/apply":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleApplyCandidate(r)
	case "/repairs/rollback":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleRollbackRepair(r)
	case "/teacher-signal":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleTeacherSignal(r)
	case "/session-event":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleSessionEvent(r)
	case "/contexts":
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleContextsList()
	case "/context-invoke":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleContextInvoke(r)
	default:
		return unknownEndpoint(r.URL.Path)
	}
}

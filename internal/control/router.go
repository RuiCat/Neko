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
		if r.Method != http.MethodGet {
			return methodNotAllowed(http.MethodGet)
		}
		return s.handleState(r)
	case "/tick":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleTick(r)
	case "/session-event":
		if r.Method != http.MethodPost {
			return methodNotAllowed(http.MethodPost)
		}
		return s.handleSessionEvent(r)
	default:
		return unknownEndpoint(r.URL.Path)
	}
}

// Routing for /monitor/api/*: read endpoints, tick and entity-field writes.
package api

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/mcpserver"
	"github.com/RuiCat/Neko/internal/monitor/auth"
)

// PREFIX is the frozen API prefix from K1.
const PREFIX = "/monitor/api"

// TickSource exposes the latest tick payload built by the single constructor.
type TickSource interface {
	// LatestTickPayload returns the last tick payload and whether one exists.
	LatestTickPayload() (map[string]any, bool)
}

// Deps is everything the API handler reads; it never mutates the runtime
// except through the shared mcpserver gates.
type Deps struct {
	Runtime   *bootstrap.Runtime
	MCP       *mcpserver.Server
	ReadOnly  bool
	Version   string
	StartedAt time.Time
	Tick      TickSource
	Audit     *auth.Audit
}

// Handler answers the /monitor/api/* endpoints.
type Handler struct {
	deps Deps
}

// New builds the API handler around an assembled runtime.
func New(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// ServeHTTP dispatches one API request.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := apiPath(r)
	if len(parts) == 0 {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
		return
	}
	switch parts[0] {
	case "health":
		h.routeGET(w, r, h.handleHealth, parts[1:])
	case "entities":
		h.routeEntities(w, r, parts[1:])
	case "diagnostics":
		h.routeGET(w, r, h.handleDiagnostics, parts[1:])
	case "scripts":
		h.routeScripts(w, r, parts[1:])
	case "logs":
		h.routeGET(w, r, h.handleLogs, parts[1:])
	case "tick":
		h.routeTick(w, r, parts[1:])
	default:
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
	}
}

// routeGET enforces GET on a leaf handler.
func (h *Handler) routeGET(w http.ResponseWriter, r *http.Request, handler func(http.ResponseWriter, *http.Request), rest []string) {
	if len(rest) != 0 {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
		return
	}
	if r.Method != http.MethodGet {
		WriteFailure(w, http.StatusMethodNotAllowed, CODE_BAD_REQUEST, "只支持 GET")
		return
	}
	handler(w, r)
}

// routeEntities handles /entities, /entities/{id} and /entities/{id}/set.
func (h *Handler) routeEntities(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 0:
		h.routeGET(w, r, h.handleEntityList, nil)
	case len(parts) == 1:
		if r.Method != http.MethodGet {
			WriteFailure(w, http.StatusMethodNotAllowed, CODE_BAD_REQUEST, "只支持 GET")
			return
		}
		h.handleEntity(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "set":
		if r.Method != http.MethodPost {
			WriteFailure(w, http.StatusMethodNotAllowed, CODE_BAD_REQUEST, "只支持 POST")
			return
		}
		h.handleSet(w, r, parts[0])
	default:
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
	}
}

// routeScripts handles /scripts and /scripts/{name}.
func (h *Handler) routeScripts(w http.ResponseWriter, r *http.Request, parts []string) {
	if len(parts) == 0 {
		h.routeGET(w, r, h.handleScriptList, nil)
		return
	}
	if len(parts) != 1 {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
		return
	}
	if r.Method != http.MethodGet {
		WriteFailure(w, http.StatusMethodNotAllowed, CODE_BAD_REQUEST, "只支持 GET")
		return
	}
	h.handleScript(w, r, parts[0])
}

// routeTick handles POST /tick and GET /tick/latest.
func (h *Handler) routeTick(w http.ResponseWriter, r *http.Request, parts []string) {
	switch {
	case len(parts) == 0:
		if r.Method != http.MethodPost {
			WriteFailure(w, http.StatusMethodNotAllowed, CODE_BAD_REQUEST, "只支持 POST")
			return
		}
		h.handleTick(w, r)
	case len(parts) == 1 && parts[0] == "latest":
		h.routeGET(w, r, h.handleLatestTick, nil)
	default:
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "未知端点 "+r.URL.Path)
	}
}

// requireWritable refuses writes in the no-token read-only shape.
func (h *Handler) requireWritable(w http.ResponseWriter) bool {
	if !h.deps.ReadOnly {
		return true
	}
	WriteFailure(w, http.StatusForbidden, CODE_WRITE_DISABLED, "未配置 token，监视器为只读")
	return false
}

// apiPath splits the escaped API path into decoded segments.
func apiPath(r *http.Request) []string {
	escaped := strings.TrimPrefix(r.URL.EscapedPath(), PREFIX)
	trimmed := strings.Trim(escaped, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	for index, part := range parts {
		if decoded, err := url.PathUnescape(part); err == nil {
			parts[index] = decoded
		}
	}
	return parts
}

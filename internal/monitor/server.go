// Package monitor serves the local web monitor inside bin/dsh-mcp: /monitor/
// static assets, /monitor/api/* REST and /monitor/ws, all over one runtime.
package monitor

import (
	"embed"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/RuiCat/Neko/internal/bootstrap"
	"github.com/RuiCat/Neko/internal/entity"
	"github.com/RuiCat/Neko/internal/mcpserver"
	"github.com/RuiCat/Neko/internal/monitor/api"
	"github.com/RuiCat/Neko/internal/monitor/auth"
	"github.com/RuiCat/Neko/internal/monitor/ws"
)

// webTree is the versioned page tree written by m5/m6 (K6 go:embed).
//
//go:embed web
var webTree embed.FS

const (
	// PREFIX is the page/static prefix.
	PREFIX = "/monitor"
	// API_PREFIX is the REST prefix from K1.
	API_PREFIX = api.PREFIX
)

// Options configures one monitor server; empty values keep the safe defaults.
type Options struct {
	// Token protects /monitor/api/* and /monitor/ws; empty means read-only.
	Token string
	// Version is reported by health and the WS hello frame.
	Version string
	// WebFS is the embedded page tree (K6 go:embed wiring lands with m6).
	WebFS fs.FS
	// AuditKeepDays overrides the 7-day audit retention.
	AuditKeepDays int
}

// Server is the /monitor HTTP handler and the owner of the process-wide
// observe hooks (tick, diagnostics, entity create/change).
type Server struct {
	runtime   *bootstrap.Runtime
	mcp       *mcpserver.Server
	api       *api.Handler
	hub       *ws.Hub
	token     string
	webFS     fs.FS
	startedAt time.Time
	closing   atomic.Bool
}

// New assembles the monitor server and installs every observe hook exactly once.
func New(runtime *bootstrap.Runtime, mcp *mcpserver.Server, options Options) *Server {
	startedAt := time.Now()
	readOnly := options.Token == ""
	webFS := options.WebFS
	if webFS == nil {
		webFS = embeddedPage()
	}
	hub := ws.NewHub(ws.Deps{
		Store: runtime.Store, Diagnostics: runtime.Diagnostics,
		ReadOnly: readOnly, Version: options.Version,
	})
	audit := auth.NewAudit(filepath.Join(runtime.DataDir, "logs"), options.AuditKeepDays)
	server := &Server{
		runtime: runtime, mcp: mcp,
		api: api.New(api.Deps{
			Runtime: runtime, MCP: mcp, ReadOnly: readOnly, Version: options.Version,
			StartedAt: startedAt, Tick: hub, Audit: audit,
		}),
		hub: hub, token: options.Token, webFS: webFS, startedAt: startedAt,
	}
	server.wireHooks()
	return server
}

// embeddedPage returns the compiled-in web subtree, or nil when the tree is
// empty or unreadable.
func embeddedPage() fs.FS {
	sub, err := fs.Sub(webTree, "web")
	if err != nil {
		return nil
	}
	return sub
}

// wireHooks installs the process-wide observers. mcpserver and diagnostics
// define the hook types themselves, so they never import this package.
func (s *Server) wireHooks() {
	s.mcp.SetTickObserver(s.hub.PublishTick)
	s.runtime.Diagnostics.SetNotify(s.hub.PublishDiagnostic)
	listener := func(ent *entity.Entity, path string, oldValue, newValue any) {
		s.hub.PublishDelta(ent.ID, path, oldValue, newValue)
	}
	previous := s.runtime.Store.OnCreate
	s.runtime.Store.OnCreate = func(ent *entity.Entity) {
		if previous != nil {
			previous(ent)
		}
		ent.AddChangeListener(listener)
	}
	for _, ent := range s.runtime.Store.ListAll() {
		ent.AddChangeListener(listener)
	}
}

// ServeHTTP enforces the Host allowlist on every /monitor request, then routes
// WS before the REST token middleware, API behind it and static without a token.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !auth.LocalAddrAllowed(r) || !auth.HostAllowed(r.Host) {
		auth.WriteError(w, http.StatusForbidden, auth.CODE_FORBIDDEN, "Host 或连接地址不在回环白名单")
		return
	}
	path := r.URL.Path
	switch {
	case path == ws.WS_PATH:
		s.hub.Serve(w, r, s.token)
	case path == PREFIX:
		http.Redirect(w, r, PREFIX+"/", http.StatusMovedPermanently)
	case path == API_PREFIX || strings.HasPrefix(path, API_PREFIX+"/"):
		auth.Middleware(s.api, s.token).ServeHTTP(w, r)
	case strings.HasPrefix(path, PREFIX+"/"):
		s.serveStatic(w, r, path)
	default:
		api.WriteFailure(w, http.StatusNotFound, api.CODE_NOT_FOUND, "未知端点 "+path)
	}
}

// serveStatic serves one embedded page asset; an unconfigured tree answers 404
// until the m6 page is embedded.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, path string) {
	if s.webFS == nil {
		api.WriteFailure(w, http.StatusNotFound, api.CODE_NOT_FOUND, "监视器页面尚未内嵌")
		return
	}
	name := strings.TrimPrefix(path, PREFIX+"/")
	if name == "" {
		name = "index.html"
	}
	if strings.Contains(name, "..") {
		api.WriteFailure(w, http.StatusNotFound, api.CODE_NOT_FOUND, "非法静态资源路径")
		return
	}
	http.ServeFileFS(w, r, s.webFS, name)
}

// Close stops accepting work and force-closes every live WebSocket connection.
func (s *Server) Close() {
	if s.closing.Swap(true) {
		return
	}
	s.hub.CloseAll()
}

// StartedAt is the process-assembly time reported by health.
func (s *Server) StartedAt() time.Time { return s.startedAt }

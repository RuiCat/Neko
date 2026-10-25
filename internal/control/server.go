// Package control serves the local DSH control plane over a Unix domain socket:
// health, state snapshots, ticks and session events.
package control

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/ruicat/astrbox/internal/bootstrap"
	"github.com/ruicat/astrbox/internal/mcpserver"
)

const (
	// CONTROL_VERSION is reported by GET /health.
	CONTROL_VERSION = "1.0.0"
	// DEFAULT_ENTITY_ID is the entity session-event callbacks resolve against.
	DEFAULT_ENTITY_ID = "猫猫"
	// DEFAULT_FUNCTION_NAME is the .neko function invoked for session events.
	DEFAULT_FUNCTION_NAME = "dsh_on_event"
	// DEFAULT_DEADLINE is the server-side processing limit per request.
	DEFAULT_DEADLINE = 5 * time.Second
)

// Options overrides the control-plane defaults. Empty values keep the defaults.
type Options struct {
	// Entity is the entity session-event callbacks run against.
	Entity string
	// FunctionName is the registered .neko callback for session events.
	FunctionName string
	// Deadline bounds one request server-side (504 beyond it).
	Deadline time.Duration
}

// Server answers HTTP/1.1 control-plane requests on a Unix socket.
type Server struct {
	runtime      *bootstrap.Runtime
	mcp          *mcpserver.Server
	dataDir      string
	scriptsDir   string
	entityID     string
	functionName string
	deadline     time.Duration
	startedAt    time.Time
	log          *dailyLogger
	closing      atomic.Bool
	httpServer   *http.Server
}

// New assembles a control-plane server around an initialized runtime.
func New(runtime *bootstrap.Runtime, mcp *mcpserver.Server, options Options) *Server {
	server := &Server{
		runtime:      runtime,
		mcp:          mcp,
		dataDir:      runtime.DataDir,
		scriptsDir:   runtime.ScriptsDir,
		entityID:     options.Entity,
		functionName: options.FunctionName,
		deadline:     options.Deadline,
		startedAt:    time.Now(),
	}
	if server.entityID == "" {
		server.entityID = DEFAULT_ENTITY_ID
	}
	if server.functionName == "" {
		server.functionName = DEFAULT_FUNCTION_NAME
	}
	if server.deadline <= 0 {
		server.deadline = DEFAULT_DEADLINE
	}
	server.log = newDailyLogger(filepath.Join(server.dataDir, "logs"), CONTROL_LOG_KEEP_DAYS)
	server.httpServer = &http.Server{Handler: server, ReadHeaderTimeout: DEFAULT_DEADLINE}
	return server
}

// Serve answers requests on listener until Shutdown is called.
func (s *Server) Serve(listener net.Listener) error {
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops accepting requests and waits for in-flight handlers.
func (s *Server) Shutdown(ctx context.Context) error {
	s.closing.Store(true)
	return s.httpServer.Shutdown(ctx)
}

// ServeHTTP enforces the shutdown flag and the server-side deadline, then logs
// the request to the daily control log and stderr.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	resp := s.dispatchWithinDeadline(r)
	writeResponse(w, resp)
	s.logRequest(r, resp, time.Since(started))
}

// dispatchWithinDeadline runs one handler under the server-side deadline so a
// blocked tick still answers 504 instead of holding the socket open.
func (s *Server) dispatchWithinDeadline(r *http.Request) response {
	if s.closing.Load() {
		return unavailable()
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.deadline)
	defer cancel()
	result := make(chan response, 1)
	go func() { result <- s.dispatch(r) }()
	select {
	case resp := <-result:
		return resp
	case <-ctx.Done():
		return deadlineExceeded(s.deadline)
	}
}

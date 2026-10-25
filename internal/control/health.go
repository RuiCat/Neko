// GET /health handler: process identity and runtime counters.
package control

import (
	"net/http"
	"os"
	"time"
)

// handleHealth reports the process identity, data locations and uptime.
func (s *Server) handleHealth() response {
	return response{status: http.StatusOK, body: map[string]any{
		"ok":          true,
		"version":     CONTROL_VERSION,
		"pid":         os.Getpid(),
		"dataDir":     s.dataDir,
		"scriptsDir":  s.scriptsDir,
		"entityCount": s.runtime.Store.Count(),
		"uptimeMs":    time.Since(s.startedAt).Milliseconds(),
	}}
}

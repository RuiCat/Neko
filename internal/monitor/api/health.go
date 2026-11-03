// GET /monitor/api/health: process identity, runtime counters and shape flags.
package api

import (
	"net/http"
	"os"
	"time"

	"github.com/RuiCat/Neko/internal/diagnostics"
)

// handleHealth reports the K1 health payload. In the no-token read-only shape
// the host filesystem paths and PID are redacted so an unauthenticated reader
// does not learn where data lives.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	runtime := h.deps.Runtime
	summary := runtime.Diagnostics.Summary(diagnostics.Filter{})
	payload := map[string]any{
		"version":  h.deps.Version,
		"uptimeMs": time.Since(h.deps.StartedAt).Milliseconds(),
		"engine": map[string]any{
			"entities":  runtime.Store.Count(),
			"templates": len(runtime.Store.ListTemplates()),
			"rules":     len(runtime.Machine.Rules()),
			"scripts":   len(runtime.Registry.List()),
		},
		"diagnostics": map[string]any{
			"generation": runtime.Diagnostics.Generation(),
			"total":      summary.Total,
			"error":      summary.Error,
			"warning":    summary.Warning,
			"info":       summary.Info,
		},
		"monitor": map[string]any{"readOnly": h.deps.ReadOnly},
	}
	if !h.deps.ReadOnly {
		payload["pid"] = os.Getpid()
		payload["dataDir"] = runtime.DataDir
		payload["scriptsDir"] = runtime.ScriptsDir
	}
	WriteOK(w, payload)
}

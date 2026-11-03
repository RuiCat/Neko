// GET /monitor/api/diagnostics: filtered diagnostic records plus summary.
package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/RuiCat/Neko/internal/diagnostics"
)

// DEFAULT_DIAGNOSTIC_LIMIT is used when limit is absent.
const DEFAULT_DIAGNOSTIC_LIMIT = 100

// MAX_DIAGNOSTIC_LIMIT caps one diagnostics response.
const MAX_DIAGNOSTIC_LIMIT = 1000

// handleDiagnostics answers the filtered diagnostics view.
func (h *Handler) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := diagnostics.Filter{
		Severity: diagnostics.Severity(query.Get("severity")),
		Source:   query.Get("source"),
		Entity:   query.Get("entity"),
		Script:   query.Get("script"),
		Code:     query.Get("code"),
	}
	limit, err := parseDiagnosticLimit(query.Get("limit"))
	if err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	store := h.deps.Runtime.Diagnostics
	WriteOK(w, map[string]any{
		"generation": store.Generation(),
		"summary":    store.Summary(filter),
		"items":      store.Snapshot(filter, limit),
	})
}

// parseDiagnosticLimit validates the optional limit parameter.
func parseDiagnosticLimit(raw string) (int, error) {
	if raw == "" {
		return DEFAULT_DIAGNOSTIC_LIMIT, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("limit 必须是正整数")
	}
	if value > MAX_DIAGNOSTIC_LIMIT {
		value = MAX_DIAGNOSTIC_LIMIT
	}
	return value, nil
}

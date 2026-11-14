// GET /v2/observations* handlers: the read side of the observation stream that
// feeds every push, the pre-step card and the panel.
package control

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/RuiCat/Neko/internal/observation"
)

// DEFAULT_OBSERVATION_LIMIT is how many delta events one read returns.
const DEFAULT_OBSERVATION_LIMIT = 50

// MAX_OBSERVATION_LIMIT bounds one delta read.
const MAX_OBSERVATION_LIMIT = 200

// handleObservations serves one scope's delta since a cursor, already filtered
// and merged by the policy. An unknown scope is an empty stream, never 404: the
// bridge treats "nothing to push" as an empty card.
func (s *Server) handleObservations(r *http.Request) response {
	query := r.URL.Query()
	scope := strings.TrimSpace(query.Get("scope"))
	if scope == "" {
		return badRequest("scope 必填")
	}
	since, err := parseQueryInt64(query.Get("since"), 0)
	if err != nil {
		return badRequest("since 必须是整数: " + err.Error())
	}
	limit, err := parseQueryInt64(query.Get("limit"), DEFAULT_OBSERVATION_LIMIT)
	if err != nil {
		return badRequest("limit 必须是整数: " + err.Error())
	}
	if limit <= 0 {
		return badRequest("limit 必须大于 0")
	}
	if limit > MAX_OBSERVATION_LIMIT {
		limit = MAX_OBSERVATION_LIMIT
	}
	policy := s.observationPolicy()
	selection := observation.Select(s.observationBus(), scope, since, int(limit), policy)
	return response{status: http.StatusOK, body: map[string]any{
		"ok":            true,
		"scope":         scope,
		"version":       selection.Version,
		"since":         since,
		"cursor":        selection.Cursor,
		"events":        nonNil(selection.Events),
		"truncated":     selection.Truncated,
		"dropped":       selection.Dropped,
		"budget":        policy.BudgetOf(),
		"policyVersion": policy.Version,
	}, note: "scope=" + scope + " events=" + strconv.Itoa(len(selection.Events))}
}

// handleObservationState serves one scope's folded current values, used by the
// pre-step card and the panel.
func (s *Server) handleObservationState(r *http.Request) response {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		return badRequest("scope 必填")
	}
	policy := s.observationPolicy()
	entries, version, truncated := observation.State(s.observationBus(), scope, policy)
	return response{status: http.StatusOK, body: map[string]any{
		"ok":        true,
		"scope":     scope,
		"version":   version,
		"entries":   nonNil(entries),
		"truncated": truncated,
		"budget":    policy.BudgetOf(),
	}, note: "scope=" + scope + " entries=" + strconv.Itoa(len(entries))}
}

// handleObservationStreams lists the scopes one entity writes to.
func (s *Server) handleObservationStreams(r *http.Request) response {
	entityID := strings.TrimSpace(r.URL.Query().Get("entity"))
	if entityID == "" {
		return badRequest("entity 必填")
	}
	scopes := s.observationBus().ScopesForEntity(entityID)
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entity": entityID, "scopes": nonNil(scopes),
	}, note: "entity=" + entityID + " scopes=" + strconv.Itoa(len(scopes))}
}

// observationBus returns the runtime's shared bus, or a private empty bus when
// the runtime was assembled without one (tests and embedders).
func (s *Server) observationBus() *observation.Bus {
	if s.runtime == nil || s.runtime.Observations == nil {
		return observation.NewBus()
	}
	return s.runtime.Observations
}

// observationPolicy returns the active policy, falling back to the built-in one.
func (s *Server) observationPolicy() *observation.Policy {
	if s.runtime == nil || s.runtime.ObservationRule == nil {
		return observation.DefaultPolicy()
	}
	return s.runtime.ObservationRule
}

// parseQueryInt64 reads an optional integer query parameter.
func parseQueryInt64(raw string, fallback int64) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, nil
	}
	return strconv.ParseInt(trimmed, 10, 64)
}

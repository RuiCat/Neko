// GET /entities handler: the read-only directory of entity ids and types.
package control

import (
	"fmt"
	"net/http"
)

// entityEntry is one directory row. Entity data is deliberately absent so the
// bridge can resolve per-session bindings (K3) without reading entity state.
type entityEntry struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// handleEntities lists every entity sorted by id. It is read-only and never
// touches the state-write gate or the entities file.
func (s *Server) handleEntities() response {
	all := s.runtime.Store.ListAll()
	entries := make([]entityEntry, 0, len(all))
	for _, ent := range all {
		entries = append(entries, entityEntry{ID: ent.ID, Type: ent.Type})
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entities": entries,
	}, note: fmt.Sprintf("entities=%d", len(entries))}
}

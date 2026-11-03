// GET /monitor/api/entities and /monitor/api/entities/{id}: bounded, read-only
// entity listings and dotted-path queries.
package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// entityRow is one item of the bounded entity listing.
type entityRow struct {
	ID   string            `json:"id"`
	Type string            `json:"type"`
	Keys map[string]string `json:"keys"`
}

// handleEntityList answers the deterministic, maxBytes-bounded entity listing.
func (h *Handler) handleEntityList(w http.ResponseWriter, r *http.Request) {
	maxBytes, err := ParseMaxBytes(r.URL.Query().Get("maxBytes"))
	if err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	entities := h.deps.Runtime.Store.ListAll()
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })

	items := make([]entityRow, 0, len(entities))
	truncated := false
	size := 2 // the brackets of the items array
	for _, ent := range entities {
		keys := ent.Keys
		if keys == nil {
			keys = map[string]string{}
		}
		row := entityRow{ID: ent.ID, Type: ent.Type, Keys: keys}
		raw, marshalErr := json.Marshal(row)
		if marshalErr != nil {
			WriteFailure(w, http.StatusInternalServerError, CODE_INTERNAL, "序列化实体摘要失败")
			return
		}
		add := len(raw) + 1 // one separator after each element
		if size+add > maxBytes {
			truncated = true
			break
		}
		items = append(items, row)
		size += add
	}
	raw, marshalErr := json.Marshal(items)
	if marshalErr != nil {
		WriteFailure(w, http.StatusInternalServerError, CODE_INTERNAL, "序列化实体列表失败")
		return
	}
	WriteOK(w, map[string]any{
		"items":     items,
		"bytes":     len(raw),
		"truncated": truncated,
	})
}

// handleEntity answers one entity snapshot or its requested dotted paths.
func (h *Handler) handleEntity(w http.ResponseWriter, r *http.Request, entityID string) {
	ent := h.deps.Runtime.Store.Get(entityID)
	if ent == nil {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "实体 '"+entityID+"' 不存在")
		return
	}
	maxBytes, err := ParseMaxBytes(r.URL.Query().Get("maxBytes"))
	if err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	paths := dedupePaths(r.URL.Query()["path"])
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, "path 参数不能为空")
			return
		}
	}
	if len(paths) == 0 {
		writeWholeEntity(w, entityID, ent.Snapshot(), maxBytes)
		return
	}
	writeEntityPaths(w, entityID, ent, paths)
}

// writeWholeEntity answers the full snapshot or a deterministic truncation.
func writeWholeEntity(w http.ResponseWriter, entityID string, snapshot map[string]any, maxBytes int) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		WriteFailure(w, http.StatusInternalServerError, CODE_INTERNAL, "序列化实体状态失败")
		return
	}
	if len(raw) > maxBytes {
		WriteOK(w, map[string]any{
			"entity": entityID, "data": nil, "bytes": len(raw), "truncated": true,
		})
		return
	}
	WriteOK(w, map[string]any{
		"entity": entityID, "data": snapshot, "bytes": len(raw), "truncated": false,
	})
}

// writeEntityPaths answers requested paths through Entity.Get, the same source
// as the MCP entity_state_get tool and the control-plane /state endpoint.
func writeEntityPaths(w http.ResponseWriter, entityID string, ent interface {
	Get(string) (any, error)
}, paths []string) {
	values := make(map[string]any, len(paths))
	missing := make([]string, 0)
	for _, path := range paths {
		value, err := ent.Get(path)
		if err != nil {
			missing = append(missing, path)
			continue
		}
		values[path] = value
	}
	raw, err := json.Marshal(values)
	if err != nil {
		WriteFailure(w, http.StatusInternalServerError, CODE_INTERNAL, "序列化路径值失败")
		return
	}
	WriteOK(w, map[string]any{
		"entity": entityID, "values": values, "missing": missing, "bytes": len(raw),
	})
}

// dedupePaths keeps the first occurrence of each requested path in order.
func dedupePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

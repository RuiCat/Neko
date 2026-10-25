// GET /state handler: one entity's whole snapshot or selected dotted paths.
package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

const (
	// MIN_MAX_BYTES is the smallest accepted maxBytes query value.
	MIN_MAX_BYTES = 1024
	// MAX_MAX_BYTES is the largest accepted maxBytes query value.
	MAX_MAX_BYTES = 1024 * 1024
	// DEFAULT_MAX_BYTES is used when maxBytes is absent.
	DEFAULT_MAX_BYTES = 64 * 1024
)

// handleState reads one entity through Snapshot (never the entity write lock)
// and answers either whole data or the requested dotted paths.
func (s *Server) handleState(r *http.Request) response {
	query := r.URL.Query()
	entityID := strings.TrimSpace(query.Get("entity"))
	if entityID == "" {
		return badRequest("缺少 entity 参数")
	}
	maxBytes, err := parseMaxBytes(query.Get("maxBytes"))
	if err != nil {
		return badRequest(err.Error())
	}
	paths := query["path"]
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			return badRequest("path 参数不能为空")
		}
	}

	entity := s.runtime.Store.Get(entityID)
	if entity == nil {
		return notFound(fmt.Sprintf("实体 '%s' 不存在", entityID))
	}
	snapshot := entity.Snapshot()
	if len(paths) == 0 {
		return wholeStateResponse(entityID, snapshot, maxBytes)
	}
	return pathsStateResponse(entityID, snapshot, paths)
}

// wholeStateResponse answers the full snapshot, or truncated:true when the
// encoded data would exceed maxBytes (never a half-written JSON body).
func wholeStateResponse(entityID string, snapshot map[string]any, maxBytes int) response {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return internalError("序列化实体状态失败: " + err.Error())
	}
	if len(raw) > maxBytes {
		return response{status: http.StatusOK, body: map[string]any{
			"ok": true, "entity": entityID, "data": nil,
			"bytes": len(raw), "truncated": true,
		}}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entity": entityID, "data": snapshot,
		"bytes": len(raw), "truncated": false,
	}}
}

// pathsStateResponse answers only the requested paths; absent ones are listed
// in missing instead of failing the whole request.
func pathsStateResponse(entityID string, snapshot map[string]any, paths []string) response {
	values := make(map[string]any, len(paths))
	missing := make([]string, 0, len(paths))
	for _, path := range paths {
		value, ok := lookupSnapshotPath(snapshot, path)
		if !ok {
			missing = append(missing, path)
			continue
		}
		values[path] = value
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return internalError("序列化路径值失败: " + err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entity": entityID, "values": values,
		"missing": missing, "bytes": len(raw),
	}, note: fmt.Sprintf("entity=%s paths=%d missing=%d", entityID, len(paths), len(missing))}
}

// parseMaxBytes validates the optional maxBytes query parameter.
func parseMaxBytes(raw string) (int, error) {
	if raw == "" {
		return DEFAULT_MAX_BYTES, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("maxBytes 必须是整数")
	}
	if value < MIN_MAX_BYTES || value > MAX_MAX_BYTES {
		return 0, fmt.Errorf("maxBytes 必须在 %d..%d 之间", MIN_MAX_BYTES, MAX_MAX_BYTES)
	}
	return value, nil
}

// lookupSnapshotPath resolves a dotted path inside a snapshot copy.
func lookupSnapshotPath(snapshot map[string]any, path string) (any, bool) {
	var current any = snapshot
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

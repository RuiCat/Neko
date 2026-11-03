// POST /monitor/api/entities/{id}/set: one gated entity-field write with a 2s
// gate limit, an on-disk save and rollback when the save fails.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/RuiCat/Neko/internal/entity"
)

// SET_GATE_TIMEOUT bounds how long a set waits for the shared state gate.
const SET_GATE_TIMEOUT = 2 * time.Second

// setRequest is the POST /entities/{id}/set body.
type setRequest struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

// saveFailure reports a failed store.Save and whether the memory value was
// rolled back to the pre-write value.
type saveFailure struct {
	err        error
	rolledBack bool
}

func (e *saveFailure) Error() string { return e.err.Error() }

// handleSet writes one entity path and saves, or rolls back and reports dirty.
func (h *Handler) handleSet(w http.ResponseWriter, r *http.Request, entityID string) {
	if !h.requireWritable(w) {
		return
	}
	request := setRequest{}
	if err := DecodeBody(r, &request); err != nil {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
		return
	}
	if request.Path == "" {
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, "path 不能为空")
		return
	}
	var value any
	if len(request.Value) > 0 {
		if err := json.Unmarshal(request.Value, &value); err != nil {
			WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, "value 不是合法 JSON")
			return
		}
	}
	ent := h.deps.Runtime.Store.Get(entityID)
	if ent == nil {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "实体 '"+entityID+"' 不存在")
		return
	}

	var saveErr *saveFailure
	acquired, err := h.deps.MCP.WithStateWriteTimeout(SET_GATE_TIMEOUT, func() error {
		saveErr = writeAndSave(h.deps.Runtime.Store, ent, request.Path, value)
		if saveErr != nil {
			return saveErr
		}
		return nil
	})
	if !acquired {
		h.audit("set", entityID, request.Path, "write_busy", r.RemoteAddr)
		WriteFailure(w, http.StatusConflict, CODE_WRITE_BUSY, "状态写 gate 被占用超过 2s，请稍后重试")
		return
	}
	if err != nil {
		h.writeSaveFailure(w, entityID, request.Path, value, saveErr, r.RemoteAddr)
		return
	}
	h.audit("set", entityID, request.Path, "ok", r.RemoteAddr)
	WriteOK(w, map[string]any{
		"entity": entityID, "path": request.Path, "value": value, "saved": true,
	})
}

// writeAndSave mutates the entity, saves and rolls the memory value back when
// the save fails and a previous value can be restored.
func writeAndSave(store *entity.Store, ent *entity.Entity, path string, value any) *saveFailure {
	oldValue, oldErr := ent.Get(path)
	if err := ent.Set(path, value); err != nil {
		return &saveFailure{err: err, rolledBack: true}
	}
	if err := store.Save(); err != nil {
		rolledBack := false
		if oldErr == nil {
			rolledBack = ent.Set(path, oldValue) == nil
		}
		return &saveFailure{err: err, rolledBack: rolledBack}
	}
	return nil
}

// writeSaveFailure answers the 500 dirty/rollback contract.
func (h *Handler) writeSaveFailure(w http.ResponseWriter, entityID, path string, value any, saveErr *saveFailure, remote string) {
	rolledBack := saveErr != nil && saveErr.rolledBack
	result := "save_failed"
	if rolledBack {
		result = "save_failed_rolled_back"
	}
	h.audit("set", entityID, path, result, remote)
	body := map[string]any{
		"ok":         false,
		"dirty":      !rolledBack,
		"rolledBack": rolledBack,
		"saved":      false,
		"entity":     entityID,
		"path":       path,
		"value":      value,
		"error":      map[string]any{"code": CODE_INTERNAL, "message": saveErr.Error()},
	}
	WriteJSON(w, http.StatusInternalServerError, body)
}

// audit appends one write-audit line without ever blocking or failing the write.
func (h *Handler) audit(action, entityID, path, result, remote string) {
	h.deps.Audit.Record(action, entityID, path, result, remote)
}

// auditResult maps a tick error onto an audit result label.
func auditResult(err error) string {
	if err == nil {
		return "ok"
	}
	return fmt.Sprintf("failed: %s", err.Error())
}

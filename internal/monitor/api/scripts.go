// GET /monitor/api/scripts and /monitor/api/scripts/{name}: registry metadata
// and script source read only through registry.ReadScript.
package api

import (
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/registry"
)

// handleScriptList answers the script inventory.
func (h *Handler) handleScriptList(w http.ResponseWriter, r *http.Request) {
	WriteOK(w, map[string]any{"items": h.deps.Runtime.Registry.ListScripts()})
}

// handleScript answers one script's metadata and, on request, its source.
func (h *Handler) handleScript(w http.ResponseWriter, r *http.Request, name string) {
	if !h.scriptKnown(name) {
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "脚本 '"+name+"' 不存在")
		return
	}
	view, err := h.deps.Runtime.Registry.ReadScript(name)
	if err != nil {
		h.writeScriptError(w, name, err)
		return
	}
	points, templates, funcs, events := entryCounts(view.Entry)
	body := map[string]any{
		"name":       view.Name,
		"path":       view.Path,
		"hash":       view.Hash,
		"actualHash": view.ActualHash,
		"rules":      points,
		"templates":  templates,
		"funcs":      funcs,
		"events":     events,
	}
	if wantSource(r) {
		body["source"] = view.Source
	}
	WriteOK(w, body)
}

// scriptKnown reports whether the registry has an entry under name.
func (h *Handler) scriptKnown(name string) bool {
	for _, info := range h.deps.Runtime.Registry.ListScripts() {
		if info.Name == name {
			return true
		}
	}
	return false
}

// writeScriptError maps registry.ReadScript failures onto the frozen codes.
func (h *Handler) writeScriptError(w http.ResponseWriter, name string, err error) {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		WriteFailure(w, http.StatusNotFound, CODE_NOT_FOUND, "脚本 '"+name+"' 不存在")
	case strings.Contains(err.Error(), "上限"):
		WriteFailure(w, http.StatusRequestEntityTooLarge, CODE_TOO_LARGE, err.Error())
	default:
		WriteFailure(w, http.StatusBadRequest, CODE_BAD_REQUEST, err.Error())
	}
}

// entryCounts returns the registration counters, tolerating a missing entry.
func entryCounts(entry *registry.Entry) (points, templates, funcs, events int) {
	if entry == nil {
		return 0, 0, 0, 0
	}
	return len(entry.Rules), len(entry.Templates), len(entry.Funcs), len(entry.Events)
}

// wantSource reports whether the source body was requested.
func wantSource(r *http.Request) bool {
	switch strings.ToLower(r.URL.Query().Get("source")) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

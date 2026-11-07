// POST /state handler: one entity's dotted-path write, single or batched (K19).
package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/hardstate"
	"github.com/RuiCat/Neko/internal/mcpserver"
)

// stateWriteRequest is the POST /state body. Path/Value and Values are
// mutually exclusive write forms.
type stateWriteRequest struct {
	Entity string                     `json:"entity"`
	Path   string                     `json:"path,omitempty"`
	Value  json.RawMessage            `json:"value,omitempty"`
	Values map[string]json.RawMessage `json:"values,omitempty"`
}

// writeValues validates the mutually exclusive forms and decodes them into
// dotted path -> value.
func (r stateWriteRequest) writeValues() (map[string]any, error) {
	if len(r.Values) > 0 && (r.Path != "" || len(r.Value) > 0) {
		return nil, fmt.Errorf("path/value 与 values 不能同时使用")
	}
	if len(r.Values) == 0 {
		if r.Path == "" && len(r.Value) == 0 {
			return nil, fmt.Errorf("缺少 path/value 或 values")
		}
		if r.Path == "" {
			return nil, fmt.Errorf("缺少 path")
		}
		if len(r.Value) == 0 {
			return nil, fmt.Errorf("path 与 value 必须同时提供")
		}
		var value any
		if err := json.Unmarshal(r.Value, &value); err != nil {
			return nil, fmt.Errorf("value 不是合法 JSON: %s", err)
		}
		return map[string]any{r.Path: value}, nil
	}
	values := make(map[string]any, len(r.Values))
	for path, raw := range r.Values {
		if strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("values 的路径不能为空")
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, fmt.Errorf("路径 %s 的值不是合法 JSON: %s", path, err)
		}
		values[path] = value
	}
	return values, nil
}

// handleStateWrite applies one or many dotted-path writes under the shared
// state gate and saves the store (K19). GET /state stays read-only.
func (s *Server) handleStateWrite(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return badRequest("请求体不能为空")
	}
	request := stateWriteRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	entityID := strings.TrimSpace(request.Entity)
	if entityID == "" {
		return badRequest("缺少 entity")
	}
	values, err := request.writeValues()
	if err != nil {
		return badRequest(err.Error())
	}
	paths, err := s.mcp.WriteEntityPaths(entityID, values, hardstate.SOURCE_CONTROL)
	if err != nil {
		switch {
		case errors.Is(err, mcpserver.ErrEntityMissing):
			return notFound(fmt.Sprintf("实体 '%s' 不存在", entityID))
		case errors.Is(err, mcpserver.ErrPathInvalid):
			return badRequest(err.Error())
		default:
			return internalError(err.Error())
		}
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entity": entityID, "paths": paths,
	}, note: fmt.Sprintf("entity=%s paths=%d", entityID, len(paths))}
}

// POST /entities/create handler: create one entity from its template chain.
package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/entity"
)

// entityCreateRequest is the POST /entities/create body.
type entityCreateRequest struct {
	Entity string         `json:"entity"`
	Type   string         `json:"type,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

// handleEntityCreate creates the entity unless it already exists. Idempotent:
// an existing entity answers 200 with created=false, a bad template or id is
// 400, and anything else is 500.
func (s *Server) handleEntityCreate(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return badRequest("请求体不能为空")
	}
	request := entityCreateRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	entityID := strings.TrimSpace(request.Entity)
	if entityID == "" {
		return badRequest("缺少 entity")
	}
	typeName := strings.TrimSpace(request.Type)
	if typeName == "" {
		typeName = "角色基础"
	}
	if !s.runtime.Store.HasTemplate(typeName) {
		return badRequest(fmt.Sprintf("实体模板类型 '%s' 不存在（可用: %s）", typeName, strings.Join(s.runtime.Store.ListTemplates(), ", ")))
	}
	created, err := s.mcp.CreateEntity(entityID, typeName, request.Data)
	if err != nil {
		var validation *entity.ValidationError
		if errors.As(err, &validation) {
			return badRequest(validation.Message)
		}
		return internalError("创建实体失败: " + err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "entity": entityID, "type": typeName, "created": created,
	}, note: fmt.Sprintf("entity=%s created=%t", entityID, created)}
}

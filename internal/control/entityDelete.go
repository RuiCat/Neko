// POST /entities/delete handler: remove one entity and its change board (K19).
package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/mcpserver"
)

// entityDeleteRequest is the POST /entities/delete body.
type entityDeleteRequest struct {
	Entity string `json:"entity"`
}

// handleEntityDelete deletes the entity, its event board and saves the store
// under the shared state gate. An unknown entity is a 404, never a silent OK.
func (s *Server) handleEntityDelete(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return badRequest("请求体不能为空")
	}
	request := entityDeleteRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	entityID := strings.TrimSpace(request.Entity)
	if entityID == "" {
		return badRequest("缺少 entity")
	}
	if err := s.mcp.DeleteEntity(entityID); err != nil {
		if errors.Is(err, mcpserver.ErrEntityMissing) {
			return notFound(fmt.Sprintf("实体 '%s' 不存在", entityID))
		}
		return internalError("删除实体失败: " + err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok": true, "deleted": entityID,
	}, note: "deleted=" + entityID}
}

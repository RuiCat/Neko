// candidateResolve.go lets a reviewer approve or reject a queued transition
// candidate. The store is append-only, so a resolution is a new line with the
// same ID and the new status.
package control

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/candidates"
)

// resolveCandidateRequest is the POST /candidates/resolve body.
type resolveCandidateRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// handleResolveCandidate updates one candidate's status.
func (s *Server) handleResolveCandidate(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := resolveCandidateRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	if strings.TrimSpace(request.ID) == "" {
		return badRequest("缺少 id")
	}
	if request.Status != candidates.STATUS_APPROVED && request.Status != candidates.STATUS_REJECTED {
		return badRequest("status 必须是 approved 或 rejected")
	}
	record, err := s.candidateStore().Resolve(request.ID, request.Status, request.Note)
	if err != nil {
		return notFound(err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{"ok": true, "item": record}}
}

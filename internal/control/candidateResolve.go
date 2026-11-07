// candidateResolve.go lets a reviewer approve or reject a queued transition
// candidate; the shared operation appends the resolution line.
package control

import (
	"encoding/json"
	"net/http"

	"github.com/RuiCat/Neko/internal/control/ops"
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
	body, err := ops.ResolveCandidate(s.opsDeps(), ops.ResolveRequest{
		ID: request.ID, Status: request.Status, Note: request.Note,
	})
	return opsResponse(body, err)
}

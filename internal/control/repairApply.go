// repairApply.go wires the repair loop endpoints to the shared operations: an
// approved candidate becomes a versioned repair, the deterministic regression
// gate re-audits it, and a failing gate rolls the version back.
package control

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/RuiCat/Neko/internal/control/ops"
)

// applyCandidateRequest is the POST /candidates/apply body.
type applyCandidateRequest struct {
	ID string `json:"id"`
	// When is the rule condition that guards the repaired write. It is required
	// so a repair never becomes an always-on writer.
	When string `json:"when,omitempty"`
	// Match is the entity selector; empty defaults to the candidate's entity.
	Match string `json:"match,omitempty"`
}

// handleApplyCandidate promotes one approved candidate to a repair version and
// gates it. A failing gate rolls the version back before answering.
func (s *Server) handleApplyCandidate(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := applyCandidateRequest{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return badRequest("请求体不是合法 JSON: " + err.Error())
		}
	}
	body, err := ops.ApplyCandidate(s.opsDeps(), ops.ApplyRequest{
		ID: request.ID, When: request.When, Match: request.Match,
	})
	return opsResponse(body, err)
}

// handleRollbackRepair rolls back the newest live repair version and removes
// the engine rules that version installed.
func (s *Server) handleRollbackRepair(r *http.Request) response {
	body, err := ops.RollbackRepair(s.opsDeps())
	return opsResponse(body, err)
}

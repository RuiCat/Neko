// judge.go wires the C3 judge endpoints to the shared candidate operations:
// legal verdicts stay read-only, uncovered ones are queued for review.
package control

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/control/ops"
)

// judgeRequest is the POST /judge body.
type judgeRequest struct {
	EntityID string   `json:"entity_id"`
	Path     string   `json:"path"`
	Proposed any      `json:"proposed,omitempty"`
	Source   string   `json:"source,omitempty"`
	Evidence []string `json:"evidence,omitempty"`
}

// candidateStore returns the shared candidate file under the data directory.
// The teacher-signal path appends directly; judge/candidates go through ops.
func (s *Server) candidateStore() *candidates.Store {
	return candidates.NewStore(filepath.Join(s.dataDir, "candidates.jsonl"))
}

// handleJudge classifies one candidate; uncovered ones are appended to the
// review queue and reported with their ID.
func (s *Server) handleJudge(r *http.Request) response {
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := judgeRequest{}
	if err := json.Unmarshal(raw, &request); err != nil {
		return badRequest("请求体不是合法 JSON: " + err.Error())
	}
	body, err := ops.Judge(s.opsDeps(), ops.JudgeRequest{
		EntityID: request.EntityID,
		Path:     request.Path,
		Proposed: request.Proposed,
		Source:   request.Source,
		Evidence: request.Evidence,
	})
	return opsResponse(body, err)
}

// handleCandidates lists pending candidates for review.
func (s *Server) handleCandidates(r *http.Request) response {
	body, err := ops.Candidates(s.opsDeps())
	return opsResponse(body, err)
}

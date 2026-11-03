// judge.go exposes the read-only C3 verdict and queues uncovered candidates
// for later review instead of blocking the turn.
package control

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/engine"
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
	if strings.TrimSpace(request.EntityID) == "" {
		return badRequest("缺少 entity_id")
	}
	if strings.TrimSpace(request.Path) == "" {
		return badRequest("缺少 path")
	}
	verdict, err := s.runtime.Machine.Judge(engine.Candidate{
		EntityID: request.EntityID,
		Path:     request.Path,
		Proposed: request.Proposed,
		Source:   request.Source,
		Evidence: request.Evidence,
	})
	if err != nil {
		return internalError("裁决失败: " + err.Error())
	}
	responseBody := map[string]any{
		"ok":      true,
		"verdict": verdict,
		"queued":  false,
	}
	if verdict.Status != engine.VERDICT_UNCOVERED {
		return response{status: http.StatusOK, body: responseBody}
	}
	record, err := s.candidateStore().Append(candidates.Record{
		EntityID: request.EntityID,
		Path:     request.Path,
		Proposed: request.Proposed,
		Source:   request.Source,
		Evidence: request.Evidence,
		Reason:   verdict.Reason,
	})
	if err != nil {
		return internalError("候选入队失败: " + err.Error())
	}
	responseBody["queued"] = true
	responseBody["candidate_id"] = record.ID
	return response{status: http.StatusOK, body: responseBody}
}

// handleCandidates lists pending candidates for review.
func (s *Server) handleCandidates(r *http.Request) response {
	records, err := s.candidateStore().List(candidates.STATUS_PENDING)
	if err != nil {
		return internalError("读取候选失败: " + err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok":    true,
		"count": len(records),
		"items": records,
	}}
}

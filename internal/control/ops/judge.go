// judge.go classifies one candidate and queues the uncovered ones for review;
// it is the shared body of POST /judge and the judge_run tool.
package ops

import (
	"strings"

	"github.com/RuiCat/Neko/internal/candidates"
	"github.com/RuiCat/Neko/internal/engine"
)

// JudgeRequest is the shared candidate request; it mirrors the POST /judge body.
type JudgeRequest struct {
	EntityID string
	Path     string
	Proposed any
	Source   string
	Evidence []string
}

// Judge returns the read-only verdict and queues the candidate when no enabled
// rule covers it. The result mirrors the POST /judge response body.
func Judge(deps Deps, request JudgeRequest) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.EntityID) == "" {
		return nil, BadRequest("缺少 entity_id")
	}
	if strings.TrimSpace(request.Path) == "" {
		return nil, BadRequest("缺少 path")
	}
	verdict, err := deps.Machine.Judge(engine.Candidate{
		EntityID: request.EntityID,
		Path:     request.Path,
		Proposed: request.Proposed,
		Source:   request.Source,
		Evidence: request.Evidence,
	})
	if err != nil {
		return nil, Internal("裁决失败: %s", err)
	}
	body := map[string]any{"ok": true, "verdict": verdict, "queued": false}
	if verdict.Status != engine.VERDICT_UNCOVERED {
		return body, nil
	}
	record, err := deps.candidateStore().Append(candidates.Record{
		EntityID: request.EntityID,
		Path:     request.Path,
		Proposed: request.Proposed,
		Source:   request.Source,
		Evidence: request.Evidence,
		Reason:   verdict.Reason,
	})
	if err != nil {
		return nil, Internal("候选入队失败: %s", err)
	}
	body["queued"] = true
	body["candidate_id"] = record.ID
	return body, nil
}

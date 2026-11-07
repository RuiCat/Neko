// candidates.go lists and resolves queued review candidates; the shared body of
// GET /candidates, POST /candidates/resolve and their MCP tools.
package ops

import (
	"strings"

	"github.com/RuiCat/Neko/internal/candidates"
)

// ResolveRequest is the shared candidate resolution request.
type ResolveRequest struct {
	ID     string
	Status string
	Note   string
}

// Candidates lists every pending candidate for review.
func Candidates(deps Deps) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	records, err := deps.candidateStore().List(candidates.STATUS_PENDING)
	if err != nil {
		return nil, Internal("读取候选失败: %s", err)
	}
	return map[string]any{"ok": true, "count": len(records), "items": records}, nil
}

// ResolveCandidate appends one resolution (approved/rejected) to a candidate.
func ResolveCandidate(deps Deps, request ResolveRequest) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.ID) == "" {
		return nil, BadRequest("缺少 id")
	}
	if request.Status != candidates.STATUS_APPROVED && request.Status != candidates.STATUS_REJECTED {
		return nil, BadRequest("status 必须是 approved 或 rejected")
	}
	record, err := deps.candidateStore().Resolve(request.ID, request.Status, request.Note)
	if err != nil {
		return nil, NotFound("%s", err)
	}
	return map[string]any{"ok": true, "item": record}, nil
}

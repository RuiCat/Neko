// nightly.go runs the offline decay pass on the experts' private contexts.
// It is the engine-side half of P3: the caller (nightly job or DSH timer) just
// posts the factor.
package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/RuiCat/Neko/internal/candidates"
)

// DEFAULT_DECAY_FACTOR halves every weight per nightly pass.
const DEFAULT_DECAY_FACTOR = 0.5

// nightlyRequest is the optional POST /nightly body.
type nightlyRequest struct {
	Factor *float64 `json:"factor"`
}

// handleNightly decays every expert context and reports what was dropped.
func (s *Server) handleNightly(r *http.Request) response {
	if s.runtime.Experts == nil {
		return internalError("专家注册表未配置")
	}
	raw, err := readBody(r)
	if err != nil {
		return badRequest(err.Error())
	}
	request := nightlyRequest{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return badRequest("请求体不是合法 JSON: " + err.Error())
		}
	}
	factor := DEFAULT_DECAY_FACTOR
	if request.Factor != nil {
		factor = *request.Factor
	}
	if factor <= 0 || factor >= 1 {
		return badRequest("factor 必须落在 (0,1) 之间")
	}
	perExpert := s.runtime.Experts.DecayAll(factor)
	total := 0
	for _, removed := range perExpert {
		total += removed
	}
	if err := s.runtime.SaveExperts(); err != nil {
		return internalError("保存衰减结果失败: " + err.Error())
	}
	expired, err := s.candidateStore().Expire(time.Now().Add(-candidates.DEFAULT_TTL))
	if err != nil {
		return internalError("候选过期处理失败: " + err.Error())
	}
	return response{status: http.StatusOK, body: map[string]any{
		"ok":         true,
		"factor":     factor,
		"removed":    total,
		"per_expert": perExpert,
		"expired":    expired,
	}}
}

// nightly.go decays the experts' private contexts and expires stale candidates;
// it is the shared body of both POST /nightly and the nightly_run tool.
package ops

import (
	"path/filepath"
	"time"

	"github.com/RuiCat/Neko/internal/expert"
)

// Nightly decays every expert context and expires candidates older than the
// configured TTL. A nil factor uses DEFAULT_DECAY_FACTOR; the result mirrors
// the POST /nightly response body.
func Nightly(deps Deps, factor *float64) (map[string]any, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}
	if deps.Experts == nil {
		return nil, Internal("专家注册表未配置")
	}
	value := DEFAULT_DECAY_FACTOR
	if factor != nil {
		value = *factor
	}
	if value <= 0 || value >= 1 {
		return nil, BadRequest("factor 必须落在 (0,1) 之间")
	}
	perExpert := deps.Experts.DecayAll(value)
	total := 0
	for _, removed := range perExpert {
		total += removed
	}
	if err := expert.SaveFile(filepath.Join(deps.DataDir, "experts.json"), deps.Experts); err != nil {
		return nil, Internal("保存衰减结果失败: %s", err)
	}
	expired, err := deps.candidateStore().Expire(time.Now().Add(-deps.candidateTTL()))
	if err != nil {
		return nil, Internal("候选过期处理失败: %s", err)
	}
	return map[string]any{
		"ok":         true,
		"factor":     value,
		"removed":    total,
		"per_expert": perExpert,
		"expired":    expired,
	}, nil
}

// modelGate.go swaps the deterministic rule gate for a trained MLP when a
// weights file is available. Any missing/mismatched/broken model falls back to
// the bundled rule gate (fail-open): a broken model must never block a turn.
package gate

import (
	"fmt"
	"sort"

	"github.com/RuiCat/Neko/internal/learn/mlp"
)

// MIN_MODEL_PROBABILITY 是第三名专家入选的概率门槛；低于它只激活前两名
// （与 RuleGate 的 2–3 个专家口径一致）。
const MIN_MODEL_PROBABILITY = 0.2

// ModelGate activates experts with a trained MLP classifier. The classifier is
// single-label over DEFAULT_EXPERT_ORDER; the top two (or three) classes by
// probability become the active set, returned in canonical expert order.
type ModelGate struct {
	model    *mlp.Model
	fallback Gate
	version  string
}

// NewModelGate wires a model with a required fallback gate. A nil model is
// legal: Activate then delegates to the fallback (same behaviour as before the
// model existed).
func NewModelGate(model *mlp.Model, fallback Gate, version string) *ModelGate {
	return &ModelGate{model: model, fallback: fallback, version: version}
}

// Version 返回权重版本标识（写入 journal 的 Gate.Version，便于后续对比）。
func (g *ModelGate) Version() string {
	if g == nil || g.version == "" {
		return "gate-mlp"
	}
	return g.version
}

// Activate 用模型打分选专家；模型缺失/维数不符/推理出错都回落 fallback。
func (g *ModelGate) Activate(features Features) ([]string, error) {
	vector := FeatureVectorV1(features)
	if g == nil || g.model == nil {
		return g.ruleFallback(features, "模型未加载")
	}
	if g.model.InputSize() != len(vector) {
		return g.ruleFallback(features, fmt.Sprintf("模型输入层 %d 与特征维数 %d 不符", g.model.InputSize(), len(vector)))
	}
	if g.model.NumClasses() != len(SEMANTIC_EXPERTS) {
		return g.ruleFallback(features, fmt.Sprintf("模型类别数 %d 与语义专家数 %d 不符", g.model.NumClasses(), len(SEMANTIC_EXPERTS)))
	}
	probabilities, err := g.model.Predict(vector)
	if err != nil {
		return g.ruleFallback(features, fmt.Sprintf("推理失败: %v", err))
	}
	return selectExpertsByProbability(probabilities), nil
}

// PrimaryExpert returns the highest-probability semantic expert; when the
// model cannot run it delegates to the fallback gate, so the training label
// stays the expert that actually drove the activation.
func (g *ModelGate) PrimaryExpert(features Features, active []string) string {
	if g != nil && g.model != nil {
		vector := FeatureVectorV1(features)
		if g.model.InputSize() == len(vector) && g.model.NumClasses() == len(SEMANTIC_EXPERTS) {
			if probabilities, err := g.model.Predict(vector); err == nil {
				best := 0
				for index := 1; index < len(probabilities); index++ {
					if probabilities[index] > probabilities[best] {
						best = index
					}
				}
				return SEMANTIC_EXPERTS[best]
			}
		}
	}
	if selector, ok := g.fallback.(PrimarySelector); ok {
		return selector.PrimaryExpert(features, active)
	}
	return ""
}

// ruleFallback 回落规则门控；fallback 缺失时返回可读错误（fail-closed 到调用方）。
func (g *ModelGate) ruleFallback(features Features, reason string) ([]string, error) {
	if g == nil || g.fallback == nil {
		return nil, fmt.Errorf("gate: 模型不可用且无 fallback（%s）", reason)
	}
	active, err := g.fallback.Activate(features)
	if err != nil {
		return nil, fmt.Errorf("gate: fallback 失败（%s）: %w", reason, err)
	}
	return active, nil
}

// selectExpertsByProbability 把 4 个语义专家的概率变成激活集：
// 第 1 名必选，第 2 名概率 ≥ MIN_MODEL_PROBABILITY 时一起选，最后固定加上表达偏好；
// 结果按 DEFAULT_EXPERT_ORDER 归位。并列时按语义专家顺序稳定取舍。
func selectExpertsByProbability(probabilities []float64) []string {
	order := make([]int, len(probabilities))
	for index := range order {
		order[index] = index
	}
	sort.SliceStable(order, func(left, right int) bool {
		return probabilities[order[left]] > probabilities[order[right]]
	})
	selected := map[string]bool{PHRASING_EXPERT: true}
	if len(order) > 0 {
		selected[SEMANTIC_EXPERTS[order[0]]] = true
	}
	if len(order) > 1 && probabilities[order[1]] >= MIN_MODEL_PROBABILITY {
		selected[SEMANTIC_EXPERTS[order[1]]] = true
	}
	active := make([]string, 0, MAX_ACTIVE_EXPERTS)
	for _, name := range DEFAULT_EXPERT_ORDER {
		if selected[name] {
			active = append(active, name)
		}
	}
	return active
}

// modelGate_test.go covers the frozen v1 feature layout and the fail-open model gate.
package gate

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RuiCat/Neko/internal/learn/mlp"
)

// stubGate 是测试用固定门控（验证 fallback 分支）。
type stubGate struct {
	active []string
	err    error
	calls  int
}

// Activate 记录调用次数并返回预设专家。
func (s *stubGate) Activate(Features) ([]string, error) {
	s.calls++
	return s.active, s.err
}

// weightsJSON 生成一个 5 类 MLP 的权重 JSON；biases 按类别顺序写入输出层偏置。
func weightsJSON(t *testing.T, layers []int, biases []float64) string {
	t.Helper()
	total := 0
	for index := 0; index < len(layers)-1; index++ {
		total += layers[index+1] * (layers[index] + 1)
	}
	values := make([]float64, total)
	if len(layers) == 2 && len(biases) == layers[1] {
		for unit := range biases {
			// 单隐藏层：每个单元 = layers[0] 个权重 + 1 个偏置。
			values[unit*(layers[0]+1)+layers[0]] = biases[unit]
		}
	}
	encodedLayers, err := json.Marshal(layers)
	if err != nil {
		t.Fatalf("marshal layers: %v", err)
	}
	encodedWeights, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal weights: %v", err)
	}
	return `{"formatVersion":1,"layers":` + string(encodedLayers) +
		`,"activation":"relu","leaky":0,"seed":1,"weights":` + string(encodedWeights) + `}`
}

// TestFeatureVectorV1Layout 锁定 v1 布局：维数、one-hot、摘要/情感/硬状态位。
func TestFeatureVectorV1Layout(t *testing.T) {
	if got, want := len(FeatureNamesV1()), FeatureSizeV1; got != want {
		t.Fatalf("FeatureNamesV1 长度 %d, want %d", got, want)
	}
	// 每个已知事件类型落在自己的槽位。
	for index, kind := range eventKinds {
		vector := FeatureVectorV1(Features{EventKind: " " + strings.ToUpper(kind) + " "})
		if len(vector) != FeatureSizeV1 {
			t.Fatalf("向量长度 %d, want %d", len(vector), FeatureSizeV1)
		}
		if vector[index] != 1 {
			t.Fatalf("kind=%s 的 one-hot 槽位 %d 没亮", kind, index)
		}
		sum := 0.0
		for _, value := range vector {
			sum += value
		}
		if sum != 1 {
			t.Fatalf("kind=%s 只应有一个 one-hot 位，sum=%v", kind, sum)
		}
	}
	// 未知/空类型落在 unknown 位。
	for _, kind := range []string{"", "custom-kind"} {
		vector := FeatureVectorV1(Features{EventKind: kind})
		if vector[featureUnknownIndex] != 1 {
			t.Fatalf("kind=%q 应落在 unknown 位", kind)
		}
	}
	// 摘要：present + 长度归一化（200 字符封顶）。
	vector := FeatureVectorV1(Features{Summary: strings.Repeat("喵", 100)})
	if vector[SUMMARY_PRESENT_INDEX] != 1 || vector[SUMMARY_LENGTH_INDEX] != 0.5 {
		t.Fatalf("summary 位 = present %v / length %v, want 1 / 0.5",
			vector[SUMMARY_PRESENT_INDEX], vector[SUMMARY_LENGTH_INDEX])
	}
	// 情感 present/value 分离。
	vector = FeatureVectorV1(Features{Affect: map[string]float64{"trust": 0.1, "valence": -0.8}})
	if vector[TRUST_PRESENT_INDEX] != 1 || vector[TRUST_INDEX] != 0.1 {
		t.Fatalf("trust 位 = %v / %v", vector[TRUST_PRESENT_INDEX], vector[TRUST_INDEX])
	}
	if vector[VALENCE_PRESENT_INDEX] != 1 || vector[VALENCE_INDEX] != -0.8 {
		t.Fatalf("valence 位 = %v / %v", vector[VALENCE_PRESENT_INDEX], vector[VALENCE_INDEX])
	}
	// 硬状态里的「知识边界」键点亮对应位。
	vector = FeatureVectorV1(Features{HardState: map[string]any{EXPERT_KNOWLEDGE + ".范围": "只谈猫"}})
	if vector[HARDSTATE_KNOWLEDGE_INDEX] != 1 {
		t.Fatal("HardState 知识边界键应点亮 HARDSTATE_KNOWLEDGE_INDEX")
	}
}

// TestModelGateUniformWeightsPicksThree 覆盖全零权重（等概率）时取前三名 + 归位顺序。
func TestModelGateUniformWeightsPicksThree(t *testing.T) {
	model, err := mlp.Load(strings.NewReader(weightsJSON(t, []int{FeatureSizeV1, len(SEMANTIC_EXPERTS)}, nil)))
	if err != nil {
		t.Fatalf("加载权重失败: %v", err)
	}
	modelGate := NewModelGate(model, nil, "test-v1")
	active, err := modelGate.Activate(Features{EventKind: "turn"})
	if err != nil {
		t.Fatalf("Activate 失败: %v", err)
	}
	want := []string{EXPERT_RELATION, EXPERT_EVENT, PHRASING_EXPERT}
	if strings.Join(active, "|") != strings.Join(want, "|") {
		t.Fatalf("active = %v, want %v", active, want)
	}
	if modelGate.Version() != "test-v1" {
		t.Fatalf("Version = %q", modelGate.Version())
	}
}

// TestModelGateUsesBiasedOutput 覆盖「高概率专家优先 + 第三名低于门槛只激活两名」。
func TestModelGateUsesBiasedOutput(t *testing.T) {
	// 把「事件记忆」（SEMANTIC_EXPERTS[1]）的 logit 抬高；第二名低于门槛时只带表达偏好。
	biases := make([]float64, len(SEMANTIC_EXPERTS))
	biases[1] = 2
	model, err := mlp.Load(strings.NewReader(weightsJSON(t, []int{FeatureSizeV1, len(SEMANTIC_EXPERTS)}, biases)))
	if err != nil {
		t.Fatalf("加载权重失败: %v", err)
	}
	active, err := NewModelGate(model, nil, "test-v1").Activate(Features{EventKind: "turn"})
	if err != nil {
		t.Fatalf("Activate 失败: %v", err)
	}
	want := []string{EXPERT_EVENT, PHRASING_EXPERT}
	if strings.Join(active, "|") != strings.Join(want, "|") {
		t.Fatalf("active = %v, want %v", active, want)
	}
}

// TestModelGateFallsBack 覆盖 fail-open：模型缺失/维数不符/无 fallback 三种情形。
func TestModelGateFallsBack(t *testing.T) {
	stub := &stubGate{active: []string{EXPERT_EVENT, EXPERT_EXPRESSION}}
	// nil 模型。
	active, err := NewModelGate(nil, stub, "test").Activate(Features{})
	if err != nil || strings.Join(active, "|") != strings.Join(stub.active, "|") || stub.calls != 1 {
		t.Fatalf("nil 模型应回落 fallback: active=%v err=%v calls=%d", active, err, stub.calls)
	}
	// 输入维数不符：2 维模型。
	mismatched, err := mlp.Load(strings.NewReader(weightsJSON(t, []int{2, len(SEMANTIC_EXPERTS)}, nil)))
	if err != nil {
		t.Fatalf("加载 2 维模型失败: %v", err)
	}
	active, err = NewModelGate(mismatched, stub, "test").Activate(Features{})
	if err != nil || strings.Join(active, "|") != strings.Join(stub.active, "|") || stub.calls != 2 {
		t.Fatalf("维数不符应回落 fallback: active=%v err=%v calls=%d", active, err, stub.calls)
	}
	// 类别数不符（旧 5 类模型）也应回落。
	fiveClasses, err := mlp.Load(strings.NewReader(weightsJSON(t, []int{FeatureSizeV1, len(DEFAULT_EXPERT_ORDER)}, nil)))
	if err != nil {
		t.Fatalf("加载 5 类模型失败: %v", err)
	}
	active, err = NewModelGate(fiveClasses, stub, "test").Activate(Features{})
	if err != nil || strings.Join(active, "|") != strings.Join(stub.active, "|") || stub.calls != 3 {
		t.Fatalf("类别数不符应回落 fallback: active=%v err=%v calls=%d", active, err, stub.calls)
	}
	// 无 fallback 时必须返回可读错误，而不是静默通过。
	if _, err := NewModelGate(nil, nil, "test").Activate(Features{}); err == nil {
		t.Fatal("模型与 fallback 都缺失时应报错")
	}
}

// TestModelGatePrimaryExpert proves the label follows the top probability even
// when 事件记忆 sorts before it in the canonical active list.
func TestModelGatePrimaryExpert(t *testing.T) {
	biases := make([]float64, len(SEMANTIC_EXPERTS))
	biases[1] = 1 // 事件记忆
	biases[3] = 2 // 知识边界：概率最高
	model, err := mlp.Load(strings.NewReader(weightsJSON(t, []int{FeatureSizeV1, len(SEMANTIC_EXPERTS)}, biases)))
	if err != nil {
		t.Fatalf("加载权重失败: %v", err)
	}
	modelGate := NewModelGate(model, nil, "test-v1")
	active, err := modelGate.Activate(Features{EventKind: "turn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) == 0 || active[0] != EXPERT_EVENT {
		t.Fatalf("前提不成立：active 首位应为事件记忆，得到 %v", active)
	}
	if primary := modelGate.PrimaryExpert(Features{EventKind: "turn"}, active); primary != EXPERT_KNOWLEDGE {
		t.Fatalf("主专家 = %q, want %q", primary, EXPERT_KNOWLEDGE)
	}
	// 模型不可用时应回落 fallback 的主专家（没有 fallback 时返回空）。
	if primary := NewModelGate(nil, nil, "test").PrimaryExpert(Features{}, nil); primary != "" {
		t.Fatalf("无模型无 fallback 时应返回空主专家: %q", primary)
	}
}

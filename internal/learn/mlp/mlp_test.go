// mlp_test.go —— 权重 JSON 推理消费端的回归测试（契约见算法导论仓库 docs/mlp-format.md）。
package mlp

import (
	"math"
	"strings"
	"testing"
)

// TestLoadExamplePredictsXOR 用 ITA 的 2-2-2 tanh 示例验证解包 + 前向 + softmax 口径。
func TestLoadExamplePredictsXOR(t *testing.T) {
	model, err := LoadFile("testdata/xor-tanh-2-2-2.json")
	if err != nil {
		t.Fatalf("加载示例权重失败: %v", err)
	}
	if got, want := model.Layers(), []int{2, 2, 2}; !equalInts(got, want) {
		t.Fatalf("layers = %v, want %v", got, want)
	}
	if model.InputSize() != 2 || model.NumClasses() != 2 || model.Activation() != "tanh" {
		t.Fatalf("模型元信息不符: input=%d classes=%d act=%s", model.InputSize(), model.NumClasses(), model.Activation())
	}
	// 契约示例：class = 1 ⇔ x₁·x₂ = −1（±1 编码）。
	cases := []struct {
		features []float64
		class    int
	}{
		{[]float64{-1, -1}, 0},
		{[]float64{-1, 1}, 1},
		{[]float64{1, -1}, 1},
		{[]float64{1, 1}, 0},
	}
	for _, item := range cases {
		class, err := model.PredictClass(item.features)
		if err != nil {
			t.Fatalf("PredictClass(%v) 失败: %v", item.features, err)
		}
		if class != item.class {
			t.Fatalf("PredictClass(%v) = %d, want %d", item.features, class, item.class)
		}
		probabilities, err := model.Predict(item.features)
		if err != nil {
			t.Fatalf("Predict(%v) 失败: %v", item.features, err)
		}
		if len(probabilities) != 2 {
			t.Fatalf("Predict(%v) 返回 %d 个概率", item.features, len(probabilities))
		}
		sum := probabilities[0] + probabilities[1]
		if math.Abs(sum-1) > 1e-12 {
			t.Fatalf("Predict(%v) 概率和 = %v, want 1", item.features, sum)
		}
	}
}

// TestLoadRejectsInvalidFiles 覆盖契约 §7 的硬失败：版本、层宽、激活名、权重个数。
func TestLoadRejectsInvalidFiles(t *testing.T) {
	base := `{"formatVersion":1,"layers":[2,2],"activation":"tanh","leaky":0,"seed":1,"weights":[1,1,0,1,1,0]}`
	if _, err := Load(strings.NewReader(base)); err != nil {
		t.Fatalf("合法权重被拒: %v", err)
	}
	cases := map[string]string{
		"版本不符":   `{"formatVersion":2,"layers":[2,2],"activation":"tanh","weights":[1,1,0,1,1,0]}`,
		"层宽不足":   `{"formatVersion":1,"layers":[2],"activation":"tanh","weights":[]}`,
		"层宽非正":   `{"formatVersion":1,"layers":[2,0],"activation":"tanh","weights":[]}`,
		"未知激活":   `{"formatVersion":1,"layers":[2,2],"activation":"swish","weights":[1,1,0,1,1,0]}`,
		"权重个数不符": `{"formatVersion":1,"layers":[2,2],"activation":"tanh","weights":[1,1,0]}`,
		"坏 JSON": `{"formatVersion":1,`,
	}
	for name, payload := range cases {
		if _, err := Load(strings.NewReader(payload)); err == nil {
			t.Fatalf("%s: 期望报错，实际通过", name)
		}
	}
}

// TestLoadIgnoresUnknownFieldsAndValidatesFeatures 覆盖「同版本可选字段忽略」与特征维数校验。
func TestLoadIgnoresUnknownFieldsAndValidatesFeatures(t *testing.T) {
	payload := `{"formatVersion":1,"layers":[2,2],"activation":"tanh","leaky":0,"seed":1,` +
		`"weights":[1,1,0,1,1,0],"train":{"epochs":3},"trainSnapshot":{"step":1},"futureField":true}`
	model, err := Load(strings.NewReader(payload))
	if err != nil {
		t.Fatalf("未知字段应被忽略: %v", err)
	}
	if _, err := model.Predict([]float64{1}); err == nil {
		t.Fatal("特征维数不符应报错")
	}
	if _, err := model.Predict([]float64{1, math.Inf(1)}); err == nil {
		t.Fatal("非有限特征应报错")
	}
}

// TestLeakyNormalization 覆盖契约 §1：leaky ≤ 0 归一为 0.01。
func TestLeakyNormalization(t *testing.T) {
	name, leaky, err := normalizeActivation("leakyrelu", 0)
	if err != nil || name != "leakyrelu" || leaky != leakyDefault {
		t.Fatalf("leakyrelu 归一化 = (%s, %v, %v), want (%s, %v, nil)", name, leaky, err, "leakyrelu", leakyDefault)
	}
	if got := activate("leakyrelu", leakyDefault, -1); math.Abs(got-(-0.01)) > 1e-15 {
		t.Fatalf("leakyrelu(-1) = %v, want -0.01", got)
	}
	// GELU 是 tanh 近似（不是 erf 版本）：x=1 时约 0.84119。
	if got := activate("gelu", 0, 1); math.Abs(got-0.8411919906082768) > 1e-9 {
		t.Fatalf("gelu(1) = %v, want ≈0.84119", got)
	}
}

// equalInts 比较两个 int 切片。
func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Package mlp 读取算法导论仓库训练的 MLP 权重 JSON（mlp-format v1）并做纯推理。
//
// 契约以外部文档为唯一载体：算法导论仓库 `docs/mlp-format.md`。本包只消费 `formatVersion=1`
// 的权重文件，不做训练、不加载 `train`/`trainSnapshot`（推理不需要，缺失不影响），
// 数值口径照 §2–§4 复现：隐藏层激活、输出层 logits 直接 softmax、weights 按层→单元→(先权重后偏置) 解包。
package mlp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// formatVersion 是本包唯一接受的权重文件格式版本（契约 §1/§7）。
const formatVersion = 1

// leakyDefault 是 LeakyReLU 的归一化斜率：JSON 里 leaky ≤ 0 时按 0.01 处理（契约 §1）。
const leakyDefault = 0.01

// weightsFile 是权重 JSON 的原始形状。未知字段由 encoding/json 忽略（契约 §7）。
type weightsFile struct {
	FormatVersion int       `json:"formatVersion"`
	Layers        []int     `json:"layers"`
	Activation    string    `json:"activation"`
	Leaky         float64   `json:"leaky"`
	Seed          int64     `json:"seed"`
	Weights       []float64 `json:"weights"`
}

// Model 是一个可推理的 MLP：层宽 + 隐藏层激活 + 解包后的权重/偏置。
type Model struct {
	layers     []int
	activation string
	leaky      float64
	// weights[l][u][j]：第 l 层第 u 个单元对上一层第 j 个输出的权重。
	weights [][][]float64
	// biases[l][u]：第 l 层第 u 个单元的偏置。
	biases [][]float64
}

// Load 从 r 读取并校验权重 JSON（契约 §1/§3/§7）；校验失败返回 error，不静默降级。
func Load(r io.Reader) (*Model, error) {
	var raw weightsFile
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("mlp: 解析权重 JSON 失败: %w", err)
	}
	return buildModel(&raw)
}

// LoadFile 读取权重文件；路径或内容错误都返回 error。
func LoadFile(path string) (*Model, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("mlp: 打开权重文件 %s 失败: %w", path, err)
	}
	defer file.Close()
	return Load(file)
}

// buildModel 校验并解包权重；所有长度/取值不合法都报错（契约 §3/§7）。
func buildModel(raw *weightsFile) (*Model, error) {
	if raw.FormatVersion != formatVersion {
		return nil, fmt.Errorf("mlp: formatVersion=%d 不受支持（只接受 %d）", raw.FormatVersion, formatVersion)
	}
	if len(raw.Layers) < 2 {
		return nil, errors.New("mlp: layers 至少要有输入层与输出层")
	}
	for index, width := range raw.Layers {
		if width <= 0 {
			return nil, fmt.Errorf("mlp: layers[%d]=%d 必须为正", index, width)
		}
	}
	activation, leaky, err := normalizeActivation(raw.Activation, raw.Leaky)
	if err != nil {
		return nil, err
	}
	total := weightCount(raw.Layers)
	if len(raw.Weights) != total {
		return nil, fmt.Errorf("mlp: weights 个数 %d 与层宽推算的 %d 不符", len(raw.Weights), total)
	}
	model := &Model{
		layers:     append([]int(nil), raw.Layers...),
		activation: activation,
		leaky:      leaky,
		weights:    make([][][]float64, len(raw.Layers)-1),
		biases:     make([][]float64, len(raw.Layers)-1),
	}
	offset := 0
	for layer := 0; layer < len(raw.Layers)-1; layer++ {
		out, in := raw.Layers[layer+1], raw.Layers[layer]
		model.weights[layer] = make([][]float64, out)
		model.biases[layer] = make([]float64, out)
		for unit := 0; unit < out; unit++ {
			model.weights[layer][unit] = append([]float64(nil), raw.Weights[offset:offset+in]...)
			offset += in
			model.biases[layer][unit] = raw.Weights[offset]
			offset++
		}
	}
	for _, row := range model.weights {
		for _, unitWeights := range row {
			for _, weight := range unitWeights {
				if !isFinite(weight) {
					return nil, errors.New("mlp: weights 含非有限值")
				}
			}
		}
	}
	for _, row := range model.biases {
		for _, bias := range row {
			if !isFinite(bias) {
				return nil, errors.New("mlp: weights 含非有限偏置")
			}
		}
	}
	return model, nil
}

// normalizeActivation 校验激活名并归一化 LeakyReLU 斜率；不支持的名字报错（契约 §7）。
func normalizeActivation(name string, leaky float64) (string, float64, error) {
	switch name {
	case "tanh", "sigmoid", "relu", "gelu":
		return name, leaky, nil
	case "leakyrelu":
		if leaky <= 0 {
			return name, leakyDefault, nil
		}
		return name, leaky, nil
	default:
		return "", 0, fmt.Errorf("mlp: 未知激活 %q", name)
	}
}

// weightCount 按契约 §3 计算 weights 元素个数。
func weightCount(layers []int) int {
	total := 0
	for layer := 0; layer < len(layers)-1; layer++ {
		total += layers[layer+1] * (layers[layer] + 1)
	}
	return total
}

// Layers 返回层宽副本（只读用途）。
func (m *Model) Layers() []int {
	return append([]int(nil), m.layers...)
}

// Activation 返回隐藏层激活名。
func (m *Model) Activation() string {
	return m.activation
}

// InputSize 返回输入特征维数（layers[0]）。
func (m *Model) InputSize() int {
	return m.layers[0]
}

// NumClasses 返回类别数（layers[len-1]）。
func (m *Model) NumClasses() int {
	return m.layers[len(m.layers)-1]
}

// Predict 对一行特征做前向 + softmax，返回各类概率（契约 §2/§4：float64 口径）。
func (m *Model) Predict(x []float64) ([]float64, error) {
	logits, err := m.forward(x)
	if err != nil {
		return nil, err
	}
	return softmax(logits), nil
}

// PredictClass 返回概率最大的类别（softmax 单调，等价于 argmax logits）。
func (m *Model) PredictClass(x []float64) (int, error) {
	probabilities, err := m.Predict(x)
	if err != nil {
		return 0, err
	}
	best := 0
	for index := 1; index < len(probabilities); index++ {
		if probabilities[index] > probabilities[best] {
			best = index
		}
	}
	return best, nil
}

// forward 按契约 §2 计算 logits（输出层不加激活）。
func (m *Model) forward(x []float64) ([]float64, error) {
	if len(x) != m.layers[0] {
		return nil, fmt.Errorf("mlp: 特征维数 %d 与输入层 %d 不符", len(x), m.layers[0])
	}
	current := append([]float64(nil), x...)
	for index, value := range current {
		if !isFinite(value) {
			return nil, fmt.Errorf("mlp: 特征[%d] 非有限值", index)
		}
	}
	for layer := 0; layer < len(m.layers)-1; layer++ {
		out := len(m.weights[layer])
		next := make([]float64, out)
		for unit := 0; unit < out; unit++ {
			pre := m.biases[layer][unit]
			for j, weight := range m.weights[layer][unit] {
				pre += weight * current[j]
			}
			if layer == len(m.layers)-2 {
				next[unit] = pre
				continue
			}
			next[unit] = activate(m.activation, m.leaky, pre)
		}
		current = next
	}
	return current, nil
}

// activate 实现契约 §2 的五个激活（GELU 用 tanh 近似，不是 erf 版本）。
func activate(name string, leaky, pre float64) float64 {
	switch name {
	case "tanh":
		return math.Tanh(pre)
	case "sigmoid":
		return 1 / (1 + math.Exp(-pre))
	case "relu":
		if pre > 0 {
			return pre
		}
		return 0
	case "leakyrelu":
		if pre > 0 {
			return pre
		}
		return leaky * pre
	case "gelu":
		const coefficient = 0.044715
		return 0.5 * pre * (1 + math.Tanh(math.Sqrt(2/math.Pi)*(pre+coefficient*pre*pre*pre)))
	default:
		return pre
	}
}

// softmax 做数值稳定归一化（契约 §2：先减最大值再 exp）。
func softmax(logits []float64) []float64 {
	maxLogit := logits[0]
	for _, value := range logits[1:] {
		if value > maxLogit {
			maxLogit = value
		}
	}
	total := 0.0
	probabilities := make([]float64, len(logits))
	for index, value := range logits {
		probabilities[index] = math.Exp(value - maxLogit)
		total += probabilities[index]
	}
	for index := range probabilities {
		probabilities[index] /= total
	}
	return probabilities
}

// isFinite 判断浮点值有限（拒绝 NaN/Inf，契约 §4）。
func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

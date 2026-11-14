// Package samples records gate decisions for training and exports them as MLP
// training rows. Recording is fail-soft: a broken disk never blocks a turn.
//
// Data flow: RecordingGate appends (features → active) JSONL under
// <data-dir>/training/gate-samples.jsonl → Export turns it into {"x":[...],"y":int}
// rows → the 算法导论 repository's training/mlp_jsonl CLI trains → weights JSON
// lands in <data-dir>/models/gate-mlp-v1.json (see docs/model-gate.md).
package samples

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/RuiCat/Neko/internal/gate"
)

// SAMPLES_DIR 是 data-dir 下的训练样本目录。
const SAMPLES_DIR = "training"

// SAMPLES_FILE 是记录器追加的原始样本文件名。
const SAMPLES_FILE = "gate-samples.jsonl"

// TEACHER_FILE 是智慧体主动提交的教师标签样本文件名：与自动记录样本分文件存，
// 导出/训练时合并，便于追溯与整体回滚。
const TEACHER_FILE = "gate-teacher.jsonl"

// DEFAULT_MAX_BYTES 是单文件轮转阈值（超过后改名为 .1 并重开）。
const DEFAULT_MAX_BYTES = 64 << 20

// Record is one recorded gate decision. HardState keeps keys only: the v1
// feature layout only needs the presence of knowledge-boundary keys.
type Record struct {
	At            time.Time          `json:"at"`
	EventKind     string             `json:"event_kind"`
	Summary       string             `json:"summary,omitempty"`
	Affect        map[string]float64 `json:"affect,omitempty"`
	HardStateKeys []string           `json:"hard_state_keys,omitempty"`
	Active        []string           `json:"active"`
	// Primary 是门控自报的主专家（可选）：active 按 DEFAULT_EXPERT_ORDER 归位，
	// 事件记忆会排在知识/自我前面，训练标签要用主专家而不是 active 首位。
	Primary string `json:"primary,omitempty"`
	// Source 标记样本来源：自动记录为空，智慧体教师样本为 "teacher"。
	Source string `json:"source,omitempty"`
}

// ToFeatures 把记录还原成门控特征（HardState 只需键存在）。
func (r Record) ToFeatures() gate.Features {
	hardState := make(map[string]any, len(r.HardStateKeys))
	for _, key := range r.HardStateKeys {
		hardState[key] = nil
	}
	return gate.Features{EventKind: r.EventKind, Summary: r.Summary, Affect: r.Affect, HardState: hardState}
}

// Sample 是 MLP 训练行（与算法导论 training/mlp_jsonl 的 -data 格式一致）。
type Sample struct {
	X []float64 `json:"x"`
	Y int       `json:"y"`
}

// LabelOf 返回 active 里第一个语义专家在 SEMANTIC_EXPERTS 的下标；表达偏好不参与分类，
// 没有语义专家时返回 -1（该记录会被导出跳过）。
func LabelOf(active []string) int {
	for _, name := range active {
		if name == gate.PHRASING_EXPERT {
			continue
		}
		for index, expert := range gate.SEMANTIC_EXPERTS {
			if name == expert {
				return index
			}
		}
	}
	return -1
}

// Writer appends records to one JSONL file and rotates it once it exceeds
// maxBytes (old file becomes <path>.1). Safe for concurrent use.
type Writer struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	file     *os.File
	size     int64
}

// NewWriter opens (or creates) path in append mode.
func NewWriter(path string) (*Writer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("samples: 创建样本目录失败: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("samples: 打开样本文件失败: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("samples: 读取样本文件状态失败: %w", err)
	}
	return &Writer{path: path, maxBytes: DEFAULT_MAX_BYTES, file: file, size: info.Size()}, nil
}

// Write 追加一条记录；超过阈值时先轮转。
func (w *Writer) Write(record Record) error {
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("samples: 编码样本失败: %w", err)
	}
	line = append(line, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return errors.New("samples: 写入器已关闭")
	}
	if w.maxBytes > 0 && w.size+int64(len(line)) > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			return err
		}
	}
	if _, err := w.file.Write(line); err != nil {
		return fmt.Errorf("samples: 写入样本失败: %w", err)
	}
	w.size += int64(len(line))
	return nil
}

// rotateLocked 把当前文件改名为 .1 并重开空文件。
func (w *Writer) rotateLocked() error {
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("samples: 关闭样本文件失败: %w", err)
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil {
		return fmt.Errorf("samples: 轮转样本文件失败: %w", err)
	}
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("samples: 重开样本文件失败: %w", err)
	}
	w.file = file
	w.size = 0
	return nil
}

// Close 关闭底层文件（可重复调用）。
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return fmt.Errorf("samples: 关闭样本文件失败: %w", err)
	}
	return nil
}

// RecordingGate wraps a gate and records each successful decision. Recording
// errors are reported through onError and never fail the turn (fail-soft).
type RecordingGate struct {
	inner   gate.Gate
	writer  *Writer
	onError func(error)
}

// NewRecordingGate wraps inner; onError may be nil.
func NewRecordingGate(inner gate.Gate, writer *Writer, onError func(error)) *RecordingGate {
	return &RecordingGate{inner: inner, writer: writer, onError: onError}
}

// Activate 调内层门控并在成功后记录样本。
func (g *RecordingGate) Activate(features gate.Features) ([]string, error) {
	active, err := g.inner.Activate(features)
	if err != nil {
		return nil, err
	}
	if g.writer != nil {
		record := NewRecord(features, active, time.Now())
		if selector, ok := g.inner.(gate.PrimarySelector); ok {
			record.Primary = selector.PrimaryExpert(features, active)
		}
		if writeErr := g.writer.Write(record); writeErr != nil && g.onError != nil {
			g.onError(writeErr)
		}
	}
	return active, nil
}

// Version 透传内层版本（模型门控仍上报 gate-mlp-v1，wrapper 不覆盖）。
func (g *RecordingGate) Version() string {
	if versioned, ok := g.inner.(gate.Versioned); ok {
		return versioned.Version()
	}
	return ""
}

// NewRecord 由门控输入与实际激活结果构造一条记录。
func NewRecord(features gate.Features, active []string, at time.Time) Record {
	keys := make([]string, 0, len(features.HardState))
	for key := range features.HardState {
		keys = append(keys, key)
	}
	sortStrings(keys)
	return Record{
		At:            at,
		EventKind:     features.EventKind,
		Summary:       features.Summary,
		Affect:        features.Affect,
		HardStateKeys: keys,
		Active:        append([]string(nil), active...),
	}
}

// sortStrings 排序字符串切片（样本文件里保持稳定顺序，便于 diff）。
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// ReadFile 读取并解析样本文件；缺失文件视为空样本（第一次收集时的正常状态）。
func ReadFile(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("samples: 打开样本文件失败: %w", err)
	}
	defer file.Close()
	records := make([]Record, 0, 64)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("samples: 解析样本行失败: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("samples: 读取样本文件失败: %w", err)
	}
	return records, nil
}

// LabelFor 返回一条记录的训练标签：优先用记录里的主专家，缺省时退回
// active 里第一个语义专家；没有语义专家返回 -1（该记录会被导出跳过）。
func LabelFor(record Record) int {
	if record.Primary != "" {
		if index := expertIndex(record.Primary); index >= 0 {
			return index
		}
	}
	return LabelOf(record.Active)
}

// expertIndex 返回语义专家的下标（表达偏好不在分类里，返回 -1）。
func expertIndex(name string) int {
	return gate.SemanticExpertIndex(name)
}

// Path 返回 data-dir 下某个样本文件的路径。
func Path(dataDir, name string) string {
	return filepath.Join(dataDir, SAMPLES_DIR, name)
}

// ReadAll 读取自动记录样本与教师样本（教师样本在后）；任一文件缺失按空处理。
func ReadAll(dataDir string) ([]Record, error) {
	auto, err := ReadFile(Path(dataDir, SAMPLES_FILE))
	if err != nil {
		return nil, err
	}
	teacher, err := ReadFile(Path(dataDir, TEACHER_FILE))
	if err != nil {
		return nil, err
	}
	return append(auto, teacher...), nil
}

// Export 把记录写成 MLP 训练行（{"x":[...],"y":int}）；无法映射标签的记录跳过。
func Export(out io.Writer, records []Record) (kept int, skipped int, err error) {
	writer := bufio.NewWriter(out)
	for _, record := range records {
		label := LabelFor(record)
		if label < 0 {
			skipped++
			continue
		}
		sample := Sample{X: gate.FeatureVectorV1(record.ToFeatures()), Y: label}
		line, marshalErr := json.Marshal(sample)
		if marshalErr != nil {
			return kept, skipped, fmt.Errorf("samples: 编码训练行失败: %w", marshalErr)
		}
		if _, writeErr := writer.Write(append(line, '\n')); writeErr != nil {
			return kept, skipped, fmt.Errorf("samples: 写出训练集失败: %w", writeErr)
		}
		kept++
	}
	if err := writer.Flush(); err != nil {
		return kept, skipped, fmt.Errorf("samples: 刷新训练集失败: %w", err)
	}
	return kept, skipped, nil
}

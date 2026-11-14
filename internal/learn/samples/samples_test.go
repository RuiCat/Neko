// samples_test.go covers sample recording, rotation, reading and training export.
package samples

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RuiCat/Neko/internal/gate"
)

// stubGate 是测试用门控，可自报版本。
type stubGate struct {
	active []string
	err    error
}

// Activate 返回预设专家或错误。
func (s stubGate) Activate(gate.Features) ([]string, error) { return s.active, s.err }

// Version 让 RecordingGate.Version 有可透传的值。
func (s stubGate) Version() string { return "gate-test" }

// TestWriteReadRoundtrip 覆盖追加写 + 读回 + 硬状态键稳定排序。
func TestWriteReadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", SAMPLES_FILE)
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	features := gate.Features{
		EventKind: "turn",
		Summary:   "主人回来了",
		Affect:    map[string]float64{"trust": 0.4},
		HardState: map[string]any{gate.EXPERT_KNOWLEDGE + ".范围": "只谈猫", "关系": map[string]any{}},
	}
	record := NewRecord(features, []string{gate.EXPERT_EVENT, gate.EXPERT_EXPRESSION}, time.Unix(1, 0))
	if err := writer.Write(record); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	records, err := ReadFile(path)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadFile = %d 条, err=%v", len(records), err)
	}
	if got, want := strings.Join(records[0].HardStateKeys, ","), "关系,"+gate.EXPERT_KNOWLEDGE+".范围"; got != want {
		t.Fatalf("HardStateKeys = %q, want %q", got, want)
	}
	restored := records[0].ToFeatures()
	if restored.EventKind != "turn" || restored.Summary != "主人回来了" || restored.Affect["trust"] != 0.4 {
		t.Fatalf("ToFeatures 还原不一致: %+v", restored)
	}
	if _, ok := restored.HardState[gate.EXPERT_KNOWLEDGE+".范围"]; !ok {
		t.Fatal("知识边界键应在还原后的 HardState 里")
	}
}

// TestExportLabels 覆盖标签映射、跳过无标签记录、向量维数。
func TestExportLabels(t *testing.T) {
	records := []Record{
		{EventKind: "turn", Active: []string{gate.EXPERT_EVENT, gate.EXPERT_EXPRESSION}},
		{EventKind: "relationship", Active: []string{gate.EXPERT_RELATION, gate.EXPERT_EVENT}},
		{EventKind: "turn", Active: []string{gate.EXPERT_EXPRESSION}},
		{EventKind: "turn", Active: []string{"未知专家"}},
		{EventKind: "turn"},
	}
	if got := LabelOf([]string{gate.EXPERT_EXPRESSION}); got != -1 {
		t.Fatalf("只有表达偏好时标签应为 -1（不参与分类），got %d", got)
	}
	if got := LabelOf([]string{gate.EXPERT_KNOWLEDGE, gate.EXPERT_EXPRESSION}); got != 3 {
		t.Fatalf("知识边界标签应为 3，got %d", got)
	}
	var out bytes.Buffer
	kept, skipped, err := Export(&out, records)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if kept != 2 || skipped != 3 {
		t.Fatalf("kept=%d skipped=%d, want 2/3", kept, skipped)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("训练行数 = %d, want 2", len(lines))
	}
	var first, second Sample
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("解析第一行: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("解析第二行: %v", err)
	}
	if len(first.X) != gate.FeatureSizeV1 {
		t.Fatalf("x 维数 = %d, want %d", len(first.X), gate.FeatureSizeV1)
	}
	if first.Y != 1 || second.Y != 0 {
		t.Fatalf("标签 = %d/%d, want 1/0", first.Y, second.Y)
	}
	if len(gate.SEMANTIC_EXPERTS) != 4 || gate.PHRASING_EXPERT != gate.EXPERT_EXPRESSION {
		t.Fatalf("语义专家契约变化: %v / %s", gate.SEMANTIC_EXPERTS, gate.PHRASING_EXPERT)
	}
}

// TestRecordingGateRecordsAndPassesVersion 覆盖 wrapper 记录与版本透传。
func TestRecordingGateRecordsAndPassesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), SAMPLES_FILE)
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	recorder := NewRecordingGate(stubGate{active: []string{gate.EXPERT_EVENT}}, writer, nil)
	active, err := recorder.Activate(gate.Features{EventKind: "turn"})
	if err != nil || len(active) != 1 {
		t.Fatalf("Activate = %v, %v", active, err)
	}
	if got := recorder.Version(); got != "gate-test" {
		t.Fatalf("Version = %q, want gate-test", got)
	}
	writer.Close()
	records, err := ReadFile(path)
	if err != nil || len(records) != 1 || records[0].EventKind != "turn" {
		t.Fatalf("记录未落盘: %d 条, err=%v", len(records), err)
	}
	// 内层出错时不应写样本。
	failing := NewRecordingGate(stubGate{err: errors.New("boom")}, nil, nil)
	if _, err := failing.Activate(gate.Features{}); err == nil {
		t.Fatal("内层错误应透传")
	}
}

// TestRotation 覆盖超过阈值后的轮转。
func TestRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), SAMPLES_FILE)
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	writer.maxBytes = 200
	for index := 0; index < 10; index++ {
		if err := writer.Write(NewRecord(gate.Features{EventKind: "turn"}, []string{gate.EXPERT_EVENT}, time.Unix(0, 0))); err != nil {
			t.Fatalf("Write #%d: %v", index, err)
		}
	}
	writer.Close()
	records, err := ReadFile(path + ".1")
	if err != nil {
		t.Fatalf("读取轮转文件: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("轮转文件不应为空")
	}
}

// TestReadMissingIsEmpty 覆盖首次收集（文件不存在）的正常状态。
func TestReadMissingIsEmpty(t *testing.T) {
	records, err := ReadFile(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil || len(records) != 0 {
		t.Fatalf("ReadFile(缺失) = %d 条, err=%v", len(records), err)
	}
}

// primaryGate 实现 PrimarySelector，模拟门控自报主专家。
type primaryGate struct {
	active  []string
	primary string
}

// Activate 返回预设专家。
func (g primaryGate) Activate(gate.Features) ([]string, error) { return g.active, nil }

// PrimaryExpert 返回预设主专家。
func (g primaryGate) PrimaryExpert(gate.Features, []string) string { return g.primary }

// TestLabelForPrefersPrimary proves a knowledge turn trains 知识边界 even
// though the canonical activation list starts with 事件记忆.
func TestLabelForPrefersPrimary(t *testing.T) {
	record := NewRecord(gate.Features{EventKind: "knowledge"},
		[]string{gate.EXPERT_EVENT, gate.EXPERT_KNOWLEDGE, gate.EXPERT_EXPRESSION}, time.Unix(1, 0))
	record.Primary = gate.EXPERT_KNOWLEDGE
	if got := LabelFor(record); got != 3 {
		t.Fatalf("主专家标签 = %d, want 3", got)
	}
	record.Primary = ""
	if got := LabelFor(record); got != 1 {
		t.Fatalf("缺省标签 = %d, want 1（active 首位）", got)
	}
	record.Primary = "未知专家"
	if got := LabelFor(record); got != 1 {
		t.Fatalf("未知主专家应退回 active: %d", got)
	}
}

// TestRecordingGateRecordsPrimary proves the recorder stores the gate's
// primary expert and the export uses it.
func TestRecordingGateRecordsPrimary(t *testing.T) {
	path := filepath.Join(t.TempDir(), SAMPLES_FILE)
	writer, err := NewWriter(path)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	recorder := NewRecordingGate(primaryGate{
		active:  []string{gate.EXPERT_EVENT, gate.EXPERT_KNOWLEDGE, gate.EXPERT_EXPRESSION},
		primary: gate.EXPERT_KNOWLEDGE,
	}, writer, nil)
	if _, err := recorder.Activate(gate.Features{EventKind: "knowledge"}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	records, err := ReadFile(path)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadFile = %d 条, err=%v", len(records), err)
	}
	if records[0].Primary != gate.EXPERT_KNOWLEDGE {
		t.Fatalf("记录的主专家 = %q, want %q", records[0].Primary, gate.EXPERT_KNOWLEDGE)
	}
	var out bytes.Buffer
	if _, _, err := Export(&out, records); err != nil {
		t.Fatalf("Export: %v", err)
	}
	var sample Sample
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &sample); err != nil {
		t.Fatalf("解析训练行: %v", err)
	}
	if sample.Y != 3 {
		t.Fatalf("导出标签 = %d, want 3", sample.Y)
	}
}

// TestReadAllMergesTeacherSamples 覆盖自动样本 + 教师样本合并与来源标记。
func TestReadAllMergesTeacherSamples(t *testing.T) {
	dataDir := t.TempDir()
	autoWriter, err := NewWriter(Path(dataDir, SAMPLES_FILE))
	if err != nil {
		t.Fatalf("auto writer: %v", err)
	}
	auto := NewRecord(gate.Features{EventKind: "turn"}, []string{gate.EXPERT_EVENT, gate.EXPERT_EXPRESSION}, time.Unix(1, 0))
	if err := autoWriter.Write(auto); err != nil {
		t.Fatalf("write auto: %v", err)
	}
	if err := autoWriter.Close(); err != nil {
		t.Fatalf("close auto: %v", err)
	}
	teacherWriter, err := NewWriter(Path(dataDir, TEACHER_FILE))
	if err != nil {
		t.Fatalf("teacher writer: %v", err)
	}
	teacher := NewRecord(gate.Features{EventKind: "question"}, []string{gate.EXPERT_KNOWLEDGE, gate.EXPERT_EXPRESSION}, time.Unix(2, 0))
	teacher.Primary = gate.EXPERT_KNOWLEDGE
	teacher.Source = "teacher"
	if err := teacherWriter.Write(teacher); err != nil {
		t.Fatalf("write teacher: %v", err)
	}
	if err := teacherWriter.Close(); err != nil {
		t.Fatalf("close teacher: %v", err)
	}
	records, err := ReadAll(dataDir)
	if err != nil || len(records) != 2 {
		t.Fatalf("ReadAll = %d 条, err=%v", len(records), err)
	}
	if records[0].Source != "" || records[1].Source != "teacher" {
		t.Fatalf("来源标记错误: %q / %q", records[0].Source, records[1].Source)
	}
	if got := LabelFor(records[1]); got != 3 {
		t.Fatalf("教师样本标签 = %d, want 3", got)
	}
	if records, err := ReadAll(t.TempDir()); err != nil || len(records) != 0 {
		t.Fatalf("空 data-dir 应返回空样本: %d, %v", len(records), err)
	}
}
